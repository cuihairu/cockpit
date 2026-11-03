package docker

import (
	"net/http"
	"strings"
	"testing"
)

// TestG6GetLogsReadsAllChunks 验证 G6：GetLogs 必须循环读到 EOF，单次 Read
// 返回部分数据时不得静默截断。
func TestG6GetLogsReadsAllChunks(t *testing.T) {
	// 返回体大于单次 Read 的缓冲区容量，但小于 1MB 上限：旧实现首读即截断。
	body := strings.Repeat("x", 200*1024) + "tail-marker"
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	logs, err := c.GetLogs("g6-c", "", "", false, false, true, true)
	if err != nil {
		t.Fatalf("GetLogs() error = %v", err)
	}
	if !strings.HasSuffix(logs, "tail-marker") {
		t.Fatalf("logs truncated: last 64 = %q", logs[len(logs)-64:])
	}
	if len(logs) != len(body) {
		t.Fatalf("len(logs) = %d, want %d", len(logs), len(body))
	}
}

// TestG6GetLogsTruncatesAt1MB 验证 G6：超 1MB 时截断并附标记，不无界增长。
func TestG6GetLogsTruncatesAt1MB(t *testing.T) {
	body := strings.Repeat("y", 2*1024*1024) // 2MB > 1MB 上限
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	logs, err := c.GetLogs("g6-c", "", "", false, false, true, true)
	if err != nil {
		t.Fatalf("GetLogs() error = %v", err)
	}
	if !strings.Contains(logs, "[logs truncated at 1MB]") {
		t.Fatal("expected truncation marker")
	}
	if len(logs) > 1024*1024+64 {
		t.Fatalf("logs not bounded: len = %d", len(logs))
	}
}
