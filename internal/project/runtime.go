package project

import (
	"path/filepath"
	"strings"
)

// runtimeOf is the language of a process that no manifest describes: the
// interpreter that runs it, or its script's extension. A script started
// as ./serve with a #! line still runs under its interpreter, so the
// executable (or the first word of the command line) names it even when
// the process name is the script's.
func runtimeOf(f Facts) string {
	args := splitArgs(f.Cmdline)
	for _, prog := range []string{f.Exe, first(args), f.Name} {
		if prog == "" {
			continue
		}
		if lang := interpreterLanguage(prog); lang != "" {
			return lang
		}
	}
	// No interpreter: a script named on the command line (a Windows
	// launcher, a wrapper) can still tell by its extension.
	for _, arg := range args {
		arg = strings.Trim(arg, `"'`)
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if lang := scriptLanguages[strings.ToLower(filepath.Ext(arg))]; lang != "" {
			return lang
		}
	}
	return ""
}

func first(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

var interpreters = map[string]string{
	"python": "Python", "pythonw": "Python", "py": "Python", "pypy": "Python",
	"php": "PHP", "php-fpm": "PHP", "php-cgi": "PHP",
	"ruby": "Ruby", "jruby": "Ruby",
	"node": "Node.js", "nodejs": "Node.js", "deno": "Deno", "bun": "Bun",
	"bash": "Bash", "sh": "Shell", "dash": "Shell", "zsh": "Shell", "ksh": "Shell",
	"perl": "Perl", "java": "Java", "javaw": "Java", "dotnet": ".NET",
	"elixir": "Elixir", "dart": "Dart", "lua": "Lua", "luajit": "Lua",
	"pwsh": "PowerShell", "powershell": "PowerShell",
}

var scriptLanguages = map[string]string{
	".py": "Python", ".php": "PHP", ".rb": "Ruby", ".js": "Node.js", ".mjs": "Node.js", ".cjs": "Node.js",
	".sh": "Shell", ".bash": "Bash", ".pl": "Perl", ".ps1": "PowerShell", ".lua": "Lua", ".exs": "Elixir", ".dart": "Dart",
}

// interpreterLanguage maps a program path to its language, ignoring a
// version suffix (python3.12, php8.3, ruby3.2) and .exe.
func interpreterLanguage(prog string) string {
	slash := strings.ReplaceAll(filepath.ToSlash(strings.Trim(prog, `"'`)), `\`, "/")
	// go run builds into a temporary go-build directory.
	if strings.Contains(slash, "/go-build") {
		return "Go"
	}
	name := strings.ToLower(slash[strings.LastIndex(slash, "/")+1:])
	name = strings.TrimSuffix(name, ".exe")
	if lang := interpreters[name]; lang != "" {
		return lang
	}
	base := strings.TrimRight(name, "0123456789.")
	if base != name {
		return interpreters[base]
	}
	return ""
}
