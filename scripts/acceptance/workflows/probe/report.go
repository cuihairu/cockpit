// Workflow 验收证据层（自 main.go 按职责拆分）：场景断言、输出聚合与证据
// 落盘（ev/check/fatal/truncate/saveEV + evFile/evMu/passes/fails 状态）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

var (
	evFile = (*os.File)(nil)
	evMu   sync.Mutex
	passes int
	fails  int
)

// ---------- 证据 ----------

func ev(format string, args ...interface{}) {
	evMu.Lock()
	defer evMu.Unlock()
	line := fmt.Sprintf(format, args...)
	fmt.Println(line)
	if evFile != nil {
		fmt.Fprintf(evFile, "%s\n", line)
	}
}

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		fails++
	} else {
		passes++
	}
	ev("[%s] %s — %s", status, name, detail)
}

func fatal(format string, args ...interface{}) {
	ev("[FATAL] "+format, args...)
	osExit(2)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// saveEV 场景原始响应落证据目录
func saveEV(name string, raw []byte) {
	if *evDir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(*evDir, name), raw, 0o644)
}
