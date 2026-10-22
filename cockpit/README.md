# Cockpit add-on

The `.deb` and `.rpm` packages install this page under
`/usr/share/cockpit/portop`, so nothing else is needed on a host that
installed portop from them or from the APT repository. The documentation
covers the other install methods:
<https://padovanl.github.io/portop/docs/cockpit.html>.

To try the page from this checkout, build the binary and install both on the
Linux host:

```bash
go build -o ./bin/portop ./cmd/portop
sudo install -m 755 ./bin/portop /usr/local/bin/portop
install -d "$HOME/.local/share/cockpit/portop"
cp cockpit/portop/{manifest.json,index.html,portop.css,portop.js,logo.png} "$HOME/.local/share/cockpit/portop/"
```

Run `cockpit-bridge --packages` and look for `portop`. Open
<https://localhost:9090/> on the host, or `https://<host>:9090/` from another
computer. Sign in as the same user, then select **Tools → Ports (portop)**.
Hard-refresh the browser if the page was already open.

The page calls `portop --json --no-dns` through Cockpit's bridge every two
seconds, and works out traffic rates from the byte counts of consecutive
calls. **Details** calls `portop --inspect-pid` for command line, user,
executable, memory and other process information. Terminate and Force kill
send SIGTERM and SIGKILL through `portop --signal-pid`, after confirmation and
a check that the process start time has not changed. A row with no PID cannot
be signaled. The Light/Dark button remembers your choice in this browser.

Every call asks for `superuser: "try"`: with Cockpit's **Administrative
access** on, portop runs as root and sees, and can stop, every process; with
it off, portop runs as your user and other users' processes show as Unknown.

If the bridge cannot find the `portop` binary, install it in a directory on
its PATH, such as `/usr/local/bin`.
