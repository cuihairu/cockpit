package server

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// 审批门 API（workflow-design M2d）：高危 Job 提交先落审批，批准后才派发。
//
//	GET  /api/approvals              审批台账（?status= 过滤；惰性过期清扫）
//	GET  /api/approvals/{id}         单条详情（含入参）
//	POST /api/approvals/{id}/approve 批准并派发（提交体里的原 payload）
//	POST /api/approvals/{id}/reject  驳回（终态）
//
// 等待态落库不落 goroutine：server 重启不丢待批（M2b 立项阻塞点在此解）。
// 提交入口在 POST /api/jobs（approval="required"），权限同 jobs:write——
// 门槛与执行权同面，审批的价值在「显式二次确认 + 审计分账」，不做权限拆分。

// approvalTTL 待批有效期：过期后批准不再命中（DecideApproval 条件闭合），
// 只能重新提交。var 便于测试缩短（同 workflowRetryInterval 惯例）。
var approvalTTL = 10 * time.Minute

// approvalSummaryMax 命令摘要截断（列表视图免点开可辨）。
const approvalSummaryMax = 160

// approvalView API 视图：Payload 解析为 parameters 返回（审批人须看到将执行什么）。
type approvalView struct {
	ID         string                 `json:"id"`
	Kind       string                 `json:"kind"`
	Target     string                 `json:"target"`
	Actor      string                 `json:"actor"`
	Status     string                 `json:"status"`
	Summary    string                 `json:"summary,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
	ExpiresAt  *time.Time             `json:"expiresAt,omitempty"`
	DecidedBy  string                 `json:"decidedBy,omitempty"`
	DecidedAt  *time.Time             `json:"decidedAt,omitempty"`
	JobID      string                 `json:"jobId,omitempty"`
	CreatedAt  time.Time              `json:"createdAt"`
}

func toApprovalView(a *storage.Approval) approvalView {
	v := approvalView{
		ID:        a.ID,
		Kind:      a.Kind,
		Target:    a.Target,
		Actor:     a.Actor,
		Status:    a.Status,
		Summary:   a.Summary,
		ExpiresAt: a.ExpiresAt,
		DecidedBy: a.DecidedBy,
		DecidedAt: a.DecidedAt,
		JobID:     a.JobID,
		CreatedAt: a.CreatedAt,
	}
	if a.Payload != "" {
		var p jobCreatePayload
		if json.Unmarshal([]byte(a.Payload), &p) == nil {
			v.Parameters = p.Parameters
		}
	}
	return v
}

// approvalSummary 命令摘要（agent.exec 取 parameters.command 截断）。
func approvalSummary(p *jobCreatePayload) string {
	if p.Type != "agent.exec" {
		return p.Type
	}
	cmd, _ := p.Parameters["command"].(string)
	if len(cmd) > approvalSummaryMax {
		return cmd[:approvalSummaryMax] + "…"
	}
	return cmd
}

// createApproval 高危提交落审批记录（未过校验的 payload 不进审批——批准后
// 原样派发，校验必须前置）。
func (s *Server) createApproval(p *jobCreatePayload, actor string) (*storage.Approval, error) {
	payloadJSON, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	expires := time.Now().Add(approvalTTL)
	appr := &storage.Approval{
		ID:        protocol.GenerateID(),
		Kind:      "job",
		Payload:   string(payloadJSON),
		Summary:   approvalSummary(p),
		Target:    p.Target,
		Actor:     actor,
		Status:    storage.ApprovalStatusPending,
		ExpiresAt: &expires,
		CreatedAt: time.Now(),
	}
	if err := s.db.CreateApproval(appr); err != nil {
		return nil, err
	}
	return appr, nil
}

// handleApprovalsAPI 分发 /api/approvals[/{id}[/approve|/reject]]
func (s *Server) handleApprovalsAPI(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "/approvals" && r.Method == http.MethodGet:
		s.handleApprovalsList(w, r)
	case strings.HasPrefix(path, "/approvals/"):
		rest := strings.TrimPrefix(path, "/approvals/")
		if rest == "" {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		if id, ok := strings.CutSuffix(rest, "/approve"); ok && id != "" {
			if r.Method != http.MethodPost {
				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.handleApprovalApprove(w, r, id)
			return
		}
		if id, ok := strings.CutSuffix(rest, "/reject"); ok && id != "" {
			if r.Method != http.MethodPost {
				s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
				return
			}
			s.handleApprovalReject(w, r, id)
			return
		}
		if r.Method == http.MethodGet {
			s.handleApprovalGet(w, r, rest)
			return
		}
		s.handleError(w, r, http.StatusMethodNotAllowed, "method not allowed")
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleApprovalsList 审批台账：先惰性清扫过期（pending → expired），再按
// 过滤返回。清扫与 DecideApproval 条件互斥（status 面），先后皆安全。
func (s *Server) handleApprovalsList(w http.ResponseWriter, r *http.Request) {
	if _, err := s.db.ExpireApprovals(time.Now()); err != nil {
		// 清扫失败只降级：列表照常返回，单条批准仍有到期条件兜底
		log.Printf("approvals: expire sweep: %v", err)
	}
	list, err := s.db.ListApprovals(r.URL.Query().Get("status"), 50)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to list approvals")
		return
	}
	out := make([]approvalView, 0, len(list))
	for i := range list {
		out = append(out, toApprovalView(&list[i]))
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{"approvals": out})
}

// handleApprovalGet 单条详情（不触发清扫；过期判定在批准条件内闭合）。
func (s *Server) handleApprovalGet(w http.ResponseWriter, r *http.Request, id string) {
	appr, err := s.db.GetApproval(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "approval not found")
		return
	}
	s.writeJSON(w, http.StatusOK, toApprovalView(appr))
}

// handleApprovalApprove 批准并派发：条件终态写先收口并发（重复批准只有
// 一笔命中，其余 409），再按原 payload 派发（actor=提交人）。批准是终态
// 不可回滚；此刻 agent 离线则如实 503（不假称已派发），重试面=重新提交。
func (s *Server) handleApprovalApprove(w http.ResponseWriter, r *http.Request, id string) {
	appr, err := s.db.GetApproval(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "approval not found")
		return
	}
	approver := actorName(r)
	now := time.Now()
	hit, err := s.db.DecideApproval(id, storage.ApprovalStatusApproved, approver, now)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to decide approval")
		return
	}
	if hit == 0 {
		s.handleError(w, r, http.StatusConflict, "approval already decided or expired")
		return
	}

	var p jobCreatePayload
	if err := json.Unmarshal([]byte(appr.Payload), &p); err != nil {
		// 批准已落、派发不可行：如实落账并 500（不假称已派发）
		s.audit.LogResource(approver, audit.ActionApprovalApprove, audit.ResourceApproval, id,
			map[string]interface{}{"actor": appr.Actor, "target": appr.Target, "dispatch": "payload corrupt"},
			s.getClientIP(r), r.UserAgent())
		s.handleError(w, r, http.StatusInternalServerError, "approval payload corrupt")
		return
	}
	s.audit.LogResource(approver, audit.ActionApprovalApprove, audit.ResourceApproval, id,
		map[string]interface{}{"actor": appr.Actor, "target": appr.Target, "summary": appr.Summary},
		s.getClientIP(r), r.UserAgent())

	view, fail := s.submitJob(&p, appr.Actor)
	if fail != nil {
		s.handleError(w, r, fail.code, fail.msg)
		return
	}
	// 对账回写最佳努力：批准终态已落，JobID 缺失只影响台账互链
	if err := s.db.SetApprovalJobID(id, view.ID); err != nil {
		log.Printf("approvals: link job for %s: %v", id, err)
	}
	appr.Status = storage.ApprovalStatusApproved
	appr.DecidedBy = approver
	appr.DecidedAt = &now
	appr.JobID = view.ID
	s.writeJSON(w, http.StatusOK, toApprovalView(appr))
}

// handleApprovalReject 驳回：终态，不建 Job 不派发。
func (s *Server) handleApprovalReject(w http.ResponseWriter, r *http.Request, id string) {
	appr, err := s.db.GetApproval(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "approval not found")
		return
	}
	approver := actorName(r)
	now := time.Now()
	hit, err := s.db.DecideApproval(id, storage.ApprovalStatusRejected, approver, now)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to decide approval")
		return
	}
	if hit == 0 {
		s.handleError(w, r, http.StatusConflict, "approval already decided or expired")
		return
	}
	s.audit.LogResource(approver, audit.ActionApprovalReject, audit.ResourceApproval, id,
		map[string]interface{}{"actor": appr.Actor, "target": appr.Target},
		s.getClientIP(r), r.UserAgent())
	appr.Status = storage.ApprovalStatusRejected
	appr.DecidedBy = approver
	appr.DecidedAt = &now
	s.writeJSON(w, http.StatusOK, toApprovalView(appr))
}
