package backend

import "time"

// Long history: one point per bucket (30 s) for the last hour, for the
// whole machine and each process, next to the per-scan history.
const (
	bucket     = 30 * time.Second
	longPoints = 120
)

type accumulator struct {
	start  time.Time
	rx, tx float64
	n      int
}

type longHistory struct {
	total  []TrafficPoint
	byPID  map[int][]TrafficPoint
	accTot accumulator
	accPID map[int]*accumulator
}

func newLongHistory() *longHistory {
	return &longHistory{byPID: map[int][]TrafficPoint{}, accPID: map[int]*accumulator{}}
}

// add folds one scan into the current bucket and closes the bucket when
// it is full. Callers hold the backend's lock.
func (h *longHistory) add(now time.Time, total TrafficPoint, perPID map[int]TrafficPoint, live map[int]bool) {
	if h.accTot.start.IsZero() {
		h.accTot.start = now
	}
	h.accTot.rx += total.RX
	h.accTot.tx += total.TX
	h.accTot.n++
	for pid := range live {
		a := h.accPID[pid]
		if a == nil {
			a = &accumulator{start: now}
			h.accPID[pid] = a
		}
		p := perPID[pid]
		a.rx += p.RX
		a.tx += p.TX
		a.n++
	}
	if now.Sub(h.accTot.start) < bucket {
		return
	}
	at := now.UnixMilli()
	h.total = appendLong(h.total, TrafficPoint{Time: at, RX: h.accTot.rx / float64(h.accTot.n), TX: h.accTot.tx / float64(h.accTot.n)})
	h.accTot = accumulator{start: now}
	for pid, a := range h.accPID {
		if !live[pid] {
			delete(h.accPID, pid)
			delete(h.byPID, pid)
			continue
		}
		h.byPID[pid] = appendLong(h.byPID[pid], TrafficPoint{Time: at, RX: a.rx / float64(a.n), TX: a.tx / float64(a.n)})
		h.accPID[pid] = &accumulator{start: now}
	}
}

func appendLong(h []TrafficPoint, p TrafficPoint) []TrafficPoint {
	h = append(h, p)
	if len(h) > longPoints {
		h = h[len(h)-longPoints:]
	}
	return h
}

// TrafficHistory returns the last hour of traffic in 30-second points,
// for the whole machine (pid 0) or one process.
func (b *Backend) TrafficHistory(pid int) []TrafficPoint {
	b.mu.RLock()
	defer b.mu.RUnlock()
	src := b.long.total
	if pid != 0 {
		src = b.long.byPID[pid]
	}
	return append([]TrafficPoint{}, src...)
}
