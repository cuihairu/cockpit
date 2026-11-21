package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// ============ 执行窗口 / 去重键 纯函数 ============

// TestWithinJobWindowTable 覆盖窗口判定：nil 放行、窗内/窗外、跨午夜、
// 星期过滤（名称与数字 1..7）、非法格式 fail-open。
func TestWithinJobWindowTable(t *testing.T) {
	wed10 := time.Date(2026, 11, 18, 10, 0, 0, 0, time.Local) // 周三 10:00
	wed02 := time.Date(2026, 11, 18, 2, 0, 0, 0, time.Local)  // 周三 02:00
	wed23 := time.Date(2026, 11, 18, 23, 0, 0, 0, time.Local) // 周三 23:00
	thu01 := time.Date(2026, 11, 19, 1, 0, 0, 0, time.Local)  // 周四 01:00

	cases := []struct {
		name string
		now  time.Time
		w    *config.JobWindowConfig
		want bool
	}{
		{"nil window open", wed10, nil, true},
		{"in window", wed10, &config.JobWindowConfig{Start: "08:00", End: "18:00"}, true},
		{"before start", wed02, &config.JobWindowConfig{Start: "08:00", End: "18:00"}, false},
		{"after end", wed23, &config.JobWindowConfig{Start: "08:00", End: "18:00"}, false},
		{"at end excluded", time.Date(2026, 11, 18, 18, 0, 0, 0, time.Local),
			&config.JobWindowConfig{Start: "08:00", End: "18:00"}, false},
		{"cross midnight late", wed23, &config.JobWindowConfig{Start: "22:00", End: "02:00"}, true},
		{"cross midnight early", thu01, &config.JobWindowConfig{Start: "22:00", End: "02:00"}, true},
		{"cross midnight outside", wed10, &config.JobWindowConfig{Start: "22:00", End: "02:00"}, false},
		{"day blocked", wed10, &config.JobWindowConfig{Start: "08:00", End: "18:00", Days: []string{"mon"}}, false},
		{"day allowed", wed10, &config.JobWindowConfig{Start: "08:00", End: "18:00", Days: []string{"wed"}}, true},
		{"day number allowed", wed10, &config.JobWindowConfig{Start: "08:00", End: "18:00", Days: []string{"3"}}, true}, // 3=Wed
		{"day number sunday", time.Date(2026, 11, 22, 10, 0, 0, 0, time.Local), // 周日
			&config.JobWindowConfig{Start: "08:00", End: "18:00", Days: []string{"7"}}, true},
		{"invalid format fail-open", wed10, &config.JobWindowConfig{Start: "bad", End: "18:00"}, true},
		{"invalid hour range fail-open", wed10, &config.JobWindowConfig{Start: "25:00", End: "18:00"}, true},
		{"invalid minute range fail-open", wed10, &config.JobWindowConfig{Start: "08:99", End: "18:00"}, true},
		{"all invalid days fail-open", wed10, &config.JobWindowConfig{Start: "08:00", End: "18:00", Days: []string{"xday"}}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, _ := withinJobWindow(c.now, c.w)
			if got != c.want {
				t.Errorf("withinJobWindow(%v, %+v) = %v, want %v", c.now, c.w, got, c.want)
			}
		})
	}
}

// TestJobDedupKey 去重键确定性：同 type+target+params 同键；不同 params/
// target 异键；nil 与空 map 同键（语义等价）。
func TestJobDedupKey(t *testing.T) {
	k1 := jobDedupKey("agent.exec", "a1", map[string]interface{}{"command": "uptime"})
	if k2 := jobDedupKey("agent.exec", "a1", map[string]interface{}{"command": "uptime"}); k1 != k2 {
		t.Fatalf("same params should dedup: %s != %s", k1, k2)
	}
	if jobDedupKey("agent.exec", "a1", map[string]interface{}{"command": "ls"}) == k1 {
		t.Fatal("different params should differ")
	}
	if jobDedupKey("agent.exec", "a2", map[string]interface{}{"command": "uptime"}) == k1 {
		t.Fatal("different target should differ")
	}
	if jobDedupKey("agent.exec", "a1", nil) != jobDedupKey("agent.exec", "a1", map[string]interface{}{}) {
		t.Fatal("nil and empty params should dedup same")
	}
}

// ============ 提交守卫（窗口 + 去重）============

