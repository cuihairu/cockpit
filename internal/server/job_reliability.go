package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// Job 可靠性（P0 Agent 第三片，见 docs/design/task-recovery-design.md）：
// 任务身份/目标去重（§6.1）、人工重试审计（§6.2）、执行窗口门控（§6.3）、
// 重启遗留孤儿清扫接线（§6 表格）。自动重试明确不做（决策项 31：不自动重放
// 所有 failed/pending、不承诺任意脚本恰好执行一次）。

// jobDedupKey 计算去重键（§6.1）：type + target + 规范化 params 的 SHA-256。
// json.Marshal(map) 键序稳定（Go 按 key 排序），同一 params 对象序列化
// 结果确定；params nil 与空 map 同键（语义等价）。
func jobDedupKey(jobType, target string, params map[string]interface{}) string {
	if params == nil {
		params = map[string]interface{}{} // nil 与空 map 同键（语义等价）
	}
	raw, _ := json.Marshal(params)
	sum := sha256.Sum256([]byte(jobType + "\x00" + target + "\x00" + string(raw)))
	return hex.EncodeToString(sum[:])
}

// jobWindowBlockReason 执行窗口门控（§6.3）：nil 窗口（未配置）或窗口内
// 返回 ""（放行）；窗外/配置无效返回人类可读原因。配置无效 fail-open 并
// 告警（错误配置不应把全部任务锁死，窗口是自愿性约束）。
func jobWindowBlockReason(now time.Time, w *config.JobWindowConfig) string {
	if w == nil {
		return ""
	}
	open, reason := withinJobWindow(now, w)
	if open {
		return ""
	}
	return reason
}

// withinJobWindow 纯判定（可单测）：Start/End 为「HH:MM」本地墙钟，
// Days 空 = 每日；Start>End 视为跨午夜（22:00-02:00）。
// 解析失败视为窗口内（fail-open + 调用方告警）。
func withinJobWindow(now time.Time, w *config.JobWindowConfig) (bool, string) {
	if w == nil {
		return true, ""
	}
	startMin, ok1 := parseHHMM(w.Start)
	endMin, ok2 := parseHHMM(w.End)
	if !ok1 || !ok2 {
		return true, ""
	}
	cur := now.Hour()*60 + now.Minute()

	inTime := false
	if startMin <= endMin {
		inTime = cur >= startMin && cur < endMin
	} else {
		inTime = cur >= startMin || cur < endMin
	}
	if !inTime {
		return false, fmt.Sprintf("outside execution window %s-%s", w.Start, w.End)
	}
	if len(w.Days) == 0 {
		return true, ""
	}
	if !dayMatches(now, w.Days) {
		return false, fmt.Sprintf("outside execution window %s-%s (day %s not allowed)",
			w.Start, w.End, now.Weekday().String())
	}
	return true, ""
}

// parseHHMM "HH:MM" → 当日分钟数（0..1439）；格式错返回 ok=false。
func parseHHMM(s string) (int, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), ":", 2)
	if len(parts) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(parts[0])
	m, err2 := strconv.Atoi(parts[1])
	if err1 != nil || err2 != nil || h < 0 || h > 23 || m < 0 || m > 59 {
		return 0, false
	}
	return h*60 + m, true
}

// dayMatches now 的星期是否在 Days 里（接受 "mon".."sun" 或 1..7，1=周一）。
// 统一比较 3 字母小写（now.Weekday().String() 是全名 "Wednesday"，取前 3）。
// Go 的 time.Weekday：Sunday=0..Saturday=6；输入 n（1..7）→ n%7（1→Mon=1,
// 7→Sun=0）。全部条目无效 → 不限制（fail-open）。
func dayMatches(now time.Time, days []string) bool {
	want := map[string]bool{}
	for _, d := range days {
		norm := strings.ToLower(strings.TrimSpace(d))
		if n, err := strconv.Atoi(norm); err == nil {
			if n >= 1 && n <= 7 {
				want[strings.ToLower(time.Weekday(n % 7).String())[:3]] = true
				continue
			}
		}
		switch norm {
		case "mon", "tue", "wed", "thu", "fri", "sat", "sun":
			want[norm] = true
		}
	}
	if len(want) == 0 {
		return true
	}
	return want[strings.ToLower(now.Weekday().String())[:3]]
}

