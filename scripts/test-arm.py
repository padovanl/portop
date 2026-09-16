#!/usr/bin/env python3
"""Run release binaries under QEMU against real host TCP/UDP sockets."""

import argparse
import json
import os
from pathlib import Path
import socket
import subprocess
import tarfile
import tempfile


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("dist", type=Path)
    parser.add_argument("--qemu-bin-dir", type=Path, default=Path("/usr/bin"))
    parser.add_argument("--static", action="store_true")
    args = parser.parse_args()
    targets = [("armv6", "arm", "arm1176"),
               ("armv7", "arm", "cortex-a7"),
               ("arm64", "aarch64", "cortex-a53")]
    with tempfile.TemporaryDirectory(prefix="portop-arm-smoke-") as temp:
        for arch, emulator, cpu in targets:
            matches = list(args.dist.glob(f"portop_*_linux_{arch}.tar.gz"))
            if len(matches) != 1:
                raise RuntimeError(f"expected one {arch} archive, found {matches}")
            binary = Path(temp) / f"portop-{arch}"
            with tarfile.open(matches[0]) as archive:
                binary.write_bytes(archive.extractfile("portop").read())
            binary.chmod(0o755)
            qemu = args.qemu_bin_dir / (f"qemu-{emulator}" + ("-static" if args.static else ""))
            command = [str(qemu), "-cpu", cpu, str(binary)]

            def run(*flags):
                return subprocess.check_output(command + list(flags), text=True, timeout=30)

            if "portop" not in run("--version"):
                raise AssertionError(f"{arch}: missing version")
            with socket.socket() as tcp, socket.socket(type=socket.SOCK_DGRAM) as udp:
                tcp.bind(("127.0.0.1", 0))
                tcp.listen(1)
                udp.bind(("127.0.0.1", 0))
                flags = ["--json", "--no-dns", "--no-systemd", "--no-docker"]
                rows = json.loads(run(*flags))
                for protocol, port, state in [("TCP", tcp.getsockname()[1], "LISTEN"),
                                               ("UDP", udp.getsockname()[1], "UNCONN")]:
                    if not any(row["protocol"] == protocol and row["local_port"] == port
                               and row["state"] == state and row["pid"] == os.getpid()
                               for row in rows):
                        raise AssertionError(f"{arch}: missing {protocol} socket {port} owned by PID {os.getpid()}")
                listeners = json.loads(run(*flags, "--listen"))
                if not listeners or any(row["state"] != "LISTEN" for row in listeners):
                    raise AssertionError(f"{arch}: --listen returned invalid rows")
            print(f"{arch} ({cpu}): version, TCP/UDP ownership and --listen passed")


if __name__ == "__main__":
    main()
