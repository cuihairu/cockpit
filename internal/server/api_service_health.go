package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/auth"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 服务健康探针与自愈 API（设计见 docs/guide/service-health-design.md）。
// 挂在 /api/agents/{id}/health 下（serveAPI 的 /agents/ 分支转发到这里）。
//
//	GET  /api/agents/{id}/health                        概览：配置 + 运行态（在线实时拉取，离线回落缓存灰态）
//	PUT  /api/agents/{id}/health                        保存探针/白名单（审计 service_health_config；在线则推送，离线待注册补推）
//	POST /api/agents/{id}/health/probes/{probeId}/check 立即探测一次（转发 service.health.now，读语义不审计）
//
// server 是配置唯一事实源（D3，Setting KV），agent 只持运行副本；校验
// 规则与 agent 侧 service_health.go 同源（D12 双端同规则），另加 D6 白名单
// 门：heal 目标不在白名单直接 400，文案指引改 heal=false 只告警。

const (
	// ServiceHealthConfigPrefix 探针配置 Setting 键前缀（+agentID）
	ServiceHealthConfigPrefix = "service_health.config."
	// ServiceHealthStatePrefix 运行态缓存 Setting 键前缀（+agentID，离线灰态）
	ServiceHealthStatePrefix = "service_health.state."
	// healthConfigBodyLimit PUT 请求体上限（与配置体积上限同量级再加余量）
	healthConfigBodyLimit = 16 << 10
	// healthMaxConfigBytes 序列化后配置体积上限（Setting.Value 4096 字节，
	// 留余量；32 条探针全量字段约 10KB，超限属配置异常）
	healthMaxConfigBytes = 3500
)

// healthProbeCfg 单条探针定义（镜像 agent 侧 rpc.HealthProbe 的 JSON 形态，
// server 不 import agent 包——双端只共享协议，与 smartScanDevice 同纪律）
type healthProbeCfg struct {
	ID                  string `json:"id"`
	Type                string `json:"type"`
	Target              string `json:"target"`
	ExpectStatus        int    `json:"expectStatus"`
	IntervalSec         int    `json:"intervalSec"`
	TimeoutSec          int    `json:"timeoutSec"`
	FailThreshold       int    `json:"failThreshold"`
	Heal                bool   `json:"heal"`
	Unit                string `json:"unit"`
	BackoffWindowSec    int    `json:"backoffWindowSec"`
	MaxRestartsInWindow int    `json:"maxRestartsInWindow"`
}

// healthConfigRecord 探针配置存储记录（含审计元数据）
type healthConfigRecord struct {
	Probes    []healthProbeCfg `json:"probes"`
	Whitelist []string         `json:"whitelist"`
	UpdatedAt int64            `json:"updatedAt"`
	UpdatedBy string           `json:"updatedBy"`
}

// healthHealRecView 最近自愈决策（镜像 agent 侧 healthHealRec）
type healthHealRecView struct {
	Time       int64  `json:"time"`
	Unit       string `json:"unit"`
	Result     string `json:"result"`
	Detail     string `json:"detail"`
	DurationMs int64  `json:"durationMs"`
}

// healthStateEntry 单探针运行态（镜像 agent 侧 healthProbeState）
type healthStateEntry struct {
	Status           string             `json:"status"`
	LastCheck        int64              `json:"lastCheck"`
	ConsecutiveFails int                `json:"consecutiveFails"`
	LastError        string             `json:"lastError"`
	LastHeal         *healthHealRecView `json:"lastHeal"`
}

// healthStateCache 运行态缓存（离线灰态 + GET 回落）
type healthStateCache struct {
	States    map[string]healthStateEntry `json:"states"`
	UpdatedAt int64                       `json:"updatedAt"`
}

// healthProbeIDRe 探针 id 白名单（agent 侧 healthProbeIDRe 同规则）
var healthProbeIDRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

