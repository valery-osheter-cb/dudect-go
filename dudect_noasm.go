//go:build (!arm64 && !amd64) || purego

package dudect

import "time"

func cpuTicks() uint64 {
	return uint64(time.Now().UnixNano())
}
