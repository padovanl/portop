package backend

import (
	"os"
	"time"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/baseline"
)

// atRisk is true for a dev server, database or container listening on
// every interface (or one network address) that the firewall does not
// block: usually a mistake, such as a database bound to 0.0.0.0. Web
// servers are meant to be reachable and are left out.
func atRisk(scope, category, firewall string) bool {
	if scope != app.ScopeAll && scope != app.ScopeAddress {
		return false
	}
	switch category {
	case "dev", "database", "container":
	default:
		return false
	}
	return firewall != "blocked"
}

// baselineChanges counts listening ports added or removed since the
// baseline, re-reading the file only when it changed; -1 means none.
// Callers hold b.mu.
func (b *Backend) baselineChanges(rows []app.Row) int {
	path, err := b.baselinePath()
	if err != nil {
		return -1
	}
	st, err := os.Stat(path)
	if err != nil {
		b.baseSaved, b.baseMod = nil, time.Time{}
		return -1
	}
	if !st.ModTime().Equal(b.baseMod) {
		saved, _, err := baseline.Load(path)
		if err != nil {
			return -1
		}
		b.baseSaved, b.baseMod = saved, st.ModTime()
	}
	added, removed := baseline.Diff(b.baseSaved, baseline.FromRows(rows))
	return len(added) + len(removed)
}
