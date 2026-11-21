package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ 产物登记 ============

func sha256hex(b []byte) string {
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:])
}

// multipartUpload 构造 multipart 产物上传请求。
func multipartUpload(version, arch, osName, minVersion string, content []byte) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("version", version)
	_ = mw.WriteField("arch", arch)
	_ = mw.WriteField("os", osName)
	_ = mw.WriteField("min_version", minVersion)
	fw, _ := mw.CreateFormFile("file", "agent.bin")
	_, _ = fw.Write(content)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/upgrade/artifacts", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// TestArtifactUpload 上传产物：201，落库带计算出的 sha256/大小。
func TestArtifactUpload(t *testing.T) {
	s := newBackupTestServer(t)
	content := []byte("fake-agent-binary")
	req := multipartUpload("1.2.0", "amd64", "linux", "", content)
	rec := httptest.NewRecorder()
	s.handleUpgradeAPI(rec, req, "/upgrade/artifacts")
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload: code = %d body = %s", rec.Code, rec.Body.String())
	}
	var art storage.AgentArtifact
	_ = json.Unmarshal(rec.Body.Bytes(), &art)
	if art.SHA256 != sha256hex(content) || art.Size != int64(len(content)) {
		t.Fatalf("artifact = %+v (want sha=%s size=%d)", art, sha256hex(content), len(content))
	}
	got, err := s.db.GetArtifact("1.2.0", "amd64", "linux")
	if err != nil || got == nil {
		t.Fatalf("persisted = %+v err %v", got, err)
	}
}

// TestArtifactUploadReject 校验拒绝：空文件/坏版本/超上界。
func TestArtifactUploadReject(t *testing.T) {
	s := newBackupTestServer(t)
	// 空文件（size=0）
	rec := httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUpload("1.2.0", "amd64", "linux", "", nil), "/upgrade/artifacts")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty file: code = %d", rec.Code)
	}
	// 坏版本
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUpload("bad", "amd64", "linux", "", []byte("x")), "/upgrade/artifacts")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad version: code = %d", rec.Code)
	}
	// 超上界（max=0 默认 256MB；用配置把上界压小）
	s.cfg = &config.Config{Upgrade: &config.UpgradeConfig{MaxArtifactMB: 1}}
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUpload("1.2.0", "amd64", "linux", "", make([]byte, 2<<20)), "/upgrade/artifacts")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("over bound: code = %d", rec.Code)
	}
}

// TestArtifactUploadIdempotent 同 version+arch+os 覆盖更新（同 id）。
func TestArtifactUploadIdempotent(t *testing.T) {
	s := newBackupTestServer(t)
	rec := httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUpload("1.2.0", "arm64", "linux", "", []byte("a")), "/upgrade/artifacts")
	var first storage.AgentArtifact
	_ = json.Unmarshal(rec.Body.Bytes(), &first)

	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUpload("1.2.0", "arm64", "linux", "", []byte("b")), "/upgrade/artifacts")
	var second storage.AgentArtifact
	_ = json.Unmarshal(rec.Body.Bytes(), &second)
	if second.ID != first.ID || second.SHA256 == first.SHA256 {
		t.Fatalf("idempotent overwrite: first=%s second=%s", first.ID, second.ID)
	}
}

// TestArtifactList 产物列表端点。
func TestArtifactList(t *testing.T) {
	s := newBackupTestServer(t)
	_ = s.db.SaveArtifact(&storage.AgentArtifact{ID: "a", Version: "1.0.0", Arch: "amd64", OS: "linux"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/upgrade/artifacts", nil)
	s.handleUpgradeAPI(rec, req, "/upgrade/artifacts")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "1.0.0") {
		t.Fatalf("list: code = %d body = %s", rec.Code, rec.Body.String())
	}
}

// ============ 升级意图 ============

// seedOnlineAgent 落一个在线 agent 行。
func seedOnlineAgent(t *testing.T, s *Server, id, version string, caps ...storage.Capability) {
	t.Helper()
	if err := s.db.UpsertAgent(&storage.Agent{ID: id, Hostname: id, Status: "online",
		Version: version, Capabilities: caps}); err != nil {
		t.Fatalf("seed agent: %v", err)
	}
}

// TestAgentUpgradeCreate 单 agent 显式升级：201 planned + 审计。
func TestAgentUpgradeCreate(t *testing.T) {
	s := newBackupTestServer(t)
	seedOnlineAgent(t, s, "a1", "1.0.0", storage.Capability{Type: "backup"})
	_ = s.db.SaveArtifact(&storage.AgentArtifact{ID: "art-1", Version: "1.2.0", Arch: "amd64", OS: "linux"})

	req := httptest.NewRequest(http.MethodPost, "/api/agents/a1/upgrade",
		strings.NewReader(`{"version":"1.2.0"}`))
	rec := httptest.NewRecorder()
	s.handleAgentUpgrade(rec, req, "a1")
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: code = %d body = %s", rec.Code, rec.Body.String())
	}
	var intent storage.AgentUpgrade
	_ = json.Unmarshal(rec.Body.Bytes(), &intent)
	if intent.Status != storage.UpgradeStatusPlanned || intent.ToVersion != "1.2.0" ||
		intent.FromVersion != "1.0.0" {
		t.Fatalf("intent = %+v", intent)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionAgentUpgrade})
	if len(logs) == 0 {
		t.Fatal("upgrade audit row missing")
	}
}

