package ui

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Up          key.Binding
	Down        key.Binding
	Top         key.Binding
	Bottom      key.Binding
	Enter       key.Binding
	Kill        key.Binding
	Open        key.Binding
	Filter      key.Binding
	Sort        key.Binding
	Protocol    key.Binding
	Established key.Binding
	NewMark     key.Binding
	Copy        key.Binding
	Refresh     key.Binding
	Help        key.Binding
	Quit        key.Binding
	Escape      key.Binding
}

// KeyActions lists every remappable action, in the config.yml order
// `--init-config` writes them in. The map key is what a config.yml
// `keybindings:` entry names.
var KeyActions = []string{
	"up", "down", "top", "bottom", "enter", "kill", "open", "filter",
	"sort", "protocol", "established", "new_mark", "copy", "refresh",
	"help", "quit", "escape",
}

var defaultKeyBindings = map[string][]string{
	"up":          {"up"},
	"down":        {"down"},
	"top":         {"g", "home"},
	"bottom":      {"G", "end"},
	"enter":       {"enter"},
	"kill":        {"k"},
	"open":        {"o"},
	"filter":      {"f", "/"},
	"sort":        {"s"},
	"protocol":    {"v"},
	"established": {"e"},
	"new_mark":    {"n"},
	"copy":        {"c"},
	"refresh":     {"r"},
	"help":        {"?"},
	"quit":        {"q", "ctrl+c"},
	"escape":      {"esc"},
}

var actionDesc = map[string]string{
	"up": "up", "down": "down", "top": "top", "bottom": "bottom",
	"enter": "process details", "kill": "kill", "open": "open URL",
	"filter": "filter/search", "sort": "sort", "protocol": "IPv4/IPv6",
	"established": "established", "new_mark": "clear new-port marks",
	"copy": "copy", "refresh": "refresh", "help": "help", "quit": "quit",
	"escape": "close",
}

var keys keyMap

func init() {
	ApplyKeyBindings(nil)
}

// ApplyKeyBindings (re)builds the package's keymap from the built-in
// defaults, with any user overrides (config.yml's `keybindings:` map,
// action name -> key list) layered on top. An action missing from
// overrides, or overridden with an empty list, keeps its default keys.
func ApplyKeyBindings(overrides map[string][]string) {
	merged := make(map[string][]string, len(defaultKeyBindings))
	for action, ks := range defaultKeyBindings {
		merged[action] = ks
	}
	for action, ks := range overrides {
		if len(ks) > 0 {
			merged[action] = ks
		}
	}

	bind := func(action string) key.Binding {
		ks := merged[action]
		help := ""
		if len(ks) > 0 {
			help = ks[0]
		}
		return key.NewBinding(key.WithKeys(ks...), key.WithHelp(help, actionDesc[action]))
	}

	keys = keyMap{
		Up:          bind("up"),
		Down:        bind("down"),
		Top:         bind("top"),
		Bottom:      bind("bottom"),
		Enter:       bind("enter"),
		Kill:        bind("kill"),
		Open:        bind("open"),
		Filter:      bind("filter"),
		Sort:        bind("sort"),
		Protocol:    bind("protocol"),
		Established: bind("established"),
		NewMark:     bind("new_mark"),
		Copy:        bind("copy"),
		Refresh:     bind("refresh"),
		Help:        bind("help"),
		Quit:        bind("quit"),
		Escape:      bind("escape"),
	}
}
