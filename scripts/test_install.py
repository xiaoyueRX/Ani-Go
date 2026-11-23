"""Exercise the release installer with local archives and an offline curl stand-in."""

import hashlib
import io
import os
import platform
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
printf '%s\\n' "$url" >> "$FETCH_LOG"
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
        (path / "sysctl").write_text(
            '#!/bin/sh\n[ "$1" = -n ] && [ "$2" = sysctl.proc_translated ] || exit 1\n'
            '[ -n "${TEST_TRANSLATED:-}" ] || exit 1\nprintf "%s\\n" "$TEST_TRANSLATED"\n'
        )
        self.fetch_log = self.root / "fetch.log"
        for command in path.iterdir():
            command.chmod(0o755)
        self.env = {
            "HOME": str(self.root), "PATH": f"{path}:{os.environ['PATH']}",
            "FIXTURES": str(self.root / "releases"), "FETCH_LOG": str(self.fetch_log), "TEST_OS": "Linux", "TEST_ARCH": "x86_64",
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
        return result

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
            ("Linux", "x86_64", "linux-amd64"), ("Linux", "aarch64", "linux-arm64"),
            ("Darwin", "arm64", "darwin-arm64"),
        ]:
            with self.subTest(platform=package):
                self.release("v1.1.0", package)
                self.install(TEST_OS=system, TEST_ARCH=arch, ANIGO_VERSION="latest")
                self.install("--uninstall")

    def test_retired_architectures_keep_pinned_releases(self):
        for system, arch, package, label, translated in [
            ("Darwin", "x86_64", "darwin-amd64", "Intel Mac", ""),
            ("Darwin", "amd64", "darwin-amd64", "Intel Mac", "0"),
            ("Linux", "armv7l", "linux-armv7", "ARMv7", ""),
            ("Linux", "armhf", "linux-armv7", "ARMv7", ""),
        ]:
            with self.subTest(arch=arch):
                env = dict(TEST_OS=system, TEST_ARCH=arch, TEST_TRANSLATED=translated)
                self.release("v0.7.0", package)
                self.install(ANIGO_VERSION="0.7.0", **env)
                program = self.program.resolve()
                self.assertEqual(program.parent.name, "v0.7.0")
                (self.state / ".env").write_text("user config")
                (self.state / "ani-go.db").write_text("user data")
                self.fetch_log.unlink()
                for version in ("", "latest", "v1.0.0-rc.1", "1.0.0", "v2.1.0"):
                    result = self.install(ANIGO_VERSION=version, success=False, **env)
                    self.assertIn(label, result.stderr)
                    self.assertIn("ANIGO_VERSION=v0.7.0", result.stderr)
                    self.assertFalse(self.fetch_log.exists())
                    self.assertEqual(self.program.resolve(), program)
                    self.assertEqual(subprocess.check_output([self.program], text=True).strip(), "v0.7.0")
                    self.assertEqual((self.state / ".env").read_text(), "user config")
                    self.assertEqual((self.state / "ani-go.db").read_text(), "user data")
                self.release("v0.8.0", "linux-amd64")
                result = self.install(ANIGO_VERSION="v0.8.0", success=False, **env)
                self.assertIn(f"ani-go-v0.8.0-{package}.tar.gz", result.stderr)
                self.assertEqual(self.fetch_log.read_text().splitlines(), [
                    "https://github.com/xiaoyueRX/Ani-Go/releases/download/v0.8.0/checksums.txt",
                ])
                self.assertEqual(self.program.resolve(), program)
                self.install("--uninstall", **env)
                self.assertFalse(self.program.is_symlink())

    def test_rosetta_selects_apple_silicon(self):
        env = dict(TEST_OS="Darwin", TEST_ARCH="x86_64", TEST_TRANSLATED="1")
        for version, tag in [("", "v1.1.0"), ("latest", "v1.1.0"), ("0.7.0", "v0.7.0")]:
            with self.subTest(version=version):
                self.release(tag, "darwin-arm64")
                self.install(ANIGO_VERSION=version, **env)
                self.assertIn(f"ani-go-{tag}-darwin-arm64.tar.gz", self.fetch_log.read_text())
                self.install("--uninstall", **env)
                self.fetch_log.unlink()

    @unittest.skipUnless(platform.system() == "Darwin" and platform.machine() == "arm64", "requires Apple Silicon")
    def test_native_rosetta_shell_selects_arm64(self):
        rosetta = ["/usr/bin/arch", "-x86_64"]
        if subprocess.run([*rosetta, "/usr/bin/true"], capture_output=True).returncode:
            self.skipTest("Rosetta is unavailable")
        for name in ("uname", "sysctl"):
            (self.root / "commands" / name).unlink()
        self.release("v1.1.0", "darwin-arm64")
        result = subprocess.run([*rosetta, "/bin/sh", str(INSTALLER)], cwd=self.root, env=self.env,
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("ani-go-v1.1.0-darwin-arm64.tar.gz", self.fetch_log.read_text())

    def test_linux_32_bit_x86_is_unsupported(self):
        for arch in ("i386", "i686"):
            with self.subTest(arch=arch):
                result = self.install(TEST_OS="Linux", TEST_ARCH=arch, success=False)
                self.assertIn("32 位 x86", result.stderr)
                self.assertFalse(self.state.exists())
                self.assertFalse(self.fetch_log.exists())


if __name__ == "__main__":
    unittest.main()
