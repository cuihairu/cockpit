package server

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ 路由接线（serveAPI 分发） ============

// TestUpgradeServeAPIRoutes 经 serveAPI 路由：/upgrade 面与 /agents/{id}/upgrade。
func TestUpgradeServeAPIRoutes(t *testing.T) {
	s := newBackupTestServer(t)
	// GET /api/upgrade → 走 handleUpgradeAPI（空库 → 200 []）
	rec := httptest.NewRecorder()
	s.serveAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "[]") {
		t.Fatalf("route /upgrade: code = %d body = %s", rec.Code, rec.Body.String())
	}
	// POST /api/agents/a1/upgrade → 走 handleAgentUpgrade（201 planned）
	seedOnlineAgent(t, s, "a1", "1.0.0")
	_ = s.db.SaveArtifact(&storage.AgentArtifact{ID: "art-1", Version: "1.2.0", Arch: "amd64", OS: "linux"})
	rec = httptest.NewRecorder()
	s.serveAPI(rec, httptest.NewRequest(http.MethodPost, "/api/agents/a1/upgrade",
		strings.NewReader(`{"version":"1.2.0"}`)))
	if rec.Code != http.StatusCreated {
		t.Fatalf("route /agents/a1/upgrade: code = %d body = %s", rec.Code, rec.Body.String())
	}
}

// ============ 空库 / 方法守卫 / 空列表归一 ============

// TestUpgradeListEmptyAndMethods 空库归一化（nil → []）与 405 面。
func TestUpgradeListEmptyAndMethods(t *testing.T) {
	s := newBackupTestServer(t)

	// 意图列表：空库返回 []（而非 null）
	rec := httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade", nil), "/upgrade")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "[]") {
		t.Fatalf("empty list: code = %d body = %s", rec.Code, rec.Body.String())
	}
	// 非 GET → 405
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodPost, "/api/upgrade", nil), "/upgrade")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST list: code = %d (want 405)", rec.Code)
	}
	// 产物列表：空库返回 []
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade/artifacts", nil), "/upgrade/artifacts")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "[]") {
		t.Fatalf("empty artifacts: code = %d body = %s", rec.Code, rec.Body.String())
	}
	// 产物面非 GET/POST → 405
	for _, m := range []string{http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec = httptest.NewRecorder()
		s.handleUpgradeAPI(rec, httptest.NewRequest(m, "/api/upgrade/artifacts", nil), "/upgrade/artifacts")
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("artifacts %s: code = %d (want 405)", m, rec.Code)
		}
	}
	// 单条意图非 GET → 405
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodPut, "/api/upgrade/u1", nil), "/upgrade/u1")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("PUT intent: code = %d (want 405)", rec.Code)
	}
}

// ============ 库故障（读路径 500 面） ============

// TestUpgradeAPIClosedDB 库不可用时读路径返回 500（不误报 404/409）。
func TestUpgradeAPIClosedDB(t *testing.T) {
	s := newBackupTestServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade", nil), "/upgrade")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("list: code = %d (want 500)", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade/artifacts", nil), "/upgrade/artifacts")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("artifacts: code = %d (want 500)", rec.Code)
	}
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, httptest.NewRequest(http.MethodGet, "/api/upgrade/u1", nil), "/upgrade/u1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("intent: code = %d (want 500)", rec.Code)
	}
	// 升级入口：agent 加载失败（非 NotFound）→ 500
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/agents/a1/upgrade",
		strings.NewReader(`{"version":"1.2.0"}`))
	s.handleAgentUpgrade(rec, req, "a1")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("upgrade: code = %d (want 500)", rec.Code)
	}
	// 产物解析兜底（无 arch/os）走列表查询，库坏 → 503
	if _, err := s.resolveArtifact("1.0.0", "", ""); err == nil {
		t.Fatal("resolveArtifact on closed db: want error")
	}
	// 产物解析显式平台：GetArtifact 读失败 → error
	if _, err := s.resolveArtifact("1.0.0", "amd64", "linux"); err == nil {
		t.Fatal("resolveArtifact explicit on closed db: want error")
	}
	// 产物上传：表单解析通过后按平台查重，库坏 → 500
	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUpload("1.2.0", "amd64", "linux", "", []byte("x")), "/upgrade/artifacts")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("upload closed db: code = %d (want 500)", rec.Code)
	}
}

