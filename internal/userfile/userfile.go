// Package userfile writes portop's per-user files (config.yml, the
// baseline) so they stay usable by the user who owns them.
package userfile

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Write writes data to path, creating parent directories as needed.
//
// Under sudo the process runs as root but often keeps the invoking user's
// HOME (macOS always does), so the directories and file it creates would
// belong to root and the next unprivileged run could no longer save
// settings or a baseline. Whatever Write creates is handed back to the
// sudo user.
func Write(path string, data []byte) error {
	var created []string
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		if _, err := os.Lstat(dir); err == nil || !errors.Is(err, fs.ErrNotExist) {
			break
		}
		created = append(created, dir)
		if parent := filepath.Dir(dir); parent == dir {
			break
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, statErr := os.Lstat(path)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return err
	}
	if errors.Is(statErr, fs.ErrNotExist) {
		created = append(created, path)
	}
	handOver(created)
	return nil
}

// handOver is giveToSudoUser, replaceable in tests.
var handOver = giveToSudoUser
