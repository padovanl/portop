package cli

import (
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/padovanl/portop/internal/app"
)

// writeMetrics renders rows in the Prometheus text exposition format
// (version 0.0.4). The per-port info series only covers listening
// sockets, whose number is bounded by the services on the host;
// connections are only counted.
func writeMetrics(w io.Writer, rows []app.Row, took time.Duration) {
	type stateKey struct{ proto, state string }
	type scopeKey struct{ proto, scope string }
	sockets := map[stateKey]int{}
	listening := map[scopeKey]int{}
	var listeners []app.Row
	for _, r := range rows {
		proto := strings.ToLower(string(r.Protocol))
		sockets[stateKey{proto, string(r.State)}]++
		if r.Listening() {
			listening[scopeKey{proto, r.Scope()}]++
			listeners = append(listeners, r)
		}
	}

	fmt.Fprintln(w, "# HELP portop_sockets Sockets on the host by protocol and state.")
	fmt.Fprintln(w, "# TYPE portop_sockets gauge")
	stateKeys := make([]stateKey, 0, len(sockets))
	for k := range sockets {
		stateKeys = append(stateKeys, k)
	}
	sort.Slice(stateKeys, func(i, j int) bool {
		if stateKeys[i].proto != stateKeys[j].proto {
			return stateKeys[i].proto < stateKeys[j].proto
		}
		return stateKeys[i].state < stateKeys[j].state
	})
	for _, k := range stateKeys {
		fmt.Fprintf(w, "portop_sockets{protocol=%s,state=%s} %d\n", label(k.proto), label(k.state), sockets[k])
	}

	fmt.Fprintln(w, "# HELP portop_listening_ports Listening sockets by protocol and exposure (loopback, all interfaces or one address).")
	fmt.Fprintln(w, "# TYPE portop_listening_ports gauge")
	for _, proto := range []string{"tcp", "udp"} {
		for _, scope := range []string{app.ScopeAll, app.ScopeAddress, app.ScopeLoopback} {
			fmt.Fprintf(w, "portop_listening_ports{protocol=%s,scope=%s} %d\n", label(proto), label(scope), listening[scopeKey{proto, scope}])
		}
	}

	fmt.Fprintln(w, "# HELP portop_listening_port_info One series per listening socket, always 1, labelled with what portop knows about it.")
	fmt.Fprintln(w, "# TYPE portop_listening_port_info gauge")
	sort.Slice(listeners, func(i, j int) bool {
		if listeners[i].LocalPort != listeners[j].LocalPort {
			return listeners[i].LocalPort < listeners[j].LocalPort
		}
		if listeners[i].Protocol != listeners[j].Protocol {
			return listeners[i].Protocol < listeners[j].Protocol
		}
		return listeners[i].LocalAddr.String() < listeners[j].LocalAddr.String()
	})
	for _, r := range listeners {
		fmt.Fprintf(w, "portop_listening_port_info{protocol=%s,port=%s,address=%s,scope=%s,process=%s,pid=%s,user=%s,container=%s} 1\n",
			label(strings.ToLower(string(r.Protocol))), label(strconv.Itoa(int(r.LocalPort))), label(r.LocalAddr.String()),
			label(r.Scope()), label(r.ProcessName), label(pidLabel(r.PID)), label(r.Username), label(r.ContainerName))
	}

	fmt.Fprintln(w, "# HELP portop_scrape_duration_seconds Time taken to scan the host for this response.")
	fmt.Fprintln(w, "# TYPE portop_scrape_duration_seconds gauge")
	fmt.Fprintf(w, "portop_scrape_duration_seconds %s\n", strconv.FormatFloat(took.Seconds(), 'f', 6, 64))
}

func pidLabel(pid int) string {
	if pid == 0 {
		return ""
	}
	return strconv.Itoa(pid)
}

// label quotes a label value, escaping what the exposition format
// requires: backslash, double quote and newline.
func label(v string) string {
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `"`, `\"`)
	v = strings.ReplaceAll(v, "\n", `\n`)
	return `"` + v + `"`
}