// validateHealthProbes server 侧配置校验（D12 + D6）：缺省补齐 + 形状/
// 范围白名单 + heal 目标必须在白名单内。与 agent 侧 validateHealthConfig
// 同规则——规则简单（十几条范围判断），双端各自维护不抽公共包。
func validateHealthProbes(probes []healthProbeCfg, whitelist map[string]bool) error {
	if len(probes) == 0 {
		return fmt.Errorf("probes must not be empty")
	}
	if len(probes) > 32 {
		return fmt.Errorf("too many probes (%d > 32)", len(probes))
	}
	seen := map[string]bool{}
	for i := range probes {
		p := &probes[i]
		if !healthProbeIDRe.MatchString(p.ID) {
			return fmt.Errorf("probe[%d]: invalid id %q (expect lowercase alnum/hyphen, <=64)", i, p.ID)
		}
		if seen[p.ID] {
			return fmt.Errorf("probe[%d]: duplicate id %q", i, p.ID)
		}
		seen[p.ID] = true
		switch p.Type {
		case "http":
			// 形状细校在 agent 侧执行器（url.Parse 双端一致）；这里挡
			// 明显的 scheme 拼写
			if !strings.HasPrefix(p.Target, "http://") && !strings.HasPrefix(p.Target, "https://") {
				return fmt.Errorf("probe %q: http target must start with http:// or https://", p.ID)
			}
			if p.ExpectStatus == 0 {
				p.ExpectStatus = 200
			}
			if p.ExpectStatus < 100 || p.ExpectStatus > 599 {
				return fmt.Errorf("probe %q: expectStatus %d out of range [100,599]", p.ID, p.ExpectStatus)
			}
		case "tcp":
			// host:port 形状由 agent 侧执行器细校；server 只挡空 target
			if p.Target == "" {
				return fmt.Errorf("probe %q: tcp target is required (host:port)", p.ID)
			}
		case "systemd":
			if !serviceUnitNamePattern.MatchString(p.Target) {
				return fmt.Errorf("probe %q: systemd target %q must be a *.service unit", p.ID, p.Target)
			}
		default:
			return fmt.Errorf("probe %q: unsupported type %q (allowed: http tcp systemd)", p.ID, p.Type)
		}
		if p.IntervalSec == 0 {
			p.IntervalSec = 30
		}
		if p.IntervalSec < 5 || p.IntervalSec > 3600 {
			return fmt.Errorf("probe %q: intervalSec %d out of range [5,3600]", p.ID, p.IntervalSec)
		}
		if p.TimeoutSec == 0 {
			p.TimeoutSec = 5
		}
		if p.TimeoutSec < 1 || p.TimeoutSec > 30 {
			return fmt.Errorf("probe %q: timeoutSec %d out of range [1,30]", p.ID, p.TimeoutSec)
		}
		if p.FailThreshold == 0 {
			p.FailThreshold = 3
		}
		if p.FailThreshold > 60 {
			return fmt.Errorf("probe %q: failThreshold %d out of range [1,60]", p.ID, p.FailThreshold)
		}
		if p.Unit == "" && p.Type == "systemd" {
			p.Unit = p.Target
		}
		if !p.Heal {
			p.BackoffWindowSec, p.MaxRestartsInWindow = 0, 0
			continue
		}
		if p.Unit == "" {
			return fmt.Errorf("probe %q: heal unit is required for %s probes", p.ID, p.Type)
		}
		if !serviceUnitNamePattern.MatchString(p.Unit) {
			return fmt.Errorf("probe %q: heal unit %q must be a *.service unit", p.ID, p.Unit)
		}
		// D6 白名单门：自愈目标必须显式点名；只想告警请关 heal
		if !whitelist[p.Unit] {
			return fmt.Errorf("probe %q: heal unit %q is not in the self-heal whitelist; add it to whitelist, or set heal=false for alert-only", p.ID, p.Unit)
		}
		if p.BackoffWindowSec == 0 {
			p.BackoffWindowSec = 600
		}
		if p.BackoffWindowSec < 60 || p.BackoffWindowSec > 86400 {
			return fmt.Errorf("probe %q: backoffWindowSec %d out of range [60,86400]", p.ID, p.BackoffWindowSec)
		}
		if p.MaxRestartsInWindow == 0 {
			p.MaxRestartsInWindow = 3
		}
		if p.MaxRestartsInWindow > 10 {
			return fmt.Errorf("probe %q: maxRestartsInWindow %d out of range [1,10]", p.ID, p.MaxRestartsInWindow)
		}
	}
	for u := range whitelist {
		if !serviceUnitNamePattern.MatchString(u) {
			return fmt.Errorf("whitelist entry %q must be a *.service unit", u)
		}
	}
	return nil
}