// ============ 上传畸形表单 ============

// multipartUploadNoFile 只带元数据字段、无 file 部分的上传请求。
func multipartUploadNoFile(version string) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("version", version)
	_ = mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/upgrade/artifacts", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// multipartUploadTruncated 截断 body 的上传请求：结尾闭合边界被切掉，
// ParseMultipartForm 读最后一个 part 时遇 unexpected EOF → 解析失败 → 413。
func multipartUploadTruncated(cut int) *http.Request {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("version", "1.2.0")
	fw, _ := mw.CreateFormFile("file", "agent.bin")
	_, _ = fw.Write([]byte("fake-agent-binary"))
	_ = mw.Close()
	body := buf.Bytes()[:buf.Len()-cut]
	req := httptest.NewRequest(http.MethodPost, "/api/upgrade/artifacts",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

// TestArtifactUploadFormErrors 缺 file 字段 → 400；表单解析失败（截断）→ 413。
func TestArtifactUploadFormErrors(t *testing.T) {
	s := newBackupTestServer(t)
	rec := httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUploadNoFile("1.2.0"), "/upgrade/artifacts")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing file: code = %d body = %s (want 400)", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	s.handleUpgradeAPI(rec, multipartUploadTruncated(8), "/upgrade/artifacts")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("truncated form: code = %d body = %s (want 413)", rec.Code, rec.Body.String())
	}
}

// ============ 平台精确匹配与 min_version 门控 ============

// TestAgentUpgradeExplicitPlatform 请求声明 arch/os：精确命中与未登记。
func TestAgentUpgradeExplicitPlatform(t *testing.T) {
	s := newBackupTestServer(t)
	seedOnlineAgent(t, s, "a1", "1.0.0")
	_ = s.db.SaveArtifact(&storage.AgentArtifact{ID: "art-1", Version: "1.2.0", Arch: "amd64", OS: "linux"})

	req := httptest.NewRequest(http.MethodPost, "/api/agents/a1/upgrade",
		strings.NewReader(`{"version":"1.2.0","arch":"amd64","os":"linux"}`))
	rec := httptest.NewRecorder()
	s.handleAgentUpgrade(rec, req, "a1")
	if rec.Code != http.StatusCreated {
		t.Fatalf("exact platform: code = %d body = %s", rec.Code, rec.Body.String())
	}
	// 同版本未登记的平台 → 503
	req = httptest.NewRequest(http.MethodPost, "/api/agents/a1/upgrade",
		strings.NewReader(`{"version":"1.2.0","arch":"riscv64","os":"linux"}`))
	rec = httptest.NewRecorder()
	s.handleAgentUpgrade(rec, req, "a1")
	if rec.Code != http.StatusServiceUnavailable ||
		!strings.Contains(rec.Body.String(), "riscv64") {
		t.Fatalf("missing platform: code = %d body = %s (want 503)", rec.Code, rec.Body.String())
	}
}

// TestAgentUpgradeMinVersionGate 产物声明的最低升级起点不满足 → 409。
func TestAgentUpgradeMinVersionGate(t *testing.T) {
	s := newBackupTestServer(t)
	seedOnlineAgent(t, s, "a1", "1.0.0")
	_ = s.db.SaveArtifact(&storage.AgentArtifact{ID: "art-1", Version: "1.4.0",
		Arch: "amd64", OS: "linux", MinVersion: "1.2.0"})

	req := httptest.NewRequest(http.MethodPost, "/api/agents/a1/upgrade",
		strings.NewReader(`{"version":"1.4.0"}`))
	rec := httptest.NewRecorder()
	s.handleAgentUpgrade(rec, req, "a1")
	if rec.Code != http.StatusConflict {
		t.Fatalf("min version gate: code = %d body = %s (want 409)", rec.Code, rec.Body.String())
	}
}

// ============ 请求上下文与 400 ============

