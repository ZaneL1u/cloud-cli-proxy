"use strict";

// ── DNS 遥测拦截模块 ──────────────────────────────────────────
// 拦截 DNS 查询和网络连接，阻止 Claude Code 向遥测/分析端点发送数据。
// 兼容 Node.js (--require) 和 Bun (--preload) 两种加载方式。
//
// ⚠️ 局限性（实测，claude 2.1.304 bun 编译二进制）：
//   bun 编译版的网络层（fetch / dns / net / tls）为内建实现，不经过
//   node 兼容层的 JS 模块对象，本模块的 patch 对其无效。
//   对 bun 编译版 Claude Code，遥测拦截只能依赖容器网络层
//   （sing-box tun + nftables 默认拒绝）；本模块仅对 node 直跑的
//   程序（旧版 cli.js、自研脚本等）有效，故不再注入 claude 启动链路。

var dns = require("dns");
var net = require("net");
var https = require("https");
var crypto = require("crypto");

// ── 域名黑名单 ────────────────────────────────────────────────
var BLOCKED_DOMAINS = [
  "statsig.anthropic.com",
  "sentry.io",
  "o1137031.ingest.sentry.io",
  "cdn.growthbook.io",
  "http-intake.logs.us5.datadoghq.com",
];

var BLOCKED_SET = Object.create(null);
BLOCKED_DOMAINS.forEach(function (d) { BLOCKED_SET[d] = true; });

function isBlocked(hostname) {
  if (!hostname) return false;
  if (BLOCKED_SET[hostname]) return true;
  // 子域名也拦截（如 xxx.sentry.io）
  for (var i = 0; i < BLOCKED_DOMAINS.length; i++) {
    if (hostname.endsWith("." + BLOCKED_DOMAINS[i])) return true;
  }
  return false;
}

var ERR_BLOCKED = Object.assign(new Error("ECONNREFUSED"), { code: "ECONNREFUSED" });

function blockCallback(cb) {
  if (typeof cb === "function") {
    process.nextTick(cb, ERR_BLOCKED);
  }
}

// ── 健康检查 bypass ───────────────────────────────────────────
var HEALTH_CHECK_HOST = "api.anthropic.com";
var HEALTH_CHECK_PATH = "/api/hello";

function isHealthCheck(hostname, path) {
  return hostname === HEALTH_CHECK_HOST && path === HEALTH_CHECK_PATH;
}

var FakeResponse = function () {
  this.statusCode = 200;
  this.headers = { "content-type": "application/json" };
  this.on = function () { return this; };
  this.emit = function () {};
};

// ── 拦截 dns.lookup (callback + promises) ─────────────────────
var _origLookup = dns.lookup.bind(dns);
dns.lookup = function (hostname, options, callback) {
  if (typeof options === "function") {
    callback = options;
    options = {};
  }
  if (isBlocked(hostname)) {
    blockCallback(callback);
    return;
  }
  return _origLookup(hostname, options, callback);
};

// dns.promises.lookup
if (dns.promises && dns.promises.lookup) {
  var _origPromisesLookup = dns.promises.lookup.bind(dns.promises);
  dns.promises.lookup = function (hostname, options) {
    if (isBlocked(hostname)) {
      return Promise.reject(ERR_BLOCKED);
    }
    return _origPromisesLookup(hostname, options);
  };
}

// ── 拦截 dns.resolve / resolve4 / resolve6 ────────────────────
["resolve", "resolve4", "resolve6"].forEach(function (method) {
  if (typeof dns[method] !== "function") return;
  var _orig = dns[method].bind(dns);
  dns[method] = function (hostname) {
    if (isBlocked(hostname)) {
      var args = Array.prototype.slice.call(arguments);
      var cb = args[args.length - 1];
      blockCallback(cb);
      return;
    }
    return _orig.apply(null, arguments);
  };
});

// dns.promises resolve family
if (dns.promises) {
  ["resolve", "resolve4", "resolve6"].forEach(function (method) {
    if (typeof dns.promises[method] !== "function") return;
    var _origP = dns.promises[method].bind(dns.promises);
    dns.promises[method] = function (hostname) {
      if (isBlocked(hostname)) {
        return Promise.reject(ERR_BLOCKED);
      }
      return _origP.apply(null, arguments);
    };
  });
}

