package server

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// DDNS 定时巡检（设计见 docs/guide/ddns-design.md D6/D7/D8）：
// 模式与 drift/smart 同构（每分钟醒来对比间隔，Setting 动态可改，
// 0 = 关闭）。每轮逐条 enabled 配置：绑定的 agent 探测公网 IP →
// 与 Cloudflare 记录比对，缺失则建、变化则改 → 回写状态，失败告警
// （真去重）。成功更新不告警（静默自愈）。

const DDNSIntervalSettingKey = "ddns.scan_interval_seconds"

const (
	ddnsMinIntervalSeconds = 60
	ddnsMaxIntervalSeconds = 86400
	ddnsDefaultInterval    = 300
)

var (
	ddnsScanTick      = time.Minute      // 醒来对比间隔的节奏
	ddnsScanStartWait = 30 * time.Second // 启动后等 agent 上线再首扫
)

var errDDNSIntervalRange = fmt.Errorf("scan_interval_seconds out of range [%d, %d]",
	ddnsMinIntervalSeconds, ddnsMaxIntervalSeconds)

// GetDDNSScanInterval 读巡检间隔；未配置/非法用默认值（0 合法 = 关闭）
func (s *Server) GetDDNSScanInterval() int {
	v, err := s.db.GetSetting(DDNSIntervalSettingKey)
	if err != nil || v == "" {
		return ddnsDefaultInterval
	}
	n, convErr := strconv.Atoi(v)
	if convErr != nil || (n != 0 && (n < ddnsMinIntervalSeconds || n > ddnsMaxIntervalSeconds)) {
		return ddnsDefaultInterval
	}
	return n
}

// SetDDNSScanInterval 校验并写入巡检间隔（0 = 关闭，合法）
func (s *Server) SetDDNSScanInterval(seconds int) error {
	if seconds != 0 && (seconds < ddnsMinIntervalSeconds || seconds > ddnsMaxIntervalSeconds) {
		return errDDNSIntervalRange
	}
	return s.db.SetSetting(DDNSIntervalSettingKey, strconv.Itoa(seconds))
}

// ddnsScanLoop 定时巡检循环（每分钟醒来对比间隔，间隔可动态改）
func (s *Server) ddnsScanLoop() {
	time.Sleep(ddnsScanStartWait)
	ticker := time.NewTicker(ddnsScanTick)
	defer ticker.Stop()

	var lastScan time.Time
	for {
		select {
		case <-ticker.C:
			interval := s.GetDDNSScanInterval()
			if interval <= 0 {
				continue // 关闭巡检
			}
			if !lastScan.IsZero() && time.Since(lastScan) < time.Duration(interval)*time.Second {
				continue
			}
			lastScan = time.Now()
			s.scanDDNSOnce()
		case <-s.ctx.Done():
			return
		}
	}
}

// scanDDNSOnce 扫一轮：全部 enabled 配置；同 zone+type 的记录查询
// 共享一次 ListRecords 分页拉取（D7 配额保护）
func (s *Server) scanDDNSOnce() {
	cfgs, err := s.db.ListDDNSConfigs()
	if err != nil || len(cfgs) == 0 {
		return
	}
	var notifCfg *config.NotificationConfig
	if s.cfg != nil {
		notifCfg = s.cfg.Notification
	}
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	recordCache := map[string][]dns.Record{}
	checked, updated := 0, 0
	for _, cfg := range cfgs {
		if !cfg.Enabled {
			continue
		}
		_, changed := s.runDDNSCheck(cfg, s.dns, recordCache, generator)
		checked++
		if changed {
			updated++
		}
	}
	if checked > 0 {
		log.Printf("DDNS scan completed: %d configs checked, %d updated", checked, updated)
	}
}

// runDDNSCheck 单条检查全流程：执行比对并回写状态、失败入告警。
// provider 与记录缓存由调用方注入（巡检共享缓存，REST check 单独拉取）。
// 返回 (同步后的 IP, 是否发生了 DNS 写入)。
func (s *Server) runDDNSCheck(cfg *storage.DDNSConfig, provider dns.Provider,
	recordCache map[string][]dns.Record, generator *alert.Generator) (string, bool) {

	ip, changed, err := s.checkDDNSConfig(cfg, provider, recordCache)
	cfg.CheckedAt = time.Now().Unix()
	if err != nil {
		cfg.LastStatus = "failed"
		cfg.LastError = err.Error()
		_ = s.db.UpdateDDNSConfig(cfg)
		generator.CheckDDNS(cfg.RecordName, err.Error())
		return "", changed
	}
	cfg.LastIP = ip
	cfg.LastStatus = "ok"
	cfg.LastError = ""
	_ = s.db.UpdateDDNSConfig(cfg)
	return ip, changed
}

