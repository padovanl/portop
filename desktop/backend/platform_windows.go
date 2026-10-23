package backend

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"github.com/padovanl/portop/internal/hostcmd"
	"golang.org/x/sys/windows"
)

// processArgv returns nil: Windows keeps a command line, not arguments,
// and that line is already quoted for cmd.
func processArgv(int) []string { return nil }

var (
	shell32            = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = shell32.NewProc("ShellExecuteExW")
)

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         windows.Handle
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	hInstApp     windows.Handle
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    windows.Handle
	dwHotKey     uint32
	hIcon        windows.Handle
	hProcess     windows.Handle
}

const (
	seeMaskNoCloseProcess = 0x00000040
	seeMaskNoAsync        = 0x00000100
	errorCancelled        = 1223
)

// runElevated runs this executable with args through a UAC prompt and
// waits for it.
func runElevated(args []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	quoted := make([]string, len(args))
	for i, a := range args {
		quoted[i] = syscall.EscapeArg(a)
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params, _ := windows.UTF16PtrFromString(strings.Join(quoted, " "))
	info := shellExecuteInfo{fMask: seeMaskNoCloseProcess | seeMaskNoAsync, lpVerb: verb, lpFile: file, lpParameters: params, nShow: windows.SW_HIDE}
	info.cbSize = uint32(unsafe.Sizeof(info))
	if r, _, e := procShellExecuteEx.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		if errno, ok := e.(syscall.Errno); ok && errno == errorCancelled {
			return errors.New("administrator authorization was cancelled")
		}
		return e
	}
	if info.hProcess == 0 {
		return errors.New("the elevated portop did not start")
	}
	defer windows.CloseHandle(info.hProcess)
	if _, err := windows.WaitForSingleObject(info.hProcess, windows.INFINITE); err != nil {
		return err
	}
	var code uint32
	if err := windows.GetExitCodeProcess(info.hProcess, &code); err != nil {
		return err
	}
	if code != 0 {
		return fmt.Errorf("the process could not be stopped even as administrator (exit code %d)", code)
	}
	return nil
}

// openTerminal runs command from dir in a new console window that stays
// open after it exits.
func openTerminal(title, dir, command string) error {
	clean := strings.NewReplacer(`"`, "", "&", "", "|", "", "<", "", ">", "", "^", "").Replace(title)
	// The intermediate cmd.exe has no window; the console that start opens
	// for the command is meant to be seen.
	cmd := hostcmd.Hide(exec.Command("cmd.exe"))
	// cmd /k strips the outer quotes of a quoted command line, so the
	// whole line gets one more pair.
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: fmt.Sprintf(`cmd.exe /c start "%s" /D "%s" cmd.exe /k "%s"`, clean, dir, command),
	}
	return cmd.Start()
}
