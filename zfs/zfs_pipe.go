// +build !linux

package zfs

import (
	"fmt"
	"os"
	"sync"
)

var zfsPipeCapacityNotSupported sync.Once

func trySetPipeCapacity(p *os.File, capacity int) {
	if debugEnabled {
		zfsPipeCapacityNotSupported.Do(func() {
			fmt.Fprintf(os.Stderr, "zfs: trySetPipeCapacity error: OS does not support setting pipe capacity\n")
		})
	}
}
