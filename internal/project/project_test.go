package project

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDetectReadsManifests(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  Info // Root is checked separately
	}{
		{"next", map[string]string{"package.json": `{"name":"storefront","dependencies":{"next":"15","react":"19"}}`},
			Info{Name: "storefront", Language: "Node.js", Framework: "Next.js"}},
		{"vite dev dependency", map[string]string{"package.json": `{"name":"@acme/ui","devDependencies":{"vite":"7"}}`},
			Info{Name: "@acme/ui", Language: "Node.js", Framework: "Vite"}},
		{"sveltekit before vite", map[string]string{"package.json": `{"devDependencies":{"vite":"7","@sveltejs/kit":"2"}}`},
			Info{Name: "sveltekit before vite", Language: "Node.js", Framework: "SvelteKit"}},
		{"go module", map[string]string{"go.mod": "module github.com/acme/billing/v2\n\ngo 1.26\n"},
			Info{Name: "billing", Language: "Go"}},
		{"cargo", map[string]string{"Cargo.toml": "[package]\nname = \"ledger\"\n\n[dependencies]\naxum = \"0.8\"\n"},
			Info{Name: "ledger", Language: "Rust", Framework: "Axum"}},
		{"poetry", map[string]string{"pyproject.toml": "[tool.poetry]\nname = \"inventory\"\n[tool.poetry.dependencies]\nFastAPI = \"*\"\n"},
			Info{Name: "inventory", Language: "Python", Framework: "FastAPI"}},
		{"django wins over assets", map[string]string{"manage.py": "import django\n", "package.json": `{"name":"assets"}`, "pyproject.toml": "[project]\nname = \"crm\"\n"},
			Info{Name: "crm", Language: "Python", Framework: "Django"}},
		{"flask-cors is not flask", map[string]string{"requirements.txt": "flask-cors==4\n"},
			Info{Name: "flask-cors is not flask", Language: "Python"}},
		{"rails", map[string]string{"Gemfile": "gem 'rails', '~> 8.0'\n"},
			Info{Name: "rails", Language: "Ruby", Framework: "Rails"}},
		{"laravel", map[string]string{"composer.json": `{"name":"acme/portal","require":{"laravel/framework":"^12"}}`},
			Info{Name: "portal", Language: "PHP", Framework: "Laravel"}},
		{"spring", map[string]string{"pom.xml": "<project><parent><artifactId>spring-boot-starter-parent</artifactId></parent><artifactId>orders</artifactId></project>"},
			Info{Name: "orders", Language: "Java", Framework: "Spring Boot"}},
		{"gradle kotlin", map[string]string{"build.gradle.kts": "plugins { id(\"io.ktor.plugin\") }", "settings.gradle.kts": "rootProject.name = \"chat\""},
			Info{Name: "chat", Language: "Kotlin", Framework: "Ktor"}},
		{"aspnet", map[string]string{"Api.csproj": `<Project Sdk="Microsoft.NET.Sdk.Web"></Project>`},
			Info{Name: "Api", Language: ".NET", Framework: "ASP.NET Core"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), c.name)
			for name, content := range c.files {
				write(t, root, name, content)
			}
			sub := filepath.Join(root, "src", "server")
			if err := os.MkdirAll(sub, 0o755); err != nil {
				t.Fatal(err)
			}
			got := Detect(Facts{Cwd: sub})
			if got.Root != root {
				t.Errorf("Root = %q, want %q", got.Root, root)
			}
			got.Root = ""
			if got != c.want {
				t.Errorf("Detect = %+v, want %+v", got, c.want)
			}
		})
	}
}

