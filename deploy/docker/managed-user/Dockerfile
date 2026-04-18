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

COPY deploy/docker/managed-user/sshd_config /etc/ssh/sshd_config
COPY deploy/docker/managed-user/entrypoint.sh /usr/local/bin/entrypoint.sh
COPY deploy/docker/managed-user/launch-chromium.sh /usr/local/bin/launch-chromium.sh
COPY deploy/docker/managed-user/restart-vnc.sh /usr/local/bin/restart-vnc
COPY deploy/docker/managed-user/claude-wrapper.sh /usr/local/bin/claude

RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/launch-chromium.sh /usr/local/bin/restart-vnc /usr/local/bin/claude

WORKDIR /workspace

EXPOSE 22
EXPOSE 6080

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