// TestAgentUpgradeReject 拒绝面：降级/离线/未知产物/未知 agent/空版本/GET。
func TestAgentUpgradeReject(t *testing.T) {
	s := newBackupTestServer(t)
	seedOnlineAgent(t, s, "a1", "1.2.0") // 当前 1.2.0
	seedOnlineAgent(t, s, "off", "1.0.0")
	if err := s.db.UpdateAgentStatus("off", "offline", time.Now()); err != nil {
		t.Fatalf("set offline: %v", err)
	}
	_ = s.db.SaveArtifact(&storage.AgentArtifact{ID: "art-1", Version: "1.4.0", Arch: "amd64", OS: "linux"})

	post := func(agentID, body string) int {
		req := httptest.NewRequest(http.MethodPost, "/api/agents/"+agentID+"/upgrade", strings.NewReader(body))
		rec := httptest.NewRecorder()
		s.handleAgentUpgrade(rec, req, agentID)
		return rec.Code
	}
	// 降级（1.2.0 → 1.4.0 可以，先测降级：目标 1.0.0 < 当前 1.2.0）
	if c := post("a1", `{"version":"1.0.0"}`); c != http.StatusConflict {
		t.Fatalf("downgrade: code = %d (want 409)", c)
	}
	// 同级重升
	if c := post("a1", `{"version":"1.2.0"}`); c != http.StatusConflict {
		t.Fatalf("same version: code = %d (want 409)", c)
	}
	// 离线 agent
	if c := post("off", `{"version":"1.4.0"}`); c != http.StatusConflict {
		t.Fatalf("offline: code = %d (want 409)", c)
	}
	// 未知产物（1.9.0 未登记）
	if c := post("a1", `{"version":"1.9.0"}`); c != http.StatusServiceUnavailable {
		t.Fatalf("missing artifact: code = %d (want 503)", c)
	}
	// 未知 agent
	if c := post("nope", `{"version":"1.4.0"}`); c != http.StatusNotFound {
		t.Fatalf("missing agent: code = %d (want 404)", c)
	}
	// 空版本
	if c := post("a1", `{}`); c != http.StatusBadRequest {
		t.Fatalf("empty version: code = %d (want 400)", c)
	}
	// GET 方法 405
	req := httptest.NewRequest(http.MethodGet, "/api/agents/a1/upgrade", nil)
	rec := httptest.NewRecorder()
	s.handleAgentUpgrade(rec, req, "a1")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET: code = %d (want 405)", rec.Code)
	}
}

// TestUpgradeListGet 意图列表与单条查询。
func TestUpgradeListGet(t *testing.T) {
	s := newBackupTestServer(t)
	now := time.Now()
	_ = s.db.CreateUpgrade(&storage.AgentUpgrade{ID: "u1", AgentID: "a1", Status: storage.UpgradeStatusDeploying, CreatedAt: now})

	rec := httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade", nil), "/upgrade")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "u1") {
		t.Fatalf("list: code = %d body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade/u1", nil), "/upgrade/u1")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "u1") {
		t.Fatalf("get: code = %d body = %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade/nope", nil), "/upgrade/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("get missing: code = %d (want 404)", rec.Code)
	}
}

// ============ 巡检推进 ============

// TestUpgradeReconcilePlannedToDeploying 巡检把 planned 武装为 deploying。
func TestUpgradeReconcilePlannedToDeploying(t *testing.T) {
	s := newBackupTestServer(t)
	seedOnlineAgent(t, s, "a1", "1.0.0")
	now := time.Now()
	_ = s.db.CreateUpgrade(&storage.AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: storage.UpgradeStatusPlanned,
		CreatedAt: now, UpdatedAt: now})
	s.reconcileUpgrades()
	got, _ := s.db.GetUpgrade("u1")
	if got.Status != storage.UpgradeStatusDeploying {
		t.Fatalf("reconcile planned: status = %s (want deploying)", got.Status)
	}
}

// TestUpgradeReconcileRollback 部署窗口过期且 agent 仍在原版本 → rolled_back。
func TestUpgradeReconcileRollback(t *testing.T) {
	s := newBackupTestServer(t)
	s.cfg = &config.Config{Upgrade: &config.UpgradeConfig{HealthWindowS: 1}}
	seedOnlineAgent(t, s, "a1", "1.0.0") // 仍在原版本
	old := time.Now().Add(-3 * time.Second)
	_ = s.db.CreateUpgrade(&storage.AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: storage.UpgradeStatusDeploying,
		DeployedAt: &old, CreatedAt: old, UpdatedAt: old})
	s.reconcileUpgrades()
	got, _ := s.db.GetUpgrade("u1")
	if got.Status != storage.UpgradeStatusRolledBack {
		t.Fatalf("reconcile rollback: status = %s (want rolled_back), reason = %s", got.Status, got.Reason)
	}
}

// TestUpgradeReconcileSucceeded 验证窗稳定在目标版本 → succeeded。
func TestUpgradeReconcileSucceeded(t *testing.T) {
	s := newBackupTestServer(t)
	s.cfg = &config.Config{Upgrade: &config.UpgradeConfig{HealthWindowS: 1}}
	seedOnlineAgent(t, s, "a1", "1.2.0") // 已升到目标
	old := time.Now().Add(-3 * time.Second)
	_ = s.db.CreateUpgrade(&storage.AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: storage.UpgradeStatusVerifying,
		DeployedAt: &old, VerifiedAt: &old, CreatedAt: old, UpdatedAt: old})
	s.reconcileUpgrades()
	got, _ := s.db.GetUpgrade("u1")
	if got.Status != storage.UpgradeStatusSucceeded {
		t.Fatalf("reconcile succeeded: status = %s (want succeeded), reason = %s", got.Status, got.Reason)
	}
}
