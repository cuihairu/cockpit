//go:build unix

package rpc

import (
	"os"
	"syscall"
)

// fileStatOwner 取文件属主 uid/gid（Lstat 语义，不跟随 symlink）。
// Stat_t 为 unix 专属，windows 侧见 file_stat_windows.go（恒 -1）。
func fileStatOwner(path string) (int, int) {
	info, err := os.Lstat(path)
	if err != nil {
		return -1, -1
	}
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return int(st.Uid), int(st.Gid)
	}
	return -1, -1
}
