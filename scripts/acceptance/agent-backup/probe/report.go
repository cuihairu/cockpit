// 证据层（自 main.go 按职责拆分）：事件流 ev/check/fatal、证据落盘 saveEV
// 与退出注入点 osExit。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	evMu   sync.Mutex
	evF    *os.File
	passes int
	fails  int
	osExit = os.Exit
)

// ---------- 证据层 ----------

func ev(format string, args ...interface{}) {
	evMu.Lock()
	defer evMu.Unlock()
	line := fmt.Sprintf(format, args...)
	if evF != nil {
		fmt.Fprintln(evF, line)
	}
	fmt.Println(line)
}

func check(name string, ok bool, detail string) {
	evMu.Lock()
	if ok {
		passes++
	} else {
		fails++
	}
	evMu.Unlock()
	if ok {
		ev("[PASS] %s — %s", name, detail)
	} else {
		ev("[FAIL] %s — %s", name, detail)
	}
}

func fatal(code int, format string, args ...interface{}) {
	ev("[FATAL] "+format, args...)
	osExit(code)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func saveEV(name string, b []byte) {
	evMu.Lock()
	dir := *evDir
	evMu.Unlock()
	if dir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name), b, 0o600)
}
