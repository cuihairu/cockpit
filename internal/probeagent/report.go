package probeagent

import (
	"encoding/json"
	"os"
	"time"

	"github.com/cuihairu/cockpit/core/healthprobe"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// Alerter 告警出口（herald 留位，简档 §4.4）：状态定性迁移即调，同步
// 语义；推送通道对接另批实现。Alert 在探测 goroutine 上被 Guard 锁内
// 回调链同步调用（core/healthprobe/state.go 契约），实现不得同步回读
// Agent/Guard 状态——会自死锁；需要快照时经 channel 交异步 goroutine。
type Alerter interface {
	Alert(tr healthprobe.Transition)
}

// AlerterFunc 函数适配器（cmd 侧组合多出口用，同 http.HandlerFunc 惯例）。
type AlerterFunc func(tr healthprobe.Transition)

// Alert 实现 Alerter。
func (f AlerterFunc) Alert(tr healthprobe.Transition) { f(tr) }

// NopAlerter 缺省空出口。
type NopAlerter struct{}

// Alert 空实现。
func (NopAlerter) Alert(healthprobe.Transition) {}

// upstreamRecentWindows 每 target 随 probe_report 携带的已关窗口条数上限：
// 负载有界，更久回查以 server 库为准（本地环仍保 64）。
const upstreamRecentWindows = 8

// unixOrZero 零时刻 → 0（上行时刻统一 Unix 秒，0 表示「无」，与心跳
// startedAt 约定一致；直接 Unix() 会得到 year-1 的巨大负数）。
func unixOrZero(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.Unix()
}

// UpstreamMessage 组装 probe_report 消息（简档 §4.2）：全量目标快照 +
// 每 target 进行中窗口与近期已关窗口。状态迁移即发与周期兜底都发全量
// payload——server 侧 upsert 幂等，重复上报安全。
func (a *Agent) UpstreamMessage() *protocol.Message {
	targets := make([]map[string]interface{}, 0, len(a.guards))
	windows := make([]map[string]interface{}, 0)
	for _, s := range a.Snapshots() {
		targets = append(targets, map[string]interface{}{
			"name":         s.Target,
			"state":        string(s.State),
			"since":        unixOrZero(s.Since),
			"last_checked": unixOrZero(s.LastChecked),
			"last_error":   s.LastError,
		})
		if s.Open != nil {
			windows = append(windows, windowPayload(s.Open.Target,
				s.Open.StartedAt, s.Open.EndedAt, s.Open.LastError))
		}
		recent := s.Recent
		if len(recent) > upstreamRecentWindows {
			recent = recent[len(recent)-upstreamRecentWindows:]
		}
		for i := range recent {
			windows = append(windows, windowPayload(recent[i].Target,
				recent[i].StartedAt, recent[i].EndedAt, recent[i].LastError))
		}
	}
	return protocol.NewMessage(protocol.MessageTypeProbeReport, map[string]interface{}{
		"targets": targets,
		"windows": windows,
	})
}

// windowPayload 单窗口上行项。
func windowPayload(target string, startedAt, endedAt time.Time, lastError string) map[string]interface{} {
	return map[string]interface{}{
		"target":     target,
		"started_at": unixOrZero(startedAt),
		"ended_at":   unixOrZero(endedAt),
		"last_error": lastError,
	}
}

// WriteStatusFile 观测快照落 JSON（-status-file，简档 §4.1）：tmp+rename
// 原子替换，读侧永远看到完整文件。
func WriteStatusFile(path string, snaps []healthprobe.Snapshot) error {
	b, err := json.MarshalIndent(snaps, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
