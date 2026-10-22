package project

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// maxManifest caps how much of a manifest is read.
const maxManifest = 1 << 20

func readFile(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	if st, err := f.Stat(); err != nil || !st.Mode().IsRegular() {
		return "", false
	}
	b, err := io.ReadAll(io.LimitReader(f, maxManifest))
	return string(b), err == nil
}

// readManifests recognises the project in dir, if dir holds a manifest.
// The order matters where a directory has several (a Django project with
// a package.json for its assets is a Python project).
func readManifests(dir string) (Info, bool) {
	at := func(name string) string { return filepath.Join(dir, name) }
	base := filepath.Base(dir)

	if s, ok := readFile(at("manage.py")); ok && strings.Contains(s, "django") {
		return Info{Name: pythonName(dir, base), Root: dir, Language: "Python", Framework: "Django"}, true
	}
	if s, ok := readFile(at("pyproject.toml")); ok {
		name := tomlValue(s, []string{"project", "tool.poetry"}, "name")
		return Info{Name: or(name, base), Root: dir, Language: "Python", Framework: pythonFramework(s)}, true
	}
	if s, ok := readFile(at("package.json")); ok {
		return nodeManifest(s, dir, base), true
	}
	if s, ok := readFile(at("deno.json")); ok {
		return Info{Name: or(jsonName(s), base), Root: dir, Language: "Deno"}, true
	}
	if s, ok := readFile(at("go.mod")); ok {
		name := base
		if m := goModule.FindStringSubmatch(s); m != nil {
			name = lastPathElem(m[1])
		}
		return Info{Name: name, Root: dir, Language: "Go"}, true
	}
	if s, ok := readFile(at("Cargo.toml")); ok {
		name := tomlValue(s, []string{"package"}, "name")
		fw := ""
		for _, c := range []struct{ dep, name string }{{"axum", "Axum"}, {"actix-web", "Actix Web"}, {"rocket", "Rocket"}, {"leptos", "Leptos"}, {"tauri", "Tauri"}} {
			if depLine(s, c.dep) {
				fw = c.name
				break
			}
		}
		return Info{Name: or(name, base), Root: dir, Language: "Rust", Framework: fw}, true
	}
	if s, ok := readFile(at("Gemfile")); ok {
		fw := ""
		if strings.Contains(s, "'rails'") || strings.Contains(s, `"rails"`) {
			fw = "Rails"
		} else if strings.Contains(s, "sinatra") {
			fw = "Sinatra"
		}
		return Info{Name: base, Root: dir, Language: "Ruby", Framework: fw}, true
	}
	if s, ok := readFile(at("composer.json")); ok {
		fw := ""
		if strings.Contains(s, `"laravel/framework"`) {
			fw = "Laravel"
		} else if strings.Contains(s, `"symfony/framework-bundle"`) {
			fw = "Symfony"
		}
		name := jsonName(s)
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		return Info{Name: or(name, base), Root: dir, Language: "PHP", Framework: fw}, true
	}
	if s, ok := readFile(at("requirements.txt")); ok {
		return Info{Name: base, Root: dir, Language: "Python", Framework: pythonFramework(s)}, true
	}
	if s, ok := readFile(at("pom.xml")); ok {
		fw := ""
		if strings.Contains(s, "spring-boot") {
			fw = "Spring Boot"
		}
		return Info{Name: or(pomArtifact(s), base), Root: dir, Language: "Java", Framework: fw}, true
	}
	for _, g := range []string{"build.gradle.kts", "build.gradle"} {
		if s, ok := readFile(at(g)); ok {
			fw := ""
			if strings.Contains(s, "org.springframework.boot") {
				fw = "Spring Boot"
			} else if strings.Contains(s, "io.ktor") {
				fw = "Ktor"
			}
			name := base
			for _, settings := range []string{"settings.gradle.kts", "settings.gradle"} {
				if st, ok := readFile(at(settings)); ok {
					if m := gradleName.FindStringSubmatch(st); m != nil {
						name = m[1]
					}
					break
				}
			}
			lang := "Java"
			if strings.HasSuffix(g, ".kts") {
				lang = "Kotlin"
			}
			return Info{Name: name, Root: dir, Language: lang, Framework: fw}, true
		}
	}
	if s, ok := readFile(at("mix.exs")); ok {
		fw := ""
		if strings.Contains(s, ":phoenix") {
			fw = "Phoenix"
		}
		return Info{Name: base, Root: dir, Language: "Elixir", Framework: fw}, true
	}
	if s, ok := readFile(at("pubspec.yaml")); ok {
		name := base
		if m := yamlName.FindStringSubmatch(s); m != nil {
			name = m[1]
		}
		return Info{Name: name, Root: dir, Language: "Dart"}, true
	}
	if entries, err := os.ReadDir(dir); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".csproj") || strings.HasSuffix(e.Name(), ".fsproj") {
				s, _ := readFile(at(e.Name()))
				fw := ""
				if strings.Contains(s, "Microsoft.NET.Sdk.Web") {
					fw = "ASP.NET Core"
				}
				return Info{Name: strings.TrimSuffix(e.Name(), filepath.Ext(e.Name())), Root: dir, Language: ".NET", Framework: fw}, true
			}
		}
	}
	return Info{}, false
}

