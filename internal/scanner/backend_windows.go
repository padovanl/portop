package scanner

import (
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The IP Helper API reports the owning PID with every socket, so on
// Windows ResolveProcesses has nothing left to do (these connections carry
// no inode). Unlike Linux, PIDs are visible for every user's sockets even
// without elevation; only process details may need an elevated terminal.

var (
	modiphlpapi             = windows.NewLazySystemDLL("iphlpapi.dll")
	procGetExtendedTcpTable = modiphlpapi.NewProc("GetExtendedTcpTable")
	procGetExtendedUdpTable = modiphlpapi.NewProc("GetExtendedUdpTable")
)

const (
	afINET              = 2
	afINET6             = 23
	tcpTableOwnerPIDAll = 5 // TCP_TABLE_OWNER_PID_ALL
	udpTableOwnerPID    = 1 // UDP_TABLE_OWNER_PID
)

// Scan returns every TCP and UDP socket with its owning PID and image name.
func Scan() ([]Connection, error) {
	sources := []struct {
		proc  *windows.LazyProc
		af    uint32
		class uint32
		parse func([]byte) []Connection
	}{
		{procGetExtendedTcpTable, afINET, tcpTableOwnerPIDAll, parseTCP4Table},
		{procGetExtendedTcpTable, afINET6, tcpTableOwnerPIDAll, parseTCP6Table},
		{procGetExtendedUdpTable, afINET, udpTableOwnerPID, parseUDP4Table},
		{procGetExtendedUdpTable, afINET6, udpTableOwnerPID, parseUDP6Table},
	}
	var out []Connection
	for _, src := range sources {
		buf, err := extendedTable(src.proc, src.af, src.class)
		if err != nil {
			if src.af == afINET6 {
				continue // IPv6 stack disabled or unavailable
			}
			return nil, err
		}
		out = append(out, src.parse(buf)...)
	}

	names := processNames()
	for i := range out {
		out[i].ProcessName = names[uint32(out[i].PID)]
	}
	return out, nil
}

// extendedTable calls GetExtended{Tcp,Udp}Table, growing the buffer until
// the table fits: sockets can appear between the size query and the call.
func extendedTable(proc *windows.LazyProc, af, class uint32) ([]byte, error) {
	size := uint32(16 * 1024)
	for attempt := 0; attempt < 8; attempt++ {
		buf := make([]byte, size)
		r, _, _ := proc.Call(
			uintptr(unsafe.Pointer(&buf[0])),
			uintptr(unsafe.Pointer(&size)),
			0, // unsorted
			uintptr(af),
			uintptr(class),
			0,
		)
		switch windows.Errno(r) {
		case windows.ERROR_SUCCESS:
			return buf[:size], nil
		case windows.ERROR_INSUFFICIENT_BUFFER:
			size += 4 * 1024
			continue
		default:
			return nil, fmt.Errorf("scanner: %s: %w", proc.Name, windows.Errno(r))
		}
	}
	return nil, fmt.Errorf("scanner: %s: socket table kept growing", proc.Name)
}

// processNames maps every running PID to its image name (e.g. "nginx.exe")
// from a single Toolhelp snapshot. PIDs 0 and 4 are the idle and kernel
// "System" processes, which own sockets of kernel-mode drivers (http.sys,
// SMB) and are named here so those rows are not left blank.
func processNames() map[uint32]string {
	names := map[uint32]string{0: "System Idle Process", 4: "System"}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return names
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		names[entry.ProcessID] = displayName(windows.UTF16ToString(entry.ExeFile[:]))
	}
	return names
}

// displayName drops the ".exe" suffix so names match what users type when
// filtering ("nginx", not "nginx.exe") and what Linux/macOS rows show.
func displayName(image string) string {
	if strings.HasSuffix(strings.ToLower(image), ".exe") {
		return image[:len(image)-4]
	}
	return image
}

// readProcessTicks returns user+kernel CPU time in clockTicksPerSec units.
// FILETIME durations are in 100ns intervals.
func readProcessTicks(pid int) (uint64, error) {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)
	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return 0, err
	}
	total := filetimeDuration(kernel) + filetimeDuration(user)
	return total / (10_000_000 / clockTicksPerSec), nil
}

func filetimeDuration(ft windows.Filetime) uint64 {
	return uint64(ft.HighDateTime)<<32 | uint64(ft.LowDateTime)
}
