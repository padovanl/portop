package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/procctl"
	"github.com/padovanl/portop/internal/scanner"
	"github.com/padovanl/portop/internal/terminaltext"
)

// portSpec is a port given to --wait or --free: "8080", "8080/tcp" or
// "53/udp". TCP is the default.
type portSpec struct {
	port  uint16
	proto scanner.Protocol
}

func (p portSpec) String() string {
	return ":" + strconv.Itoa(int(p.port)) + "/" + strings.ToLower(string(p.proto))
}

func parsePortSpec(s string) (portSpec, error) {
	num, proto, _ := strings.Cut(strings.TrimSpace(s), "/")
	spec := portSpec{proto: scanner.TCP}
	switch strings.ToLower(proto) {
	case "", "tcp":
	case "udp":
		spec.proto = scanner.UDP
	default:
		return portSpec{}, fmt.Errorf("unknown protocol %q (use tcp or udp)", proto)
	}
	n, err := strconv.Atoi(num)
	if err != nil || n < 1 || n > 65535 {
		return portSpec{}, fmt.Errorf("%q is not a port between 1 and 65535", num)
	}
	spec.port = uint16(n)
	return spec, nil
}

func (p portSpec) listeners(rows []app.Row) []app.Row {
	var out []app.Row
	for _, r := range rows {
		if r.Protocol == p.proto && r.LocalPort == p.port && r.Listening() {
			out = append(out, r)
		}
	}
	return out
}

// portEnv is what --wait and --free need from the system, replaceable in
// tests.
type portEnv struct {
	scan      func(context.Context) ([]app.Row, error)
	startTime func(pid int) (time.Time, error)
	signal    func(pid int, start time.Time, force bool) error
	sleep     func(time.Duration)
	now       func() time.Time
	stdin     io.Reader
	tty       bool
}

func systemPortEnv() portEnv {
	collector := app.NewCollector()
	tty := false
	if fi, err := os.Stdin.Stat(); err == nil {
		tty = fi.Mode()&os.ModeCharDevice != 0
	}
	return portEnv{
		scan: func(ctx context.Context) ([]app.Row, error) {
			return collector.Collect(ctx, app.Options{})
		},
		startTime: procctl.StartTime,
		signal:    procctl.SignalIfUnchanged,
		sleep:     time.Sleep,
		now:       time.Now,
		stdin:     os.Stdin,
		tty:       tty,
	}
}

const pollInterval = 250 * time.Millisecond

var isWindows = runtime.GOOS == "windows"

// waitFor polls until done reports true or timeout elapses (0 waits
// forever). It returns whether done became true.
func waitFor(env portEnv, timeout time.Duration, done func([]app.Row) bool) (bool, error) {
	deadline := env.now().Add(timeout)
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		rows, err := env.scan(ctx)
		cancel()
		if err != nil {
			return false, err
		}
		if done(rows) {
			return true, nil
		}
		if timeout > 0 && !env.now().Before(deadline) {
			return false, nil
		}
		env.sleep(pollInterval)
	}
}

// runWait implements --wait: exit 0 once something listens on the port,
// 1 if the timeout passes first.
func runWait(stdout, stderr io.Writer, arg string, timeout time.Duration, env portEnv) int {
	spec, err := parsePortSpec(arg)
	if err != nil {
		fmt.Fprintln(stderr, "portop: --wait: "+err.Error())
		return 2
	}
	ok, err := waitFor(env, timeout, func(rows []app.Row) bool { return len(spec.listeners(rows)) > 0 })
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}
	if !ok {
		fmt.Fprintf(stderr, "portop: nothing listened on %s within %s\n", spec, timeout)
		return 1
	}
	fmt.Fprintf(stdout, "%s is listening\n", spec)
	return 0
}

// runFree implements --free: stop whatever listens on the port and wait
// for it to be released.
func runFree(stdout, stderr io.Writer, arg string, force, yes bool, timeout time.Duration, env portEnv) int {
	spec, err := parsePortSpec(arg)
	if err != nil {
		fmt.Fprintln(stderr, "portop: --free: "+err.Error())
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	rows, err := env.scan(ctx)
	cancel()
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}
	listeners := spec.listeners(rows)
	if len(listeners) == 0 {
		fmt.Fprintf(stdout, "nothing is listening on %s\n", spec)
		return 0
	}

	// One entry per process; its start time identifies it until the
	// signal is sent, however long the confirmation takes.
	type target struct {
		row   app.Row
		start time.Time
	}
	var targets []target
	seen := map[int]bool{}
	for _, r := range listeners {
		if r.PID == 0 {
			fmt.Fprintf(stderr, "portop: %s is held by a process portop cannot see; try again with %s\n", spec, elevateHint())
			return 1
		}
		// SIGTERM to a server's master stops its workers too; SIGKILL
		// does not, so --force signals every process holding the socket.
		pids := []int{r.PID}
		if force {
			pids = append(pids, r.Workers...)
		}
		for _, pid := range pids {
			if seen[pid] {
				continue
			}
			seen[pid] = true
			start, err := env.startTime(pid)
			if err != nil {
				fmt.Fprintf(stderr, "portop: cannot verify PID %d: %v\n", pid, err)
				return 1
			}
			owner := r
			owner.PID = pid
			targets = append(targets, target{owner, start})
		}
	}
	sort.Slice(targets, func(i, j int) bool { return targets[i].row.PID < targets[j].row.PID })

	action := "terminate"
	if force {
		action = "force-kill"
	}
	for _, t := range targets {
		fmt.Fprintf(stdout, "%s %s (PID %d%s) on %s\n", action, terminaltext.Sanitize(orDash(t.row.ProcessName)),
			t.row.PID, userSuffix(t.row.Username), spec)
	}
	if env.tty && !yes {
		fmt.Fprint(stdout, "Proceed? [y/N] ")
		answer, _ := bufio.NewReader(env.stdin).ReadString('\n')
		if a := strings.ToLower(strings.TrimSpace(answer)); a != "y" && a != "yes" {
			fmt.Fprintln(stdout, "aborted")
			return 1
		}
	}
	for _, t := range targets {
		if err := env.signal(t.row.PID, t.start, force); err != nil {
			fmt.Fprintf(stderr, "portop: PID %d: %v\n", t.row.PID, err)
			return 1
		}
	}

	freed, err := waitFor(env, timeout, func(rows []app.Row) bool { return len(spec.listeners(rows)) == 0 })
	if err != nil {
		fmt.Fprintln(stderr, "portop: "+err.Error())
		return 1
	}
	if !freed {
		hint := ""
		if !force {
			hint = "; add --force to kill it"
		}
		fmt.Fprintf(stderr, "portop: %s is still in use after %s%s\n", spec, timeout, hint)
		return 1
	}
	fmt.Fprintf(stdout, "%s is free\n", spec)
	return 0
}

func userSuffix(user string) string {
	if user == "" {
		return ""
	}
	return ", " + terminaltext.Sanitize(user)
}

func elevateHint() string {
	if isWindows {
		return "an Administrator terminal"
	}
	return "sudo"
}
