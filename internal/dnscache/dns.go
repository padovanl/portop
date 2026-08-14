// Package dnscache provides a caching, non-blocking reverse DNS resolver
// suitable for calling from a UI render loop: a lookup that has not
// completed yet returns immediately with ok=false and completes in the
// background, so the UI never stalls on a slow or dead resolver.
package dnscache

import (
	"context"
	"net"
	"sync"
	"time"
)

// Resolver is a caching reverse-DNS lookup helper. The zero value is not
// usable; construct with New.
type Resolver struct {
	timeout time.Duration
	lookup  func(ctx context.Context, addr string) ([]string, error)

	mu      sync.Mutex
	entries map[string]entry
}

type entry struct {
	name    string
	pending bool
}

// New returns a Resolver that gives up on a single lookup after timeout.
func New(timeout time.Duration) *Resolver {
	r := &Resolver{
		timeout: timeout,
		entries: make(map[string]entry),
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
	if seen && !e.pending {
		r.mu.Unlock()
		return e.name, true
	}
	if seen && e.pending {
		r.mu.Unlock()
		return "", false
	}
	r.entries[ip] = entry{pending: true}
	r.mu.Unlock()

	go r.resolve(ip)
	return "", false
}

func (r *Resolver) resolve(ip string) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	names, err := r.lookup(ctx, ip)
	name := ""
	if err == nil && len(names) > 0 {
		name = trimTrailingDot(names[0])
	}

	r.mu.Lock()
	r.entries[ip] = entry{name: name, pending: false}
	r.mu.Unlock()
}

func trimTrailingDot(s string) string {
	if len(s) > 0 && s[len(s)-1] == '.' {
		return s[:len(s)-1]
	}
	return s
}
