#!/usr/bin/env bash
#
# SkillsManager (sm) 一键安装脚本。
#
# 从 GitHub 的 latest release 拉取与当前平台匹配的预编译二进制,
# 解压安装到 ~/.local/bin(可用 BIN_DIR 环境变量覆盖)。
#
# 用法:
#   curl -fsSL https://raw.githubusercontent.com/woyin/SkillsManager/latest/install.sh | bash
#
set -euo pipefail

REPO="woyin/SkillsManager"

# --- 检测操作系统与 CPU 架构 ----------------------------------------------
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"   # darwin / linux
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64)  ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *) echo "✗ 不支持的 CPU 架构: $ARCH" >&2; exit 1 ;;
esac
case "$OS" in
  darwin|linux) ;;
  *) echo "✗ 不支持的操作系统: $OS(仅支持 macOS / Linux)" >&2; exit 1 ;;
esac

# --- 解析 latest release 的 tag(经 GitHub API 的 latest 重定向)-----------
TAG="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
  | grep -m1 '"tag_name"' \
  | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')"
if [ -z "${TAG}" ]; then
  echo "✗ 无法获取 latest release(检查网络或 GitHub API 速率限制)" >&2
  exit 1
fi

ASSET="sm_${TAG}_${OS}_${ARCH}.tar.gz"
URL="https://github.com/${REPO}/releases/download/${TAG}/${ASSET}"

# --- 确定安装目录 ---------------------------------------------------------
INSTALL_DIR="${BIN_DIR:-${HOME}/.local/bin}"
mkdir -p "${INSTALL_DIR}"

# --- 下载并解压 -----------------------------------------------------------
TMP="$(mktemp -d)"
trap 'rm -rf "${TMP}"' EXIT

echo "→ 下载 ${URL}"
curl -fsSL "${URL}" -o "${TMP}/${ASSET}"

echo "→ 解压到 ${INSTALL_DIR}"
tar -xzf "${TMP}/${ASSET}" -C "${INSTALL_DIR}"
chmod +x "${INSTALL_DIR}/sm"

# --- 校验并给出 PATH 提示 -------------------------------------------------
INSTALLED_VERSION="$("${INSTALL_DIR}/sm" --version 2>/dev/null || echo "unknown")"
echo "✓ 已安装 sm ${INSTALLED_VERSION}"
echo "  位置: ${INSTALL_DIR}/sm"

case ":${PATH}:" in
  *":${INSTALL_DIR}:"*) ;;
  *) echo "  ⚠ ${INSTALL_DIR} 不在 PATH 中,请将其加入(如 export PATH=\"${INSTALL_DIR}:\$PATH\")" ;;
esac
