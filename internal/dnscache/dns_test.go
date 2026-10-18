package dnscache

import (
	"context"
	"errors"
	"fmt"
	"sync"
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

// A name is refreshed after entryTTL, and the old one is still shown
// while the refresh runs.
func TestLookupRefreshesExpiredNames(t *testing.T) {
	r := New(time.Second)
	var mu sync.Mutex
	now := time.Unix(1700000000, 0)
	r.now = func() time.Time { mu.Lock(); defer mu.Unlock(); return now }
	names := []string{"old.example.", "new.example."}
	calls := 0
	r.lookup = func(context.Context, string) ([]string, error) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		return names[min(calls, len(names))-1:][:1], nil
	}

	r.Lookup("1.2.3.4")
	waitUntil(t, func() bool { name, ok := r.Lookup("1.2.3.4"); return ok && name == "old.example" })

	mu.Lock()
	now = now.Add(entryTTL)
	mu.Unlock()
	if name, ok := r.Lookup("1.2.3.4"); !ok || name != "old.example" {
		t.Fatalf("expired entry: %q %v, want the old name while refreshing", name, ok)
	}
	waitUntil(t, func() bool { name, _ := r.Lookup("1.2.3.4"); return name == "new.example" })
}

// The cache stays bounded however many addresses go through it.
func TestLookupCacheIsBounded(t *testing.T) {
	r := New(time.Second)
	r.lookup = func(context.Context, string) ([]string, error) { return nil, errors.New("no such host") }
	r.mu.Lock()
	for i := 0; i < maxEntries; i++ {
		r.entries[fmt.Sprintf("10.0.%d.%d", i>>8, i&255)] = entry{at: r.now()}
	}
	r.mu.Unlock()

	r.Lookup("192.0.2.1")
	r.mu.Lock()
	n := len(r.entries)
	r.mu.Unlock()
	if n > maxEntries {
		t.Fatalf("cache holds %d entries, want at most %d", n, maxEntries)
	}
}