func TestDetectFromScriptPath(t *testing.T) {
	root := filepath.Join(t.TempDir(), "api")
	write(t, root, "package.json", `{"name":"api","dependencies":{"express":"5"}}`)
	write(t, root, "node_modules/vite/bin/vite.js", "")
	write(t, root, "server.js", "")

	// Started from / (a service): the script's directory gives the project.
	got := Detect(Facts{Cwd: "/", Cmdline: "/usr/bin/node " + filepath.Join(root, "server.js"), Exe: "/usr/bin/node"})
	if got.Name != "api" || got.Framework != "Express" {
		t.Errorf("script path: %+v", got)
	}
	// A tool inside node_modules belongs to the project above it, and the
	// command line names the framework.
	got = Detect(Facts{Cmdline: "node " + filepath.Join(root, "node_modules/vite/bin/vite.js") + " --port 5173"})
	if got.Name != "api" || got.Framework != "Vite" || got.Root != root {
		t.Errorf("node_modules tool: %+v", got)
	}
}

func TestDetectIgnoresToolsAndSystemDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	ext := filepath.Join(home, ".vscode-server", "extensions", "pylance")
	write(t, ext, "package.json", `{"name":"vscode-pylance"}`)
	write(t, home, "package.json", `{"name":"dotfiles"}`)

	for _, f := range []Facts{
		{Cwd: ext},
		{Cmdline: "node " + filepath.Join(ext, "server.js")},
		{Cwd: home},
		{Cwd: "/"},
		{Cwd: "/usr/lib/postgresql"},
	} {
		// The language of the interpreter is still known; the project is not.
		if got := Detect(f); got.Name != "" || got.Root != "" || got.Framework != "" {
			t.Errorf("Detect(%+v) = %+v, want no project", f, got)
		}
	}
}

func TestFrameworkFromCmdline(t *testing.T) {
	cases := map[string]string{
		"node /srv/app/node_modules/.bin/vite --host":                                         "Vite",
		"node /srv/app/node_modules/next/dist/bin/next dev":                                   "Next.js",
		`"C:\Program Files\nodejs\node.exe" C:\app\node_modules\@angular\cli\bin\ng.js serve`: "Angular",
		"node /app/node_modules/@nestjs/cli/bin/nest.js start":                                "NestJS",
		"python3 manage.py runserver 0.0.0.0:8000":                                            "Django",
		"/home/u/.venv/bin/python -m uvicorn main:app --reload":                               "Uvicorn",
		"/usr/bin/python3 -m http.server 8000":                                                "Python http.server",
		"php artisan serve":                                                                   "Laravel",
		"bundle exec rails server":                                                            "Rails",
		"hugo server -D":                                                                      "Hugo",
		"npx storybook dev -p 6006":                                                           "Storybook",
		"vim next.config.js":                                                                  "",
		"/usr/sbin/nginx -g daemon off;":                                                      "",
		"node server.js":                                                                      "",
		"grep -r vite .":                                                                      "",
	}
	for cmd, want := range cases {
		if got := frameworkFromCmdline(cmd); got != want {
			t.Errorf("frameworkFromCmdline(%q) = %q, want %q", cmd, got, want)
		}
	}
}

func TestCategory(t *testing.T) {
	dev := Info{Root: "/home/u/app", Framework: "Vite"}
	cases := []struct {
		info                           Info
		process, user, container, unit string
		want                           string
	}{
		{Info{}, "postgres", "postgres", "", "postgresql@17-main.service", CategoryDatabase},
		{Info{}, "redis-server.exe", "u", "", "", CategoryDatabase},
		{Info{}, "nginx", "root", "", "nginx.service", CategoryWeb},
		{Info{}, "docker-proxy", "root", "", "docker.service", CategoryContainer},
		{Info{}, "node", "u", "web-1", "", CategoryContainer},
		{dev, "node", "u", "", "session-2.scope", CategoryDev},
		{Info{}, "sshd", "root", "", "ssh.service", CategorySystem},
		{Info{}, "svchost", "NT AUTHORITY\\SYSTEM", "", "Dnscache", CategorySystem},
		{Info{}, "rapportd", "_rapport", "", "", CategorySystem},
		{Info{}, "spotify", "u", "", "app-spotify.scope", CategoryApp},
		{Info{}, "Code", "u", "", "", CategoryApp},
		{Info{Language: "Python"}, "serve", "root", "", "session-2.scope", CategoryDev},
		{Info{Language: "PHP"}, "php", "u", "", "", CategoryDev},
		{Info{Language: "Python"}, "fail2ban-server", "root", "", "fail2ban.service", CategorySystem},
	}
	for _, c := range cases {
		if got := Category(c.info, c.process, c.user, c.container, c.unit); got != c.want {
			t.Errorf("Category(%q, %q, %q) = %q, want %q", c.process, c.user, c.unit, got, c.want)
		}
	}
}

