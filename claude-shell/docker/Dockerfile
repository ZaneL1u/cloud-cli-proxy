FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# ---------------------------------------------------------------------------
# 1. 系统工具安装（最小必要集 — D-02）
# ---------------------------------------------------------------------------
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        curl git jq procps iproute2 ca-certificates bash sudo nftables tzdata \
    && rm -rf /var/lib/apt/lists/*

# ---------------------------------------------------------------------------
# 2. sing-box 二进制安装（D-03，复用 sing-box-gateway 模式）
# ---------------------------------------------------------------------------
ARG SINGBOX_VERSION=1.13.3
RUN set -eux; \
    ARCH="$(dpkg --print-architecture)"; \
    curl -fsSL -o /tmp/sb.tgz \
      "https://github.com/SagerNet/sing-box/releases/download/v${SINGBOX_VERSION}/sing-box-${SINGBOX_VERSION}-linux-${ARCH}.tar.gz"; \
    tar -xzf /tmp/sb.tgz -C /tmp; \
    install -m 0755 "/tmp/sing-box-${SINGBOX_VERSION}-linux-${ARCH}/sing-box" /usr/local/bin/sing-box; \
    rm -rf /tmp/sb.tgz "/tmp/sing-box-${SINGBOX_VERSION}-linux-${ARCH}"; \
    sing-box version

# ---------------------------------------------------------------------------
# 3. 容器工作用户创建（UID 1000, GID 1000, sudoers NOPASSWD）
# ---------------------------------------------------------------------------
ARG CLAUDE_USER=claude
ARG CLAUDE_UID=1000
ARG CLAUDE_GID=1000

RUN userdel -f ubuntu 2>/dev/null || true \
    && groupdel ubuntu 2>/dev/null || true \
    && groupadd --force --gid "${CLAUDE_GID}" "${CLAUDE_USER}" \
    && useradd --uid "${CLAUDE_UID}" --gid "${CLAUDE_GID}" \
       --home-dir "/home/${CLAUDE_USER}" --create-home \
       --shell /bin/bash "${CLAUDE_USER}" \
    && echo "${CLAUDE_USER} ALL=(ALL) NOPASSWD:ALL" \
       > "/etc/sudoers.d/${CLAUDE_USER}" \
    && chmod 0440 "/etc/sudoers.d/${CLAUDE_USER}"

# ---------------------------------------------------------------------------
# 4. Claude Code 安装（D-04, D-05 — 必须在 USER claude 之后执行）
#    重试策略：官方脚本最多尝试 3 次，失败后直接下载 binary 回退
# ---------------------------------------------------------------------------
USER ${CLAUDE_USER}

ARG CLAUDE_CODE_VERSION=latest
ARG INSTALL_MAX_RETRIES=3

RUN set -eux; \
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
      echo "Official install script failed after ${INSTALL_MAX_RETRIES} attempts, falling back to GitHub release..."; \
      ARCH="$(dpkg --print-architecture)"; \
      if [ "${CLAUDE_CODE_VERSION}" = "latest" ]; then \
        CLAUDE_CODE_VERSION="$(curl -fsSL https://api.github.com/repos/anthropics/claude-code/releases/latest | jq -r '.tag_name' | sed 's/^v//')"; \
      fi; \
      curl -fsSL -o /tmp/claude.tar.gz \
        "https://github.com/anthropics/claude-code/releases/download/v${CLAUDE_CODE_VERSION}/claude-linux-${ARCH}.tar.gz"; \
      mkdir -p "${HOME}/.local/bin"; \
      tar -xzf /tmp/claude.tar.gz -C "${HOME}/.local/bin" claude; \
      chmod 0755 "${HOME}/.local/bin/claude"; \
      rm -f /tmp/claude.tar.gz; \
    fi; \
    "${HOME}/.local/bin/claude" --version

# ---------------------------------------------------------------------------
# 5. PATH 和环境变量（D-06）
# ---------------------------------------------------------------------------
ENV PATH="/home/claude/.local/bin:${PATH}"
ENV DISABLE_AUTOUPDATER=1

# ---------------------------------------------------------------------------
# 6. entrypoint 安装（需要 root 权限 COPY 到 /usr/local/bin）
# ---------------------------------------------------------------------------
USER root
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# ---------------------------------------------------------------------------
# 7. 最终设置（exec form ENTRYPOINT 避免 shell 成为 PID 1）
# ---------------------------------------------------------------------------
USER ${CLAUDE_USER}
WORKDIR /home/claude
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
