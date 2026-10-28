package project

import (
	"path/filepath"
	"strings"
)

// commandFrameworks maps the program or script a server was started with
// to the framework it serves. The command line is a stronger hint than
// the dependencies: a Next.js project can also run Storybook.
var commandFrameworks = map[string]string{
	"vite":               "Vite",
	"next":               "Next.js",
	"nuxt":               "Nuxt",
	"nuxi":               "Nuxt",
	"astro":              "Astro",
	"remix":              "Remix",
	"react-router":       "React Router",
	"react-scripts":      "Create React App",
	"ng":                 "Angular",
	"svelte-kit":         "SvelteKit",
	"gatsby":             "Gatsby",
	"parcel":             "Parcel",
	"webpack":            "webpack",
	"webpack-dev-server": "webpack",
	"storybook":          "Storybook",
	"start-storybook":    "Storybook",
	"docusaurus":         "Docusaurus",
	"vitepress":          "VitePress",
	"expo":               "Expo",
	"nest":               "NestJS",
	"wrangler":           "Wrangler",
	"uvicorn":            "Uvicorn",
	"gunicorn":           "Gunicorn",
	"hypercorn":          "Hypercorn",
	"flask":              "Flask",
	"streamlit":          "Streamlit",
	"jupyter-lab":        "Jupyter",
	"jupyter-notebook":   "Jupyter",
	"jupyter-server":     "Jupyter",
	"rails":              "Rails",
	"puma":               "Puma",
	"hugo":               "Hugo",
	"jekyll":             "Jekyll",
	"air":                "Air",
	"trunk":              "Trunk",
	"mkdocs":             "MkDocs",
}

// argFrameworks are recognised by an argument rather than the program.
var argFrameworks = []struct {
	args []string // every one must appear
	name string
}{
	{[]string{"manage.py", "runserver"}, "Django"},
	{[]string{"artisan", "serve"}, "Laravel"},
	{[]string{"-m", "http.server"}, "Python http.server"},
	{[]string{"jupyter", "lab"}, "Jupyter"},
	{[]string{"jupyter", "notebook"}, "Jupyter"},
	{[]string{"dotnet", "watch"}, "ASP.NET Core"},
}

// frameworkFromCmdline recognises the framework a command line starts.
func frameworkFromCmdline(cmdline string) string {
	fields := splitArgs(cmdline)
	names := make([]string, len(fields))
	for i, f := range fields {
		names[i] = commandName(f)
	}
	for _, a := range argFrameworks {
		all := true
		for _, want := range a.args {
			found := false
			for _, n := range names {
				if n == want {
					found = true
					break
				}
			}
			all = all && found
		}
		if all {
			return a.name
		}
	}
	// Only the program and the scripts it runs count, not their options
	// or arguments: "vim next.config.js" is no Next.js server.
	for i, n := range names {
		if i > 0 && !isRunner(names[0]) {
			break
		}
		if i > 0 && strings.HasPrefix(fields[i], "-") {
			continue
		}
		if fw, ok := commandFrameworks[n]; ok {
			return fw
		}
	}
	return ""
}

// splitArgs splits a command line on spaces, keeping a double-quoted
// argument whole: Windows reports "C:\Program Files\nodejs\node.exe" ...
func splitArgs(cmdline string) []string {
	var args []string
	var cur strings.Builder
	quoted, any := false, false
	for _, r := range cmdline {
		switch {
		case r == '"':
			quoted, any = !quoted, true
		case (r == ' ' || r == '\t') && !quoted:
			if any {
				args = append(args, cur.String())
				cur.Reset()
				any = false
			}
		default:
			cur.WriteRune(r)
			any = true
		}
	}
	if any {
		args = append(args, cur.String())
	}
	return args
}

// isRunner is true for programs that run another tool named after them on
// the command line (node .../vite.js, npx next, python -m flask).
func isRunner(name string) bool {
	switch name {
	case "node", "nodejs", "bun", "deno", "npx", "pnpm", "yarn", "npm", "python", "python3", "py", "ruby", "bundle", "php", "dotnet", "go", "cargo", "uv", "poetry", "pipenv", "sh", "bash", "cmd":
		return true
	}
	return strings.HasPrefix(name, "python3.")
}

// commandName reduces an argument to a bare program or script name:
// "/app/node_modules/vite/bin/vite.js" is "vite", "C:\...\next.cmd" is
// "next", and node_modules/.bin/next is "next".
func commandName(arg string) string {
	arg = strings.Trim(arg, `"'`)
	if strings.HasPrefix(arg, "-") {
		return arg
	}
	slash := strings.ReplaceAll(filepath.ToSlash(arg), `\`, "/")
	name := slash[strings.LastIndex(slash, "/")+1:]
	lower := strings.ToLower(name)
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1", ".js", ".mjs", ".cjs", ".py", ".rb"} {
		lower = strings.TrimSuffix(lower, ext)
	}
	// node_modules/<pkg>/bin/<pkg>.js and node_modules/<pkg>/dist/cli.js
	// name the package rather than the file.
	if lower == "cli" || lower == "index" || lower == "bin" {
		if i := strings.Index(slash, "node_modules/"); i >= 0 {
			rest := slash[i+len("node_modules/"):]
			if strings.HasPrefix(rest, "@") {
				parts := strings.SplitN(rest, "/", 3)
				if len(parts) >= 2 {
					return strings.ToLower(parts[1])
				}
			}
			return strings.ToLower(strings.SplitN(rest, "/", 2)[0])
		}
	}
	if lower == "manage" && strings.HasSuffix(strings.ToLower(name), ".py") {
		return "manage.py"
	}
	return lower
}

// languageOf is the language a framework found on the command line implies,
// for processes whose project could not be read.
func languageOf(framework string) string {
	switch framework {
	case "Django", "FastAPI", "Flask", "Uvicorn", "Gunicorn", "Hypercorn", "Streamlit", "Jupyter", "Python http.server", "MkDocs":
		return "Python"
	case "Rails", "Puma", "Jekyll", "Sinatra":
		return "Ruby"
	case "Laravel", "Symfony":
		return "PHP"
	case "Hugo", "Air":
		return "Go"
	case "Trunk":
		return "Rust"
	case "ASP.NET Core":
		return ".NET"
	}
	return "Node.js"
}
