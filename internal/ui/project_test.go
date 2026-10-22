package ui

import (
	"strings"
	"testing"

	"github.com/padovanl/portop/internal/app"
	"github.com/padovanl/portop/internal/project"
)

func TestProjectColumn(t *testing.T) {
	cases := []struct {
		info project.Info
		want string
	}{
		{project.Info{Name: "storefront", Framework: "Next.js"}, "storefront Next.js"},
		{project.Info{Name: "billing"}, "billing"},
		{project.Info{Framework: "Django"}, "Django"},
		{project.Info{}, "-"},
	}
	for _, c := range cases {
		if got := cellValue(app.Row{Project: c.info}, "PROJECT"); got != c.want {
			t.Errorf("PROJECT for %+v = %q, want %q", c.info, got, c.want)
		}
	}

	// Wide terminals show PROJECT before the other optional columns.
	cols := columnsFor(false, 200)
	titles := make([]string, len(cols))
	for i, c := range cols {
		titles[i] = c.title
	}
	if got := strings.Join(titles, " "); !strings.Contains(got, "CPU PROJECT IFACE") {
		t.Errorf("listening columns: %s", got)
	}
	if got := columnsFor(true, 200); got[6].title != "REMOTE" || got[7].title != "PROJECT" {
		t.Errorf("established columns: %v", got)
	}
}

func TestSortByProjectPutsRowsWithoutOneLast(t *testing.T) {
	rows := []app.Row{
		{LocalPort: 1},
		{LocalPort: 2, Project: project.Info{Name: "zeta"}},
		{LocalPort: 3, Project: project.Info{Name: "Alpha", Framework: "Vite"}},
	}
	sortRowsDirection(rows, sortByProject, false)
	if rows[0].LocalPort != 3 || rows[1].LocalPort != 2 || rows[2].LocalPort != 1 {
		t.Errorf("order: %d %d %d", rows[0].LocalPort, rows[1].LocalPort, rows[2].LocalPort)
	}
}