// submitJobGuard 提交时可靠性守卫（§6.1 去重 + §6.3 窗口）：返回非 nil 即
// 提交应被拒绝（含 HTTP 状态与文案，冲突附既有 job id）。
func (s *Server) submitJobGuard(p *jobCreatePayload) *submitFail {
	if reason := jobWindowBlockReason(time.Now(), s.jobWindow()); reason != "" {
		return &submitFail{http.StatusConflict, reason}
	}
	key := jobDedupKey(p.Type, p.Target, p.Parameters)
	dup, err := s.db.GetActiveJobByDedupKey(key)
	if err != nil {
		return &submitFail{http.StatusInternalServerError, "failed to check duplicate job"}
	}
	if dup != nil {
		return &submitFail{http.StatusConflict,
			fmt.Sprintf("duplicate job in flight: %s", dup.ID)}
	}
	return nil
}

// jobWindow 读取配置窗口（nil 安全）。
func (s *Server) jobWindow() *config.JobWindowConfig {
	if s.cfg == nil || s.cfg.Jobs == nil {
		return nil
	}
	return s.cfg.Jobs.Window
}

// handleJobRetry 人工重试（§6.2）：POST /api/jobs/{id}/retry。
// 仅 failed/cancelled 可重试；重试＝新 job（新 id，retried_from 溯源），
// 不原地改原 job；审计 operator/原 id/新 id。任意脚本重试亦仅人工显式，
// 不自动（决策项 31）。
func (s *Server) handleJobRetry(w http.ResponseWriter, r *http.Request, id string) {
	orig, err := s.db.GetJob(id)
	if err != nil {
		s.handleError(w, r, http.StatusNotFound, "job not found")
		return
	}
	if orig.Status != storage.JobStatusFailed && orig.Status != storage.JobStatusCancelled {
		s.handleError(w, r, http.StatusConflict,
			fmt.Sprintf("job is not retryable (status %s)", orig.Status))
		return
	}

	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}

	if reason := jobWindowBlockReason(time.Now(), s.jobWindow()); reason != "" {
		s.handleError(w, r, http.StatusConflict, reason)
		return
	}

	var params map[string]interface{}
	if orig.Parameters != "" {
		_ = json.Unmarshal([]byte(orig.Parameters), &params)
	}
	if params == nil {
		params = map[string]interface{}{}
	}

	// 重试用重算的去重键：旧 Job 可能无 dedup_key（feature 前建行），
	// 复用空串会与「无键」行互相去重；重算保证键恒有效
	retry := &storage.Job{
		ID:          protocol.GenerateID(),
		Type:        orig.Type,
		Target:      orig.Target,
		Actor:       username,
		Parameters:  orig.Parameters,
		Status:      storage.JobStatusPending,
		CreatedAt:   time.Now(),
		DedupKey:    jobDedupKey(orig.Type, orig.Target, params),
		RetriedFrom: orig.ID,
	}
	if err := s.db.CreateJob(retry); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to create retry job")
		return
	}

	s.audit.LogResource(username, audit.ActionJobRetry, audit.ResourceJob, retry.ID,
		map[string]interface{}{
			"type": retry.Type, "target": retry.Target,
			"originalJobId": orig.ID, "retriedFrom": orig.ID,
		}, s.getClientIP(r), r.UserAgent())

	view := toJobView(retry)
	s.goBg(func() {
		s.dispatchJobAsync(retry, jobCreatePayload{
			Type: retry.Type, Target: retry.Target, Parameters: params,
		}, username)
	})
	s.writeJSON(w, http.StatusCreated, view)
}
