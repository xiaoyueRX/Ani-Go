#!/usr/bin/env bash
# 在原生 CI 主机上准备 Transmission，供同一作业内的下载和备份恢复步骤复用。
set -euo pipefail

if [[ "$RUNNER_OS" == Windows ]]; then
  curl --fail --silent --show-error --location --output transmission.msi https://github.com/transmission/transmission/releases/download/4.1.4/transmission-4.1.4-x64.msi
  echo "cf51e5772f39a3ead3c8f763b689399b9b4b8b016a4c0ca68ab5d6c634be2e1c  transmission.msi" | sha256sum --check
  pwsh -NoProfile -Command '
    $out = Join-Path $env:RUNNER_TEMP transmission-files
    $msi = (Resolve-Path transmission.msi).Path
    $result = Start-Process msiexec.exe -Wait -PassThru -ArgumentList "/a `"$msi`" /qn TARGETDIR=`"$out`""
    exit $result.ExitCode'
  daemon=$(find "$(cygpath -u "$RUNNER_TEMP")/transmission-files" -iname transmission-daemon.exe -print -quit)
elif [[ "$RUNNER_OS" == macOS ]]; then
  if ! brew list --versions transmission-cli >/dev/null; then
    HOMEBREW_NO_AUTO_UPDATE=1 brew install transmission-cli
  fi
  daemon="$(brew --prefix transmission-cli)/bin/transmission-daemon"
else
  if ! command -v transmission-daemon >/dev/null; then
    sudo systemctl mask --runtime transmission-daemon.service
    sudo apt-get update -qq
    sudo apt-get install -y --no-install-recommends transmission-daemon
  fi
  daemon=$(command -v transmission-daemon)
fi
test -x "$daemon"
file "$daemon"
printf 'executable=%s\n' "$daemon" >> "$GITHUB_OUTPUT"
