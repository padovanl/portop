// Command portop shows the processes using TCP and UDP ports.
package main

import (
	"os"

	"github.com/padovanl/portop/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:], os.Stdout, os.Stderr))
}
