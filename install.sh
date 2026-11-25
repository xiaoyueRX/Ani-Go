#!/bin/sh
# 安装 GitHub Releases 中的 Linux 或 macOS 二进制包。
# curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | sh
# curl -fsSL https://raw.githubusercontent.com/xiaoyueRX/Ani-Go/main/install.sh | ANIGO_VERSION=v0.7.0 sh
set -eu

repo=xiaoyueRX/Ani-Go
install_dir=${ANIGO_INSTALL_DIR:-"$HOME/.anigo"}
bin_dir=${ANIGO_BIN_DIR:-"$HOME/.local/bin"}

die() { printf '%s\n' "$*" >&2; exit 1; }

case ${1:-} in
  --uninstall)
    [ "$#" -eq 1 ] || die '用法: sh install.sh [--uninstall]'
    if [ -d "$install_dir" ]; then
      install_dir=$(cd "$install_dir" && pwd -P)
      case $(readlink "$bin_dir/anigo" 2>/dev/null || :) in
        "$install_dir"/current/anigo|"$install_dir"/versions/v*/anigo) rm -f "$bin_dir/anigo" ;;
      esac
      previous=$(readlink "$install_dir/current" 2>/dev/null || :)
      case $previous in
        "$install_dir"/versions/v*) rm -f "$install_dir/current" ;;
      esac
      for dest in "$install_dir"/versions/v*; do
        [ -d "$dest" ] && [ ! -L "$dest" ] && [ -f "$dest/anigo" ] || continue
        [ -f "$dest/.anigo-installed" ] || [ "$dest" = "$previous" ] || continue
        rm -f "$dest/anigo" "$dest/.env.example" "$dest/LICENSE" "$dest/README.md" "$dest/.anigo-installed"
        rmdir "$dest" 2>/dev/null || :
      done
      rmdir "$install_dir/versions" 2>/dev/null || :
    fi
    printf '已卸载安装器管理的程序，配置和运行数据保留在 %s。\n' "$install_dir"
    exit 0 ;;
  '') [ "$#" -eq 0 ] || die '用法: sh install.sh [--uninstall]' ;;
  *) die '用法: sh install.sh [--uninstall]' ;;
esac

case $(uname -s) in
  Darwin) os=darwin ;;
  Linux) os=linux ;;
  *) die "暂不支持的操作系统: $(uname -s)" ;;
esac
case $(uname -m) in
  arm64|aarch64) arch=arm64 ;;
  x86_64|amd64) arch=amd64 ;;
  i[3-6]86|x86) die 'Ani-Go 不提供 32 位 x86 发行包。' ;;
  armv7*|armhf) [ "$os" = linux ] || die 'armv7 发行包仅支持 Linux'; arch=armv7 ;;
  *) die "暂不支持的架构: $(uname -m)" ;;
esac

fetch() { curl -fsSL --retry 2 --connect-timeout 10 --max-time 180 "$@"; }
version=${ANIGO_VERSION:-}
[ "$version" != latest ] || version=
legacy_platform=
case $os-$arch in
  darwin-amd64)
    # Rosetta 中 uname 显示 x86_64，宿主可直接运行 Apple Silicon 原生程序。
    if [ "$(sysctl -n sysctl.proc_translated 2>/dev/null || :)" = 1 ]; then
      arch=arm64
    else
      legacy_platform='Intel Mac'
    fi ;;
  linux-armv7) legacy_platform='32 位 ARMv7 Linux' ;;
esac
if [ -n "$legacy_platform" ]; then
  case $version in
    0.[0-7].*|v0.[0-7].*) ;; # 仅允许显式固定历史版本，并照常核验官方校验清单。
    *) die "Ani-Go 从 v0.8.0 起不再为 $legacy_platform 提供发行包。需要历史版本时可指定 ANIGO_VERSION=v0.7.0。" ;;
  esac
