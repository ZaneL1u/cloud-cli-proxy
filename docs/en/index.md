---
layout: home

hero:
  name: Cloud CLI Proxy
  text: Containerized SSH Cloud Platform
  tagline: One command to get a cloud host with Claude Code pre-installed. All traffic through your exit IP. Zero leaks.
  image:
    src: /logo.svg
    alt: Cloud CLI Proxy Logo
  actions:
    - theme: brand
      text: Quick Start
      link: /en/guide/quickstart
    - theme: alt
      text: GitHub
      link: https://github.com/ZaneL1u/cloud-cli-proxy

features:
  - icon: 🚀
    title: One-command Access
    details: curl | bash to authenticate, create container, and SSH in — zero user configuration needed
  - icon: 🤖
    title: Claude Code Ready
    details: Pre-installed in every container. Start AI-assisted coding immediately with all API requests auto-routed through exit IP
  - icon: 🔒
    title: Full-tunnel Egress
    details: sing-box tun + Linux netns full-tunnel with nftables default-deny. No DNS or WebRTC leaks
  - icon: 🌐
    title: Multi-protocol Proxy
    details: Egress IPs support 6 proxy protocols — SOCKS5, VMess, VLESS, Shadowsocks, Trojan, HTTP
  - icon: 🖥️
    title: Remote Desktop
    details: Built-in KasmVNC + Chromium browser desktop, accessible via admin or user panel
  - icon: 📊
    title: Admin Dashboard
    details: React SPA for users, hosts, egress IPs, event logs, proxy testing — all in one place
---
