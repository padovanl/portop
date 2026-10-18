package traffic

import "github.com/padovanl/portop/internal/hostcmd"

var nettopArgs = []string{"-L", "1", "-n", "-x", "-m", "tcp", "-J", "bytes_in,bytes_out"}

func nettopOutput() ([]byte, error) {
	return hostcmd.Output("/usr/bin/nettop", nettopArgs...)
}

// sample reads one nettop snapshot. Other users' connections need root,
// as they do for lsof.
func sample() (map[Key]Counter, error) {
	out, err := nettopOutput()
	if err != nil {
		return nil, err
	}
	return parseNettop(string(out)), nil
}
