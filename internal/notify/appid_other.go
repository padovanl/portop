//go:build !windows

package notify

// registeredAppID only applies to Windows toasts.
func registeredAppID() string { return "" }
