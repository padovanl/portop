package dnscache

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestLookupResolvesInBackgroundAndCaches(t *testing.T) {
	r := New(time.Second)
	calls := 0
	r.lookup = func(ctx context.Context, addr string) ([]string, error) {
		calls++
		return []string{"example.com."}, nil
	}

	name, ok := r.Lookup("1.2.3.4")
	if ok {
		t.Fatalf("first call should not be resolved yet, got ok=true name=%q", name)
	}

	waitUntil(t, func() bool {
		name, ok := r.Lookup("1.2.3.4")
		return ok && name == "example.com"
	})

	if calls != 1 {
		t.Errorf("lookup called %d times, want exactly 1 (dedup + cache)", calls)
	}
}

func TestLookupNoRecordIsCachedAsEmpty(t *testing.T) {
	r := New(time.Second)
	r.lookup = func(ctx context.Context, addr string) ([]string, error) {
		return nil, errors.New("no such host")
	}

	r.Lookup("5.6.7.8")
	waitUntil(t, func() bool {
		_, ok := r.Lookup("5.6.7.8")
		return ok
	})
	name, ok := r.Lookup("5.6.7.8")
	if !ok || name != "" {
		t.Errorf("got (%q, %v), want (\"\", true)", name, ok)
	}
}

func TestLookupSkipsUnspecifiedAddresses(t *testing.T) {
	r := New(time.Second)
	r.lookup = func(ctx context.Context, addr string) ([]string, error) {
		t.Fatal("lookup should not be called for unspecified addresses")
		return nil, nil
	}
	for _, ip := range []string{"", "0.0.0.0", "::"} {
		name, ok := r.Lookup(ip)
		if !ok || name != "" {
			t.Errorf("Lookup(%q) = (%q, %v), want (\"\", true)", ip, name, ok)
		}
	}
}

func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before deadline")
}
