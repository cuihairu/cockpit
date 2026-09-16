package audit

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/storage"
)

func covNewTestLogger(t *testing.T) *Logger {
	t.Helper()
	db, err := storage.Open(storage.Config{})
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return NewLogger(db)
}

func TestCovLogRemoteSession(t *testing.T) {
	logger := covNewTestLogger(t)

	details := &RemoteSessionDetails{
		Session:  "sess-cov-1",
		Protocol: "terminal",
		AgentID:  "agent-1",
		Host:     "192.168.1.20",
		Port:     22,
	}

	err := logger.LogRemoteSession(ActionRemoteStart, "user-cov", "covuser", StatusSuccess, "192.168.1.10", "cov-agent/1.0", details)
	if err != nil {
		t.Fatalf("LogRemoteSession(start) error = %v", err)
	}

	endDetails := &RemoteSessionDetails{
		Session:  "sess-cov-1",
		Protocol: "terminal",
		AgentID:  "agent-1",
		Duration: "2m0s",
		Reason:   "user closed",
	}
	err = logger.LogRemoteSession(ActionRemoteEnd, "user-cov", "covuser", StatusSuccess, "192.168.1.10", "cov-agent/1.0", endDetails)
	if err != nil {
		t.Fatalf("LogRemoteSession(end) error = %v", err)
	}

	// 注意：details 为 nil 时 LogRemoteSession 会因 details.Session 解引用而 panic
	// （疑似产品 bug，见最终报告），因此这里只测非 nil 的失败场景。
	failDetails := &RemoteSessionDetails{Session: "sess-cov-2", Protocol: "desktop"}
	err = logger.LogRemoteSession(ActionRemoteStart, "user-cov", "covuser", StatusFailure, "192.168.1.10", "cov-agent/1.0", failDetails)
	if err != nil {
		t.Fatalf("LogRemoteSession(failure) error = %v", err)
	}
}
