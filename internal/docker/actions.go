package docker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"regexp"
)

// Container actions portop desktop offers on a container's ports.
const (
	ActionStop    = "stop"
	ActionStart   = "start"
	ActionRestart = "restart"
)

// validName matches Docker container names and IDs; anything else is
// refused before it reaches a URL or a command line.
var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,254}$`)

// ErrUnavailable means neither the Engine API nor the docker CLI is there.
var ErrUnavailable = errors.New("docker is not reachable")

func checkAction(name, action string) error {
	if !validName.MatchString(name) {
		return fmt.Errorf("invalid container name %q", name)
	}
	switch action {
	case ActionStop, ActionStart, ActionRestart:
		return nil
	}
	return fmt.Errorf("unknown container action %q", action)
}

// demuxLogs turns the Engine API's multiplexed log stream (8-byte frame
// headers, stdout and stderr interleaved) into plain text. A container
// with a TTY sends plain text already, which is returned as is.
func demuxLogs(b []byte) []byte {
	var out bytes.Buffer
	rest := b
	for len(rest) >= 8 {
		stream := rest[0]
		size := int(binary.BigEndian.Uint32(rest[4:8]))
		if stream > 2 || rest[1] != 0 || rest[2] != 0 || rest[3] != 0 || size > len(rest)-8 {
			return b // not multiplexed
		}
		out.Write(rest[8 : 8+size])
		rest = rest[8+size:]
	}
	if len(rest) != 0 {
		return b
	}
	return out.Bytes()
}
