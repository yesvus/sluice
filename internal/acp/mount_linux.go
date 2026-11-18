//go:build linux

package acp

import (
	"os"
	"syscall"
)

func readOnlyMount(file *os.File) bool {
	var stat syscall.Statfs_t
	// Linux statfs f_flags ST_RDONLY is 0x0001. Check the opened object,
	// not an independently resolved path that could name another mount.
	return syscall.Fstatfs(int(file.Fd()), &stat) == nil && stat.Flags&1 != 0
}
