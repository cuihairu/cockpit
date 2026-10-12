package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// approvalAgent 带假 agent 的审批测试环境（job.exec 应答桩）。
func approvalAgent(t *testing.T) (*Server, string) {
	t.Helper()
	s := newBackupTestServer(t)
	withFakeBackupAgent(t, s, "a1", func(string, map[string]interface{}) (interface{}, string) {
		return map[string]interface{}{"exit_code": 0, "output": "ok", "error": ""}, ""
	})
	return s, "a1"
}

// submitForApproval 走 POST /api/jobs 提交需审批请求，返回审批视图。
func submitForApproval(t *testing.T, s *Server, body string) approvalView {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs", strings.NewReader(body))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusAccepted {
		t.Fatalf("submit code = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v approvalView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return v
}

// decideApproval 走 approve/reject 端点，返回响应码与响应体。
func decideApproval(t *testing.T, s *Server, id, action string) (int, approvalView) {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/approvals/"+id+"/"+action, nil)
	s.handleApprovalsAPI(rec, req, "/approvals/"+id+"/"+action)
	var v approvalView
	_ = json.Unmarshal(rec.Body.Bytes(), &v)
	return rec.Code, v
}

// TestApprovalGateFullChain 提交（202，无 Job 行）→ 批准（派发真跑 → 终态）
// → 对账（approval.JobID 回写 + 审计 submit/approve 两笔分账）。
func TestApprovalGateFullChain(t *testing.T) {
	s, target := approvalAgent(t)

	appr := submitForApproval(t, s,
		`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"uptime"},"approval":"required"}`)
	if appr.ID == "" || appr.Status != storage.ApprovalStatusPending || appr.Summary != "uptime" {
		t.Fatalf("view = %+v", appr)
	}
	if appr.ExpiresAt == nil {
		t.Fatal("expiresAt missing")
	}

	// 单条详情：payload 解析为 parameters（审批人须看到将执行什么）
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/approvals/"+appr.ID, nil)
	s.handleApprovalsAPI(rec, req, "/approvals/"+appr.ID)
	if rec.Code != http.StatusOK {
		t.Fatalf("get approval = %d", rec.Code)
	}
	var got approvalView
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Parameters["command"] != "uptime" {
		t.Fatalf("parameters = %+v", got.Parameters)
	}

	// 待批阶段不建 Job 行（审计上「提交」与「执行」分账的库面体现）
	jobs, err := s.db.ListJobs(50)
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 0 {
		t.Fatalf("jobs before approve = %d, want 0", len(jobs))
	}

	code, v := decideApproval(t, s, appr.ID, "approve")
	if code != http.StatusOK {
		t.Fatalf("approve code = %d", code)
	}
	if v.Status != storage.ApprovalStatusApproved || v.JobID == "" {
		t.Fatalf("after approve = %+v", v)
	}

	// 派发真跑：Job 走完整异步链到终态
	job := waitJobTerminal(t, s, v.JobID)
	if job.Status != storage.JobStatusSuccess || job.Actor != "unknown" {
		t.Fatalf("job = %+v", job)
	}

	logs, _, err := s.db.GetAuditLogs(0, 50, map[string]interface{}{"resource": audit.ResourceApproval})
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]bool{}
	for _, l := range logs {
		actions[l.Action] = true
	}
	if !actions[audit.ActionApprovalSubmit] || !actions[audit.ActionApprovalApprove] {
		t.Fatalf("audit actions = %+v", actions)
	}
}

// TestApprovalReject 驳回：终态不建 Job 不派发，审计分账。
func TestApprovalReject(t *testing.T) {
	s, target := approvalAgent(t)
	appr := submitForApproval(t, s,
		`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"reboot-now"},"approval":"required"}`)

	code, v := decideApproval(t, s, appr.ID, "reject")
	if code != http.StatusOK || v.Status != storage.ApprovalStatusRejected || v.JobID != "" {
		t.Fatalf("reject = %d %+v", code, v)
	}
	jobs, _ := s.db.ListJobs(50)
	if len(jobs) != 0 {
		t.Fatalf("jobs after reject = %d, want 0", len(jobs))
	}
	logs, _, _ := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionApprovalReject})
	if len(logs) != 1 {
		t.Fatalf("reject audit = %d, want 1", len(logs))
	}
}

// TestApprovalDuplicateDecide409 并发双决条件收口：重复批准/批准后驳回均
// 409（DecideApproval 条件 UPDATE 只有一笔命中）。
func TestApprovalDuplicateDecide409(t *testing.T) {
	s, target := approvalAgent(t)
	appr := submitForApproval(t, s,
		`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"uptime"},"approval":"required"}`)

	if code, _ := decideApproval(t, s, appr.ID, "approve"); code != http.StatusOK {
		t.Fatalf("first approve = %d", code)
	}
	if code, _ := decideApproval(t, s, appr.ID, "approve"); code != http.StatusConflict {
		t.Fatalf("second approve = %d, want 409", code)
	}
	if code, _ := decideApproval(t, s, appr.ID, "reject"); code != http.StatusConflict {
		t.Fatalf("reject after approve = %d, want 409", code)
	}
}

