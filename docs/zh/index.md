---
layout: home

hero:
  name: Cloud CLI Proxy
  text: 为团队提供隔离的云端开发环境
  tagline: 每个用户一个独立容器，预装 Claude Code，所有出网流量走指定出口 IP
  image:
    src: /logo.svg
    alt: Cloud CLI Proxy Logo
  actions:
    - theme: brand
      text: 快速开始
      link: /zh/guide/quickstart
    - theme: alt
      text: GitHub
      link: https://github.com/ZaneL1u/cloud-cli-proxy

features:
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg>
    title: 每用户独立容器
    details: 每个用户一个 Ubuntu 24.04 容器，通过 sshfs 将本地目录映射到容器内同名路径。CPU、内存、磁盘均可限制。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M12 22s8-4 8-10V5l-8-3-8 3v7c0 6 8 10 8 10z"/></svg>
    title: 全流量隧道出口
    details: sing-box tun + Linux netns 接管容器所有出网流量，nftables 默认拒绝直连。支持 6 种代理协议，出口 IP 可绑定、可测试、可自动修正。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
    title: 环境伪装
    details: 替换 CPU 型号、MAC 地址、machine-id 等硬件指纹，拦截系统探测命令。自动生成 Windows 风格主机名，uTLS Chrome 指纹，屏蔽遥测上报。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.77a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.76z"/></svg>
    title: cloud-claude CLI
    details: 本地终端透明运行远端 Claude Code，支持 Auto / Full / SSHFS-Only 三层映射、tmux 多端会话、断线自动重连、五维度自检和错误码解释。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="2" y="3" width="20" height="14" rx="2" ry="2"/><line x1="8" y1="21" x2="16" y2="21"/><line x1="12" y1="17" x2="12" y2="21"/></svg>
    title: 远程桌面
    details: 容器内置 KasmVNC + Chromium，管理后台一键打开，直接在浏览器里操作桌面环境。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="3" y="3" width="18" height="18" rx="2" ry="2"/><line x1="3" y1="9" x2="21" y2="9"/><line x1="9" y1="21" x2="9" y2="9"/></svg>
    title: 管理后台
    details: 用户与主机的全生命周期管理、出口 IP 增删改查与连通性测试、bypass 防火墙配置、事件审计、SSE 实时推送、用户自助门户。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="4 17 10 11 4 5"/><line x1="12" y1="19" x2="20" y2="19"/></svg>
    title: 一条命令接入
    details: 管理员在后台创建用户和容器后，把 curl 命令发给用户。用户在终端输入密码，等容器启动后自动 SSH 进入。
  - icon: <svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><line x1="12" y1="17" x2="12.01" y2="17"/></svg>
    title: 到期自动回收
    details: 后台定时扫描用户到期状态，过期自动停机并禁止登录。所有操作写入审计事件，完整可追溯。
---
