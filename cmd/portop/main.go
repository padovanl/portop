// GOPATH builds do not read go.mod's Go version; the web routes require
// the method-aware ServeMux introduced in Go 1.22.
//go:debug httpmuxgo121=0

// Command portop shows the processes using TCP and UDP ports.
package main

import (
	"os"

	"github.com/padovanl/portop/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
