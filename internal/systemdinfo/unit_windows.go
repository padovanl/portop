package systemdinfo

import (
	"sort"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// On Windows there is no systemd; the closest equivalent is the Windows
// service hosted by a process. UnitForPID returns its service name (e.g.
// "Spooler"), or several comma-separated names for shared svchost
// processes. The table is rebuilt at most every serviceCacheTTL because
// the TUI asks once per row on every refresh.

const serviceCacheTTL = 5 * time.Second

var (
	svcMu    sync.Mutex
	svcAt    time.Time
	svcByPID map[uint32]string
)

// UnitForPID returns the Windows service(s) running in pid, or "".
func UnitForPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	svcMu.Lock()
	defer svcMu.Unlock()
	if svcByPID == nil || time.Since(svcAt) > serviceCacheTTL {
		svcByPID = runningServices()
		svcAt = time.Now()
	}
	return svcByPID[uint32(pid)]
}

func runningServices() map[uint32]string {
	out := map[uint32]string{}
	scm, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_ENUMERATE_SERVICE)
	if err != nil {
		return out
	}
	defer windows.CloseServiceHandle(scm)

	var needed, count, resume uint32
	buf := make([]byte, 64*1024)
	byPID := map[uint32][]string{}
	for attempt := 0; attempt < 16; attempt++ {
		err := windows.EnumServicesStatusEx(scm, windows.SC_ENUM_PROCESS_INFO,
			windows.SERVICE_WIN32, windows.SERVICE_ACTIVE,
			&buf[0], uint32(len(buf)), &needed, &count, &resume, nil)
		if err != nil && err != windows.ERROR_MORE_DATA {
			return out
		}
		if count > 0 {
			entries := unsafe.Slice((*windows.ENUM_SERVICE_STATUS_PROCESS)(unsafe.Pointer(&buf[0])), count)
			for _, e := range entries {
				if pid := e.ServiceStatusProcess.ProcessId; pid != 0 {
					byPID[pid] = append(byPID[pid], windows.UTF16PtrToString(e.ServiceName))
				}
			}
		}
		if err == nil {
			break
		}
		if needed > uint32(len(buf)) {
			buf = make([]byte, needed)
		}
	}
	for pid, names := range byPID {
		sort.Strings(names)
		out[pid] = strings.Join(names, ",")
	}
	return out
}
