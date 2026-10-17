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
	accounts := map[int]string{}
	for i := range out {
		pid := out[i].PID
		out[i].ProcessName = names[uint32(pid)]
		account, seen := accounts[pid]
		if !seen {
			account = processAccount(pid)
			accounts[pid] = account
		}
		out[i].User = account
	}
	return out, nil
}

// processAccount returns the account a process runs as ("DOMAIN\user",
// or a bare name for built-in accounts such as SYSTEM), or "" when the
// process cannot be opened (elevated or protected, without elevation).
func processAccount(pid int) string {
	if pid <= 0 {
		return ""
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var token windows.Token
	if err := windows.OpenProcessToken(h, windows.TOKEN_QUERY, &token); err != nil {
		return ""
	}
	defer token.Close()
	tu, err := token.GetTokenUser()
	if err != nil {
		return ""
	}
	account, domain, _, err := tu.User.Sid.LookupAccount("")
	switch {
	case err != nil:
		return tu.User.Sid.String()
	case domain == "" || domain == "NT AUTHORITY":
		return account
	default:
		return domain + `\` + account
	}
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
// from a single Toolhelp snapshot. PID 4 is the kernel "System" process,
// which owns sockets of kernel-mode drivers (http.sys, SMB) and is named
// here so those rows are not left blank. PID 0 is not an owner: the table
// reports it for TIME_WAIT connections no process holds any more, so it
// stays unnamed rather than showing up as "System Idle Process".
func processNames() map[uint32]string {
	names := map[uint32]string{4: "System"}
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return names
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID != 0 {
			names[entry.ProcessID] = displayName(windows.UTF16ToString(entry.ExeFile[:]))
		}
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
