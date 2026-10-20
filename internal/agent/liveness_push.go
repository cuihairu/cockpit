package agent

import (
	"time"

	"github.com/cuihairu/cockpit/internal/oob/liveness"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 带外探活调度（OpenWrt 自举节点片B，docs/design/oob-openwrt-bootstrap.md）：
// agent 侧周期对目标清单跑三源探测，结果以 liveness_report 推送上行。
// 调度器本体在 internal/oob/liveness（纯组件，无 agent 依赖）。

// newLivenessScheduler 组装探活调度：未配置目标清单返回 nil（不装配，不启动
// goroutine）。全局端口清单回填给未自带端口的目标（自带端口的目标优先）。
func newLivenessScheduler(cfg Config, send func(*protocol.Message) error) *liveness.Scheduler {
	if len(cfg.LivenessTargets) == 0 {
		return nil
	}
	s := &liveness.Scheduler{
		Targets: cfg.LivenessTargets,
		OnCycle: func(reports []liveness.Report) {
			_ = send(buildLivenessReport(reports))
		},
	}
	if cfg.LivenessIntervalSeconds > 0 {
		s.Interval = time.Duration(cfg.LivenessIntervalSeconds) * time.Second
	}
	ports := make([]int, len(cfg.LivenessPorts))
	copy(ports, cfg.LivenessPorts)
	for i := range s.Targets {
		if len(s.Targets[i].Ports) == 0 {
			s.Targets[i].Ports = ports
		}
	}
	return s
}

// buildLivenessReport 组上报送报文：每个目标一项，三源判定逐源带 verdict
// 与证据描述（空描述省略）。
func buildLivenessReport(reports []liveness.Report) *protocol.Message {
	items := make([]map[string]interface{}, 0, len(reports))
	for _, r := range reports {
		items = append(items, map[string]interface{}{
			"ip":   r.IP,
			"arp":  sourcePayload(r.ARP),
			"icmp": sourcePayload(r.ICMP),
			"tcp":  sourcePayload(r.TCP),
			"at":   r.At,
		})
	}
	return protocol.NewMessage(protocol.MessageTypeLivenessReport, map[string]interface{}{
		"reports": items,
	})
}

// sourcePayload 单源上行项（Detail 为空时不带键，避免空串噪音）。
func sourcePayload(s liveness.Source) map[string]interface{} {
	out := map[string]interface{}{"verdict": string(s.Verdict)}
	if s.Detail != "" {
		out["detail"] = s.Detail
	}
	return out
}
