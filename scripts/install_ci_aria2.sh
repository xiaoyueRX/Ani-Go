#!/usr/bin/env bash
# 为同一原生 CI 作业中的选集下载和备份恢复准备 aria2c。
set -euo pipefail

if [[ "$RUNNER_OS" == Windows ]]; then
  archive=aria2-1.37.0-win-64bit-build1.zip
  cd "$(cygpath -u "$RUNNER_TEMP")"
  curl --fail --silent --show-error --location --output "$archive" "https://github.com/aria2/aria2/releases/download/release-1.37.0/$archive"
  echo "67d015301eef0b612191212d564c5bb0a14b5b9c4796b76454276a4d28d9b288  $archive" | sha256sum --check
  unzip -q -o "$archive"
  aria="$PWD/${archive%.zip}/aria2c.exe"
elif [[ "$RUNNER_OS" == macOS ]]; then
  command -v aria2c >/dev/null || HOMEBREW_NO_AUTO_UPDATE=1 brew install aria2
  aria=$(command -v aria2c)
else
  if ! command -v aria2c >/dev/null; then
    sudo apt-get update -qq
    sudo apt-get install -y --no-install-recommends aria2
  fi
  aria=$(command -v aria2c)
fi
test -x "$aria"
file "$aria"
printf 'executable=%s\n' "$aria" >> "$GITHUB_OUTPUT"
