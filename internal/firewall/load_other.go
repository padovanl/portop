//go:build !linux && !darwin && !windows

package firewall

func load() Policy { return nil }
