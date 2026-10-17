package notify

import (
	_ "embed"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed portop.png
var logoPNG []byte

const appID = "padovanl.portop"

var (
	appIDOnce sync.Once
	appIDOK   bool
)

// registeredAppID registers portop as a notification sender for the
// current user, once per run, and returns its AppUserModelID. Windows
// shows a toast under the name and icon registered for its app ID; an
// unpackaged program can register one under
// HKCU\Software\Classes\AppUserModelId without an installer or a Start
// menu shortcut. If registration fails, toasts fall back to Windows
// PowerShell's identity.
func registeredAppID() string {
	appIDOnce.Do(func() { appIDOK = registerAppID() == nil })
	if appIDOK {
		return appID
	}
	return ""
}

func registerAppID() error {
	if packaged() {
		return errPackaged
	}
	dir, err := os.UserCacheDir() // %LOCALAPPDATA%
	if err != nil {
		return err
	}
	icon := filepath.Join(dir, "portop", "portop.png")
	if err := os.MkdirAll(filepath.Dir(icon), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(icon, logoPNG, 0o644); err != nil {
		return err
	}
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appID, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.SetStringValue("DisplayName", "portop"); err != nil {
		return err
	}
	return key.SetStringValue("IconUri", icon)
}

var errPackaged = errors.New("running from an MSIX package")

var procGetCurrentPackageFullName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentPackageFullName")

// packaged reports whether portop runs with package identity (installed
// from the Microsoft Store). Writes to HKCU are virtualized inside a
// package, so the registration would only look successful; such installs
// keep the Windows PowerShell identity that already works for them.
func packaged() bool {
	if procGetCurrentPackageFullName.Find() != nil {
		return false // before Windows 8
	}
	var length uint32
	r, _, _ := procGetCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&length)), 0)
	const appmodelErrorNoPackage = 15700
	return r != appmodelErrorNoPackage
}
