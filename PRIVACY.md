# Privacy policy

*Last updated: 15 October 2026*

portop is a terminal application that shows which processes are using the
network ports of the computer it runs on. This policy applies to every
distribution of portop, including the Microsoft Store, winget, Homebrew, the
Snap Store and the release archives on GitHub.

## Data collection

portop does not collect, store or send personal data, and it contains no
telemetry, analytics, advertising or crash reporting. The author receives no
information about you or your use of the app.

## What portop reads on your computer

To do its job, portop reads information that stays on your computer:

- the system's TCP/UDP socket tables;
- details of the processes that own those sockets (name, command line,
  executable path, owner, CPU usage);
- the Windows services or systemd units hosted by those processes;
- the containers known to a local Docker installation, when one is present.

This information is shown on screen, or printed when you use `--json`, and is
not transmitted anywhere.

## Network activity

- **Reverse DNS**: to show host names for remote addresses, portop asks your
  operating system's configured DNS resolver, as any network tool does. Run
  `portop --no-dns` to turn this off.
- **Web dashboard**: only when you start it with `--web`, portop serves a
  dashboard that listens on `127.0.0.1` by default. It is reachable from other
  machines only if you choose a non-loopback address, which requires sign-in.

portop makes no other network connections and does not check for updates.

## Files stored locally

portop may write its settings and an optional port baseline (`baseline.json`)
to your user configuration folder. These files stay on your computer and you
can delete them at any time.

## Contact

Questions about this policy: open an issue at
<https://github.com/padovanl/portop/issues>.
