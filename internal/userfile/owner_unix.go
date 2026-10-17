//go:build !windows

package userfile

import (
	"os"
	"strconv"
)

// giveToSudoUser chowns paths to the user who ran sudo. Lchown never
// follows a symlink, so a link planted in the path cannot redirect it.
func giveToSudoUser(paths []string) {
	uid, gid, ok := sudoUser(os.Geteuid(), os.Getenv)
	if !ok {
		return
	}
	for _, p := range paths {
		_ = os.Lchown(p, uid, gid)
	}
}

func sudoUser(euid int, getenv func(string) string) (uid, gid int, ok bool) {
	if euid != 0 {
		return 0, 0, false
	}
	uid, errU := strconv.Atoi(getenv("SUDO_UID"))
	gid, errG := strconv.Atoi(getenv("SUDO_GID"))
	if errU != nil || errG != nil || uid <= 0 {
		return 0, 0, false
	}
	return uid, gid, true
}
