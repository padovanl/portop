package project

import "testing"

func TestRuntimeOf(t *testing.T) {
	for _, c := range []struct {
		f    Facts
		want string
	}{
		// ./serve with #!/usr/bin/env python3: the process is named after
		// the script, but the kernel runs the interpreter.
		{Facts{Name: "serve", Exe: "/usr/bin/python3.12", Cmdline: "python3 ./serve --port 8000"}, "Python"},
		{Facts{Name: "php", Exe: "/usr/bin/php8.3", Cmdline: "php -S 0.0.0.0:8080"}, "PHP"},
		{Facts{Name: "ruby", Exe: "/usr/bin/ruby3.2", Cmdline: "ruby app.rb"}, "Ruby"},
		{Facts{Name: "listen.sh", Exe: "/usr/bin/bash", Cmdline: "/bin/bash ./listen.sh"}, "Bash"},
		{Facts{Name: "python.exe", Exe: `C:\Python313\python.exe`, Cmdline: `"C:\Python313\python.exe" server.py`}, "Python"},
		{Facts{Name: "pwsh.exe", Exe: `C:\Program Files\PowerShell\7\pwsh.exe`, Cmdline: `pwsh -File listen.ps1`}, "PowerShell"},
		{Facts{Name: "main", Exe: "/tmp/go-build2615/b001/exe/main", Cmdline: "/tmp/go-build2615/b001/exe/main"}, "Go"},
		// The exe is unreadable (another user's process): the command line still tells.
		{Facts{Name: "serve", Cmdline: "/usr/local/bin/launcher ./serve.py"}, "Python"},
		{Facts{Name: "sshd", Exe: "/usr/sbin/sshd", Cmdline: "sshd: /usr/sbin/sshd -D"}, ""},
		{Facts{Name: "postgres", Exe: "/usr/lib/postgresql/17/bin/postgres", Cmdline: "postgres -D /var/lib/postgresql/17/main"}, ""},
	} {
		if got := runtimeOf(c.f); got != c.want {
			t.Errorf("runtimeOf(%+v) = %q, want %q", c.f, got, c.want)
		}
	}
}
