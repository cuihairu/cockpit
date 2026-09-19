//go:build !unix

package rpc

// fileStatOwner windows 无 unix 属主语义，恒 -1（UI 显示「—」）
func fileStatOwner(string) (int, int) { return -1, -1 }
