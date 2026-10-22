// Command portop-desktop is portop's desktop app: the same scanner as the
// terminal UI, in a window, with a tray (or menu bar) icon.
package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/padovanl/portop/desktop/backend"
	"github.com/padovanl/portop/internal/cli"
	"github.com/padovanl/portop/internal/notify"
)

//go:embed all:frontend
var assets embed.FS

//go:embed frontend/logo.png
var trayIcon []byte

// version is set at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	// The window asks for administrator rights by running this executable
	// again with --signal-pid (through pkexec, osascript or UAC); that
	// path is the CLI's own, which checks the process start time.
	if len(os.Args) > 1 && os.Args[1] == "--signal-pid" {
		os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
	}
	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("portop desktop " + version)
		return
	}

	b := backend.New("")
	b.Version = version
	front, err := fs.Sub(assets, "frontend")
	if err != nil {
		log.Fatal(err)
	}

	var window *application.WebviewWindow
	showWindow := func() {
		if window == nil {
			return
		}
		window.Show()
		window.UnMinimise()
		window.Focus()
	}

	app := application.New(application.Options{
		Name:        "portop",
		Description: "Which process is using each port, in a window",
		Services:    []application.Service{application.NewService(b)},
		Assets:      application.AssetOptions{Handler: application.AssetFileServerFS(front)},
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               "dev.lucapadovan.portop.desktop",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { showWindow() },
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:             "main",
		Title:            "portop",
		Width:            1320,
		Height:           860,
		MinWidth:         880,
		MinHeight:        580,
		URL:              "/",
		BackgroundColour: application.NewRGB(7, 11, 22),
		Mac: application.MacWindow{
			TitleBar:                application.MacTitleBarHiddenInset,
			InvisibleTitleBarHeight: 38,
			Backdrop:                application.MacBackdropTranslucent,
		},
		Windows: application.WindowsWindow{BackdropType: application.Mica},
	})
	// Closing the window keeps portop in the tray, unless the user turned
	// that off.
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if b.Settings().CloseToTray {
			window.Hide()
			e.Cancel()
			return
		}
		app.Quit()
	})

	tray := app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		tray.SetTemplateIcon(trayIcon)
	} else {
		tray.SetIcon(trayIcon)
	}
	tray.SetTooltip("portop")
	tray.OnClick(showWindow)
	menus := newTrayMenu(app, tray, b, showWindow)
	menus.build(backend.Snapshot{})

	b.OnSnapshot = func(s backend.Snapshot) {
		app.Event.Emit("snapshot", s)
		menus.update(s)
	}
	b.OnNotify = func(title, body string) {
		notify.Send(title, body)
		app.Event.Emit("notify", map[string]string{"title": title, "body": body})
	}
	app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		b.Start(context.Background())
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// trayMenu keeps the tray menu in step with the dev servers that run.
type trayMenu struct {
	app  *application.App
	tray *application.SystemTray
	b    *backend.Backend
	show func()

	mu  sync.Mutex
	sig string
}

func newTrayMenu(app *application.App, tray *application.SystemTray, b *backend.Backend, show func()) *trayMenu {
	return &trayMenu{app: app, tray: tray, b: b, show: show}
}

type devServer struct {
	pid   int
	name  string
	ports []int
	url   string
}

func devServers(s backend.Snapshot) []devServer {
	byPID := map[int]*devServer{}
	for _, r := range s.Rows {
		if !r.Listening || r.Category != "dev" || r.PID == 0 {
			continue
		}
		d := byPID[r.PID]
		if d == nil {
			name := r.Project
			if name == "" {
				name = r.Process
			}
			if r.Framework != "" {
				name += " (" + r.Framework + ")"
			}
			d = &devServer{pid: r.PID, name: name}
			byPID[r.PID] = d
		}
		known := false
		for _, p := range d.ports {
			known = known || p == int(r.LocalPort)
		}
		if !known {
			d.ports = append(d.ports, int(r.LocalPort))
		}
		if d.url == "" {
			d.url = r.URL
		}
	}
	out := make([]devServer, 0, len(byPID))
	for _, d := range byPID {
		sort.Ints(d.ports)
		out = append(out, *d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ports[0] < out[j].ports[0] })
	return out
}

func (t *trayMenu) update(s backend.Snapshot) {
	t.tray.SetTooltip(fmt.Sprintf("portop · %d ports open, %d dev servers", s.Summary.Listening, s.Summary.DevServers))
	t.build(s)
}

// build replaces the menu when the list of dev servers changed.
func (t *trayMenu) build(s backend.Snapshot) {
	servers := devServers(s)
	var sig strings.Builder
	for _, d := range servers {
		fmt.Fprintf(&sig, "%d:%s:%v;", d.pid, d.name, d.ports)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.sig == sig.String() && t.sig != "" {
		return
	}
	t.sig = sig.String()

	menu := t.app.NewMenu()
	menu.Add("Open portop").OnClick(func(*application.Context) { t.show() })
	menu.AddSeparator()
	if len(servers) == 0 {
		menu.Add("No dev servers running").SetEnabled(false)
	} else {
		menu.Add(fmt.Sprintf("Dev servers (%d)", len(servers))).SetEnabled(false)
		for _, d := range servers {
			d := d
			ports := make([]string, len(d.ports))
			for i, p := range d.ports {
				ports[i] = fmt.Sprintf(":%d", p)
			}
			sub := menu.AddSubmenu(strings.Join(ports, " ") + "  " + d.name)
			if d.url != "" {
				sub.Add("Open in browser").OnClick(func(*application.Context) {
					if err := t.b.OpenURL(d.url); err != nil {
						notify.Send("portop", err.Error())
					}
				})
			}
			sub.Add("Show details").OnClick(func(*application.Context) {
				t.show()
				t.app.Event.Emit("navigate", "overview")
			})
			sub.Add(fmt.Sprintf("Stop (PID %d)…", d.pid)).OnClick(func(*application.Context) { t.confirmStop(d) })
		}
	}
	menu.AddSeparator()
	menu.Add("Ports").OnClick(func(*application.Context) { t.show(); t.app.Event.Emit("navigate", "ports") })
	menu.Add("Map").OnClick(func(*application.Context) { t.show(); t.app.Event.Emit("navigate", "map") })
	menu.AddSeparator()
	menu.Add("Quit portop").OnClick(func(*application.Context) { t.app.Quit() })
	t.tray.SetMenu(menu)
}

// confirmStop asks before stopping a dev server from the tray, as the
// window does: a stray click in a menu must not end anyone's work.
func (t *trayMenu) confirmStop(d devServer) {
	info, err := t.b.Process(d.pid)
	if err != nil {
		notify.Send("portop", fmt.Sprintf("Could not read %s: %v", d.name, err))
		return
	}
	q := t.app.Dialog.Question().
		SetTitle("Stop " + d.name + "?").
		SetMessage(fmt.Sprintf("portop sends SIGTERM to PID %d. Its connections will close.", d.pid))
	q.AddButton("Stop").OnClick(func() {
		if err := t.b.Stop(d.pid, info.Started, false); err != nil {
			notify.Send("portop", fmt.Sprintf("Could not stop %s: %v", d.name, err))
		}
	})
	q.AddButton("Cancel").SetAsDefault().SetAsCancel()
	q.Show()
}