// loadHealthConfig 读配置记录；未配置/读失败返回 nil（无配置是常态，
// 与 acme_scan 等 GetSetting 读径同口径）；JSON 损坏才作为错误上抛
func (s *Server) loadHealthConfig(agentID string) (*healthConfigRecord, error) {
	raw, err := s.db.GetSetting(ServiceHealthConfigPrefix + agentID)
	if err != nil || raw == "" {
		return nil, nil
	}
	var rec healthConfigRecord
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// handleAgentHealthAPI 分发 /api/agents/{id}/health{,/probes/{id}/check}
// rest 是 "/agents/" 之后的部分（形如 "a1/health" 或 "a1/health/probes/p/check"）
func (s *Server) handleAgentHealthAPI(w http.ResponseWriter, r *http.Request, rest string) {
	const suffix = "/health"
	idx := strings.Index(rest, suffix)
	if idx <= 0 {
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
		return
	}
	agentID := rest[:idx]
	sub := strings.TrimPrefix(rest[idx+len(suffix):], "/")

	switch {
	case sub == "" && r.Method == http.MethodGet:
		s.handleHealthOverview(w, r, agentID)
	case sub == "" && r.Method == http.MethodPut:
		s.handleHealthConfigSave(w, r, agentID)
	case strings.HasPrefix(sub, "probes/") && strings.HasSuffix(sub, "/check") && r.Method == http.MethodPost:
		probeID := strings.TrimSuffix(strings.TrimPrefix(sub, "probes/"), "/check")
		if probeID == "" || strings.Contains(probeID, "/") {
			s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
			return
		}
		s.handleHealthProbeCheck(w, r, agentID, probeID)
	default:
		s.handleError(w, r, http.StatusNotFound, "API endpoint not found")
	}
}

// handleHealthOverview GET：配置 + 运行态。在线实时拉取（drain=false，
// 事件留给归集循环）并顺带刷新缓存；失败/离线回落缓存灰态（D8）。
func (s *Server) handleHealthOverview(w http.ResponseWriter, r *http.Request, agentID string) {
	config, err := s.loadHealthConfig(agentID)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to load health config: "+err.Error())
		return
	}
	online := false
	var states map[string]healthStateEntry
	if agent, ok := s.registry.Get(agentID); ok && agent.IsOnline(s.registry.timeouts.Heartbeat) {
		online = true
		if st, err := s.fetchHealthStates(agentID, false); err == nil {
			states = st
			s.cacheHealthState(agentID, st)
		}
	}
	if states == nil {
		if cached := s.loadHealthStateCache(agentID); cached != nil {
			states = cached.States
		}
	}
	if states == nil {
		states = map[string]healthStateEntry{}
	}
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"config": config,
		"states": states,
		"online": online,
	})
}

// fetchHealthStates 调 agent service.health.status；drain=true 仅供归集
// 循环（事件取走即清），仪表盘读路径恒 drain=false 防止事件被浏览吞掉
func (s *Server) fetchHealthStates(agentID string, drain bool) (map[string]healthStateEntry, error) {
	resp, err := s.CallAgent(agentID, "service.health.status", map[string]interface{}{"drain": drain})
	if err != nil {
		return nil, err
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		return nil, err
	}
	if rpcResp.Status == "error" {
		return nil, fmt.Errorf("%s", rpcResp.Error)
	}
	raw, err := healthJSONMarshal(rpcResp.Data)
	if err != nil {
		return nil, err
	}
	var payload struct {
		States map[string]healthStateEntry `json:"states"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload.States, nil
}

// handleHealthConfigSave PUT：校验（D12/D6）→ 落 Setting KV → 在线推送 →
// 审计 service_health_config。离线保存成功（pushed=false，注册上线补推）。
func (s *Server) handleHealthConfigSave(w http.ResponseWriter, r *http.Request, agentID string) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, healthConfigBodyLimit))
	if err != nil {
		s.handleError(w, r, http.StatusRequestEntityTooLarge, "config body too large")
		return
	}
	var payload struct {
		Probes    []healthProbeCfg `json:"probes"`
		Whitelist []string         `json:"whitelist"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		s.handleError(w, r, http.StatusBadRequest, "invalid JSON body")
		return
	}
	wl := map[string]bool{}
	for _, u := range payload.Whitelist {
		wl[u] = true
	}
	if err := validateHealthProbes(payload.Probes, wl); err != nil {
		s.handleError(w, r, http.StatusBadRequest, err.Error())
		return
	}
	rec := healthConfigRecord{Probes: payload.Probes, Whitelist: payload.Whitelist}
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		rec.UpdatedBy = userInfo.Username
	}
	rec.UpdatedAt = timeNowUnix()
	// 元数据先落结构再序列化：体积上限按最终落库字节计
	raw, err := healthJSONMarshal(rec)
	if err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "marshal config: "+err.Error())
		return
	}
	if len(raw) > healthMaxConfigBytes {
		s.handleError(w, r, http.StatusBadRequest,
			fmt.Sprintf("config too large (%d bytes > %d)", len(raw), healthMaxConfigBytes))
		return
	}
	if err := s.db.SetSetting(ServiceHealthConfigPrefix+agentID, string(raw)); err != nil {
		s.handleError(w, r, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	pushed := s.pushServiceHealthConfig(agentID)

	username := "unknown"
	if userInfo, ok := auth.GetUserFromContext(r); ok {
		username = userInfo.Username
	}
	healCount := 0
	for i := range rec.Probes {
		if rec.Probes[i].Heal {
			healCount++
		}
	}
	s.audit.LogResource(username, audit.ActionServiceHealthConfig, audit.ResourceService, agentID,
		map[string]interface{}{"probes": len(rec.Probes), "healProbes": healCount,
			"whitelist": len(rec.Whitelist), "pushed": pushed},
		s.getClientIP(r), r.UserAgent())
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"applied": len(rec.Probes), "pushed": pushed,
	})
}

