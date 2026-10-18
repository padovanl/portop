//go:build !windows

package userfile

import "testing"

func TestSudoUser(t *testing.T) {
	env := func(uid, gid string) func(string) string {
		return func(k string) string {
			return map[string]string{"SUDO_UID": uid, "SUDO_GID": gid}[k]
		}
	}
	if uid, gid, ok := sudoUser(0, env("501", "20")); !ok || uid != 501 || gid != 20 {
		t.Fatalf("root under sudo: %d %d %v", uid, gid, ok)
	}
	if _, _, ok := sudoUser(1000, env("501", "20")); ok {
		t.Fatal("chowned although not running as root")
	}
	if _, _, ok := sudoUser(0, env("", "")); ok {
		t.Fatal("chowned for a root login without sudo")
	}
	if _, _, ok := sudoUser(0, env("0", "0")); ok {
		t.Fatal("chowned to root itself")
	}
}