// TestApprovalExpired 过期不可批（TTL 取负构造）：批准 409，列表惰性清扫
// 落 expired。批准/驳回都对提交后到期的窗口诚实。
func TestApprovalExpired(t *testing.T) {
	s, target := approvalAgent(t)
	old := approvalTTL
	approvalTTL = -time.Second
	t.Cleanup(func() { approvalTTL = old })

	appr := submitForApproval(t, s,
		`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"uptime"},"approval":"required"}`)

	if code, _ := decideApproval(t, s, appr.ID, "approve"); code != http.StatusConflict {
		t.Fatalf("expired approve = %d, want 409", code)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
	s.handleApprovalsAPI(rec, req, "/approvals")
	var list struct {
		Approvals []approvalView `json:"approvals"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Approvals) != 1 || list.Approvals[0].Status != storage.ApprovalStatusExpired {
		t.Fatalf("list = %+v, want expired", list.Approvals)
	}
}

// TestApprovalSubmitGuards 守卫面：未知 approval 值 400、坏 payload 400
// （不建审批）、404、405。目标离线不拦提交——审批等待窗口内 agent 可回线，
// 离线判定在批准派发时如实 503。
func TestApprovalSubmitGuards(t *testing.T) {
	s, target := approvalAgent(t)

	// 未知 approval 值
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"x"},"approval":"maybe"}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad approval value = %d, want 400", rec.Code)
	}

	// 坏 payload 不进审批（批准后原样派发，校验必须前置）
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"`+target+`","parameters":{},"approval":"required"}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad payload = %d, want 400", rec.Code)
	}
	list, _ := s.db.ListApprovals("", 50)
	if len(list) != 0 {
		t.Fatalf("approvals = %d, want 0", len(list))
	}

	// targets 扇出在审批提交同样拒绝（单目标语义）
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"`+target+`","targets":["x"],"parameters":{"command":"x"},"approval":"required"}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("targets in approval = %d, want 400", rec.Code)
	}

	// 404 / 405
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/nope/approve", nil)
	s.handleApprovalsAPI(rec, req, "/approvals/nope/approve")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown approve = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/approvals/nope", nil)
	s.handleApprovalsAPI(rec, req, "/approvals/nope")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown get = %d, want 404", rec.Code)
	}
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/api/approvals", nil)
	s.handleApprovalsAPI(rec, req, "/approvals")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("delete collection = %d, want 404", rec.Code)
	}
}

// TestApprovalsRBACRules 权限面：GET=approvals:read，写（提交/approve/reject）
// = approvals:write（D8 同口径）。
func TestApprovalsRBACRules(t *testing.T) {
	perms, governed := requiredPerms("/api/approvals", http.MethodGet)
	if !governed || len(perms) != 1 || perms[0] != "approvals:read" {
		t.Fatalf("GET perms = %v, %v", perms, governed)
	}
	perms, _ = requiredPerms("/api/approvals/x/approve", http.MethodPost)
	if len(perms) != 1 || perms[0] != "approvals:write" {
		t.Fatalf("approve perms = %v", perms)
	}
	perms, _ = requiredPerms("/api/approvals/x/reject", http.MethodPost)
	if len(perms) != 1 || perms[0] != "approvals:write" {
		t.Fatalf("reject perms = %v", perms)
	}
}

// TestApprovalSubmitNoJobSideEffects 无审批开关的旧请求行为零变化：201 直建
// pending（W1 契约回归，防审批开关改坏主路径）。
func TestApprovalSubmitNoJobSideEffects(t *testing.T) {
	s, target := approvalAgent(t)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"uptime"}}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusCreated {
		t.Fatalf("plain create = %d, body: %s", rec.Code, rec.Body.String())
	}
	var v jobView
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if v.Status != storage.JobStatusPending {
		t.Fatalf("status = %s", v.Status)
	}
	// 异步派发 goroutine（dispatchJobAsync）在测试结束后继续回写终态——不等
	// 终态收口，db.Close 后 goroutine 仍在写库，TempDir RemoveAll 与 WAL
	// 落盘竞态（unlinkat ENOTEMPTY，CI race 腿 flaky 根因）
	deadline := time.Now().Add(5 * time.Second)
	for {
		job, err := s.db.GetJob(v.ID)
		if err != nil {
			t.Fatal(err)
		}
		if job.Status != storage.JobStatusPending && job.Status != storage.JobStatusRunning {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("job %s never reached terminal: %s", v.ID, job.Status)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestApprovalRoutingGuards 路由分发面：serveAPI → approvals 分发 case、
// /approvals/ 空 id 404、approve/reject 只收 POST（405）、无后缀 POST 405、
// 未知 id 驳回 404。
func TestApprovalRoutingGuards(t *testing.T) {
	s, _ := approvalAgent(t)

	// serveAPI 分发（api.go 路由 case）→ 空台账 200
	rec := httptest.NewRecorder()
	s.serveAPI(rec, covReq(http.MethodGet, "/api/approvals", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("serveAPI list = %d, body: %s", rec.Code, rec.Body.String())
	}

	// /approvals/ 空 id
	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/approvals/", nil)
	s.handleApprovalsAPI(rec, req, "/approvals/")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("empty id = %d, want 404", rec.Code)
	}

	// approve/reject 只收 POST；裸 /{id} 无 GET 外语义
	for _, c := range []struct{ path, method string }{
		{"/approvals/x/approve", http.MethodGet},
		{"/approvals/x/reject", http.MethodGet},
		{"/approvals/x", http.MethodPost},
	} {
		rec = httptest.NewRecorder()
		req = httptest.NewRequest(c.method, "/api"+c.path, nil)
		s.handleApprovalsAPI(rec, req, c.path)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s %s = %d, want 405", c.method, c.path, rec.Code)
		}
	}

	// 未知 id 驳回 404
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/approvals/nope/reject", nil)
	s.handleApprovalsAPI(rec, req, "/approvals/nope/reject")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown reject = %d, want 404", rec.Code)
	}
}

// TestApprovalDBFailureDegraded 关库构造存储失败面：审批落库失败 500；
// 台账清扫失败只降级 log（列表继续走 ListApprovals，同关库 500）。
func TestApprovalDBFailureDegraded(t *testing.T) {
	s, target := approvalAgent(t)
	if err := s.db.Close(); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/jobs",
		strings.NewReader(`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"uptime"},"approval":"required"}`))
	s.handleJobsAPI(rec, req, "/jobs")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("closed-db submit = %d, want 500", rec.Code)
	}

	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/approvals", nil)
	s.handleApprovalsAPI(rec, req, "/approvals")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("closed-db list = %d, want 500", rec.Code)
	}
}

// TestApprovalCorruptPayloadApprove payload 损坏仍批准：终态已落、派发不可
// 行——如实 500，审计记 dispatch=payload corrupt（不假称已派发）。
func TestApprovalCorruptPayloadApprove(t *testing.T) {
	s, target := approvalAgent(t)
	appr := &storage.Approval{
		ID:        protocol.GenerateID(),
		Kind:      "job",
		Payload:   "{not json",
		Target:    target,
		Actor:     "someone",
		Status:    storage.ApprovalStatusPending,
		CreatedAt: time.Now(),
	}
	if err := s.db.CreateApproval(appr); err != nil {
		t.Fatal(err)
	}

	code, _ := decideApproval(t, s, appr.ID, "approve")
	if code != http.StatusInternalServerError {
		t.Fatalf("corrupt approve = %d, want 500", code)
	}
	got, err := s.db.GetApproval(appr.ID)
	if err != nil || got.Status != storage.ApprovalStatusApproved {
		t.Fatalf("status after corrupt approve = %+v err=%v", got, err)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 50, map[string]interface{}{"action": audit.ActionApprovalApprove})
	if len(logs) != 1 || !strings.Contains(logs[0].Details, "payload corrupt") {
		t.Fatalf("corrupt audit = %+v", logs)
	}
}

// TestApprovalApproveOfflineTarget 批准时 agent 离线：批准终态已落、派发
// 如实 503（提交时不拦——等待窗口内 agent 可回线），无 Job 行。
func TestApprovalApproveOfflineTarget(t *testing.T) {
	s, _ := approvalAgent(t)
	appr := submitForApproval(t, s,
		`{"type":"agent.exec","target":"ghost","parameters":{"command":"uptime"},"approval":"required"}`)

	code, _ := decideApproval(t, s, appr.ID, "approve")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("offline approve = %d, want 503", code)
	}
	got, err := s.db.GetApproval(appr.ID)
	if err != nil || got.Status != storage.ApprovalStatusApproved || got.JobID != "" {
		t.Fatalf("approval after offline approve = %+v err=%v", got, err)
	}
	jobs, _ := s.db.ListJobs(50)
	if len(jobs) != 0 {
		t.Fatalf("jobs = %d, want 0", len(jobs))
	}
}

// TestApprovalSummaryTruncation 长命令摘要截断（列表视图免点开可辨）。
func TestApprovalSummaryTruncation(t *testing.T) {
	s, target := approvalAgent(t)
	cmd := strings.Repeat("a", approvalSummaryMax+30)
	appr := submitForApproval(t, s,
		`{"type":"agent.exec","target":"`+target+`","parameters":{"command":"`+cmd+`"},"approval":"required"}`)
	want := cmd[:approvalSummaryMax] + "…"
	if appr.Summary != want {
		t.Fatalf("summary len = %d, want %d", len(appr.Summary), len(want))
	}
}
