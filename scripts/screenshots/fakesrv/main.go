// fakesrv stands in for real services in documentation screenshots. It
// takes its settings from the environment, so its command line can look
// like the program it is copied as: SERVE_PORT, SERVE_BIND, SERVE_REPLY
// (bytes per reply), SERVE_WORKERS (child processes sharing the socket)
// and SERVE_CONNECT (host:port:bytes:seconds,... to keep busy).
package main

import (
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func serve(l net.Listener, reply int) {
	for {
		c, err := l.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			buf := make([]byte, 65536)
			out := make([]byte, reply)
			for {
				if _, err := c.Read(buf); err != nil {
					return
				}
				if _, err := c.Write(out); err != nil {
					return
				}
			}
		}(c)
	}
}

func client(target string) {
	p := strings.Split(target, ":")
	size, _ := strconv.Atoi(p[2])
	every, _ := strconv.ParseFloat(p[3], 64)
	buf := make([]byte, 1<<20)
	for {
		c, err := net.Dial("tcp", p[0]+":"+p[1])
		if err != nil {
			time.Sleep(500 * time.Millisecond)
			continue
		}
		for {
			if _, err := c.Write(make([]byte, size)); err != nil {
				break
			}
			if _, err := c.Read(buf); err != nil {
				break
			}
			time.Sleep(time.Duration(every * float64(time.Second)))
		}
		c.Close()
	}
}

func main() {
	reply, _ := strconv.Atoi(env("SERVE_REPLY", "20000"))
	var listeners []net.Listener
	if os.Getenv("SERVE_CHILD") != "" {
		// A worker: the listeners come as inherited descriptors.
		for i := 0; i < 2; i++ {
			if l, err := net.FileListener(os.NewFile(uintptr(3+i), "l")); err == nil {
				listeners = append(listeners, l)
			}
		}
	} else if port := os.Getenv("SERVE_PORT"); port != "" {
		bind := env("SERVE_BIND", "127.0.0.1")
		six := "::1"
		if bind == "0.0.0.0" {
			six = "::"
		}
		for _, a := range []string{bind, six} {
			network := "tcp4"
			if strings.Contains(a, ":") {
				network = "tcp6"
			}
			l, err := net.Listen(network, net.JoinHostPort(a, port))
			if err != nil {
				panic(err)
			}
			listeners = append(listeners, l)
		}
		if n, _ := strconv.Atoi(os.Getenv("SERVE_WORKERS")); n > 0 {
			var files []*os.File
			for _, l := range listeners {
				f, _ := l.(*net.TCPListener).File()
				files = append(files, f)
			}
			self, _ := os.Executable()
			for i := 0; i < n; i++ {
				cmd := exec.Command(self, os.Args[1:]...)
				cmd.Args[0] = os.Args[0]
				cmd.Env = append(os.Environ(), "SERVE_CHILD=1", "SERVE_CONNECT=")
				cmd.ExtraFiles = files
				_ = cmd.Start()
			}
		}
	}
	for _, t := range strings.Split(os.Getenv("SERVE_CONNECT"), ",") {
		if t != "" {
			go client(t)
		}
	}
	for _, l := range listeners {
		go serve(l, reply)
	}
	select {}
}
