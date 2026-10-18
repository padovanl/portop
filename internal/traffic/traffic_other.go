//go:build !linux && !darwin

package traffic

import "errors"

func sample() (map[Key]Counter, error) {
	return nil, errors.New("traffic: per-connection counters are not available on this platform")
}
