package project

import "sync"

// Resolver detects the project of each process once and remembers it
// while the process lives: a server's working directory and command line
// do not change, and reading them for every PID on every scan would cost
// a process launch per PID on macOS.
type Resolver struct {
	mu    sync.Mutex
	cache map[int]Info
	facts func(pids []int) map[int]Facts // platform hook, replaced in tests
}

// NewResolver returns a Resolver reading the platform's process table.
func NewResolver() *Resolver {
	return &Resolver{cache: map[int]Info{}, facts: processFacts}
}

// Resolve returns the project of every PID, detecting the ones it has not
// seen and forgetting the PIDs that are gone.
func (r *Resolver) Resolve(pids []int) map[int]Info {
	r.mu.Lock()
	defer r.mu.Unlock()
	live := make(map[int]bool, len(pids))
	var missing []int
	for _, pid := range pids {
		if pid <= 0 || live[pid] {
			continue
		}
		live[pid] = true
		if _, ok := r.cache[pid]; !ok {
			missing = append(missing, pid)
		}
	}
	if len(missing) > 0 {
		facts := r.facts(missing)
		for _, pid := range missing {
			f, ok := facts[pid]
			if !ok {
				r.cache[pid] = Info{}
				continue
			}
			r.cache[pid] = Detect(f)
		}
	}
	out := make(map[int]Info, len(live))
	for pid := range r.cache {
		if !live[pid] {
			delete(r.cache, pid)
			continue
		}
		out[pid] = r.cache[pid]
	}
	return out
}
