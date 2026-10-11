// Package execlog agent 侧执行留痕（agent-core §2 能力 8）：每次执行动作
// 进内存环，攒批随 exec_log 消息上行，server 归集落审计表。发送按尽力而
// 为处置——该批失败即弃，绝不反压/阻塞业务执行路径；上游 core/report 自
// 带队列与重连，此处不做二次缓冲。
package execlog

import (
	"context"
	"log"
	"regexp"
	"strconv"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// DefaultCapacity 待上行缓冲上限：满丢最旧（留痕是观测面，不反压业务）。
const DefaultCapacity = 256

// errorClip 上行错误摘要截断（与 server 侧 detail 行宽同量级）。
const errorClip = 256

// valueClip 入参标量值截断。
const valueClip = 128

// sensitiveKeyRe 入参键命中即整条剔除（凭据纪律：口令/私钥/令牌不进留痕，
// 同 proxy 凭据「不落盘、不进日志/审计」口径）。
var sensitiveKeyRe = regexp.MustCompile(`(?i)pass|secret|token|credential|private`)

// Recorder 执行留痕记录器：并发安全，Record 任意 goroutine 可调。
type Recorder struct {
	mu      sync.Mutex
	pending []protocol.ExecLogEntry
	send    func(*protocol.Message) error
}

// New 创建记录器；send 为上行出口（生产=core/report Enqueue 适配，测试=捕获桩）。
func New(capacity int, send func(*protocol.Message) error) *Recorder {
	if capacity <= 0 {
		capacity = DefaultCapacity
	}
	return &Recorder{
		pending: make([]protocol.ExecLogEntry, 0, capacity),
		send:    send,
	}
}

// Record 追加一条留痕（ID/Time 缺省补齐）。
func (r *Recorder) Record(e protocol.ExecLogEntry) {
	if e.ID == "" {
		e.ID = protocol.GenerateID()
	}
	if e.Time == 0 {
		e.Time = time.Now().Unix()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.pending) >= cap(r.pending) {
		r.pending = r.pending[1:]
	}
	r.pending = append(r.pending, e)
}

// RecordCall 记一次执行调用（router 埋点专用）：状态由 callErr 推导，
// 入参过 Sanitize，error 摘要截断。
func (r *Recorder) RecordCall(providerName, action string, d time.Duration, callErr error, params map[string]interface{}) {
	e := protocol.ExecLogEntry{
		Provider:   providerName,
		Action:     action,
		Status:     protocol.ExecStatusSuccess,
		DurationMs: d.Milliseconds(),
		Params:     Sanitize(params),
	}
	if callErr != nil {
		e.Status = protocol.ExecStatusFailure
		e.Error = clip(callErr.Error(), errorClip)
	}
	r.Record(e)
}

// Flush 把缓冲批量上行（空批 no-op）。发送失败丢弃该批：执行留痕是观测
// 面，失败重试会在执行高峰放大队列压力，server 审计以「尽力而为」为契约。
func (r *Recorder) Flush() error {
	r.mu.Lock()
	if len(r.pending) == 0 {
		r.mu.Unlock()
		return nil
	}
	entries := r.pending
	r.pending = make([]protocol.ExecLogEntry, 0, cap(entries))
	r.mu.Unlock()
	return r.send(protocol.NewMessage(protocol.MessageTypeExecLog, map[string]interface{}{
		"entries": entries,
	}))
}

// Run 周期攒批上行；ctx 取消后做最后一次 Flush（停机不丢已留痕未上行批）。
func (r *Recorder) Run(ctx context.Context, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			if err := r.Flush(); err != nil {
				log.Printf("execlog final flush: %v", err)
			}
			return
		case <-t.C:
			if err := r.Flush(); err != nil {
				log.Printf("execlog flush: %v", err)
			}
		}
	}
}

// Sanitize 入参摘要：敏感键整条剔除；标量取值截断；复合值只记形状不展开
// （嵌套里可能藏凭据，展开等于绕过键名检查）。
func Sanitize(params map[string]interface{}) map[string]string {
	if len(params) == 0 {
		return nil
	}
	out := make(map[string]string, len(params))
	for k, v := range params {
		if sensitiveKeyRe.MatchString(k) {
			continue
		}
		switch x := v.(type) {
		case string:
			out[k] = clip(x, valueClip)
		case float64:
			out[k] = strconv.FormatFloat(x, 'g', -1, 64)
		case bool:
			out[k] = strconv.FormatBool(x)
		case nil:
			out[k] = ""
		default:
			out[k] = "…"
		}
	}
	return out
}

// clip 摘要截断（超长加 … 尾标）。
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
