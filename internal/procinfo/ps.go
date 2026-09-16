package procinfo

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func parsePSInfo(pid int, output string) (Info, error) {
	fields := strings.Fields(output)
	if len(fields) != 7 {
		return Info{}, fmt.Errorf("procinfo: malformed ps output for pid %d", pid)
	}
	rss, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return Info{}, err
	}
	start, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", strings.Join(fields[2:], " "), time.Local)
	if err != nil {
		return Info{}, err
	}
	return Info{PID: pid, User: fields[0], RSSBytes: rss * 1024, StartTime: start, OpenFiles: -1}, nil
}
