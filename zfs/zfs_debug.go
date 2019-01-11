package zfs

import "os"

var debugEnabled bool = false

func init() {
	if os.Getenv("ZREPL_ZFS_DEBUG") != "" {
		debugEnabled = true
	}
}