// TestSubmitJobGuardWindowBlock 窗外提交 → 409，不建行。
func TestSubmitJobGuardWindowBlock(t *testing.T) {
	s := newBackupTestServer(t)
	// 全天除 00:00-00:01 外都关闭 → 现在几乎必在外（跨午夜窗口不覆盖当前）
	s.cfg = &config.Config{Jobs: &config.JobsConfig{
		Window: &config.JobWindowConfig{Start: "00:00", End: "00:01"},
	}}
	withFakeBackupAgent(t, s, "a1", nil)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusConflict {
		t.Fatalf("window block: code = %d body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "execution window") {
		t.Fatalf("block reason should mention window: %s", rec.Body.String())
	}
	// 未建行
	if jobs, _ := s.db.ListJobs(10); len(jobs) != 0 {
		t.Fatalf("blocked submit should not create job, got %d", len(jobs))
	}
}

// TestSubmitJobGuardWindowOpen 窗口内提交放行（回归：默认 nil 窗口不拦）。
func TestSubmitJobGuardWindowOpen(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0}, ""
	})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("default window should allow: code = %d body = %s", rec.Code, rec.Body.String())
	}
}

// TestSubmitJobGuardDedupConflict 同 type+target+params 在途重复提交 → 409，
// 响应带既有 job id；第一单落终态后再提交放行（不误伤完成后再跑一次）。
func TestSubmitJobGuardDedupConflict(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		time.Sleep(400 * time.Millisecond) // 拖住第一单，保持 in-flight
		return map[string]interface{}{"exit_code": 0}, ""
	})

	post := func() (*httptest.ResponseRecorder, jobView) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/jobs",
			strings.NewReader(`{"type":"agent.exec","target":"a1","parameters":{"command":"uptime"}}`))
		s.handleJobsAPI(rec, req, "/jobs")
		var v jobView
		_ = json.Unmarshal(rec.Body.Bytes(), &v)
		return rec, v
	}

	rec1, v1 := post()
	if rec1.Code != http.StatusCreated {
		t.Fatalf("first submit: code = %d body = %s", rec1.Code, rec1.Body.String())
	}
	rec2, _ := post()
	if rec2.Code != http.StatusConflict {
		t.Fatalf("dup submit: code = %d body = %s", rec2.Code, rec2.Body.String())
	}
	if !strings.Contains(rec2.Body.String(), v1.ID) {
		t.Fatalf("dup response should carry existing job id, got %s", rec2.Body.String())
	}

	// 第一单落终态后，同参数再提交放行（终态不参与在途去重）
	waitJobTerminal(t, s, v1.ID)
	rec3, _ := post()
	if rec3.Code != http.StatusCreated {
		t.Fatalf("after terminal, resubmit should allow: code = %d body = %s", rec3.Code, rec3.Body.String())
	}
}

// ============ 人工重试 ============

// TestJobRetry 失败 job 重试：201 新建 job（新 id，retried_from 溯源）+ 审计。
func TestJobRetry(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "retried ok"}, ""
	})
	orig := &storage.Job{
		ID: "job-failed-1", Type: "agent.exec", Target: "a1", Actor: "alice",
		Parameters:  `{"command":"uptime"}`,
		Status:      storage.JobStatusFailed,
		CreatedAt:   time.Now(),
		FinishedAt:  &[]time.Time{time.Now()}[0],
		DedupKey:    jobDedupKey("agent.exec", "a1", map[string]interface{}{"command": "uptime"}),
		RetriedFrom: "",
	}
	if err := s.db.CreateJob(orig); err != nil {
		t.Fatalf("seed failed job: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/job-failed-1/retry", nil)
	s.handleJobsAPI(rec, req, "/jobs/job-failed-1/retry")
	if rec.Code != http.StatusCreated {
		t.Fatalf("retry: code = %d body = %s", rec.Code, rec.Body.String())
	}
	var v jobView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if v.RetriedFrom != "job-failed-1" || v.ID == "" || v.ID == "job-failed-1" {
		t.Fatalf("retry view = %+v (want new id, retriedFrom=job-failed-1)", v)
	}

	// 状态断言去掉：派发经 goBg 异步（pending→running→终态），读回时可能已在
	// running（快机）或仍 pending（慢机），状态是竞态；只断言稳定字段（存在 +
	// 溯源链），终态由末尾 waitJobTerminal 验证。
	got, err := s.db.GetJob(v.ID)
	if err != nil || got == nil || got.RetriedFrom != "job-failed-1" {
		t.Fatalf("stored retry = %+v, err %v", got, err)
	}

	logs, _, _ := s.db.GetAuditLogs(0, 10, map[string]interface{}{"action": audit.ActionJobRetry})
	if len(logs) == 0 {
		t.Fatal("retry audit row missing")
	}
	waitJobTerminal(t, s, v.ID) // 后台派发终态（假 agent 应答成功）
}

