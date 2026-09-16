package storage

import (
	"testing"
	"time"
)

// covNewRecording 构造一条录制元数据
func covNewRecording(sessionID string, startedAt time.Time) *TerminalRecording {
	return &TerminalRecording{
		SessionID: sessionID,
		Username:  "admin",
		AgentID:   "agent-1",
		Host:      "192.168.1.10",
		Port:      22,
		Protocol:  "ssh",
		StartedAt: startedAt,
	}
}

func TestCovTerminalRecordingLifecycle(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC().Add(-time.Minute)
	if err := db.CreateTerminalRecording(covNewRecording("sess-1", now)); err != nil {
		t.Fatalf("CreateTerminalRecording() error = %v", err)
	}
	if err := db.CreateTerminalRecording(covNewRecording("sess-2", now.Add(-2*time.Minute))); err != nil {
		t.Fatalf("CreateTerminalRecording() second error = %v", err)
	}

	if err := db.FinishTerminalRecording("sess-1", 53000, 2048); err != nil {
		t.Fatalf("FinishTerminalRecording() error = %v", err)
	}

	rec, err := db.GetTerminalRecording("sess-1")
	if err != nil {
		t.Fatalf("GetTerminalRecording() error = %v", err)
	}
	if rec.DurationMs != 53000 || rec.Bytes != 2048 {
		t.Errorf("DurationMs = %d, Bytes = %d, want 53000, 2048", rec.DurationMs, rec.Bytes)
	}
	if rec.Username != "admin" {
		t.Errorf("Username = %s, want admin", rec.Username)
	}

	// 不存在的会话
	if _, err := db.GetTerminalRecording("no-such-session"); err == nil {
		t.Error("GetTerminalRecording() should fail for missing session")
	}
}

func TestCovListTerminalRecordings(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC()
	for i, sid := range []string{"s1", "s2", "s3"} {
		if err := db.CreateTerminalRecording(covNewRecording(sid, now.Add(time.Duration(-i)*time.Minute))); err != nil {
			t.Fatalf("CreateTerminalRecording(%s) error = %v", sid, err)
		}
	}

	// 正常 limit
	list, err := db.ListTerminalRecordings(2)
	if err != nil {
		t.Fatalf("ListTerminalRecords() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("length = %d, want 2", len(list))
	}
	// 倒序：最新插入的 id 最大
	if list[0].SessionID != "s3" {
		t.Errorf("first = %s, want s3", list[0].SessionID)
	}

	// limit<=0 与 limit>500 都回退到 100
	for _, limit := range []int{0, -5, 501} {
		list, err := db.ListTerminalRecordings(limit)
		if err != nil {
			t.Fatalf("ListTerminalRecordings(%d) error = %v", limit, err)
		}
		if len(list) != 3 {
			t.Errorf("limit=%d length = %d, want 3", limit, len(list))
		}
	}
}

func TestCovDeleteTerminalRecording(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	if err := db.CreateTerminalRecording(covNewRecording("sess-del", time.Now().UTC())); err != nil {
		t.Fatalf("CreateTerminalRecording() error = %v", err)
	}
	if err := db.DeleteTerminalRecording("sess-del"); err != nil {
		t.Fatalf("DeleteTerminalRecording() error = %v", err)
	}
	if _, err := db.GetTerminalRecording("sess-del"); err == nil {
		t.Error("recording should be deleted")
	}
}

func TestCovListExpiredTerminalRecordings(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	if err := db.CreateTerminalRecording(covNewRecording("old-sess", old)); err != nil {
		t.Fatalf("CreateTerminalRecording() error = %v", err)
	}
	if err := db.CreateTerminalRecording(covNewRecording("new-sess", now)); err != nil {
		t.Fatalf("CreateTerminalRecording() error = %v", err)
	}

	expired, err := db.ListExpiredTerminalRecordings(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("ListExpiredTerminalRecordings() error = %v", err)
	}
	if len(expired) != 1 {
		t.Fatalf("length = %d, want 1", len(expired))
	}
	if expired[0].SessionID != "old-sess" {
		t.Errorf("SessionID = %s, want old-sess", expired[0].SessionID)
	}
}
