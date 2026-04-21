# syntax=docker/dockerfile:1.7
FROM ubuntu:24.04

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
        nodejs \
        npm \
        locales \
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
    && rm -rf /var/lib/apt/lists/*

# Chromium：Ubuntu 24.04 的 chromium-browser 是 snap 包装器，Docker 不支持 snap。
# 从 Debian bookworm 仓库安装真正的 .deb（支持 amd64 + arm64）。
RUN echo "deb [trusted=yes arch=$(dpkg --print-architecture)] http://deb.debian.org/debian bookworm main" \
        > /etc/apt/sources.list.d/debian-chromium.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t bookworm chromium \
    && rm /etc/apt/sources.list.d/debian-chromium.list \
    && apt-get update \
    && rm -rf /var/lib/apt/lists/*

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

RUN locale-gen en_US.UTF-8
ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

RUN userdel -f ubuntu 2>/dev/null || true \
    && groupdel ubuntu 2>/dev/null || true \
    && groupadd --force --gid "${WORKSPACE_GID}" "${WORKSPACE_USER}" \
    && useradd --uid "${WORKSPACE_UID}" --gid "${WORKSPACE_GID}" --home-dir /workspace --create-home --shell /bin/bash "${WORKSPACE_USER}" \
    && echo "${WORKSPACE_USER} ALL=(ALL) NOPASSWD:ALL" >/etc/sudoers.d/workspace \
    && chmod 0440 /etc/sudoers.d/workspace

RUN adduser "${WORKSPACE_USER}" ssl-cert 2>/dev/null || true

RUN npm install -g @anthropic-ai/claude-code \
    && CLAUDE_BIN="$(command -v claude)" \
    && mv "${CLAUDE_BIN}" /usr/local/bin/claude-real \
    && chmod +x /usr/local/bin/claude-real

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
COPY deploy/docker/managed-user/claude-wrapper.sh /usr/local/bin/claude

RUN chmod 0644 /etc/tmux.conf /etc/profile.d/cloud-claude.sh
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/launch-chromium.sh /usr/local/bin/restart-vnc /usr/local/bin/claude

WORKDIR /workspace

EXPOSE 22
EXPOSE 6080

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