// handleHealthProbeCheck POST：立即探测一次（诊断动作，读语义不审计 D9）
func (s *Server) handleHealthProbeCheck(w http.ResponseWriter, r *http.Request, agentID, probeID string) {
	resp, err := s.CallAgent(agentID, "service.health.now", map[string]interface{}{"id": probeID})
	if err != nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "agent offline: "+err.Error())
		return
	}
	rpcResp, err := protocol.DecodeRPCResponse(resp)
	if err != nil {
		s.handleError(w, r, http.StatusBadGateway, "agent returned invalid response")
		return
	}
	if rpcResp.Status == "error" {
		msg := rpcResp.Error
		if msg == "" {
			msg = "agent rejected the operation"
		}
		s.handleError(w, r, http.StatusBadGateway, msg)
		return
	}
	s.writeJSON(w, http.StatusOK, rpcResp.Data)
}

// loadHealthStateCache 读离线灰态缓存；无缓存返回 nil
func (s *Server) loadHealthStateCache(agentID string) *healthStateCache {
	raw, err := s.db.GetSetting(ServiceHealthStatePrefix + agentID)
	if err != nil || raw == "" {
		return nil
	}
	var cached healthStateCache
	if err := json.Unmarshal([]byte(raw), &cached); err != nil {
		return nil
	}
	return &cached
}

// cacheHealthState 写运行态缓存（失败只记日志，灰态是尽力而为）
func (s *Server) cacheHealthState(agentID string, states map[string]healthStateEntry) {
	raw, err := healthJSONMarshal(healthStateCache{States: states, UpdatedAt: timeNowUnix()})
	if err != nil {
		return
	}
	_ = s.db.SetSetting(ServiceHealthStatePrefix+agentID, string(raw))
}

// pushServiceHealthConfig 把库内配置推给 agent（注册上线补推与保存后推送
// 共用）；agent 离线/推送失败返回 false（配置已落库，不阻塞保存）。
// params 直接带类型化值：CallAgent 在 wire 上统一 JSON 序列化，与手工
// 转 map 等价且免掉不可达的二次 marshal/unmarshal 错误分支
func (s *Server) pushServiceHealthConfig(agentID string) bool {
	rec, err := s.loadHealthConfig(agentID)
	if err != nil || rec == nil {
		return false
	}
	params := map[string]interface{}{"probes": rec.Probes, "whitelist": rec.Whitelist}
	if _, err := s.CallAgent(agentID, "service.health.config", params); err != nil {
		return false
	}
	return true
}

// timeNowUnix 当前 unix 秒（var 仅为测试可注入）
var timeNowUnix = func() int64 { return time.Now().Unix() }

// healthJSONMarshal json.Marshal 注入点（行为中性，stdinPipeFn 先例）：
// 四个调用点（配置落库、状态缓存、fetch/drain 的 Data 重编码）的对象都是
// 解码后的纯 JSON 数据，真实错误路径不可达，仅供测试覆盖防御分支
var healthJSONMarshal = json.Marshal
