package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// NAS 存储定时巡检（设计见 docs/guide/nas-design.md D5），模式与 smart_scan.go
// 同构：每分钟醒来对比间隔（Setting 键动态可改，0 = 关闭）→ 全部带 nas
// capability 的在线 agent → nas.status → 池异常与容量超阈值入告警（真去重）。
// 不落库：NAS 本机是唯一事实源，巡检只产出告警。

const (
	NASIntervalSettingKey  = "nas.scan_interval_seconds"
	NASUsageWarnSettingKey = "nas.usage_warn_percent"
	nasMinIntervalSeconds  = 300
	nasMaxIntervalSeconds  = 86400
	nasDefaultInterval     = 1800
	nasMinUsageWarnPercent = 50
	nasMaxUsageWarnPercent = 99
	nasDefaultUsageWarn    = 80
)

var (
	nasScanTick      = time.Minute      // 醒来对比间隔的节奏
	nasScanStartWait = 90 * time.Second // 启动后等 agent 上线再首扫
)

var (
	errNASIntervalRange = fmt.Errorf("scan_interval_seconds out of range [%d, %d]",
		nasMinIntervalSeconds, nasMaxIntervalSeconds)
	errNASUsageRange = fmt.Errorf("usage_warn_percent out of range [%d, %d]",
		nasMinUsageWarnPercent, nasMaxUsageWarnPercent)
)

// GetNASScanInterval 读巡检间隔；未配置/非法用默认值（0 合法 = 关闭）
func (s *Server) GetNASScanInterval() int {
	v, err := s.db.GetSetting(NASIntervalSettingKey)
	if err != nil || v == "" {
		return nasDefaultInterval
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || (n != 0 && (n < nasMinIntervalSeconds || n > nasMaxIntervalSeconds)) {
		return nasDefaultInterval
	}
	return n
}

// SetNASScanInterval 校验并写入巡检间隔（0 = 关闭，合法）
func (s *Server) SetNASScanInterval(seconds int) error {
	if seconds != 0 && (seconds < nasMinIntervalSeconds || seconds > nasMaxIntervalSeconds) {
		return errNASIntervalRange
	}
	return s.db.SetSetting(NASIntervalSettingKey, strconv.Itoa(seconds))
}

// GetNASUsageWarnPercent 读容量告警阈值；未配置/非法用默认值
func (s *Server) GetNASUsageWarnPercent() int {
	v, err := s.db.GetSetting(NASUsageWarnSettingKey)
	if err != nil || v == "" {
		return nasDefaultUsageWarn
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || n < nasMinUsageWarnPercent || n > nasMaxUsageWarnPercent {
		return nasDefaultUsageWarn
	}
	return n
}

// SetNASUsageWarnPercent 校验并写入容量告警阈值
func (s *Server) SetNASUsageWarnPercent(p int) error {
	if p < nasMinUsageWarnPercent || p > nasMaxUsageWarnPercent {
		return errNASUsageRange
	}
	return s.db.SetSetting(NASUsageWarnSettingKey, strconv.Itoa(p))
}

// nasScanLoop 定时巡检循环（每分钟醒来对比间隔，间隔可动态改）
func (s *Server) nasScanLoop() {
	time.Sleep(nasScanStartWait)
	ticker := time.NewTicker(nasScanTick)
	defer ticker.Stop()

	var lastScan time.Time
	for {
		select {
		case <-ticker.C:
			interval := s.GetNASScanInterval()
			if interval <= 0 {
				continue // 关闭巡检
			}
			if !lastScan.IsZero() && time.Since(lastScan) < time.Duration(interval)*time.Second {
				continue
			}
			lastScan = time.Now()
			s.scanNASOnce()
		case <-s.ctx.Done():
			return
		}
	}
}

// nasScanSnapshot 巡检消费的快照（nas.status 返回子集）
type nasScanSnapshot struct {
	Pools []nasScanPool  `json:"pools"`
	Usage []nasScanMount `json:"mounts"`
}

type nasScanPool struct {
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	State  string `json:"state"`
	Detail string `json:"detail"`
}

type nasScanMount struct {
	Device    string  `json:"device"`
	MountPath string  `json:"mountPath"`
	TotalGB   float64 `json:"totalGB"`
	UsedGB    float64 `json:"usedGB"`
}

// scanNASOnce 扫一轮：带 nas capability 的在线 agent
func (s *Server) scanNASOnce() {
	agents := s.registry.ListByCapability("nas")
	if len(agents) == 0 {
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	warnPct := s.GetNASUsageWarnPercent()

	scanned, flagged := 0, 0
	for _, agent := range agents {
		if !agent.IsOnline(s.registry.timeouts.Heartbeat) {
			continue
		}
		snap, err := s.fetchNASSnapshot(agent.ID)
		if err != nil {
			log.Printf("NAS scan skip agent %s: %v", agent.ID, err)
			continue
		}
		scanned++
		hostname := agent.Hostname
		if hostname == "" {
			hostname = agent.ID
		}
		hasFlag := false
		for _, pool := range snap.Pools {
			switch pool.State {
			case "failed":
				generator.CheckNasPool(agent.ID, hostname, pool.Name, pool.State, pool.Detail)
				hasFlag = true
			case "degraded", "resync":
				// unknown 不告警（观测缺失≠故障）；degraded/resync 是真实异常
				generator.CheckNasPool(agent.ID, hostname, pool.Name, pool.State, pool.Detail)
				hasFlag = true
			}
		}
		for _, m := range snap.Usage {
			if m.TotalGB <= 0 {
				continue
			}
			usedPct := int(m.UsedGB / m.TotalGB * 100)
			if usedPct >= warnPct {
				generator.CheckNasUsage(agent.ID, hostname, m.MountPath, usedPct)
				hasFlag = true
			}
		}
		if hasFlag {
			flagged++
		}
	}
	if scanned > 0 {
		log.Printf("NAS scan completed: %d agents scanned, %d flagged", scanned, flagged)
	}
}

// fetchNASSnapshot 调 agent nas.status 并解出巡检消费字段
func (s *Server) fetchNASSnapshot(agentID string) (*nasScanSnapshot, error) {
	resp, err := s.CallAgent(agentID, "nas.status", nil)
	if err != nil {
		return nil, err
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return nil, err
	}
	if rpcResp.Status == "error" {
		return nil, errors.New(rpcResp.Error)
	}
	raw, err := json.Marshal(rpcResp.Data)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Available bool           `json:"available"`
		Pools     []nasScanPool  `json:"pools"`
		Mounts    []nasScanMount `json:"mounts"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if !payload.Available {
		return nil, errors.New("nas observation unavailable")
	}
	return &nasScanSnapshot{Pools: payload.Pools, Usage: payload.Mounts}, nil
}
