package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// newRecordingTestServer 带临时 DB 与 recordings 目录的测试 server
func newRecordingTestServer(t *testing.T) *Server {
	t.Helper()
	s := newBackupTestServer(t)
	s.cfg = &config.Config{
		Database: &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "test.db")},
	}
	return s
}

func castPath(s *Server, sessionID string) string {
	return filepath.Join(s.recordingsDir(), sessionID+".cast")
}

// TestCastRecorderFormat 文件格式：asciinema v2 header + [dt,"o",data] 行
func TestCastRecorderFormat(t *testing.T) {
	dir := t.TempDir()
	started := time.Now()
	rec, err := newCastRecorder(filepath.Join(dir, "s1.cast"), "s1", "cockpit u@h", started)
	if err != nil {
		t.Fatal(err)
	}
	rec.WriteOutput([]byte("hello "))
	rec.WriteOutput([]byte("world"))
	finishCalls := 0
	rec.finish = func(ms, b int64) {
		finishCalls++
		if b <= 0 {
			t.Errorf("bytes = %d, want > 0", b)
		}
	}
	rec.Close()
	rec.Close() // 幂等
	if finishCalls != 1 {
		t.Fatalf("finish calls = %d, want 1", finishCalls)
	}
	// nil receiver 安全（未开启录制的会话）
	var nilRec *castRecorder
	nilRec.WriteOutput([]byte("x"))
	nilRec.Close()

	f, err := os.Open(filepath.Join(dir, "s1.cast"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if len(lines) != 3 {
		t.Fatalf("lines = %d, want 3 (header + 2 events)", len(lines))
	}
	var header map[string]interface{}
	if err := json.Unmarshal([]byte(lines[0]), &header); err != nil {
		t.Fatalf("header not json: %v", err)
	}
	if header["version"] != float64(2) {
		t.Fatalf("version = %v, want 2", header["version"])
	}
	for i, want := range []string{"hello ", "world"} {
		var event []interface{}
		if err := json.Unmarshal([]byte(lines[i+1]), &event); err != nil {
			t.Fatalf("event %d not json: %v", i+1, err)
		}
		if len(event) != 3 || event[1] != "o" || event[2] != want {
			t.Fatalf("event %d = %v", i+1, event)
		}
		if dt, ok := event[0].(float64); !ok || dt < 0 {
			t.Fatalf("event %d dt = %v", i+1, event[0])
		}
	}
	// 权限：仅属主可读写
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("cast file mode = %v, want 0600", info.Mode().Perm())
	}
}

// TestStartRecordingAndPipeline startRecording 登记元数据 + WriteOutput 落盘 + Close 回填
func TestStartRecordingAndPipeline(t *testing.T) {
	s := newRecordingTestServer(t)
	session := &TerminalSession{
		ID: "sess-1", Username: "alice", AgentID: "a1",
		Protocol: protocol.RemoteProtocolSSH, Host: "10.0.0.5", Port: 22,
		CreatedAt: time.Now(),
	}
	rec := s.startRecording(session)
	if rec == nil {
		t.Fatal("startRecording returned nil")
	}
	session.recorder = rec
	session.recorder.WriteOutput([]byte("$ ls\n"))
	session.recorder.Close()

	meta, err := s.db.GetTerminalRecording("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if meta.Username != "alice" || meta.Protocol != "ssh" || meta.Host != "10.0.0.5" {
		t.Fatalf("meta = %+v", meta)
	}
	if meta.DurationMs < 0 || meta.Bytes <= 0 {
		t.Fatalf("duration/bytes = %d/%d", meta.DurationMs, meta.Bytes)
	}
	data, err := os.ReadFile(castPath(s, "sess-1"))
	if err != nil || !strings.Contains(string(data), "$ ls") {
		t.Fatalf("cast file: %v len=%d", err, len(data))
	}
}

// TestRecordingEnabledSetting 开关读取语义：默认开，"false" 关
func TestRecordingEnabledSetting(t *testing.T) {
	s := newRecordingTestServer(t)
	if !s.recordingEnabled() {
		t.Fatal("default should be enabled")
	}
	s.db.SetSetting(RecordingEnabledSettingKey, "false")
	if s.recordingEnabled() {
		t.Fatal("explicit false should disable")
	}
	// 关闭时 startRecording 不产出（接线处判断，这里验证语义护栏）
	s.db.SetSetting(RecordingRetentionSettingKey, "30")
	if got := s.recordingRetentionDays(); got != 30 {
		t.Fatalf("retention = %d, want 30", got)
	}
	s.db.SetSetting(RecordingRetentionSettingKey, "0")
	if got := s.recordingRetentionDays(); got != 0 {
		t.Fatalf("retention = %d, want 0 (永久)", got)
	}
	s.db.SetSetting(RecordingRetentionSettingKey, "abc")
	if got := s.recordingRetentionDays(); got != recordingDefaultRetentionDays {
		t.Fatalf("retention = %d, want default", got)
	}
}

// TestCleanupExpiredRecordings 过期清理：文件+记录同删；0=永久不删
func TestCleanupExpiredRecordings(t *testing.T) {
	s := newRecordingTestServer(t)
	old := &storage.TerminalRecording{
		SessionID: "old-1", Username: "bob", Protocol: "ssh",
		Host: "h", Port: 22, StartedAt: time.Now().AddDate(0, 0, -10),
	}
	if err := s.db.CreateTerminalRecording(old); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(castPath(s, "old-1"), []byte("{}\n"), 0600)

	s.cleanupExpiredRecordings()
	if _, err := s.db.GetTerminalRecording("old-1"); err == nil {
		t.Fatal("expired meta should be deleted")
	}
	if _, err := os.Stat(castPath(s, "old-1")); !os.IsNotExist(err) {
		t.Fatal("expired file should be deleted")
	}

	// 0 = 永久：不删
	fresh := &storage.TerminalRecording{
		SessionID: "keep-1", Username: "bob", Protocol: "ssh",
		Host: "h", Port: 22, StartedAt: time.Now().AddDate(0, 0, -30),
	}
	s.db.CreateTerminalRecording(fresh)
	os.WriteFile(castPath(s, "keep-1"), []byte("{}\n"), 0600)
	s.db.SetSetting(RecordingRetentionSettingKey, "0")
	s.cleanupExpiredRecordings()
	if _, err := s.db.GetTerminalRecording("keep-1"); err != nil {
		t.Fatal("retention=0 should keep forever")
	}
}

// TestRecordingsAPI 列表 / 取内容 / 删除 / 404
func TestRecordingsAPI(t *testing.T) {
	s := newRecordingTestServer(t)
	session := &TerminalSession{
		ID: "api-1", Username: "carol", AgentID: "a1",
		Protocol: protocol.RemoteProtocolSSH, Host: "10.0.0.9", Port: 22,
		CreatedAt: time.Now(),
	}
	rec := s.startRecording(session)
	rec.WriteOutput([]byte("payload"))
	rec.Close()

	// 列表
	r1 := httptest.NewRequest(http.MethodGet, "/recordings", nil)
	w1 := httptest.NewRecorder()
	s.handleRecordings(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("list code = %d body=%s", w1.Code, w1.Body.String())
	}
	if !strings.Contains(w1.Body.String(), "api-1") || !strings.Contains(w1.Body.String(), "carol") {
		t.Fatalf("list body = %s", w1.Body.String())
	}

	// 取内容
	r2 := httptest.NewRequest(http.MethodGet, "/recordings/api-1/cast", nil)
	w2 := httptest.NewRecorder()
	s.handleRecordings(w2, r2)
	if w2.Code != http.StatusOK || !strings.Contains(w2.Body.String(), "payload") {
		t.Fatalf("cast code = %d body = %s", w2.Code, w2.Body.String())
	}
	if ct := w2.Header().Get("Content-Type"); ct != "application/octet-stream" {
		t.Fatalf("content-type = %q", ct)
	}

	// 未知 404 / 错误方法 405
	r3 := httptest.NewRequest(http.MethodGet, "/recordings/nope/cast", nil)
	w3 := httptest.NewRecorder()
	s.handleRecordings(w3, r3)
	if w3.Code != http.StatusNotFound {
		t.Fatalf("missing cast code = %d, want 404", w3.Code)
	}
	r4 := httptest.NewRequest(http.MethodPost, "/recordings", nil)
	w4 := httptest.NewRecorder()
	s.handleRecordings(w4, r4)
	if w4.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST list code = %d, want 405", w4.Code)
	}

	// 删除：记录与文件同删
	r5 := httptest.NewRequest(http.MethodDelete, "/recordings/api-1", nil)
	w5 := httptest.NewRecorder()
	s.handleRecordings(w5, r5)
	if w5.Code != http.StatusNoContent {
		t.Fatalf("delete code = %d body=%s", w5.Code, w5.Body.String())
	}

	// 取内容/删除都记审计（内容访问可追溯，D10）
	logs, _, err := s.db.GetAuditLogs(0, 20, map[string]interface{}{"resource": "recording"})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, l := range logs {
		if l.ResourceID == "api-1" {
			actions[l.Action] = true
		}
	}
	if !actions["view"] || !actions["delete"] {
		t.Fatalf("audit actions = %v, want view+delete", actions)
	}
	if _, err := s.db.GetTerminalRecording("api-1"); err == nil {
		t.Fatal("meta should be deleted")
	}
	if _, err := os.Stat(castPath(s, "api-1")); !os.IsNotExist(err) {
		t.Fatal("file should be deleted")
	}
	r6 := httptest.NewRequest(http.MethodDelete, "/recordings/api-1", nil)
	w6 := httptest.NewRecorder()
	s.handleRecordings(w6, r6)
	if w6.Code != http.StatusNotFound {
		t.Fatalf("re-delete code = %d, want 404", w6.Code)
	}
}
