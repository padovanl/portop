// Package dnscache provides a caching, non-blocking reverse DNS resolver
// suitable for calling from a UI render loop: a lookup that has not
// completed yet returns immediately with ok=false and completes in the
// background, so the UI never stalls on a slow or dead resolver.
package dnscache

import (
	"context"
	"net"
	"strings"
	"sync"
	"time"
)

// Resolver is a caching reverse-DNS lookup helper. The zero value is not
// usable; construct with New.
type Resolver struct {
	timeout time.Duration
	lookup  func(ctx context.Context, addr string) ([]string, error)
	now     func() time.Time

	mu      sync.Mutex
	entries map[string]entry
}

type entry struct {
	name    string
	pending bool
	at      time.Time // when the lookup completed
}

// A resolved name is refreshed after entryTTL, and the cache never holds
// more than maxEntries addresses: a long-running TUI or dashboard on a
// busy server sees an endless stream of client addresses.
const (
	entryTTL   = 10 * time.Minute
	maxEntries = 4096
)

// New returns a Resolver that gives up on a single lookup after timeout.
func New(timeout time.Duration) *Resolver {
	r := &Resolver{
		timeout: timeout,
		entries: make(map[string]entry),
		now:     time.Now,
	}
	r.lookup = net.DefaultResolver.LookupAddr
	return r
}

// Lookup returns the cached hostname for ip, if a completed lookup
// already produced one. If no cache entry exists yet, it kicks off a
// background resolution (deduplicated: a lookup already in flight for
// this IP is not started twice) and returns ("", false) immediately.
// A completed lookup that found no PTR record is cached as ("", true)
// so we don't keep hammering the resolver for addresses with no reverse
// record.
func (r *Resolver) Lookup(ip string) (hostname string, ok bool) {
	if ip == "" || ip == "0.0.0.0" || ip == "::" {
		return "", true
	}

	r.mu.Lock()
	e, seen := r.entries[ip]
	switch {
	case seen && e.pending:
		r.mu.Unlock()
		return e.name, !e.at.IsZero()
	case seen && r.now().Sub(e.at) < entryTTL:
		r.mu.Unlock()
		return e.name, true
	case seen:
		// Expired: keep showing the old name while it is refreshed.
		r.entries[ip] = entry{name: e.name, pending: true, at: e.at}
		r.mu.Unlock()
		go r.resolve(ip)
		return e.name, true
	}
	if len(r.entries) >= maxEntries {
		r.prune()
	}
	r.entries[ip] = entry{pending: true}
	r.mu.Unlock()

	go r.resolve(ip)
	return "", false
}

// prune drops expired entries, or every completed one if that is not
// enough. Lookups in flight are kept so they are not started twice.
// Callers hold r.mu.
func (r *Resolver) prune() {
	now := r.now()
	for ip, e := range r.entries {
		if !e.pending && now.Sub(e.at) >= entryTTL {
			delete(r.entries, ip)
		}
	}
	if len(r.entries) < maxEntries {
		return
	}
	for ip, e := range r.entries {
		if !e.pending {
			delete(r.entries, ip)
		}
	}
}

func (r *Resolver) resolve(ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	names, err := r.lookup(ctx, ip)
	name := ""
	if err == nil {
		for _, n := range names {
			n = trimTrailingDot(n)
			// Some resolvers answer a PTR query with its own question
			// (34.149.56.165.in-addr.arpa) when there is no real name;
			// that is no host name.
			lower := strings.ToLower(n)
			if n == "" || strings.HasSuffix(lower, ".in-addr.arpa") || strings.HasSuffix(lower, ".ip6.arpa") {
				continue
			}
			name = n
			break
		}
	}

	r.mu.Lock()
	r.entries[ip] = entry{name: name, at: r.now()}
	r.mu.Unlock()
}

func trimTrailingDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}