// TestUpgradeRequestUsername 登录上下文解析与降级；坏 body → 400。
func TestUpgradeRequestUsername(t *testing.T) {
	if got := requestUsername(httptest.NewRequest(http.MethodGet, "/x", nil)); got != "unknown" {
		t.Fatalf("no context: username = %q (want unknown)", got)
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), "u1", "alice", "admin"))
	if got := requestUsername(req); got != "alice" {
		t.Fatalf("with context: username = %q (want alice)", got)
	}

	s := newBackupTestServer(t)
	rec := httptest.NewRecorder()
	s.handleAgentUpgrade(rec, httptest.NewRequest(http.MethodPost,
		"/api/agents/a1/upgrade", strings.NewReader(`{"version":`)), "a1")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad json: code = %d (want 400)", rec.Code)
	}
}

// ============ 巡检：保持 / 并发放弃 / 库故障 ============

// TestUpgradeReconcileHolds 窗口内等待 → 保持现状（不推进）。
func TestUpgradeReconcileHolds(t *testing.T) {
	s := newBackupTestServer(t)
	seedOnlineAgent(t, s, "a1", "1.0.0")
	now := time.Now()
	_ = s.db.CreateUpgrade(&storage.AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: storage.UpgradeStatusDeploying,
		DeployedAt: &now, CreatedAt: now, UpdatedAt: now})
	s.reconcileUpgrades()
	got, _ := s.db.GetUpgrade("u1")
	if got.Status != storage.UpgradeStatusDeploying || got.FinishedAt != nil {
		t.Fatalf("holds: status = %s finished = %v", got.Status, got.FinishedAt)
	}
}

// TestUpgradeReconcileStaleAdvance 意图已被并发推进（条件 UPDATE 命中 0）→ 放弃本轮。
func TestUpgradeReconcileStaleAdvance(t *testing.T) {
	s := newBackupTestServer(t)
	s.cfg = &config.Config{Upgrade: &config.UpgradeConfig{HealthWindowS: 1}}
	seedOnlineAgent(t, s, "a1", "1.2.0")
	old := time.Now().Add(-3 * time.Second)
	// 库里是 planned；调用方拿到的仍是 stale 的 verifying 快照
	_ = s.db.CreateUpgrade(&storage.AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: storage.UpgradeStatusPlanned,
		CreatedAt: old, UpdatedAt: old})
	u := &storage.AgentUpgrade{ID: "u1", AgentID: "a1", FromVersion: "1.0.0",
		ToVersion: "1.2.0", Status: storage.UpgradeStatusVerifying, VerifiedAt: &old}
	s.advanceUpgrade(u)
	got, _ := s.db.GetUpgrade("u1")
	if got.Status != storage.UpgradeStatusPlanned {
		t.Fatalf("stale advance: status = %s (want planned, untouched)", got.Status)
	}
}

// TestUpgradeReconcileDBErrors 库故障：列表失败与 agent 加载失败只记日志。
func TestUpgradeReconcileDBErrors(t *testing.T) {
	s := newBackupTestServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}
	s.reconcileUpgrades() // 列表失败 → 记日志返回
	u := &storage.AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: storage.UpgradeStatusPlanned}
	s.advanceUpgrade(u) // agent 加载失败（非 NotFound）→ 记日志返回
}

// TestUpgradeReconcileLoopTicks 巡检循环随 ticker 推进并随 ctx 退出。
func TestUpgradeReconcileLoopTicks(t *testing.T) {
	s := newBackupTestServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	interval := upgradeReconcileInterval
	upgradeReconcileInterval = 10 * time.Millisecond
	defer func() { upgradeReconcileInterval = interval }()

	now := time.Now()
	_ = s.db.CreateUpgrade(&storage.AgentUpgrade{ID: "u1", AgentID: "a1",
		FromVersion: "1.0.0", ToVersion: "1.2.0", Status: storage.UpgradeStatusPlanned,
		CreatedAt: now, UpdatedAt: now})

	done := make(chan struct{})
	go func() { s.upgradeReconcileLoop(); close(done) }()
	time.Sleep(80 * time.Millisecond)
	cancel()
	<-done

	got, _ := s.db.GetUpgrade("u1")
	if got.Status != storage.UpgradeStatusDeploying {
		t.Fatalf("loop: status = %s (want deploying)", got.Status)
	}
}