func TestResolverCachesAndForgets(t *testing.T) {
	root := filepath.Join(t.TempDir(), "web")
	write(t, root, "go.mod", "module example.com/web\n")
	calls := 0
	r := &Resolver{cache: map[int]Info{}, facts: func(pids []int) map[int]Facts {
		calls++
		out := map[int]Facts{}
		for _, pid := range pids {
			if pid == 10 {
				out[pid] = Facts{Cwd: root}
			}
		}
		return out
	}}
	got := r.Resolve([]int{10, 10, 11, 0})
	if got[10].Name != "web" || got[11] != (Info{}) || calls != 1 {
		t.Fatalf("first resolve: %+v (calls %d)", got, calls)
	}
	r.Resolve([]int{10, 11})
	if calls != 1 {
		t.Errorf("known PIDs were looked up again (%d calls)", calls)
	}
	r.Resolve([]int{11})
	if _, ok := r.cache[10]; ok {
		t.Error("a PID that is gone is still cached")
	}
}

func TestParseDarwinOutput(t *testing.T) {
	cmds := parsePSCommands("  412 /usr/local/bin/node /Users/u/app/node_modules/.bin/vite\n 98 redis-server *:6379\n")
	if cmds[412] != "/usr/local/bin/node /Users/u/app/node_modules/.bin/vite" || cmds[98] != "redis-server *:6379" {
		t.Errorf("parsePSCommands = %v", cmds)
	}
	cwds := parseLsofCwd("p412\nfcwd\nn/Users/u/app\np98\nfcwd\nn/\n")
	if cwds[412] != "/Users/u/app" || cwds[98] != "/" {
		t.Errorf("parseLsofCwd = %v", cwds)
	}
}

func TestLinuxFactsDropADeletedWorkingDirectory(t *testing.T) {
	if procRootForTest == nil {
		t.Skip("Linux only")
	}
	root := t.TempDir()
	pidDir := filepath.Join(root, "42")
	if err := os.MkdirAll(pidDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write(t, pidDir, "cmdline", "node\x00server.js\x00")
	write(t, pidDir, "comm", "node\n")
	if err := os.Symlink("/srv/app (deleted)", filepath.Join(pidDir, "cwd")); err != nil {
		t.Fatal(err)
	}
	old := *procRootForTest
	*procRootForTest = root
	t.Cleanup(func() { *procRootForTest = old })
	got := processFacts([]int{42})[42]
	if got.Cwd != "" || got.Cmdline != "node server.js" || got.Name != "node" {
		t.Errorf("facts = %+v", got)
	}
}

func TestAppDataInstallsAreNotProjects(t *testing.T) {
	cases := map[string]bool{
		"C:/Users/u/AppData/Local/Programs/Microsoft VS Code/resources/app": true,
		"C:/Users/u/AppData/Roaming/npm/node_modules/serve":                 true,
		"C:/Users/u/AppData/Local/Temp/shop":                                false,
		"C:/Users/u/source/repos/shop":                                      false,
	}
	for dir, dropped := range cases {
		if got := trimDependencyDir(filepath.FromSlash(dir)) == ""; got != dropped {
			t.Errorf("trimDependencyDir(%q) dropped = %v, want %v", dir, got, dropped)
		}
	}
}
