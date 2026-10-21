"use strict";

// spoof-fingerprint.js — 进程内环境伪装模块
//
// 注入方式：
//   - Claude Code 2.x（bun 编译二进制）：BUN_OPTIONS="--preload <本文件>"
//     （实测 NODE_OPTIONS="--require" 对 bun 编译版无效，BUN_OPTIONS 是唯一有效通道）
//   - node 运行的旧版 cli.js：NODE_OPTIONS="--require <本文件>"
//
// 覆盖的采集路径（依据对 claude 2.1.304 二进制的逆向）：
//   - os.cpus()/totalmem()/freemem()          宿主 CPU 型号与物理内存（容器内 /proc 透传宿主）
//   - os.networkInterfaces()                   docker bridge 172.x 地址与 02:42 MAC 前缀
//   - existsSync/statSync("/.dockerenv")       容器标志文件（与 entrypoint 删除双保险）
//   - statSync("/run/.containerenv")           podman 标志（防御性）
//   - readFileSync("/proc/self/cgroup")        claude 直接 includes("docker")
//   - readFile("/proc/1/cgroup")               正则 /docker|kubepods|containerd|libpod/i
//   - /proc/self/mountinfo                     overlay 文件系统与 /var/lib/docker 挂载痕迹
//   - child_process: uname / hostname          内核名与主机名
//   - Intl timeZone / getTimezoneOffset        本地时区（服务端与出口 IP 地理交叉验证）
//
// 明确不覆盖（保持一致性优先）：
//   - /etc/machine-id：entrypoint 已生成每容器唯一且持久化的值，此处透传，
//     保证 fs 层读取与 shell 层 `cat /etc/machine-id` 返回一致。
//
// 无法覆盖（bun 编译版内建实现，JS 层 patch 免疫）：
//   - fetch / dns / net / tls。遥测拦截依赖容器网络层
//    （sing-box tun + nftables 默认拒绝），进程内无解。

const os = require("os");
const crypto = require("crypto");

const realHostname = os.hostname();

// ── 配置：全部可经环境变量覆盖 ─────────────────────────────────
// 默认取真实容器主机名（控制面创建容器时已注入定制 hostname，无需二次伪装）；
// 单机使用 claude-spoofed.sh 时由其导出 SPOOF_HOSTNAME 随机假名。
const SPOOF = {
  hostname: process.env.SPOOF_HOSTNAME || realHostname,
  username: process.env.SPOOF_USERNAME || "user",
  homedir:  process.env.SPOOF_HOMEDIR  || "/home/user",
  platform: process.env.SPOOF_PLATFORM || "linux",
  type:     process.env.SPOOF_TYPE     || "Linux",
  // 与宿主内核解耦的普通发行版内核版本；uname 拦截输出同一值，保持一致。
  release:  process.env.SPOOF_RELEASE  || "6.8.0-45-generic",
  arch:     process.env.SPOOF_ARCH     || os.arch(),
  machine:  process.env.SPOOF_MACHINE  || (os.arch() === "arm64" ? "aarch64" : "x86_64"),
  tmpdir:   process.env.SPOOF_TMPDIR   || "/tmp",
  shell:    process.env.SPOOF_SHELL    || "/bin/bash",

  // 时区：优先 SPOOF_TIMEZONE，其次 TZ 环境变量，最后缺省美东
  // （与出口 IP 地理位置配套，由控制面/entrypoint 注入）。
  timezone: process.env.SPOOF_TIMEZONE || process.env.TZ || "America/New_York",

  // 基于主机名派生的稳定 MAC（locally administered 位 02 前缀）
  mac: process.env.SPOOF_MAC || (() => {
    const h = process.env.SPOOF_HOSTNAME || realHostname;
    const hash = crypto.createHash("sha256").update(h).digest("hex");
    return ["02", hash.slice(0, 2), hash.slice(2, 4), hash.slice(4, 6),
            hash.slice(6, 8), hash.slice(8, 10)].join(":");
  })(),

  cpuModel: process.env.SPOOF_CPU_MODEL || "AMD EPYC 7763 64-Core Processor",
  cpuCores: parseInt(process.env.SPOOF_CPU_CORES || "4", 10),
  // 缺省 8G；产品容器由 entrypoint 按内存配额导出真实限额，避免泄漏宿主物理内存。
  totalMem: parseInt(process.env.SPOOF_TOTAL_MEM || String(8 * 1024 * 1024 * 1024), 10),
};