fi
if [ -z "$version" ]; then
  release_url=$(fetch -I -o /dev/null -w '%{url_effective}' "https://github.com/$repo/releases/latest") || release_url=
  case $release_url in
    "https://github.com/$repo/releases/tag/"*) version=${release_url##*/} ;;
  esac
  if [ -z "$version" ]; then
    metadata=$(fetch "https://api.github.com/repos/$repo/releases/latest") || die '无法查询最新发行版，请检查网络或设置 ANIGO_VERSION。'
    version=$(printf '%s' "$metadata" | sed -n 's/.*"tag_name" *: *"\([^"]*\)".*/\1/p')
  fi
fi
case $version in v*) ;; *) version=v$version ;; esac
case $version in *[!a-zA-Z0-9.+-]*) die '发行版标签无效' ;; esac
printf '%s\n' "$version" | grep -Eq '^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$' || die '发行版标签无效'

pkg=ani-go-$version-$os-$arch
asset=$pkg.tar.gz
release=https://github.com/$repo/releases/download/$version
tmp=$(mktemp -d)
stage=
trap 'rm -rf "$tmp"; if [ -n "$stage" ]; then rm -rf "$stage"; fi' 0
trap 'exit 1' HUP INT TERM

printf '正在下载 %s 并核验 SHA-256…\n' "$asset"
fetch "$release/checksums.txt" -o "$tmp/checksums.txt" || die '无法下载官方校验清单。'
expected=$(awk -v name="$asset" 'NF == 2 && ($2 == name || $2 == "*" name) { print $1; n++ } END { if (n != 1) exit 1 }' "$tmp/checksums.txt") || die "该版本的官方清单中缺少唯一匹配的发行包: $asset"
fetch "$release/$asset" -o "$tmp/archive" || die "无法下载发行包: $release/$asset"
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum < "$tmp/archive")
else
  actual=$(shasum -a 256 < "$tmp/archive") || die '需要 sha256sum 或 shasum。'
fi
[ "${actual%% *}" = "$expected" ] || die '发行包校验失败，已保留原安装。'

mkdir -p "$install_dir/versions" "$bin_dir"
install_dir=$(cd "$install_dir" && pwd -P)
bin_dir=$(cd "$bin_dir" && pwd -P)
dest=$install_dir/versions/$version
[ ! -L "$dest" ] || die "安装路径是符号链接: $dest"
for link in "$bin_dir/anigo" "$install_dir/current"; do
  [ -L "$link" ] || [ ! -e "$link" ] || die "目标路径已经存在: $link"
done
stage=$(mktemp -d "$install_dir/versions/.install.XXXXXXXX")
tar -xzf "$tmp/archive" -C "$stage"
program=$stage/$pkg/anigo
[ -s "$program" ] && [ -f "$program" ] && [ ! -L "$program" ] || die '发行包中缺少 anigo。'
chmod 755 "$program"
mkdir -p "$dest"
for file in .env.example README.md LICENSE; do
  if [ -f "$stage/$pkg/$file" ]; then
    mv -f "$stage/$pkg/$file" "$dest/$file"
  fi
done
mv -f "$program" "$dest/anigo"
: > "$dest/.anigo-installed"
if [ ! -e "$install_dir/.env" ] && [ -f "$dest/.env.example" ]; then
  (umask 077; set -C; cat "$dest/.env.example" > "$install_dir/.env") || [ -f "$install_dir/.env" ]
fi
# 旧版安装器的两个链接曾直接指向同一发行目录，升级时记录其归属。
previous=$(readlink "$install_dir/current" 2>/dev/null || :)
case $previous in
  "$install_dir"/versions/v*)
    if [ "${previous%/*}" = "$install_dir/versions" ] && [ ! -L "$previous" ] && [ -f "$previous/anigo" ] &&
       [ "$(readlink "$bin_dir/anigo" 2>/dev/null || :)" = "$previous/anigo" ]; then
      : > "$previous/.anigo-installed"
    fi ;;
esac
ln -sfn "$dest" "$install_dir/current"
ln -sfn "$install_dir/current/anigo" "$bin_dir/anigo"
printf '已安装 Ani-Go %s。\n配置目录: %s\n程序: %s/anigo\n启动时请先进入配置目录，让程序读取 .env 和原有数据库。\n' "$version" "$install_dir" "$bin_dir"
