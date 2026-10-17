package traffic

import "github.com/padovanl/portop/internal/hostcmd"

// sample reads one nettop snapshot. Other users' connections need root,
// as they do for lsof.
func sample() (map[Key]Counter, error) {
	out, err := hostcmd.Output("/usr/bin/nettop", "-L", "1", "-n", "-x", "-m", "tcp", "-J", "bytes_in,bytes_out")
	if err != nil {
		return nil, err
	}
	return parseNettop(string(out)), nil
}
