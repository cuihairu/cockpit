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

// SMART 磁盘健康定时巡检（设计见 docs/guide/disk-health-design.md D8/D9），
// 模式与 drift_scan.go 同构：每分钟醒来对比间隔（Setting 键动态可改，
// 0 = 关闭）→ 全部带 hardware-monitor 且 metadata.smart 的在线 agent
// → smart.status → 异常盘入告警（真去重）。

const SmartIntervalSettingKey = "smart.scan_interval_seconds"

const (
	smartMinIntervalSeconds = 300
	smartMaxIntervalSeconds = 86400
	smartDefaultInterval    = 3600
)

var (
	smartScanTick      = time.Minute      // 醒来对比间隔的节奏
	smartScanStartWait = 90 * time.Second // 启动后等 agent 上线再首扫
)

var errSmartIntervalRange = fmt.Errorf("scan_interval_seconds out of range [%d, %d]",
	smartMinIntervalSeconds, smartMaxIntervalSeconds)

// GetSmartScanInterval 读巡检间隔；未配置/非法用默认值（0 合法 = 关闭）
func (s *Server) GetSmartScanInterval() int {
	v, err := s.db.GetSetting(SmartIntervalSettingKey)
	if err != nil || v == "" {
		return smartDefaultInterval
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || (n != 0 && (n < smartMinIntervalSeconds || n > smartMaxIntervalSeconds)) {
		return smartDefaultInterval
	}
	return n
}

// SetSmartScanInterval 校验并写入巡检间隔（0 = 关闭，合法）
func (s *Server) SetSmartScanInterval(seconds int) error {
	if seconds != 0 && (seconds < smartMinIntervalSeconds || seconds > smartMaxIntervalSeconds) {
		return errSmartIntervalRange
	}
	return s.db.SetSetting(SmartIntervalSettingKey, strconv.Itoa(seconds))
}

// smartScanDevice 巡检消费的盘读数（smart.status devices[] 子集）
type smartScanDevice struct {
	Name               string  `json:"name"`
	Health             string  `json:"health"`
	ReallocatedSectors *uint64 `json:"reallocatedSectors,omitempty"`
	PendingSectors     *uint64 `json:"pendingSectors,omitempty"`
	MediaErrors        *uint64 `json:"mediaErrors,omitempty"`
}

// agentHasSmart 该 agent 的 hardware-monitor capability 是否带 smart 标志（D1）
func agentHasSmart(a *Agent) bool {
	for _, c := range a.GetCapabilities() {
		if c.Type == "hardware-monitor" {
			v, ok := c.Metadata["smart"].(bool)
			return ok && v
		}
	}
	return false
}

// smartScanLoop 定时巡检循环（每分钟醒来对比间隔，间隔可动态改）
func (s *Server) smartScanLoop() {
	time.Sleep(smartScanStartWait)
	ticker := time.NewTicker(smartScanTick)
	defer ticker.Stop()

	var lastScan time.Time
	for {
		select {
		case <-ticker.C:
			interval := s.GetSmartScanInterval()
			if interval <= 0 {
				continue // 关闭巡检
			}
			if !lastScan.IsZero() && time.Since(lastScan) < time.Duration(interval)*time.Second {
				continue
			}
			lastScan = time.Now()
			s.scanSmartOnce()
		case <-s.ctx.Done():
			return
		}
	}
}

// scanSmartOnce 扫一轮：带 hardware-monitor ∩ metadata.smart 的在线 agent
func (s *Server) scanSmartOnce() {
	agents := s.registry.ListByCapability("hardware-monitor")
	if len(agents) == 0 {
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	scanned, flagged := 0, 0
	for _, agent := range agents {
		if !agent.IsOnline(s.registry.timeouts.Heartbeat) || !agentHasSmart(agent) {
			continue
		}
		devices, err := s.fetchSmartDevices(agent.ID)
		if err != nil {
			log.Printf("Smart scan skip agent %s: %v", agent.ID, err)
			continue
		}
		scanned++
		var issues []string
		hasFailed := false
		for _, d := range devices {
			switch d.Health {
			case "failed":
				hasFailed = true
				issues = append(issues, d.Name+"：SMART 判定 FAILED，建议立即备份并更换")
			case "passed":
				if d.ReallocatedSectors != nil && *d.ReallocatedSectors > 0 {
					issues = append(issues, fmt.Sprintf("%s：重映射扇区 %d", d.Name, *d.ReallocatedSectors))
				}
				if d.PendingSectors != nil && *d.PendingSectors > 0 {
					issues = append(issues, fmt.Sprintf("%s：待定扇区 %d", d.Name, *d.PendingSectors))
				}
				if d.MediaErrors != nil && *d.MediaErrors > 0 {
					issues = append(issues, fmt.Sprintf("%s：介质错误 %d", d.Name, *d.MediaErrors))
				}
			default:
				// unknown（权限不足/盘不支持 SMART）只记日志不告警（D5/D9）
				log.Printf("Smart scan agent %s: disk %s health unknown", agent.ID, d.Name)
			}
		}
		if len(issues) == 0 {
			continue
		}
		flagged++
		hostname := agent.Hostname
		if hostname == "" {
			hostname = agent.ID
		}
		generator.CheckDiskHealth(agent.ID, hostname, hasFailed, issues)
	}
	if scanned > 0 {
		log.Printf("Smart scan completed: %d agents scanned, %d flagged", scanned, flagged)
	}
}

// fetchSmartDevices 调 agent smart.status 并解出 devices
func (s *Server) fetchSmartDevices(agentID string) ([]smartScanDevice, error) {
	resp, err := s.CallAgent(agentID, "hardware-monitor.status", nil)
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
	// DecodePayload 已对同一 payload 整体 Marshal 过，重新 Marshal 恒成功
	raw, _ := json.Marshal(rpcResp.Data)
	var payload struct {
		Available bool              `json:"available"`
		Devices   []smartScanDevice `json:"devices"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if !payload.Available {
		return nil, errors.New("smartctl unavailable")
	}
	return payload.Devices, nil
}
