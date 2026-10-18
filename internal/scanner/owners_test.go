package scanner

import "testing"

func TestMainOwner(t *testing.T) {
	parents := map[int]int{
		100: 1, 101: 100, 102: 100, // nginx master and workers
		500: 1,         // service started by systemd socket activation
		700: 1, 701: 1, // two unrelated processes sharing a port (SO_REUSEPORT)
		900: 1, 901: 900, 902: 901, // a worker of a worker
	}
	parent := func(pid int) int { return parents[pid] }
	cases := []struct {
		owners []int
		want   int
	}{
		{[]int{102, 100, 101}, 100},
		{[]int{1, 500}, 500},
		{[]int{701, 700}, 700},
		{[]int{902, 901, 900}, 900},
		{[]int{42}, 42},
		{[]int{1}, 1},
		{nil, 0},
	}
	for _, c := range cases {
		if got := mainOwner(c.owners, parent); got != c.want {
			t.Errorf("mainOwner(%v) = %d, want %d", c.owners, got, c.want)
		}
	}
}
