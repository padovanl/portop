//go:build !darwin

package scanner

func Scan() ([]Connection, error) { return scanProc() }

func readProcessTicks(pid int) (uint64, error) { return readProcTicks(pid) }
