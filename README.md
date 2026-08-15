# portop

[![CI](https://github.com/padovan93/portop/actions/workflows/ci.yml/badge.svg)](https://github.com/padovan93/portop/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/padovan93/portop)](https://github.com/padovan93/portop/releases/latest)
[![Downloads](https://img.shields.io/github/downloads/padovan93/portop/total)](https://github.com/padovan93/portop/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/padovan93/portop)](https://goreportcard.com/report/github.com/padovan93/portop)
[![Go version](https://img.shields.io/github/go-mod/go-version/padovan93/portop)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**What's really using your ports?** A terminal UI for TCP/UDP ports — like `htop`, but for ports.

**[padovan93.github.io/portop →](https://padovan93.github.io/portop/)**

![portop demo: filtering to a port, viewing process details, the kill confirmation, and the help overlay](docs/assets/demo.gif)

`ss`, `netstat` and `lsof` tell you *what* is bound to a port. portop adds the
part they leave out: which process, which systemd unit, which Docker
container, how much CPU it's burning right now, and a one-key way to open,
inspect or kill it — refreshed live, in a keyboard-driven UI.

## Features

- Live, color-coded table of TCP/UDP sockets, refreshed on an interval
- Per-connection CPU% (sampled like `top`, no extra permissions needed)
- Process detail view: cmdline, executable, cwd, user, RSS, thread count, start time
- One-key **kill** (graceful SIGTERM, or forced SIGKILL) with a confirmation prompt
- One-key **open** — launches `http(s)://localhost:<port>` in your browser
- Instant filter/search by port, process name or PID
- IPv4 / IPv6 / both toggle
- Show/hide `ESTABLISHED` connections (with reverse DNS on remote hosts)
- **systemd** unit association (which `.service` owns this port)
- **Docker** container association (which container owns this port)
- Well-known port names (`:22` shows as `ssh`, `:443` as `https`, ...)
- **New-port detection**: newly opened listening ports are highlighted as
  soon as they appear, with an optional desktop notification
  (`--watch-new`) — handy for noticing something started listening that you
  didn't expect
- **Baseline drift detection**: `--save-baseline` now, `--diff` later (e.g.
  from a cron job or systemd timer) — exits non-zero the moment a port
  that wasn't in your baseline starts listening
- Copy a row to the clipboard
- Non-interactive `--json` snapshot mode for scripting and monitoring
- Adapts its columns to your terminal width — useful even at 80 columns

## Why not just `ss -tlnp`?

You can. But you'll be doing all of this by hand, every time:

|                              | ss / netstat | lsof | bandwhich | portop |
|------------------------------|:---:|:---:|:---:|:---:|
| Live, refreshing view        | – | – | ✓ | ✓ |
| Kill from the UI              | – | – | – | ✓ |
| Open port in browser          | – | – | – | ✓ |
| systemd unit shown            | – | – | – | ✓ |
| Docker container shown        | – | – | – | ✓ |
| New-port alerts               | – | – | – | ✓ |
| Baseline drift / audit mode   | – | – | – | ✓ |
| Fuzzy filter/search           | – | – | – | ✓ |

## Install

### One-line installer (Linux/macOS)

```sh
curl -fsSL https://raw.githubusercontent.com/padovan93/portop/main/install.sh | sh
```

Detects your OS/arch, verifies the release checksum, and installs to
`/usr/local/bin` (or `~/.local/bin` if that's not writable).

### Debian/Ubuntu (.deb)

Download the `.deb` for your architecture from the
[latest release](https://github.com/padovan93/portop/releases/latest) and:

```sh
sudo dpkg -i portop_*_amd64.deb
```

### Any Linux/macOS (tarball)

Download the `tar.gz` for your OS/architecture from the
[latest release](https://github.com/padovan93/portop/releases/latest):

```sh
tar -xzf portop_*_linux_amd64.tar.gz
sudo install -m 755 portop /usr/local/bin/portop
```

### From source

```sh
go install github.com/padovan93/portop/cmd/portop@latest
```

## Usage

```sh
portop                 # launch the TUI (LISTEN + ESTABLISHED)
portop 8080             # launch the TUI pre-filtered on port 8080
portop redis            # launch the TUI pre-filtered on a process name
portop --listen          # show only listening (LISTEN) sockets
portop --json            # print one JSON snapshot and exit
portop --watch-new       # desktop-notify when a new port starts listening

portop --save-baseline   # remember which ports are currently listening
portop --diff            # compare live ports against the saved baseline
                          # (exit code 3 if something changed — great for cron)
```

Run `portop --help` for the full flag list.

> Rows with no process/PID are owned by another user (root's `docker-proxy`,
> `systemd-resolved`, ...) — the same limitation `lsof`/`ss -p` have without
> `sudo`. portop tells you when that's happening instead of leaving you to
> guess.

### Keybindings

| Key       | Action                                          |
|-----------|--------------------------------------------------|
| `↑` `↓`   | move the cursor                                  |
| `g` `G`   | jump to top / bottom                             |
| `enter`   | process details                                  |
| `k`       | kill process (then `y`=SIGTERM, `f`=SIGKILL)     |
| `o`       | open `http(s)://localhost:PORT` in the browser   |
| `f` `/`   | filter/search by port, process or PID            |
| `v`       | toggle IPv4 / IPv6 / both                        |
| `e`       | show/hide `ESTABLISHED` connections              |
| `s`       | cycle sort column                                |
| `c`       | copy selected row to clipboard                   |
| `n`       | clear the new-port highlight                     |
| `r`       | refresh now                                      |
| `?`       | help                                             |
| `q`       | quit                                             |

## How it works

portop reads `/proc/net/{tcp,tcp6,udp,udp6}` for the socket table and walks
`/proc/<pid>/fd` to match socket inodes to owning processes — the same
technique `lsof`/`ss` use, no root required beyond what's needed to see other
users' processes. systemd unit and Docker container association are derived
from each process's cgroup path, so no D-Bus or Docker SDK dependency is
needed; Docker container names are resolved via a couple of read-only calls
to the Docker Engine API over its unix socket when available. Well-known port
names come from parsing `/etc/services`, and baseline diffing just snapshots
the LISTEN set to a small JSON file under your OS's config directory.

## Development

Requires Go 1.24+.

```sh
make build     # go build ./cmd/portop
make test      # unit tests
make e2e       # black-box tests against the compiled binary
make lint      # go vet + gofmt check
make release   # local snapshot build via goreleaser (deb + tar.gz, unpublished)
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for the branching model and PR
requirements.

## License

[MIT](LICENSE)