// TestJobRetryReject 非 failed/cancelled 拒绝；不存在 404。
func TestJobRetryReject(t *testing.T) {
	s := newBackupTestServer(t)
	ok := &storage.Job{ID: "job-ok", Type: "agent.exec", Target: "a1", Status: storage.JobStatusSuccess, CreatedAt: time.Now()}
	if err := s.db.CreateJob(ok); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/job-ok/retry", nil)
	s.handleJobsAPI(rec, req, "/jobs/job-ok/retry")
	if rec.Code != http.StatusConflict {
		t.Fatalf("success job retry: code = %d body = %s", rec.Code, rec.Body.String())
	}

	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/api/jobs/nope/retry", nil)
	s.handleJobsAPI(rec2, req2, "/jobs/nope/retry")
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("missing job retry: code = %d", rec2.Code)
	}
}

// TestJobWindowBlockReason 纯函数：配置窗开→空串放行（覆盖 open 短路），
// 窗外→非空原因。
func TestJobWindowBlockReason(t *testing.T) {
	at := time.Date(2026, 11, 18, 10, 0, 0, 0, time.Local)
	if r := jobWindowBlockReason(at, &config.JobWindowConfig{Start: "08:00", End: "18:00"}); r != "" {
		t.Fatalf("open window should return empty reason, got %q", r)
	}
	if r := jobWindowBlockReason(at, nil); r != "" {
		t.Fatalf("nil window should return empty reason, got %q", r)
	}
	if r := jobWindowBlockReason(at, &config.JobWindowConfig{Start: "11:00", End: "12:00"}); r == "" {
		t.Fatal("closed window should return reason")
	}
}

// TestJobRetryWindowBlocked 窗外重试 → 409（"00:00"-"00:00" 恒关：end 边界
// 半开，任何时刻 cur<0 不成立——确定性，不依赖测试时刻）。
func TestJobRetryWindowBlocked(t *testing.T) {
	s := newBackupTestServer(t)
	s.cfg = &config.Config{Jobs: &config.JobsConfig{
		Window: &config.JobWindowConfig{Start: "00:00", End: "00:00"},
	}}
	orig := &storage.Job{ID: "job-wb", Type: "agent.exec", Target: "a1",
		Parameters: `{"command":"x"}`, Status: storage.JobStatusFailed, CreatedAt: time.Now()}
	if err := s.db.CreateJob(orig); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/job-wb/retry", nil)
	s.handleJobsAPI(rec, req, "/jobs/job-wb/retry")
	if rec.Code != http.StatusConflict {
		t.Fatalf("window blocked retry: code = %d body = %s", rec.Code, rec.Body.String())
	}
}

// TestJobRetryEmptyParamsWithUser 空 Parameters 重试（params nil→空 map 兜底）
// + 带登录上下文（actor 取真名而非 unknown）。
func TestJobRetryEmptyParamsWithUser(t *testing.T) {
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(method string, params map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0}, ""
	})
	orig := &storage.Job{ID: "job-noparam", Type: "agent.exec", Target: "a1",
		Parameters: "", Status: storage.JobStatusFailed, CreatedAt: time.Now()}
	if err := s.db.CreateJob(orig); err != nil {
		t.Fatalf("seed: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs/job-noparam/retry", nil)
	req = req.WithContext(auth.ContextWithUser(req.Context(), "u1", "alice", "admin"))
	s.handleJobsAPI(rec, req, "/jobs/job-noparam/retry")
	if rec.Code != http.StatusCreated {
		t.Fatalf("retry empty params: code = %d body = %s", rec.Code, rec.Body.String())
	}
	var v jobView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Actor != "alice" {
		t.Fatalf("actor = %q, want alice (logged-in user)", v.Actor)
	}
	waitJobTerminal(t, s, v.ID)
	got, _ := s.db.GetJob(v.ID)
	if got == nil || got.Actor != "alice" {
		t.Fatalf("stored retry = %+v, want actor alice", got)
	}
}

// TestJobRetryMethodNotAllowed retry 路由仅收 POST（GET → 405）。
func TestJobRetryMethodNotAllowed(t *testing.T) {
	s := newBackupTestServer(t)
	orig := &storage.Job{ID: "job-m", Type: "agent.exec", Target: "a1",
		Parameters: `{"command":"x"}`, Status: storage.JobStatusFailed, CreatedAt: time.Now()}
	if err := s.db.CreateJob(orig); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/jobs/job-m/retry", nil)
	s.handleJobsAPI(rec, req, "/jobs/job-m/retry")
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET retry: code = %d body = %s", rec.Code, rec.Body.String())
	}
}
