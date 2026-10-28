package procinfo

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	modkernel32              = windows.NewLazySystemDLL("kernel32.dll")
	procK32GetProcessMemInfo = modkernel32.NewProc("K32GetProcessMemoryInfo")
)

// processMemoryCounters mirrors PROCESS_MEMORY_COUNTERS.
type processMemoryCounters struct {
	cb                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
}

// Load reads process details through the Win32 API. Only
// PROCESS_QUERY_LIMITED_INFORMATION is requested, which an unelevated
// user is granted for most processes; elevated or protected processes may
// still refuse it, in which case Load fails like a vanished Linux PID.
//
// The working directory lives in the target's PEB and needs VM-read
// access, which is only asked for separately: it is filled for the user's
// own processes and left empty for the rest. OpenFiles is -1: Windows
// exposes a handle count, which is not comparable to Linux's open file
// descriptors.
func Load(pid int) (Info, error) {
	if pid <= 0 {
		return Info{}, fmt.Errorf("procinfo: invalid pid %d", pid)
	}
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return Info{}, fmt.Errorf("procinfo: pid %d: %w", pid, err)
	}
	defer windows.CloseHandle(h)

	info := Info{PID: pid, OpenFiles: -1}

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err == nil {
		info.StartTime = time.Unix(0, creation.Nanoseconds())
	}

	info.Exe = imagePath(h)
	if info.Exe != "" {
		info.Name = strings.TrimSuffix(filepath.Base(info.Exe), filepath.Ext(info.Exe))
	}
	info.Cmdline = commandLine(h)
	info.User = owner(h)

	var mem processMemoryCounters
	mem.cb = uint32(unsafe.Sizeof(mem))
	if r, _, _ := procK32GetProcessMemInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&mem)), uintptr(mem.cb)); r != 0 {
		info.RSSBytes = uint64(mem.WorkingSetSize)
	}

	info.NumThreads = threadCount(uint32(pid))
	info.Cwd = workingDir(uint32(pid))
	return info, nil
}

// workingDir reads the current directory from a process's PEB. It needs
// PROCESS_VM_READ, and the PEB layout of a process with the same bitness
// as portop (a 32-bit process under WOW64 has its own).
func workingDir(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_INFORMATION|windows.PROCESS_VM_READ, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	var wow64 bool
	if err := windows.IsWow64Process(h, &wow64); err != nil || wow64 {
		return ""
	}
	var pbi windows.PROCESS_BASIC_INFORMATION
	if err := windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&pbi), uint32(unsafe.Sizeof(pbi)), nil); err != nil || pbi.PebBaseAddress == nil {
		return ""
	}
	var peb windows.PEB
	if !readMemory(h, uintptr(unsafe.Pointer(pbi.PebBaseAddress)), unsafe.Pointer(&peb), unsafe.Sizeof(peb)) || peb.ProcessParameters == nil {
		return ""
	}
	var params windows.RTL_USER_PROCESS_PARAMETERS
	if !readMemory(h, uintptr(unsafe.Pointer(peb.ProcessParameters)), unsafe.Pointer(&params), unsafe.Sizeof(params)) {
		return ""
	}
	path := params.CurrentDirectory.DosPath
	if path.Length == 0 || path.Length > 32767*2 || path.Buffer == nil {
		return ""
	}
	buf := make([]uint16, path.Length/2)
	if !readMemory(h, uintptr(unsafe.Pointer(path.Buffer)), unsafe.Pointer(&buf[0]), uintptr(path.Length)) {
		return ""
	}
	dir := windows.UTF16ToString(buf)
	if len(dir) > 3 {
		dir = strings.TrimSuffix(dir, `\`)
	}
	return dir
}

func readMemory(h windows.Handle, addr uintptr, dst unsafe.Pointer, size uintptr) bool {
	var n uintptr
	err := windows.ReadProcessMemory(h, addr, (*byte)(dst), size, &n)
	return err == nil && n == size
}

func imagePath(h windows.Handle) string {
	buf := make([]uint16, windows.MAX_LONG_PATH)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:size])
}

// commandLine uses ProcessCommandLineInformation (Windows 8.1+), which,
// unlike reading the PEB, works with limited query access.
func commandLine(h windows.Handle) string {
	size := uint32(4096)
	for attempt := 0; attempt < 4; attempt++ {
		buf := make([]byte, size)
		var ret uint32
		err := windows.NtQueryInformationProcess(h, windows.ProcessCommandLineInformation,
			unsafe.Pointer(&buf[0]), size, &ret)
		if err == windows.STATUS_INFO_LENGTH_MISMATCH && ret > size {
			size = ret
			continue
		}
		if err != nil {
			return ""
		}
		return (*windows.NTUnicodeString)(unsafe.Pointer(&buf[0])).String()
	}
	return ""
}

func owner(h windows.Handle) string {
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
	if err != nil {
		return tu.User.Sid.String()
	}
	if domain != "" {
		return domain + `\` + account
	}
	return account
}

func threadCount(pid uint32) int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return 0
	}
	defer windows.CloseHandle(snap)
	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if entry.ProcessID == pid {
			return int(entry.Threads)
		}
	}
	return 0
}