var (
	goModule   = regexp.MustCompile(`(?m)^module\s+"?([^\s"]+)"?`)
	gradleName = regexp.MustCompile(`rootProject\.name\s*=\s*["']([^"']+)["']`)
	yamlName   = regexp.MustCompile(`(?m)^name:\s*["']?([\w.-]+)`)
	pomID      = regexp.MustCompile(`<artifactId>\s*([^<\s]+)\s*</artifactId>`)
)

func or(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func lastPathElem(module string) string {
	module = strings.TrimSuffix(module, "/")
	parts := strings.Split(module, "/")
	last := parts[len(parts)-1]
	// github.com/x/app/v2 is "app".
	if len(parts) > 1 && len(last) > 1 && last[0] == 'v' && strings.Trim(last[1:], "0123456789") == "" {
		last = parts[len(parts)-2]
	}
	return last
}

// pomArtifact is the project's own artifactId: the first one outside
// <parent>.
func pomArtifact(s string) string {
	if i := strings.Index(s, "</parent>"); i >= 0 {
		s = s[i:]
	}
	if m := pomID.FindStringSubmatch(s); m != nil {
		return m[1]
	}
	return ""
}

func jsonName(s string) string {
	var v struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal([]byte(s), &v)
	return v.Name
}

// nodeFrameworks are checked in order: meta-frameworks before the tools
// they are built on (Next.js uses React, SvelteKit uses Vite).
var nodeFrameworks = []struct{ dep, name string }{
	{"next", "Next.js"},
	{"nuxt", "Nuxt"},
	{"astro", "Astro"},
	{"@remix-run/dev", "Remix"},
	{"@react-router/dev", "React Router"},
	{"@sveltejs/kit", "SvelteKit"},
	{"@angular/core", "Angular"},
	{"gatsby", "Gatsby"},
	{"@docusaurus/core", "Docusaurus"},
	{"vitepress", "VitePress"},
	{"expo", "Expo"},
	{"electron", "Electron"},
	{"@nestjs/core", "NestJS"},
	{"vite", "Vite"},
	{"react-scripts", "Create React App"},
	{"webpack-dev-server", "webpack"},
	{"parcel", "Parcel"},
	{"express", "Express"},
	{"fastify", "Fastify"},
	{"hono", "Hono"},
	{"koa", "Koa"},
}

func nodeManifest(s, dir, base string) Info {
	var pkg struct {
		Name            string            `json:"name"`
		Dependencies    map[string]string `json:"dependencies"`
		DevDependencies map[string]string `json:"devDependencies"`
	}
	_ = json.Unmarshal([]byte(s), &pkg)
	info := Info{Name: or(pkg.Name, base), Root: dir, Language: "Node.js"}
	for _, f := range nodeFrameworks {
		_, dep := pkg.Dependencies[f.dep]
		_, dev := pkg.DevDependencies[f.dep]
		if dep || dev {
			info.Framework = f.name
			break
		}
	}
	return info
}

var pythonFrameworks = func() []struct {
	re   *regexp.Regexp
	name string
} {
	var out []struct {
		re   *regexp.Regexp
		name string
	}
	for _, c := range [][2]string{{"django", "Django"}, {"fastapi", "FastAPI"}, {"flask", "Flask"}, {"streamlit", "Streamlit"}, {"starlette", "Starlette"}, {"aiohttp", "aiohttp"}} {
		out = append(out, struct {
			re   *regexp.Regexp
			name string
		}{regexp.MustCompile(`(^|[^a-z0-9_-])` + c[0] + `([^a-z0-9_-]|$)`), c[1]})
	}
	return out
}()

func pythonFramework(s string) string {
	l := strings.ToLower(s)
	for _, c := range pythonFrameworks {
		if c.re.MatchString(l) {
			return c.name
		}
	}
	return ""
}

// pythonName prefers pyproject.toml's name for a Django project.
func pythonName(dir, base string) string {
	if s, ok := readFile(filepath.Join(dir, "pyproject.toml")); ok {
		if name := tomlValue(s, []string{"project", "tool.poetry"}, "name"); name != "" {
			return name
		}
	}
	return base
}

// tomlValue reads key = "value" from the first of tables that has it. It
// is enough for names in Cargo.toml and pyproject.toml, not a TOML parser.
func tomlValue(s string, tables []string, key string) string {
	for _, table := range tables {
		in := false
		sc := bufio.NewScanner(strings.NewReader(s))
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if strings.HasPrefix(line, "[") {
				in = line == "["+table+"]"
				continue
			}
			if !in {
				continue
			}
			k, v, ok := strings.Cut(line, "=")
			if ok && strings.TrimSpace(k) == key {
				v = strings.TrimSpace(v)
				if len(v) >= 2 && (v[0] == '"' || v[0] == '\'') {
					if end := strings.IndexByte(v[1:], v[0]); end >= 0 {
						return v[1 : end+1]
					}
				}
			}
		}
	}
	return ""
}

// depLine is true if a Cargo.toml lists dep as a dependency.
func depLine(s, dep string) bool {
	return regexp.MustCompile(`(?m)^\s*` + regexp.QuoteMeta(dep) + `\s*=`).MatchString(s)
}
