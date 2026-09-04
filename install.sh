#!/bin/sh
#
# Ani-Go 独立单文件二进制一键安装脚本
# 自动从 GitHub Releases 下载对应操作系统与架构的最新二进制包并安装到系统 PATH 中。
#
# 快速安装命令：
#   curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | sh
#
# 指定版本安装：
#   ANIGO_VERSION=v0.5.1 curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | sh
#
# 卸载：
#   curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | sh -s -- --uninstall
#
set -eu

REPO="xiaoyueRX/Ani-Go"
INSTALL_DIR="${ANIGO_INSTALL_DIR:-$HOME/.anigo}"
BIN_DIR="${ANIGO_BIN_DIR:-$HOME/.local/bin}"

if [ "${1:-}" = "--uninstall" ]; then
  rm -f "$BIN_DIR/anigo"
  rm -rf "$INSTALL_DIR"
  echo "✅ Ani-Go 已成功卸载 (清理了 $INSTALL_DIR 与 $BIN_DIR/anigo)。"
  exit 0
fi

# 1. 检测系统与硬件架构
os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Darwin) os="darwin" ;;
  Linux)  os="linux" ;;
  *) echo "❌ 暂不支持的操作系统: '$os'" >&2; exit 1 ;;
esac

case "$arch" in
  arm64|aarch64) arch="arm64" ;;
  x86_64|amd64)  arch="amd64" ;;
  armv7*|armhf)  arch="armv7" ;;
  *) echo "❌ 暂不支持的架构: '$arch'" >&2; exit 1 ;;
esac

# 2. 解析目标版本号 (默认抓取最新版本)
version="${ANIGO_VERSION:-}"
if [ -z "$version" ]; then
  version="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" 2>/dev/null \
    | sed -n 's#.*/releases/tag/##p' || true)"
fi

if [ -z "$version" ]; then
  version="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -n1 || true)"
fi

# 默认回退版本
if [ -z "$version" ]; then
  version="v0.5.1"
fi

case "$version" in v*) ;; *) version="v$version" ;; esac

# 3. 下载并解压发布包
pkg_name="ani-go-${version}-${os}-${arch}"
archive_name="${pkg_name}.tar.gz"
url="https://github.com/$REPO/releases/download/${version}/${archive_name}"

echo "🚀 正在为 ${os}/${arch} 下载 Ani-Go ${version}..."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

if ! curl -fSL "$url" -o "$tmp/$archive_name" 2>/dev/null; then
  # 国内加速镜像备用尝试
  mirror_url="https://mirror.ghproxy.com/${url}"
  echo "ℹ️  从主源下载失败，正在尝试国内镜像源下载..."
  curl -fSL "$mirror_url" -o "$tmp/$archive_name" || {
    echo "❌ 下载失败: 找不到对应架构的发布包 ($url)" >&2
    exit 1
  }
fi

dest="$INSTALL_DIR/versions/$version"
rm -rf "$dest"
mkdir -p "$dest"
tar -xzf "$tmp/$archive_name" -C "$dest" --strip-components=1

# 4. 创建可执行软链接
mkdir -p "$BIN_DIR"
chmod +x "$dest/anigo"
ln -sf "$dest/anigo" "$BIN_DIR/anigo"
ln -sfn "$dest" "$INSTALL_DIR/current"

# 如果没有 .env 则自动提供默认示例
if [ ! -f "$INSTALL_DIR/.env" ] && [ -f "$dest/.env.example" ]; then
  cp "$dest/.env.example" "$INSTALL_DIR/.env"
fi

echo ""
echo "🎉 Ani-Go 安装成功！"
echo "  📂 安装目录: $dest"
echo "  🔗 执行软链: $BIN_DIR/anigo"
echo "  ⚙️  配置模板: $INSTALL_DIR/.env"

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *)
    echo ""
    echo "⚠️  注意: $BIN_DIR 尚未加入到系统 PATH。请将以下内容加入 ~/.bashrc 或 ~/.zshrc:"
    echo "    export PATH=\"$BIN_DIR:\$PATH\""
    ;;
esac

echo ""
echo "💡 启动方式:"
echo "  anigo"
echo ""
