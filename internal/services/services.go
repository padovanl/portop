// Package services resolves well-known port numbers to service names
// (22 -> ssh, 443 -> https) by parsing the system's /etc/services file,
// so portop can hint at what a port is for even when the owning process
// can't be resolved (e.g. insufficient permissions).
package services

import (
	"bufio"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
)

// Path is the services database to read, overridable in tests.
var Path = "/etc/services"

type key struct {
	port  uint16
	proto string
}

var (
	once  sync.Once
	names map[key]string
)

// Lookup returns the well-known service name for a port/protocol (e.g.
// Lookup(22, "TCP") -> "ssh", true), or ("", false) if unknown.
func Lookup(port uint16, proto string) (string, bool) {
	once.Do(load)
	name, ok := names[key{port: port, proto: strings.ToLower(proto)}]
	return name, ok
}

func load() {
	names = make(map[key]string)
	f, err := os.Open(Path)
	if err != nil {
		return
	}
	defer f.Close()
	parse(f, names)
}

func parse(f io.Reader, out map[key]string) {
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		portProto := strings.SplitN(fields[1], "/", 2)
		if len(portProto) != 2 {
			continue
		}
		portNum, err := strconv.ParseUint(portProto[0], 10, 16)
		if err != nil {
			continue
		}
		k := key{port: uint16(portNum), proto: strings.ToLower(portProto[1])}
		if _, exists := out[k]; !exists {
			out[k] = name
		}
	}
}