// checkDDNSConfig 比对核心：agent 报 IP → Cloudflare 记录缺失则建、
// 变化则改（D6）。返回 (当前公网 IP, 是否写入了 DNS, 错误)。
func (s *Server) checkDDNSConfig(cfg *storage.DDNSConfig, provider dns.Provider,
	recordCache map[string][]dns.Record) (string, bool, error) {

	if provider == nil {
		return "", false, errors.New("DNS provider not configured")
	}
	if _, ok := s.registry.Get(cfg.AgentID); !ok {
		return "", false, errors.New("agent offline")
	}

	ip, err := s.fetchAgentIP(cfg.AgentID, cfg.Type)
	if err != nil {
		return "", false, err
	}
	if ip == "" {
		return "", false, fmt.Errorf("agent 未返回 %s 公网地址（主机可能没有该地址族）", cfg.Type)
	}

	rec, err := s.findDDNSRecord(provider, cfg, recordCache)
	if err != nil {
		return ip, false, fmt.Errorf("查询 DNS 记录失败: %w", err)
	}

	if rec == nil {
		// 记录缺失 → 创建（TTL auto，不开代理：DDNS 需要真实解析）
		input := dns.RecordInput{Type: cfg.Type, Name: cfg.RecordName, Content: ip}
		if _, err := provider.CreateRecord(context.Background(), cfg.ZoneID, input); err != nil {
			return ip, false, fmt.Errorf("创建 DNS 记录失败: %w", err)
		}
		return ip, true, nil
	}
	if rec.Content == ip {
		return ip, false, nil // 无变化，安静轮（D7：不写 API）
	}
	// 更新保留原记录的 TTL/Proxied（不破坏用户手动设置）
	input := dns.RecordInput{Type: cfg.Type, Name: cfg.RecordName, Content: ip, TTL: rec.TTL, Proxied: rec.Proxied}
	if _, err := provider.UpdateRecord(context.Background(), cfg.ZoneID, rec.ID, input); err != nil {
		return ip, false, fmt.Errorf("更新 DNS 记录失败: %w", err)
	}
	return ip, true, nil
}

// fetchAgentIP 调 agent ddns.ip 并取出 cfg.Type 对应地址族的 IP
func (s *Server) fetchAgentIP(agentID, recordType string) (string, error) {
	resp, err := s.CallAgent(agentID, "ddns.ip", nil)
	if err != nil {
		return "", err
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return "", err
	}
	if rpcResp.Status == "error" {
		return "", errors.New(rpcResp.Error)
	}
	// DecodePayload 已对同一 payload 整体 Marshal 过，合法 JSON 解出的
	// 值重新 Marshal 恒成功
	raw, _ := json.Marshal(rpcResp.Data)
	var payload struct {
		IPv4 string `json:"ipv4"`
		IPv6 string `json:"ipv6"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", err
	}
	if recordType == "AAAA" {
		return payload.IPv6, nil
	}
	return payload.IPv4, nil
}

// findDDNSRecord 在 zone 下找 name/type 匹配的记录（分页拉全，同
// zone+type 共享缓存，D7）。返回 nil 表示记录缺失。
func (s *Server) findDDNSRecord(provider dns.Provider, cfg *storage.DDNSConfig,
	recordCache map[string][]dns.Record) (*dns.Record, error) {

	key := cfg.ZoneID + "|" + cfg.Type
	records, ok := recordCache[key]
	if !ok {
		for page := 1; ; page++ {
			rp, err := provider.ListRecords(context.Background(), cfg.ZoneID, cfg.Type, page)
			if err != nil {
				return nil, err
			}
			records = append(records, rp.Records...)
			if page >= rp.TotalPage {
				break
			}
		}
		recordCache[key] = records
	}
	for i := range records {
		if strings.EqualFold(records[i].Name, cfg.RecordName) {
			return &records[i], nil
		}
	}
	return nil, nil
}
