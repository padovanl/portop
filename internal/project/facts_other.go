//go:build !linux && !darwin && !windows

package project

func processFacts(pids []int) map[int]Facts { return nil }
