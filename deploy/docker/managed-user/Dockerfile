FROM ubuntu:24.04

# v4.0 (Phase 53): managed-user 镜像内置 sing-box，作单容器同容器架构基线。
# - sing-box 1.13.3 via GitHub release，文件 cap cap_net_admin+eip
# - singbox 系统账号 uid=9000 用于 sing-box 进程降权（D-V4-1 / D-53-2）
# - workspace 用户去 sudo（D-53-4），breaking change vs v3.x
# 详见 .planning/phases/53-entrypoint/53-CONTEXT.md

ENV DEBIAN_FRONTEND=noninteractive
ENV HOME=/workspace
ENV WORKSPACE_USER=workspace
ENV WORKSPACE_UID=1000
ENV WORKSPACE_GID=1000

# 启用 BuildKit apt cache mount（必须先移除 docker-clean，否则 cache 会被自动清空）
# 参考：https://docs.docker.com/build/cache/optimize (D-03, M18 防御)
RUN rm -f /etc/apt/apt.conf.d/docker-clean \
    && echo 'Binary::apt::APT::Keep-Downloaded-Packages "true";' \
        > /etc/apt/apt.conf.d/keep-cache

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends \
        openssh-server \
        bash \
        zsh \
        curl \
        git \
        tmux \
        sudo \
        ca-certificates \
        jq \
        procps \
        iproute2 \
        locales \
        language-pack-zh-hans \
        fluxbox \
        pcmanfm \
        dbus-x11 \
        fonts-liberation \
        fonts-noto-cjk \
        xdg-utils \
        xclip \
        xsel \
        gnupg \
        libegl1 \
        libgl1 \
        x11-utils \
        x11-xserver-utils \
        xterm \
        sshfs \
        fuse3 \
        tini \
        nftables \
        libcap2-bin \
        dnsutils \
        nano \
        less \
        unzip \
    && rm -rf /var/lib/apt/lists/*

RUN sed -i 's/^#user_allow_other/user_allow_other/' /etc/fuse.conf \
    && chmod a+r /etc/fuse.conf

# KasmVNC：替代 x11vnc + noVNC + websockify + Xvfb，自带剪贴板同步和 Web 客户端。
ARG KASMVNC_VERSION=1.4.0
RUN ARCH=$(dpkg --print-architecture) \
    && curl -fsSL -o /tmp/kasmvnc.deb \
        "https://github.com/kasmtech/KasmVNC/releases/download/v${KASMVNC_VERSION}/kasmvncserver_noble_${KASMVNC_VERSION}_${ARCH}.deb" \
    && apt-get update \
    && apt-get install -y --no-install-recommends /tmp/kasmvnc.deb \
    && rm /tmp/kasmvnc.deb \
    && rm -rf /var/lib/apt/lists/* /var/cache/apt/*

# Chromium：Ubuntu 24.04 的 chromium-browser 是 snap 包装器，Docker 不支持 snap。
# 从 Debian bookworm 仓库安装真正的 .deb（支持 amd64 + arm64）。
# 通过 Debian 官方 release-12.asc GPG 密钥 + HTTPS 源防止供应链投毒。
RUN curl -fsSL https://ftp-master.debian.org/keys/release-12.asc \
    | gpg --dearmor -o /usr/share/keyrings/debian-bookworm.gpg \
    && echo "deb [signed-by=/usr/share/keyrings/debian-bookworm.gpg] https://deb.debian.org/debian bookworm main" \
        > /etc/apt/sources.list.d/debian-bookworm.list
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt/lists,sharing=locked \
    apt-get update \
    && apt-get install -y --no-install-recommends -t bookworm chromium \
    && rm /etc/apt/sources.list.d/debian-bookworm.list

# mergerfs 2.41.1 — GitHub release static .deb（D-04 / M3 防御）
ARG MERGERFS_VERSION=2.41.1
ARG MERGERFS_SHA256_AMD64=118a95b62dc1dd16f1bb9529670ef412a78b4a1a0b576ffbd319b41511a5c12d
ARG MERGERFS_SHA256_ARM64=06161f44dff1050933566b25ca9d9a4599ae3aad857e53bf7c12b77e5ff808f5
RUN ARCH=$(dpkg --print-architecture) \
    && case "${ARCH}" in \
         amd64) EXPECTED_SHA="${MERGERFS_SHA256_AMD64}" ;; \
         arm64) EXPECTED_SHA="${MERGERFS_SHA256_ARM64}" ;; \
         *) echo "unsupported arch: ${ARCH}" >&2; exit 1 ;; \
       esac \
    && if [ -z "${EXPECTED_SHA}" ]; then \
         echo "MERGERFS sha256 not set for architecture: ${ARCH}" >&2; exit 1; \
       fi \
    && curl -fsSL -o /tmp/mergerfs.deb \
        "https://github.com/trapexit/mergerfs/releases/download/${MERGERFS_VERSION}/mergerfs_${MERGERFS_VERSION}.ubuntu-noble_${ARCH}.deb" \
    && echo "${EXPECTED_SHA}  /tmp/mergerfs.deb" | sha256sum -c - \
    && dpkg -i /tmp/mergerfs.deb \
    && rm /tmp/mergerfs.deb \
    && mergerfs --version

# 版本元数据（mergerfs / tmux 供控制面与诊断读取）
RUN mkdir -p /etc/cloud-claude \
    && echo "${MERGERFS_VERSION}" > /etc/cloud-claude/mergerfs.version \
    && echo "runtime-filled" > /etc/cloud-claude/tmux.version

RUN locale-gen en_US.UTF-8 zh_CN.UTF-8
ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

# v4.0: 删除 workspace 的 sudo NOPASSWD（IMG-04 / D-53-4）。
# 原 `echo "${WORKSPACE_USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/workspace` 已删除。
RUN userdel -f ubuntu 2>/dev/null || true \
    && groupdel ubuntu 2>/dev/null || true \
    && groupadd --force --gid "${WORKSPACE_GID}" "${WORKSPACE_USER}" \
    && useradd --uid "${WORKSPACE_UID}" --gid "${WORKSPACE_GID}" --home-dir /workspace --create-home --shell /bin/bash "${WORKSPACE_USER}"

# v4.0: singbox 系统账号（uid=9000 固定，避免与未来用户 uid 冲突）（IMG-02 / D-53-2）
RUN groupadd --force --gid 9000 singbox \
    && useradd --uid 9000 --gid 9000 --home-dir /nonexistent --no-create-home --shell /usr/sbin/nologin singbox

RUN adduser "${WORKSPACE_USER}" ssl-cert 2>/dev/null || true

# Claude Code 独立二进制安装（不依赖 Node.js）
# 重试策略：官方脚本最多尝试 3 次，失败后从 GitHub release 下载 binary 回退
RUN set -eux; \
    INSTALL_MAX_RETRIES=3; \
    installed=false; \
    for attempt in $(seq 1 ${INSTALL_MAX_RETRIES}); do \
      echo "Attempt ${attempt}/${INSTALL_MAX_RETRIES}: installing Claude Code via official script..."; \
      if curl -fsSL -o /tmp/claude-install.sh https://claude.ai/install.sh && bash /tmp/claude-install.sh; then \
        installed=true; \
        break; \
      fi; \
      echo "Attempt ${attempt} failed, waiting before retry..."; \
      sleep $((attempt * 2)); \
    done; \
    rm -f /tmp/claude-install.sh; \
    if [ "$installed" != "true" ]; then \
      echo "Official install failed after ${INSTALL_MAX_RETRIES} attempts, falling back to GitHub release binary..."; \
      ARCH=$(dpkg --print-architecture); \
      case "${ARCH}" in \
        amd64) GOARCH="x64" ;; \
        arm64) GOARCH="arm64" ;; \
        *) echo "unsupported arch: ${ARCH}" >&2; exit 1 ;; \
      esac; \
      LATEST=$(curl -fsSL https://api.github.com/repos/anthropics/claude-code/releases/latest | jq -r .tag_name); \
      if [ -z "${LATEST}" ] || [ "${LATEST}" = "null" ]; then \
        echo "Failed to get latest release version" >&2; exit 1; \
      fi; \
      BINARY_URL="https://github.com/anthropics/claude-code/releases/download/${LATEST}/claude-linux-${GOARCH}.tar.gz"; \
      echo "Downloading ${BINARY_URL}..."; \
      curl -fsSL -o /tmp/claude-binary.tar.gz "${BINARY_URL}"; \
      tar -xzf /tmp/claude-binary.tar.gz -C /tmp; \
      mv /tmp/claude /usr/local/bin/claude; \
      rm -f /tmp/claude-binary.tar.gz; \
    fi; \
    CLAUDE_BIN=""; \
    for candidate in /usr/local/bin/claude "${HOME}/.local/bin/claude" /root/.local/bin/claude; do \
      if [ -x "${candidate}" ]; then CLAUDE_BIN="${candidate}"; break; fi; \
    done; \
    if [ -z "${CLAUDE_BIN}" ]; then \
      echo "Claude binary not found in /usr/local/bin, ${HOME}/.local/bin, or /root/.local/bin" >&2; exit 1; \
    fi; \
    if [ "${CLAUDE_BIN}" != "/usr/local/bin/claude" ]; then \
      cp -fL "${CLAUDE_BIN}" /usr/local/bin/claude; \
    fi; \
    chmod +x /usr/local/bin/claude; \
    claude --version

# v4.0: sing-box tun 模式同容器透明代理（IMG-01 / D-53-1）。
# 与 deploy/docker/sing-box-gateway/Dockerfile 完全一致；版本 ARG 共享，
# 便于 Phase 54 退役 gateway 镜像时统一管理。
ARG SINGBOX_VERSION=1.13.3
RUN set -eux; \
    ARCH="$(dpkg --print-architecture)"; \
    curl -fsSL -o /tmp/sb.tgz "https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-${ARCH}.tar.gz"; \
    tar -xzf /tmp/sb.tgz -C /tmp; \
    install -m 0755 "/tmp/sing-box-${SINGBOX_VERSION}-linux-${ARCH}/sing-box" /usr/local/bin/sing-box; \
    rm -rf /tmp/sb.tgz "/tmp/sing-box-${SINGBOX_VERSION}-linux-${ARCH}"; \
    sing-box version

# v4.0: 给 sing-box binary 打文件 cap，进程拿 NET_ADMIN 而不需 root（D-V4-1 / D-53-2）。
# cap_net_admin: 必须，sing-box 建 tun 设备需要
# cap_net_bind_service: 备用，stub DNS 监听 127.0.0.1:53 需要绑定 <1024 端口
# +eip: Effective + Inheritable + Permitted
RUN setcap 'cap_net_admin+eip cap_net_bind_service+eip' /usr/local/bin/sing-box \
    && getcap /usr/local/bin/sing-box | grep -q 'cap_net_admin' \
    || { echo 'setcap failed' >&2; exit 1; }

RUN mkdir -p /workspace/.ssh /var/run/sshd \
    && chown -R "${WORKSPACE_UID}:${WORKSPACE_GID}" /workspace \
    && chmod 0700 /workspace/.ssh

# v3: 预建 mergerfs / 持久化 volume 挂载点 + chown 到 workspace 用户。
# C5 / M17 防御：named volume 首次挂载会继承镜像中目标路径的权限；
# entrypoint 会在运行时再做一次 chown 作为二次保险（Plan 03 prepare_v3_dirs）。
RUN mkdir -p \
        /home/claude \
        /home/claude/.claude \
        /home/claude/.cache/claude \
        /workspace-hot \
        /workspace-cold \
        /var/lib/claude-persist \
    && chown -R "${WORKSPACE_UID}:${WORKSPACE_GID}" \
        /home/claude \
        /workspace-hot \
        /workspace-cold \
        /var/lib/claude-persist

COPY deploy/docker/managed-user/sshd_config /etc/ssh/sshd_config
COPY deploy/docker/managed-user/tmux.conf /etc/tmux.conf
COPY deploy/docker/managed-user/profile.d-cloud-claude.sh /etc/profile.d/cloud-claude.sh
COPY deploy/docker/managed-user/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY deploy/docker/managed-user/launch-chromium.sh /usr/local/bin/launch-chromium.sh
COPY deploy/docker/managed-user/restart-vnc.sh /usr/local/bin/restart-vnc

# v4.0: 容器内 nft default-deny 规则集（D-V4-3 / NET-01 / NET-02）
COPY deploy/docker/managed-user/default-deny.nft /etc/cloud-claude/default-deny.nft
RUN chmod 0644 /etc/cloud-claude/default-deny.nft

RUN chmod 0644 /etc/tmux.conf /etc/profile.d/cloud-claude.sh
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/launch-chromium.sh /usr/local/bin/restart-vnc

WORKDIR /workspace

EXPOSE 22
EXPOSE 6080

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
