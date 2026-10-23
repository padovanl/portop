//go:build !linux && !darwin && !windows

package backend

func appIcon(exe, name string) string { return "" }
