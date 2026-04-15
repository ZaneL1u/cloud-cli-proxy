---
layout: home

hero:
  name: Cloud CLI Proxy
  text: 容器化 SSH 云主机平台
  tagline: 一条命令获取预装 Claude Code 的云主机，所有流量走指定出口 IP，零泄漏
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
  - icon: 🚀
    title: 一条命令接入
    details: curl | bash 自动认证、创建容器、建立 SSH 会话，用户无需任何手动配置
  - icon: 🤖
    title: Claude Code 开箱即用
    details: 每个容器预装 Claude Code，进入即可使用 AI 编程，所有 API 请求自动走指定出口
  - icon: 🔒
    title: 全流量强制出口
    details: sing-box tun + Linux netns 全隧道，nftables 默认拒绝，杜绝 DNS / WebRTC 泄漏
  - icon: 🌐
    title: 多协议代理支持
    details: 出口 IP 支持 6 种代理协议 — SOCKS5、VMess、VLESS、Shadowsocks、Trojan、HTTP
  - icon: 🖥️
    title: 远程桌面
    details: 容器内置 KasmVNC + Chromium，可通过管理后台或用户面板直接访问浏览器桌面环境
  - icon: 📊
    title: 管理后台
    details: React SPA 仪表盘，用户、主机、出口 IP、事件日志、代理测试一站式管理
---