// ── 纯函数：cgroup / mountinfo 内容改写（导出供自测） ──────────
function sanitizeCgroupContent(text) {
  return text.replace(/docker|containerd|kubepods|libpod|lxc/gi, "system.slice");
}

function sanitizeMountinfoContent(text) {
  return text
    .split("\n")
    .map((line) => {
      if (!line.trim()) return line;
      const f = line.split(" ");
      // docker overlay 根挂载：重建为普通 ext4 根挂载行，
      // 去掉 lowerdir/upperdir 等 /var/lib/docker 路径细节。
      if (f[4] === "/" && /\boverlay\b/.test(line)) {
        return `${f[0]} ${f[1]} 259:1 / / rw,relatime - ext4 /dev/vda1 rw`;
      }
      return line;
    })
    // 丢弃其余 docker/containerd 内部挂载行（volume、工作目录等）
    .filter((line) => !/\/var\/lib\/(docker|containerd)\//.test(line))
    .map((line) => line
      // 残余 overlay 挂载类型与 cgroup 层级中的容器词
      .replace(/\boverlay\b/g, "ext4")
      .replace(/docker|containerd|kubepods|libpod/gi, "system.slice"))
    .join("\n");
}

// ── 安全赋值：bun 对部分内置模块导出做了只读冻结，需逐个降级尝试 ──
function safeAssign(obj, key, value) {
  try { obj[key] = value; return true; } catch (_) {}
  try {
    Object.defineProperty(obj, key, { value, writable: true, configurable: true });
    return true;
  } catch (_) {
    return false;
  }
}

// ── Patch os.* ────────────────────────────────────────────────
safeAssign(os, "hostname", () => SPOOF.hostname);

const _userInfo = os.userInfo.bind(os);
safeAssign(os, "userInfo", (opts) => {
  const real = _userInfo(opts);
  return { ...real, username: SPOOF.username, homedir: SPOOF.homedir, shell: SPOOF.shell };
});

safeAssign(os, "platform", () => SPOOF.platform);
safeAssign(os, "type", () => SPOOF.type);
safeAssign(os, "release", () => SPOOF.release);
safeAssign(os, "arch", () => SPOOF.arch);
safeAssign(os, "tmpdir", () => SPOOF.tmpdir);

safeAssign(os, "totalmem", () => SPOOF.totalMem);
safeAssign(os, "freemem", () => Math.floor(SPOOF.totalMem * 0.6));

safeAssign(os, "cpus", () =>
  Array.from({ length: SPOOF.cpuCores }, (_, i) => ({
    model: SPOOF.cpuModel,
    speed: 2450,
    times: { user: 100000 + i * 1000, nice: 0, sys: 50000, idle: 900000, irq: 0 },
  })));

// 宿主逻辑核数与伪装核数不一致时暴露配额与拓扑，一并伪装
safeAssign(os, "availableParallelism", () => SPOOF.cpuCores);

safeAssign(os, "networkInterfaces", () => ({
  eth0: [{
    address: "10.0.0.2", netmask: "255.255.255.0", family: "IPv4",
    mac: SPOOF.mac, internal: false, cidr: "10.0.0.2/24",
  }],
  lo: [{
    address: "127.0.0.1", netmask: "255.0.0.0", family: "IPv4",
    mac: "00:00:00:00:00:00", internal: true, cidr: "127.0.0.1/8",
  }],
}));

safeAssign(os, "homedir", () => SPOOF.homedir);

// ── Patch 时区 ────────────────────────────────────────────────
// claude 通过 Intl.DateTimeFormat().resolvedOptions().timeZone 获取 IANA 时区，
// 通过 new Date().getTimezoneOffset() 获取偏移；两者必须指向同一目标时区。
const RealDateTimeFormat = Intl.DateTimeFormat;

const origResolvedOptions = RealDateTimeFormat.prototype.resolvedOptions;
safeAssign(RealDateTimeFormat.prototype, "resolvedOptions", function (...args) {
  const r = origResolvedOptions.apply(this, args);
  // 仅改写"未显式指定 timeZone"的查询结果，不影响显式传入 timeZone 的格式化。
  if (r && r.timeZone && r.locale) r.timeZone = SPOOF.timezone;
  return r;
});

function tzOffsetMinutes(timeZone, date) {
  const dtf = new RealDateTimeFormat("en-US", {
    timeZone, hour12: false,
    year: "numeric", month: "2-digit", day: "2-digit",
    hour: "2-digit", minute: "2-digit", second: "2-digit",
  });
  const parts = {};
  for (const p of dtf.formatToParts(date)) parts[p.type] = p.value;
  const asUTC = Date.UTC(+parts.year, +parts.month - 1, +parts.day,
    +parts.hour % 24, +parts.minute, +parts.second);
  return Math.round((asUTC - date.getTime()) / 60000);
}

safeAssign(Date.prototype, "getTimezoneOffset", function () {
  try {
    return tzOffsetMinutes(SPOOF.timezone, this);
  } catch (_) {
    return 0;
  }
});

// ── Patch child_process 中的指纹命令 ──────────────────────────
const childProcess = require("child_process");

const unameFull =
  `Linux ${SPOOF.hostname} ${SPOOF.release} #1 SMP ${SPOOF.machine} GNU/Linux`;

const FINGERPRINT_COMMANDS = [
  {
    pattern: /^uname\b/,
    build: (cmd) => {
      if (/\s-a/.test(cmd)) return unameFull;
      if (/\s-s/.test(cmd)) return "Linux";
      if (/\s-r/.test(cmd)) return SPOOF.release;
      if (/\s-m/.test(cmd)) return SPOOF.machine;
      if (/\s-n/.test(cmd)) return SPOOF.hostname;
      if (/\s-v/.test(cmd)) return `#1 SMP ${SPOOF.machine}`;
      return SPOOF.hostname;
    },
  },
  { pattern: /^hostname\b/, build: () => SPOOF.hostname },
  {
    pattern: /ioreg.*IOPlatformSerialNumber/,
    build: () => '"IOPlatformSerialNumber" = "CLOUD000000"',
  },
  {
    pattern: /system_profiler\s+SPHardwareDataType/,
    build: () => "Serial Number (system): CLOUD000000\nHardware UUID: 00000000-0000-0000-0000-000000000000",
  },
  {
    pattern: /sysctl\s+kern\.uuid/,
    build: () => "kern.uuid: 00000000-0000-0000-0000-000000000000",
  },
];

function maybeSpoofCommand(cmd) {
  for (const { pattern, build } of FINGERPRINT_COMMANDS) {
    if (pattern.test(cmd)) return build(cmd);
  }
  return null;
}

const _execSync = childProcess.execSync;
safeAssign(childProcess, "execSync", function (cmd, opts) {
  const spoofed = maybeSpoofCommand(String(cmd));
  if (spoofed !== null) return Buffer.from(spoofed + "\n");
  return _execSync.call(this, cmd, opts);
});

const _exec = childProcess.exec;
safeAssign(childProcess, "exec", function (cmd, opts, cb) {
  if (typeof opts === "function") { cb = opts; opts = {}; }
  const spoofed = maybeSpoofCommand(String(cmd));
  if (spoofed !== null) {
    const { EventEmitter } = require("events");
    const fake = new EventEmitter();
    fake.stdout = new EventEmitter();
    fake.stderr = new EventEmitter();
    process.nextTick(() => {
      fake.stdout.emit("data", spoofed + "\n");
      fake.emit("close", 0);
      if (cb) cb(null, spoofed + "\n", "");
    });
    return fake;
  }
  return _exec.call(this, cmd, opts, cb);
});

const _execFileSync = childProcess.execFileSync;
safeAssign(childProcess, "execFileSync", function (file, args, opts) {
  const fullCmd = [file, ...(Array.isArray(args) ? args : [])].join(" ");
  const spoofed = maybeSpoofCommand(fullCmd);
  if (spoofed !== null) return Buffer.from(spoofed + "\n");
  return _execFileSync.call(this, file, args, opts);
});

const _spawnSync = childProcess.spawnSync;
safeAssign(childProcess, "spawnSync", function (cmd, args, opts) {
  const fullCmd = [cmd, ...(Array.isArray(args) ? args : [])].join(" ");
  const spoofed = maybeSpoofCommand(fullCmd);
  if (spoofed !== null) {
    return {
      stdout: Buffer.from(spoofed + "\n"), stderr: Buffer.from(""),
      status: 0, signal: null, pid: 0,
      output: [null, Buffer.from(spoofed + "\n"), Buffer.from("")],
    };
  }
  return _spawnSync.call(this, cmd, args, opts);
});

// ── Patch fs 层：容器标志与 procfs 探测 ────────────────────────
const fs = require("fs");

const CONTAINER_FLAG_PATHS = new Set([
  "/.dockerenv", "/.dockerinit", "/run/.containerenv",
]);
const CGROUP_PATHS = new Set(["/proc/1/cgroup", "/proc/self/cgroup"]);
const MOUNTINFO_PATHS = new Set(["/proc/self/mountinfo", "/proc/1/mountinfo"]);

const pathKey = (p) => (typeof p === "string" ? p : (p && p.toString ? p.toString() : ""));

function rewriteContent(path, content) {
  const p = pathKey(path);
  if (CGROUP_PATHS.has(p)) return sanitizeCgroupContent(content);
  if (MOUNTINFO_PATHS.has(p)) return sanitizeMountinfoContent(content);
  return content;
}

// machine-id / dbus machine-id：透传（entrypoint 已保证唯一与持久化，
// fs 层与 shell 层返回一致值，避免自相矛盾）。

const _origReadFileSync = fs.readFileSync.bind(fs);
safeAssign(fs, "readFileSync", function (path, options) {
  const p = pathKey(path);
  if (CONTAINER_FLAG_PATHS.has(p)) {
    const err = new Error(`ENOENT: no such file or directory, stat '${p}'`);
    err.code = "ENOENT"; err.errno = -2; err.syscall = "stat"; err.path = p;
    throw err;
  }
  const r = _origReadFileSync(path, options);
  return rewriteContent(p, r);
});

const _origReadFile = fs.readFile.bind(fs);
safeAssign(fs, "readFile", function (path, ...args) {
  const cb = typeof args[args.length - 1] === "function" ? args[args.length - 1] : null;
  if (!cb) return _origReadFile(path, ...args);
  _origReadFile(path, ...args.slice(0, -1), (err, data) => {
    if (err) return cb(err);
    cb(null, rewriteContent(path, data));
  });
});

try {
  const fsp = fs.promises;
  const _origPromiseReadFile = fsp.readFile.bind(fsp);
  safeAssign(fsp, "readFile", function (path, options) {
    return _origPromiseReadFile(path, options).then((data) => rewriteContent(path, data));
  });
} catch (_) {}

const _origExistsSync = fs.existsSync.bind(fs);
safeAssign(fs, "existsSync", function (p) {
  if (CONTAINER_FLAG_PATHS.has(pathKey(p))) return false;
  return _origExistsSync(p);
});

const _origStatSync = fs.statSync;
safeAssign(fs, "statSync", function (p, ...rest) {
  if (CONTAINER_FLAG_PATHS.has(pathKey(p))) {
    const err = new Error(`ENOENT: no such file or directory, stat '${pathKey(p)}'`);
    err.code = "ENOENT"; err.errno = -2; err.syscall = "stat"; err.path = pathKey(p);
    throw err;
  }
  return _origStatSync.call(this, p, ...rest);
});

const _origAccessSync = fs.accessSync;
safeAssign(fs, "accessSync", function (p, ...rest) {
  if (CONTAINER_FLAG_PATHS.has(pathKey(p))) {
    const err = new Error(`ENOENT: no such file or directory, access '${pathKey(p)}'`);
    err.code = "ENOENT"; err.errno = -2; err.syscall = "access"; err.path = pathKey(p);
    throw err;
  }
  return _origAccessSync.call(this, p, ...rest);
});

// ── 调试输出 ──────────────────────────────────────────────────
if (process.env.SPOOF_DEBUG === "1") {
  console.error("[spoof-fingerprint] Loaded. Spoofing as:", JSON.stringify({
    hostname: SPOOF.hostname, platform: SPOOF.platform, timezone: SPOOF.timezone,
    username: SPOOF.username, mac: SPOOF.mac, cpuModel: SPOOF.cpuModel,
    cpuCores: SPOOF.cpuCores, totalMem: SPOOF.totalMem, release: SPOOF.release,
  }, null, 2));
}

// 导出供自测（preload 场景下无副作用）
module.exports = { sanitizeCgroupContent, sanitizeMountinfoContent, __config: SPOOF };
