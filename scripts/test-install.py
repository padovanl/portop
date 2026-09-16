#!/usr/bin/env python3
"""Exercise the actual installer offline against fake release archives."""

import hashlib
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest


INSTALLER = Path(__file__).resolve().parents[1] / "install.sh"


class InstallerTests(unittest.TestCase):
    def run_installer(self, system, machine, bits="64", override=None, corrupt=False):
        with tempfile.TemporaryDirectory(prefix="portop-installer-") as temp:
            root = Path(temp)
            tools = root / "tools"
            tools.mkdir()
            assets = root / "assets"
            assets.mkdir()
            binary = root / "portop"
            binary.write_text("#!/bin/sh\necho portop-test\n")
            binary.chmod(0o755)
            checksums = []
            for target in ("linux_amd64", "linux_arm64", "linux_armv6", "linux_armv7",
                           "darwin_amd64", "darwin_arm64"):
                archive = assets / f"portop_1.2.3_{target}.tar.gz"
                with tarfile.open(archive, "w:gz") as tar:
                    tar.add(binary, arcname="portop")
                digest = "0" * 64 if corrupt else hashlib.sha256(archive.read_bytes()).hexdigest()
                checksums.append(f"{digest}  {archive.name}\n")
            (assets / "checksums.txt").write_text("".join(checksums))

            def executable(name, source):
                path = tools / name
                path.write_text(source)
                path.chmod(0o755)

            executable("uname", '#!/bin/sh\ncase "$1" in -s) echo "$TEST_OS";; -m) echo "$TEST_MACHINE";; esac\n')
            executable("getconf", '#!/bin/sh\n[ "$TEST_BITS" != unavailable ] || exit 1\necho "$TEST_BITS"\n')
            executable("curl", '''#!/bin/sh
printf '%s\n' "$2" >> "$TEST_REQUESTS"
name="${2##*/}"
[ -f "$TEST_ASSETS/$name" ] || exit 22
cp "$TEST_ASSETS/$name" "$4"
''')
            installer = root / "install.sh"
            installer.write_bytes(INSTALLER.read_bytes())
            env = {k: v for k, v in os.environ.items() if not k.startswith("PORTOP_")}
            env.update(PATH=f"{tools}:/usr/bin:/bin", PORTOP_VERSION="v1.2.3",
                       PORTOP_INSTALL_DIR=str(root / "installed"), TEST_OS=system,
                       TEST_MACHINE=machine, TEST_BITS=bits, TEST_ASSETS=str(assets),
                       TEST_REQUESTS=str(root / "requests"))
            if override is not None:
                env["PORTOP_ARCH"] = override
            result = subprocess.run(["/bin/sh", str(installer)], env=env,
                                    capture_output=True, text=True, timeout=15)
            requests = (root / "requests").read_text() if (root / "requests").exists() else ""
            installed = root / "installed" / "portop"
            installed_ok = installed.is_file() and installed.read_bytes() == binary.read_bytes()
            return result, requests, installed_ok

    def test_architecture_selection(self):
        cases = [
            ("Linux", "x86_64", "64", "amd64"),
            ("Linux", "aarch64", "64", "arm64"),
            ("Linux", "aarch64", "32", "armv7"),
            ("Linux", "arm64", "32", "armv7"),
            ("Linux", "armv6l", "32", "armv6"),
            ("Linux", "armv7l", "32", "armv7"),
            ("Linux", "armv8l", "32", "armv7"),
            ("Linux", "aarch64", "unavailable", "arm64"),
            ("Darwin", "x86_64", "64", "amd64"),
            ("Darwin", "arm64", "64", "arm64"),
        ]
        for system, machine, bits, arch in cases:
            with self.subTest(system=system, machine=machine, bits=bits):
                result, requests, installed = self.run_installer(system, machine, bits)
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                target = "darwin" if system == "Darwin" else "linux"
                self.assertIn(f"portop_1.2.3_{target}_{arch}.tar.gz", requests)
                self.assertTrue(installed)

    def test_explicit_override(self):
        result, requests, installed = self.run_installer("Linux", "aarch64", override="armv6")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("linux_armv6.tar.gz", requests)
        self.assertTrue(installed)

    def test_reject_unsupported_targets_before_download(self):
        for system, machine, override in [("Linux", "armv5tel", None),
                                         ("Linux", "i686", None),
                                         ("Darwin", "armv7l", None),
                                         ("Linux", "x86_64", "armel"),
                                         ("FreeBSD", "amd64", None)]:
            with self.subTest(system=system, machine=machine, override=override):
                result, requests, installed = self.run_installer(system, machine, override=override)
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(requests, "")
                self.assertFalse(installed)

    def test_bad_checksum_does_not_install(self):
        result, _, installed = self.run_installer("Linux", "armv6l", "32", corrupt=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("checksum mismatch", result.stderr)
        self.assertFalse(installed)


if __name__ == "__main__":
    unittest.main()
