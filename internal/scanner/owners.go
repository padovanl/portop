package scanner

import "sort"

// mainOwner picks the process to show for a socket that several processes
// hold: pre-forking servers (nginx, gunicorn, php-fpm, Apache) open the
// listening socket in a master and share it with their workers. The
// master is the owner whose parent does not hold the socket too; PID 1 is
// skipped while others hold it, since with socket activation systemd
// keeps the socket open next to the service it started. Among several
// candidates the lowest PID wins. parent returns 0 when unknown.
func mainOwner(owners []int, parent func(pid int) int) int {
	if len(owners) == 0 {
		return 0
	}
	pids := append([]int(nil), owners...)
	sort.Ints(pids)
	if len(pids) > 1 && pids[0] == 1 {
		pids = pids[1:]
	}
	holds := make(map[int]bool, len(pids))
	for _, pid := range pids {
		holds[pid] = true
	}
	for _, pid := range pids {
		if !holds[parent(pid)] {
			return pid
		}
	}
	return pids[0]
}

// setOwners records every holder of c's socket and makes the main one
// its PID. name returns a process name.
func (c *Connection) setOwners(owners []int, parent func(int) int, name func(int) string) {
	sort.Ints(owners)
	c.Owners = owners
	if main := mainOwner(owners, parent); main != c.PID {
		c.PID = main
		c.ProcessName = name(main)
	}
}
