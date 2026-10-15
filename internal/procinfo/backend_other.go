//go:build !darwin && !windows

package procinfo

func Load(pid int) (Info, error) { return loadProc(pid) }
