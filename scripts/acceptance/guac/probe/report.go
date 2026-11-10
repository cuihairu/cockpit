// 证据与断言（自 main.go 按职责拆分）：事件流 ev/check/fatal 与输出聚合
// truncate/marshalShort/auditSample。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

var (
	evFile *os.File
	evMu   sync.Mutex
	fails  int
	passes int
)

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

func marshalShort(v interface{}) string {
	b, _ := json.Marshal(v)
	return truncate(string(b), 300)
}

func auditSample() string {
	_, b := api("GET", "/api/admin/audit/logs?action=remote_start&page_size=1", nil)
	return truncate(string(b), 400)
}
