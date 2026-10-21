"use strict";

// spoof-selftest.js — spoof-fingerprint.js 的行为自测
// 用法：node tools/spoof-selftest.js
// 说明：在任意平台可跑；平台特有的路径（/proc/self/cgroup 等）通过
//       导出的纯函数验证内容改写逻辑，API 层验证 patch 生效与否。

const path = require("path");
process.env.SPOOF_TIMEZONE = process.env.SPOOF_TIMEZONE || "America/New_York";
process.env.SPOOF_HOSTNAME = process.env.SPOOF_HOSTNAME || "selftest-host";

const spoof = require(path.join(__dirname, "spoof-fingerprint.js"));
const os = require("os");
const fs = require("fs");
const cp = require("child_process");

let pass = 0, fail = 0;
function check(name, cond, detail) {
  if (cond) { pass++; console.log(`  ok  ${name}`); }
  else { fail++; console.log(`FAIL  ${name}${detail ? " — " + detail : ""}`); }
}

console.log("== 纯函数：cgroup / mountinfo 改写 ==");
const cg = spoof.sanitizeCgroupContent(
  "12:devices:/docker/abc123\n0::/system.slice/docker-containerd.scope\n");
check("cgroup 容器词替换", !/docker|containerd|kubepods|libpod|lxc/i.test(cg), cg);

const mi = spoof.sanitizeMountinfoContent([
  '36 35 98:0 /mnt/docker/overlaydirs/x/merged / rw,relatime - overlay overlay rw,lowerdir=/var/lib/docker/x,upperdir=/var/lib/docker/y',
  '40 1 0:40 / /home/cl anthy rw - tmpfs tmpfs rw',
  '41 1 0:41 / /var/lib/docker/volumes/vol rw - ext4 /dev/sda1 rw',
  '99 1 0:42 / /data rw - ext4 /dev/sdb1 rw',
].join("\n"));
check("mountinfo 无 overlay 残留", !/overlay/.test(mi));
check("mountinfo 根挂载重建为 ext4", / - ext4 \/dev\/vda1 rw$/.test(mi.split("\n")[0]), mi.split("\n")[0]);
check("mountinfo 丢弃 docker 卷行", !/docker\/volumes/.test(mi));
check("mountinfo 保留正常行", /\/data rw - ext4/.test(mi));

console.log("== API patch ==");

// dockerenv（mac 上文件本就不存在，patch 后应一致表现为不存在）
check("existsSync(/.dockerenv) === false", fs.existsSync("/.dockerenv") === false);
let statThrew = false;
try { fs.statSync("/.dockerenv"); } catch (e) { statThrew = e.code === "ENOENT"; }
check("statSync(/.dockerenv) 抛 ENOENT", statThrew);
let accessThrew = false;
try { fs.accessSync("/run/.containerenv"); } catch (e) { accessThrew = e.code === "ENOENT"; }
check("accessSync(/run/.containerenv) 抛 ENOENT", accessThrew);

// 时区
const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
check(`resolvedOptions().timeZone === ${process.env.SPOOF_TIMEZONE}`,
  tz === process.env.SPOOF_TIMEZONE, tz);
const off = new Date().getTimezoneOffset();
const expect = (() => {
  const Real = Object.getPrototypeOf(Intl.DateTimeFormat).constructor;
  return off; // offset 一致性在下方用独立计算验证
})();
// 用未 patch 的时区换算交叉验证 getTimezoneOffset
const dtf = new Intl.DateTimeFormat("en-US", {
  timeZone: process.env.SPOOF_TIMEZONE, hour12: false,
  year: "numeric", month: "2-digit", day: "2-digit",
  hour: "2-digit", minute: "2-digit", second: "2-digit",
});
const parts = {}; for (const p of dtf.formatToParts(new Date())) parts[p.type] = p.value;
const asUTC = Date.UTC(+parts.year, +parts.month - 1, +parts.day,
  +parts.hour % 24, +parts.minute, +parts.second);
const expectOff = Math.round((asUTC - Date.now()) / 60000);
check(`getTimezoneOffset 与目标时区一致 (${expectOff})`, Math.abs(off - expectOff) <= 1, off);
void expect;

// uname 拦截
check(`execSync("uname -r") → 伪装 release`, String(cp.execSync("uname -r")).trim() === spoof.__config.release);
check(`execSync("uname -s") → Linux`, String(cp.execSync("uname -s")).trim() === "Linux");
check("execSync('hostname') → 伪装 hostname", String(cp.execSync("hostname")).trim() === spoof.__config.hostname);
const sync = cp.spawnSync("uname", ["-m"]);
check("spawnSync uname -m → machine", String(sync.stdout).trim() === spoof.__config.machine);

// os.*
check("os.cpus 型号伪装", os.cpus().every((c) => c.model === spoof.__config.cpuModel));
check("os.availableParallelism 伪装", os.availableParallelism() === spoof.__config.cpuCores);
check("os.totalmem 伪装", os.totalmem() === spoof.__config.totalMem);
const ni = os.networkInterfaces();
check("networkInterfaces 无 docker 网段", !/172\.(1[6-9]|2\d|3[01])\./.test(JSON.stringify(ni)));
check("networkInterfaces 无 02:42 MAC", !/02:42:/.test(JSON.stringify(ni)));
check("os.hostname 伪装", os.hostname() === spoof.__config.hostname);
check("os.release 伪装", os.release() === spoof.__config.release);

// machine-id 透传：不拦截（mac 上无此文件 → 原样 ENOENT；Linux 容器内返回真实文件）
let midPassthrough = false;
try { fs.readFileSync("/etc/machine-id"); midPassthrough = false; }
catch (e) { midPassthrough = e.code === "ENOENT"; }
if (process.platform === "linux") {
  check("machine-id 透传（读到真实内容）", fs.readFileSync("/etc/machine-id", "utf8").length >= 32);
} else {
  check("machine-id 不拦截（非 Linux 原样 ENOENT）", midPassthrough);
}

console.log(`\n结果: ${pass} 通过, ${fail} 失败`);
process.exit(fail ? 1 : 0);
