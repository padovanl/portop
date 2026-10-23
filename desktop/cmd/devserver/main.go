// Command devserver serves portop desktop's window in a browser, for
// working on the frontend without building the Wails app (which needs
// WebKitGTK headers on Linux). It reads the frontend from disk on every
// request and stands in for the Wails runtime with a small shim: method
// calls go over POST /api/call and events over Server-Sent Events.
//
//	go run ./cmd/devserver            # http://127.0.0.1:34115
//
// It listens on loopback only and is not part of any release.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"

	"github.com/padovanl/portop/desktop/backend"
	"github.com/padovanl/portop/internal/cli"
)

const servicePrefix = "github.com/padovanl/portop/desktop/backend.Backend."

func main() {
	// Show all runs this executable as administrator, as the app does.
	if len(os.Args) == 4 && os.Args[1] == "--elevated-helper" {
		parent, _ := strconv.Atoi(os.Args[3])
		os.Exit(cli.ServeHelper(os.Args[2], parent, os.Stderr))
	}
	addr := flag.String("addr", "127.0.0.1:34115", "listen address")
	dir := flag.String("frontend", "frontend", "frontend directory")
	settings := flag.String("settings", "", "settings file (default: a temporary one)")
	hideSelf := flag.Bool("hide-self", false, "leave this server's own socket out (for screenshots)")
	shellPreview := flag.Bool("shell-preview", false, "show the settings of the native shell (autostart, shortcut), which do nothing here (for screenshots)")
	flag.Parse()
	// Settings, recent projects and the baseline go to a private
	// temporary directory, so runs never see each other's data.
	tmp, err := os.MkdirTemp("", "portop-desktop-dev-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	if *settings == "" {
		*settings = filepath.Join(tmp, "desktop.json")
	}
	b := backend.New(*settings)
	// Keep the baseline next to the temporary settings, not in the file
	// portop --save-baseline uses.
	b.BaselinePath = filepath.Join(tmp, "baseline.json")
	if *hideSelf {
		b.HidePID = os.Getpid()
	}
	if *shellPreview {
		b.OnSettings = func(backend.Settings) {}
	}
	var mu sync.Mutex
	clients := map[chan []byte]bool{}
	broadcast := func(name string, data any) {
		msg, _ := json.Marshal(map[string]any{"name": name, "data": data})
		mu.Lock()
		defer mu.Unlock()
		for c := range clients {
			select {
			case c <- msg:
			default:
			}
		}
	}
	b.OnSnapshot = func(s backend.Snapshot) { broadcast("snapshot", s) }
	b.OnNotify = func(title, body string) { broadcast("notify", map[string]string{"title": title, "body": body}) }
	b.Start(context.Background())

	mux := http.NewServeMux()
	mux.HandleFunc("GET /wails/runtime.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		fmt.Fprint(w, shim)
	})
	mux.HandleFunc("POST /api/call", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name string            `json:"name"`
			Args []json.RawMessage `json:"args"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := call(b, req.Name, req.Args)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"result": result})
	})
	mux.HandleFunc("GET /api/events", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		c := make(chan []byte, 8)
		mu.Lock()
		clients[c] = true
		mu.Unlock()
		defer func() { mu.Lock(); delete(clients, c); mu.Unlock() }()
		flusher, _ := w.(http.Flusher)
		for {
			select {
			case <-r.Context().Done():
				return
			case msg := <-c:
				fmt.Fprintf(w, "data: %s\n\n", msg)
				if flusher != nil {
					flusher.Flush()
				}
			}
		}
	})
	mux.Handle("/", http.FileServer(http.Dir(*dir)))
	log.Printf("portop desktop (browser) on http://%s", *addr)
	log.Fatal(http.ListenAndServe(*addr, mux))
}

// call invokes a Backend method by its Wails name with JSON arguments.
func call(b *backend.Backend, name string, raw []json.RawMessage) (any, error) {
	method := reflect.ValueOf(b).MethodByName(strings.TrimPrefix(name, servicePrefix))
	if !strings.HasPrefix(name, servicePrefix) || !method.IsValid() {
		return nil, fmt.Errorf("unknown method %q", name)
	}
	t := method.Type()
	if len(raw) != t.NumIn() {
		return nil, fmt.Errorf("%s takes %d arguments", name, t.NumIn())
	}
	args := make([]reflect.Value, len(raw))
	for i, r := range raw {
		v := reflect.New(t.In(i))
		if err := json.Unmarshal(r, v.Interface()); err != nil {
			return nil, err
		}
		args[i] = v.Elem()
	}
	errType := reflect.TypeOf((*error)(nil)).Elem()
	var result any
	for _, o := range method.Call(args) {
		if o.Type() == errType {
			if !o.IsNil() {
				return nil, o.Interface().(error)
			}
			continue
		}
		result = o.Interface()
	}
	return result, nil
}

// shim is the part of @wailsio/runtime the window uses.
const shim = `
const listeners = new Map();
const source = new EventSource("/api/events");
source.onmessage = (m) => {
  const ev = JSON.parse(m.data);
  for (const cb of listeners.get(ev.name) || []) cb({ name: ev.name, data: ev.data });
};
export const Call = {
  async ByName(name, ...args) {
    const r = await fetch("/api/call", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ name, args }) });
    const body = await r.json();
    if (!r.ok) throw new Error(body.error);
    return body.result;
  },
};
export const Events = {
  On(name, cb) {
    if (!listeners.has(name)) listeners.set(name, new Set());
    listeners.get(name).add(cb);
    return () => listeners.get(name).delete(cb);
  },
  Emit() {},
};
export const Window = { Minimise() {}, ToggleMaximise() {}, Close() {}, Hide() {} };
export const Browser = { OpenURL(url) { window.open(url, "_blank"); } };
export const Clipboard = { SetText(t) { return navigator.clipboard.writeText(t); } };
export const System = { IsMac: () => /Mac/.test(navigator.platform), IsWindows: () => /Win/.test(navigator.platform), IsLinux: () => /Linux/.test(navigator.platform) };
`
