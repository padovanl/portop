package services

import (
	"os"
	"path/filepath"
)

// Windows ships the same services database format under the system root.
func init() {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = `C:\Windows`
	}
	Path = filepath.Join(root, "System32", "drivers", "etc", "services")
}
