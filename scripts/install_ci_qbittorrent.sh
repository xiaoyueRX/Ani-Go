#!/usr/bin/env bash
# 在隔离的原生 CI 主机上准备 qBittorrent，供本次作业的各项验收复用。
set -euo pipefail

if [[ "$RUNNER_OS" == Windows ]]; then
  installer=qbittorrent_5.3.0_x64_setup.exe
  curl --fail --silent --show-error --location --output "$installer" "https://github.com/qbittorrent/qBittorrent/releases/download/release-5.3.0/$installer"
  echo "aff5b3bdc00d6e5773d6099ba30f9696dd3dfd1669ab0f64ce5c4e7168486436  $installer" | sha256sum --check
  pwsh -NoProfile -Command '$p = Start-Process .\qbittorrent_5.3.0_x64_setup.exe -ArgumentList /S -Wait -PassThru; exit $p.ExitCode'
  qb=$(pwsh -NoProfile -Command 'Join-Path $env:ProgramFiles qBittorrent/qbittorrent.exe' | tr -d '\r')
  qb=$(cygpath -u "$qb")
elif [[ "$RUNNER_OS" == macOS ]]; then
  dmg=qbittorrent-5.2.3.dmg
  curl --fail --silent --show-error --location --output "$dmg" "https://github.com/qbittorrent/qBittorrent/releases/download/release-5.2.3/$dmg"
  echo "9e37f6c7ff848c7bdd3c10167614c0cb78c00e2ddcc323f1ad3ac6c008a0481f  $dmg" | shasum -a 256 --check
  volume="$RUNNER_TEMP/qb-volume"
  mkdir "$volume"
  hdiutil attach -readonly -nobrowse -mountpoint "$volume" "$dmg"
  trap 'hdiutil detach "$volume"' EXIT
  app="$RUNNER_TEMP/qbittorrent.app"
  ditto "$volume/qbittorrent.app" "$app"
  hdiutil detach "$volume" || hdiutil detach -force "$volume"
  trap - EXIT
  qb="$app/Contents/MacOS/qbittorrent"
  test ! -e "$volume/qbittorrent.app"
  lipo -archs "$qb"
else
  if ! command -v qbittorrent-nox >/dev/null; then
    sudo apt-get update -qq
    sudo apt-get install -y --no-install-recommends qbittorrent-nox
  fi
  qb=$(command -v qbittorrent-nox)
fi
printf 'executable=%s\n' "$qb" >> "$GITHUB_OUTPUT"
