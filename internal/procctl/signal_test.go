package procctl

import (
	"os/exec"
	"testing"
	"time"
)

func TestTerminateStopsRealProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start test subprocess: %v", err)
	}
	pid := cmd.Process.Pid

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if err := Terminate(pid); err != nil {
		t.Fatalf("Terminate: %v", err)
	}

	select {
	case <-done:
		// exited, as expected
	case <-time.After(3 * time.Second):
		_ = Kill(pid)
		t.Fatal("process did not exit within 3s of Terminate")
	}
}

func TestKillStopsRealProcess(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot start test subprocess: %v", err)
	}
	pid := cmd.Process.Pid

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if err := Kill(pid); err != nil {
		t.Fatalf("Kill: %v", err)
	}

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("process did not exit within 3s of Kill")
	}
}
