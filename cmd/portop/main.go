// GOPATH builds (dh-golang in Debian) do not read the Go version in
// go.mod and fall back to old defaults, such as the Go 1.21 ServeMux,
// which does not match the web routes, and SHA-1 in TLS; keep the
// defaults of the Go version in go.mod.
//go:debug default=go1.26

// Command portop shows the processes using TCP and UDP ports.
package main

import (
	"os"

	"github.com/padovanl/portop/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