// ── 拦截 net.connect / net.createConnection ────────────────────
var _origConnect = net.connect;
var _origCreateConnection = net.createConnection;

function patchedConnect() {
  var args = Array.prototype.slice.call(arguments);
  var opts = args[0];
  var host = null;
  if (typeof opts === "object" && opts !== null) {
    host = opts.host || opts.hostname;
  } else if (typeof opts === "string") {
    host = opts;
  } else if (typeof opts === "number") {
    // port-only, host may be in args[1]
    if (typeof args[1] === "string") host = args[1];
  }
  if (isBlocked(host)) {
    var socket = new net.Socket();
    process.nextTick(function () { socket.destroy(ERR_BLOCKED); });
    return socket;
  }
  return _origConnect.apply(null, arguments);
}

net.connect = patchedConnect;
net.createConnection = patchedConnect;

// ── 拦截 globalThis.fetch (undici / 原生) ────────────────────
var _origFetch = globalThis.fetch;
if (typeof _origFetch === "function") {
  globalThis.fetch = function (input, init) {
    var url = null;
    if (typeof input === "string") {
      url = input;
    } else if (input && input.url) {
      url = input.url;
    }
    if (url) {
      try {
        var parsed = new URL(url);
        if (isBlocked(parsed.hostname)) {
          return Promise.resolve(new Response("", { status: 0, statusText: "Blocked" }));
        }
        // Health check bypass
        if (isHealthCheck(parsed.hostname, parsed.pathname)) {
          return Promise.resolve(new Response('{"status":"ok"}', { status: 200, headers: { "content-type": "application/json" } }));
        }
      } catch (_) {}
    }
    return _origFetch.apply(this, arguments);
  };
}

// ── 拦截 https.request / https.get ────────────────────────────
var _origHttpsRequest = https.request;
var _origHttpsGet = https.get;

function patchedHttpsCall(orig) {
  return function (url, options, callback) {
    var hostname = null;
    var pathname = "/";
    if (typeof url === "string") {
      try {
        var parsed = new URL(url);
        hostname = parsed.hostname;
        pathname = parsed.pathname;
      } catch (_) {}
    } else if (url && typeof url === "object") {
      hostname = url.hostname || url.host;
      pathname = url.pathname || "/";
    }
    if (isBlocked(hostname)) {
      var fakeReq = new (require("events").EventEmitter)();
      fakeReq.on = fakeReq.on.bind(fakeReq);
      fakeReq.write = function () { return fakeReq; };
      fakeReq.end = function () {
        process.nextTick(function () { fakeReq.emit("error", ERR_BLOCKED); });
      };
      return fakeReq;
    }
    // Health check bypass
    if (isHealthCheck(hostname, pathname)) {
      if (typeof callback === "function") {
        process.nextTick(function () {
          var fakeResp = new (require("events").EventEmitter)();
          fakeResp.statusCode = 200;
          fakeResp.headers = { "content-type": "application/json" };
          fakeResp.on = fakeResp.on.bind(fakeResp);
          var body = '{"status":"ok"}';
          fakeResp.pipe = function (dest) {
            dest.write(body);
            dest.end();
            return dest;
          };
          callback(fakeResp);
          fakeResp.emit("data", body);
          fakeResp.emit("end");
        });
      }
      var fakeReq2 = new (require("events").EventEmitter)();
      fakeReq2.on = fakeReq2.on.bind(fakeReq2);
      fakeReq2.write = function () { return fakeReq2; };
      fakeReq2.end = function () {};
      return fakeReq2;
    }
    return orig.apply(this, arguments);
  };
}

https.request = patchedHttpsCall(_origHttpsRequest);
https.get = patchedHttpsCall(_origHttpsGet);

// ── 日志（调试时打开） ────────────────────────────────────────
if (process.env.DNS_GUARD_DEBUG === "1") {
  console.error("[dns-guard] Loaded. Blocked domains:", BLOCKED_DOMAINS);
}
