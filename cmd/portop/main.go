// Command portop is a terminal UI showing which processes are really
// using your TCP/UDP ports — like htop, but for ports.
package main

import (
	"os"

	"github.com/padovanl/portop/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
