"""Exercise the release installer with local archives and an offline curl stand-in."""

import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"
CURL = """#!/bin/sh
set -eu
for arg do
    case "$arg" in https://*) url=$arg ;; esac
done
case $url in
    https://github.com/xiaoyueRX/Ani-Go/releases/latest)
        [ -z "${OFFLINE:-}" ] || exit 22
        printf 'https://github.com/xiaoyueRX/Ani-Go/releases/tag/v1.1.0' ;;
    https://api.github.com/repos/xiaoyueRX/Ani-Go/releases/latest)
        exit 22 ;;
    https://github.com/xiaoyueRX/Ani-Go/releases/download/*)
        while [ "$1" != -o ]; do shift; done
        cp "$FIXTURES/${url#*/releases/download/}" "$2" ;;
    *) exit 22 ;;
esac
"""


class InstallerTest(unittest.TestCase):
    def setUp(self):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        self.root = Path(tmp.name)
        path = self.root / "commands"
        path.mkdir()
        (path / "curl").write_text(CURL)
        (path / "uname").write_text(
            '#!/bin/sh\ncase "$1" in -s) echo "$TEST_OS";; -m) echo "$TEST_ARCH";; esac\n'
        )
        for command in path.iterdir():
            command.chmod(0o755)
        self.env = {
            "HOME": str(self.root), "PATH": f"{path}:{os.environ['PATH']}",
            "FIXTURES": str(self.root / "releases"), "TEST_OS": "Linux", "TEST_ARCH": "x86_64",
        }
        self.program = self.root / ".local/bin/anigo"
        self.state = self.root / ".anigo"

    def release(self, version, platform="linux-amd64", executable=True):
        folder = self.root / "releases" / version
        folder.mkdir(parents=True, exist_ok=True)
        package = f"ani-go-{version}-{platform}"
        archive = folder / f"{package}.tar.gz"
        files = {".env.example": b"sample"}
        if executable:
            files["anigo"] = f"#!/bin/sh\necho {version}\n".encode()
        with tarfile.open(archive, "w:gz") as tar:
            for name, data in files.items():
                entry = tarfile.TarInfo(f"{package}/{name}")
                entry.size = len(data)
                tar.addfile(entry, io.BytesIO(data))
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (folder / "checksums.txt").write_text(f"{digest}  {archive.name}\n")
        return archive

    def install(self, *args, success=True, **overrides):
        result = subprocess.run(
            ["sh", str(INSTALLER), *args], cwd=self.root, env={**self.env, **overrides},
            capture_output=True, text=True, timeout=10,
        )
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)

    def test_upgrade_recovery_and_uninstall(self):
        self.release("v1.0.0")
        self.install(ANIGO_VERSION="1.0.0")
        config = self.state / ".env"
        self.assertEqual(config.read_text(), "sample")
        config.write_text("user settings")
        database = self.state / "ani-go.db"
        database.write_text("existing database")

        # Reproduce the original installer's unmarked directory and direct command link.
        old_release = self.program.resolve().parent
        (old_release / ".anigo-installed").unlink()
        self.program.unlink()
        self.program.symlink_to(old_release / "anigo")

        self.install(success=False, OFFLINE="1")
        self.install(success=False, ANIGO_VERSION="v9.0.0")
        self.release("v1.1.0").write_bytes(b"incomplete download")
        self.install(success=False)
        self.release("v1.0.0", executable=False)
        self.install(success=False, ANIGO_VERSION="1.0.0")
        self.assertEqual(subprocess.check_output([self.program], text=True).strip(), "v1.0.0")

        self.release("v1.1.0")
        self.install()
        self.assertEqual(subprocess.check_output([self.program], text=True).strip(), "v1.1.0")
        self.install("--uninstall")
        self.assertEqual(config.read_text(), "user settings")
        self.assertEqual(database.read_text(), "existing database")
        self.assertFalse(self.program.is_symlink())
        self.assertFalse((self.state / "versions").exists())

    def test_platform_archives(self):
        for system, arch, package in [
            ("Linux", "armv7l", "linux-armv7"), ("Linux", "aarch64", "linux-arm64"),
            ("Darwin", "x86_64", "darwin-amd64"), ("Darwin", "arm64", "darwin-arm64"),
        ]:
            with self.subTest(platform=package):
                self.release("v1.1.0", package)
                self.install(TEST_OS=system, TEST_ARCH=arch)
                self.install("--uninstall")


if __name__ == "__main__":
    unittest.main()
