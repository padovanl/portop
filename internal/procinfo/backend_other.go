//go:build !darwin

package procinfo

func Load(pid int) (Info, error) { return loadProc(pid) }
