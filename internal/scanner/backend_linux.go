package scanner

func Scan() ([]Connection, error) {
	conns, err := scanProc()
	if err != nil {
		return nil, err
	}
	// A proc fixture or alternate proc mount must not include host sockets.
	if ProcRoot == "/proc" {
		// MPTCP diagnostics may be unavailable or restricted on this kernel.
		mptcp, _ := scanMPTCP()
		markMPTCP(conns, mptcp)
	}
	return conns, nil
}

func readProcessTicks(pid int) (uint64, error) { return readProcTicks(pid) }
