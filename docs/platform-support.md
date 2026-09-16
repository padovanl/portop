# Platform support

Linux builds are produced for amd64, arm64, ARMv6 and ARMv7. macOS builds
are produced for amd64 and arm64. Linux retains its
`/proc` backend; macOS uses the system `lsof` and `ps` tools without adding Go
dependencies or requiring CGO. Linux releases include both DEB and RPM packages.

## Complete release matrix

| OS | Target suffix | Formats | DEB architecture | RPM architecture |
|----|---------------|---------|------------------|------------------|
| Linux | `linux_amd64` | tar.gz, deb, rpm | amd64 | x86_64 |
| Linux | `linux_arm64` | tar.gz, deb, rpm | arm64 | aarch64 |
| Linux | `linux_armv6` | tar.gz, deb, rpm | armhf | armv6hl |
| Linux | `linux_armv7` | tar.gz, deb, rpm | armhf | armv7hl |
| macOS | `darwin_amd64` | tar.gz | — | — |
| macOS | `darwin_arm64` | tar.gz | — | — |

ARM32 builds use `GOARM=6` / `GOARM=7` with hardware floating point and
`CGO_ENABLED=0`. ARMv5, soft-float ARM, big-endian ARM, 32-bit x86 and unlisted
platforms are not release targets. ARM64 uses the ARMv8.0 baseline; amd64 uses v1.
The two ARM32 DEBs both advertise `armhf`; their filenames retain `armv6` / `armv7`
to prevent collisions. Install only the CPU-compatible variant. Package metadata
has been inspected, but installing on each target distribution has not been tested.
Use tarballs where the package manager does not accept the architecture.

The installer checks userland bitness on an ARM64 Linux kernel using
`getconf LONG_BIT`; if that command is unavailable, it falls back to `uname -m`.
`PORTOP_ARCH=armv6|armv7|arm64|amd64` allows an explicit choice. Select for the
installed OS, not just the board's CPU. Hardware/OS compatibility layers are not
assumed. See the README's architecture table for the user-facing limitations.

## ARM execution checks

The actual release archives were extracted and run under QEMU 8.2.2 with
ARM1176 (ARMv6), Cortex-A7 (ARMv7) and Cortex-A53 (ARM64) CPU models.
For all three targets, `scripts/test-arm.py` verified `--version`, JSON output,
TCP/UDP socket discovery, owning PID and `--listen` filtering using live host
sockets. CI repeats these checks after every snapshot release build.
This is user-mode emulation on a Linux amd64 host, not a physical-board or
full guest-kernel test; no claim is made that older board images have been tested.
Native macOS runtime checks remain pending, as before.

## macOS behavior

- TCP/UDP, IPv4/IPv6, process ownership, CPU sampling and process details are supported.
- CPU usage is derived from successive cumulative `ps` CPU-time samples. Each PID
  is sampled once per collection, even when it owns multiple sockets.
- System commands have a five-second timeout and use the C locale for parsing.
- Run with `sudo` to include other users' processes. Non-root scans can omit sockets
  entirely; they are not necessarily returned as rows with an unknown PID.
- Systemd units and Docker Desktop VM process metadata are unavailable.
- Thread count is unavailable and displayed as `-`. Other process details are
  best-effort when a process exits or access is restricted.
- Scans and CPU sampling can be slower than Linux because they invoke system tools.

## Validation performed locally

- Linux unit tests and end-to-end tests passed.
- `go vet` and `gofmt` passed in a temporary checkout with LF line endings.
- macOS binaries and all unit-test binaries compile for Intel and Apple Silicon.
  Cross-compilation does **not** run the macOS tests.
- `goreleaser check` and the snapshot release passed, producing six binary
  archives, four DEB packages, four RPM packages and checksums.
- Installer simulations verified archive selection, checksum validation and
  extraction for all six targets, including ARM64 kernels with 32-bit userlands,
  explicit architecture overrides, unsupported targets and checksum mismatches.
- Native macOS socket/process tests and end-to-end tests are configured on
  `macos-15` and `macos-15-intel` in GitHub Actions. They have not been run here.
- Docker Compose port audit tests cover short and long `ports:` syntax, simple
  ranges, dynamic host ports, expected Compose service matches via Docker labels,
  listener conflicts, unused ports and JSON CLI output.
- `go test -race -p 1 -count=1 ./...` passed after installing GCC 13.3 and
  development headers under `~/.local/share/portop-toolchain`. Packages were
  run sequentially because the baseline test observes all system listeners;
  concurrent socket tests can change its snapshot. No Go data race was reported.

Go, Go module and shell files previously contained committed CRLF line endings.
They have been normalized to LF so the real checkout passes gofmt and shell
scripts execute on Linux/macOS. `.gitattributes` preserves LF for Go, Go module,
shell and Python files.
No release was published.

## Implementation references

- [lsof machine-readable output](https://github.com/lsof-org/lsof/blob/master/Lsof.8)
- [Apple ps manual](https://github.com/apple-oss-distributions/adv_cmds/blob/main/ps/ps.1)
- [GoReleaser nFPM packaging](https://www.goreleaser.com/customization/package/nfpm/)
- [GitHub macOS runner architectures](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)

## Local C compiler

The user-local compiler does not require changes to system packages. To run the
race checks with it:

```sh
CGO_ENABLED=1 CC="$HOME/.local/share/portop-toolchain/bin/gcc" \
  go test -race -p 1 -count=1 ./...
```

Run these checks separately from other suites that open listening sockets.
