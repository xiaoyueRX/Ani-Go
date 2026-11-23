#!/usr/bin/env bash
set -euo pipefail

if [[ ${GITHUB_ACTIONS:-} != true || ${RUNNER_ENVIRONMENT:-} != github-hosted || ${RUNNER_OS:-} != macOS || ! -d ${RUNNER_TEMP:-} ]]; then
  echo "This fixture requires a GitHub-hosted macOS runner" >&2
  exit 1
fi
if [[ $# != 2 ]]; then
  echo "Usage: $0 <Ani-Go executable> <qBittorrent executable>" >&2
  exit 2
fi

probe="$(dirname "$0")/test_cross_volume_runtime.py"
scratch=$(mktemp -d "$RUNNER_TEMP/anigo-cross-volume.XXXXXX")
mountpoint="$scratch/media"
image="$scratch/library.sparseimage"

cleanup() {
  local status=$?
  trap - EXIT
  if [[ -d "$mountpoint" && $(stat -f %d "$mountpoint") != "$(stat -f %d "$scratch")" ]]; then
    hdiutil detach "$mountpoint" || hdiutil detach -force "$mountpoint" || exit 1
  fi
  rm -f "$image"
  [[ ! -d "$mountpoint" ]] || rmdir "$mountpoint"
  rmdir "$scratch"
  echo "Cross-volume fixture cleanup complete"
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

mkdir "$mountpoint"
hdiutil create -size 256m -type SPARSE -fs APFS -volname AniGoCI "$image"
hdiutil attach -nobrowse -mountpoint "$mountpoint" "$image"

source_dir=$(python3 -c 'import tempfile; print(tempfile.gettempdir())')
source_device=$(df -P "$source_dir" | awk 'NR == 2 {print $1}')
source_uuid=$(diskutil info -plist "$source_device" | plutil -extract VolumeUUID raw -o - -)
target_info=$(diskutil info -plist "$mountpoint")
target_uuid=$(plutil -extract VolumeUUID raw -o - - <<<"$target_info")
target_mount=$(plutil -extract MountPoint raw -o - - <<<"$target_info")
target_fs=$(plutil -extract FilesystemType raw -o - - <<<"$target_info")
if [[ -z "$source_uuid" || -z "$target_uuid" || "$source_uuid" == "$target_uuid" || "$target_fs" != apfs || ! "$mountpoint" -ef "$target_mount" ]]; then
  echo "The private APFS image must be mounted on a distinct volume" >&2
  exit 1
fi
echo "Cross-volume fixture: source=$source_uuid destination=$target_uuid type=APFS"
python3 "$probe" "$1" "$2" "$mountpoint"
