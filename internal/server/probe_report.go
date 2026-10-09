package server

import (
	"encoding/json"
	"log"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// probeTargetReport probe_report 负载项（与 probeagent 上行结构一一对应）。
type probeTargetReport struct {
	Name        string `json:"name"`
	State       string `json:"state"`
	Since       int64  `json:"since"`
	LastChecked int64  `json:"last_checked"`
	LastError   string `json:"last_error"`
}

// probeWindowReport probe_report 窗口负载项（ended_at 0 = 仍在故障中）。
type probeWindowReport struct {
	Target    string `json:"target"`
	StartedAt int64  `json:"started_at"`
	EndedAt   int64  `json:"ended_at"`
	LastError string `json:"last_error"`
}

// probeReportPayload probe_report 负载（简档 §4.2）。
type probeReportPayload struct {
	Targets []probeTargetReport `json:"targets"`
	Windows []probeWindowReport `json:"windows"`
}

// handleProbeReport 处理探针 agent 的 probe_report：目标快照 upsert +
// 故障窗口 upsert（均幂等，重复上报安全；agent 重发全量不产生重复行）。
func (s *Server) handleProbeReport(agent *Agent, msg *protocol.Message) {
	if s.db == nil {
		return
	}
	p, err := decodeProbeReport(msg)
	if err != nil {
		log.Printf("probe_report decode failed from agent %s: %v", agent.ID, err)
		return
	}

	targets := make([]storage.ProbeTargetSnapshot, 0, len(p.Targets))
	for _, t := range p.Targets {
		targets = append(targets, storage.ProbeTargetSnapshot{
			Target:      t.Name,
			State:       t.State,
			Since:       probeUnixToTime(t.Since),
			LastChecked: probeUnixToTime(t.LastChecked),
			LastError:   t.LastError,
		})
	}
	if err := s.db.UpsertProbeTargets(agent.ID, targets); err != nil {
		// 单表失败不堵另一表：窗口与快照互不依赖
		log.Printf("probe targets upsert failed for agent %s: %v", agent.ID, err)
	}

	windows := make([]storage.ProbeWindow, 0, len(p.Windows))
	for _, w := range p.Windows {
		var ended *time.Time
		if w.EndedAt > 0 {
			t := probeUnixToTime(w.EndedAt)
			ended = &t
		}
		windows = append(windows, storage.ProbeWindow{
			Target:    w.Target,
			StartedAt: probeUnixToTime(w.StartedAt),
			EndedAt:   ended,
			LastError: w.LastError,
		})
	}
	if err := s.db.UpsertProbeWindows(agent.ID, windows); err != nil {
		log.Printf("probe windows upsert failed for agent %s: %v", agent.ID, err)
	}
}

// decodeProbeReport 解码 probe_report 负载（payload 缺失 = 空上报，合法）。
func decodeProbeReport(msg *protocol.Message) (probeReportPayload, error) {
	var p probeReportPayload
	if msg.Payload == nil {
		return p, nil
	}
	raw, err := json.Marshal(msg.Payload)
	if err != nil {
		// payload 来自 JSON 解码（值恒为 string/number/bool/nil/map/slice），
		// marshal 不可失败——防御登记见 tool/known_uncoverable.txt
		return p, err
	}
	err = json.Unmarshal(raw, &p)
	return p, err
}

// probeUnixToTime 上行 Unix 秒 → time.Time（0 = 零时刻，对应 agent 侧
// 「无」；直接 time.Unix(0,0) 会得到 1970-01-01）。
func probeUnixToTime(sec int64) time.Time {
	if sec <= 0 {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
