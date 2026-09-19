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

// 漂移定时巡检（见 docs/guide/drift-design.md M2/D13-D17）：
// server 侧定时对在线且带 drift capability 的 agent 执行 drift.check，
// 发现 drifted/missing 即产生汇总告警（真去重 + 非阻塞通知）。结果不落库。

// DriftIntervalSettingKey 巡检间隔 Setting 键；0 = 关闭，默认 30 分钟
const DriftIntervalSettingKey = "drift.scan_interval_seconds"

const (
	driftMinIntervalSeconds = 0
	driftMaxIntervalSeconds = 86400
	driftDefaultInterval    = 1800
)

// 巡检循环节奏。包级变量仅为测试可注入，默认值即生产取值。
var (
	driftScanTick      = time.Minute      // 醒来对比间隔的节奏
	driftScanStartWait = 90 * time.Second // 启动后等 agent 上线再首扫
)

var errDriftIntervalRange = fmt.Errorf("scan_interval_seconds out of range [%d, %d]",
	driftMinIntervalSeconds, driftMaxIntervalSeconds)

// GetDriftScanInterval 读巡检间隔；未配置/非法用默认值（0 合法 = 关闭）
func (s *Server) GetDriftScanInterval() int {
	v, err := s.db.GetSetting(DriftIntervalSettingKey)
	if err != nil || v == "" {
		return driftDefaultInterval
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || n < driftMinIntervalSeconds || n > driftMaxIntervalSeconds {
		return driftDefaultInterval
	}
	return n
}

// SetDriftScanInterval 校验并写入巡检间隔
func (s *Server) SetDriftScanInterval(seconds int) error {
	if seconds < driftMinIntervalSeconds || seconds > driftMaxIntervalSeconds {
		return errDriftIntervalRange
	}
	return s.db.SetSetting(DriftIntervalSettingKey, strconv.Itoa(seconds))
}

// driftScanItem drift.check 结果单条（巡检消费的最小字段）
type driftScanItem struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// driftScanLoop 定时巡检循环（每分钟醒来对比间隔，间隔可动态改）
func (s *Server) driftScanLoop() {
	time.Sleep(driftScanStartWait)
	ticker := time.NewTicker(driftScanTick)
	defer ticker.Stop()

	var lastScan time.Time
	for {
		select {
		case <-ticker.C:
			interval := s.GetDriftScanInterval()
			if interval <= 0 {
				continue // 关闭巡检
			}
			if !lastScan.IsZero() && time.Since(lastScan) < time.Duration(interval)*time.Second {
				continue
			}
			lastScan = time.Now()
			s.scanDriftOnce()
		case <-s.ctx.Done():
			return
		}
	}
}

// scanDriftOnce 扫一轮：全部带 drift capability 的在线 agent
func (s *Server) scanDriftOnce() {
	agents := s.registry.ListByCapability("drift")
	if len(agents) == 0 {
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	scanned, driftedAgents := 0, 0
	for _, agent := range agents {
		items, err := s.fetchDriftItems(agent.ID)
		if err != nil {
			log.Printf("Drift scan skip agent %s: %v", agent.ID, err)
			continue
		}
		scanned++
		var drifted []string
		for _, it := range items {
			switch it.Status {
			case "drifted", "missing":
				drifted = append(drifted, it.Kind+"/"+it.Name+"（"+it.Status+"）")
			case "error":
				// 读取失败可能是权限问题，只记日志不告警（D15）
				log.Printf("Drift scan agent %s: item %s/%s unreadable", agent.ID, it.Kind, it.Name)
			}
		}
		if len(drifted) == 0 {
			continue
		}
		driftedAgents++
		hostname := agent.Hostname
		if hostname == "" {
			hostname = agent.ID
		}
		generator.CheckDriftScan(agent.ID, hostname, drifted)
	}
	if scanned > 0 {
		log.Printf("Drift scan completed: %d agents scanned, %d with drift", scanned, driftedAgents)
	}
}

// fetchDriftItems 调 agent drift.check 并解出 items（错误响应归一为 error）
func (s *Server) fetchDriftItems(agentID string) ([]driftScanItem, error) {
	resp, err := s.CallAgent(agentID, "drift.check", nil)
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
		Items []driftScanItem `json:"items"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}
