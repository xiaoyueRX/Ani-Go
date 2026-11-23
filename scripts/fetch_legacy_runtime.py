"""从 v0.7.0 官方发行包提取用于升级验收的旧程序。"""

import argparse
import hashlib
from pathlib import Path
import shutil
import tarfile
import tempfile
from urllib.request import urlopen
import zipfile


RELEASE = "v0.7.0"
BASE_URL = f"https://github.com/xiaoyueRX/Ani-Go/releases/download/{RELEASE}"
# 与发布时的 checksums.txt 和 GitHub Release 资产摘要逐一核对。
SHA256 = {
    "linux-amd64": "7e0d45f85a6bf3753543eedf8500517b8b22f21ea07ed805fa84f915b34d0cb1",
    "linux-arm64": "f0f75ed946dc7976417cc481250ee3c564313f3872bd230ed7819ed046d91337",
    "linux-armv7": "465bbb3e27e66fc7d87276cd485f95bcaefa74aba3105bb4c72d349996816add",
    "darwin-amd64": "af57962615d2b13cf5c680879d434172483cfd1a8fa8557cc7693f907dad4d73",
    "darwin-arm64": "7ad4ede5707fbdda0039cf61208543afc39224d6a049ebd169fcd8f5e417999b",
    "windows-amd64": "7d2668e51d2068d30e4fb1f83a169973df24749e13b01a3520de268ec4ceb9ea",
    "windows-arm64": "8e814482d0ed795376b4d96dff1170aa0795e77f2fcc1d79e323f57995a73f48",
}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--os", required=True, choices=("linux", "darwin", "windows"))
    parser.add_argument("--arch", required=True, choices=("amd64", "arm64", "armv7"))
    parser.add_argument("--output", required=True, type=Path, help="临时测试目录中的程序路径")
    args = parser.parse_args()
    target = f"{args.os}-{args.arch}"
    if target not in SHA256:
        parser.error(f"{RELEASE} 没有发布 {target} 程序")

    package = f"ani-go-{RELEASE}-{target}"
    windows = args.os == "windows"
    filename = package + (".zip" if windows else ".tar.gz")
    binary = "anigo.exe" if windows else "anigo"
    entry = f"{package}/{binary}"
    with tempfile.TemporaryDirectory(dir=args.output.parent) as tmp:
        archive = Path(tmp) / filename
        digest = hashlib.sha256()
        with urlopen(f"{BASE_URL}/{filename}", timeout=60) as source, archive.open("wb") as dest:
            while chunk := source.read(1024 * 1024):
                digest.update(chunk)
                dest.write(chunk)
        if digest.hexdigest() != SHA256[target]:
            raise ValueError(f"{filename} SHA256 校验失败")

        staged = Path(tmp) / binary
        with staged.open("wb") as dest:
            if windows:
                with zipfile.ZipFile(archive) as package_zip, package_zip.open(entry) as source:
                    shutil.copyfileobj(source, dest)
            else:
                with tarfile.open(archive, "r:gz") as package_tar:
                    member = package_tar.getmember(entry)
                    if not member.isfile() or not member.mode & 0o100:
                        raise ValueError(f"{entry} 不是可执行的普通文件")
                    with package_tar.extractfile(member) as source:
                        shutil.copyfileobj(source, dest)
        staged.chmod(0o755)
        staged.replace(args.output)
    print(f"{filename} SHA256={digest.hexdigest()} -> {args.output}")


if __name__ == "__main__":
    main()
