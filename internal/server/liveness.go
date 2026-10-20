package server

import (
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 带外探活最新态（OpenWrt 自举节点片B，docs/design/oob-openwrt-bootstrap.md）：
// agent 推送 liveness_report，server 侧只留内存最新态供查询。不落库——
// 观测秒级新鲜，进程重启后由下一周期回填（与内存型 RemoteSessionManager
// 同纪律）；容量按目标数天然有界，同 agent 同目标只保留最新一份。
//
// 边界：这里回答「网络层在位」，服务面在位归 /api/probe（probe_report），
// 两个上报面不混流。

// defaultLivenessMaxAge 新鲜度门槛：超过即标 stale（探测不到 ≠ 不存在，
// 过期报告仍返回，由调用方按 stale 判断）。
var defaultLivenessMaxAge = 5 * time.Minute

// livenessSourcePayload 单源上行项（与 agent 侧组包一一对应）。
type livenessSourcePayload struct {
	Verdict string `json:"verdict"`
	Detail  string `json:"detail,omitempty"`
}

// livenessTargetPayload 单目标上行项。
type livenessTargetPayload struct {
	IP   string                `json:"ip"`
	ARP  livenessSourcePayload `json:"arp"`
	ICMP livenessSourcePayload `json:"icmp"`
	TCP  livenessSourcePayload `json:"tcp"`
	At   int64                 `json:"at"` // agent 侧判定时刻（unix 秒）
}

// livenessReportPayload liveness_report 负载。
type livenessReportPayload struct {
	Reports []livenessTargetPayload `json:"reports"`
}

// livenessEntry 查询返回项：最近一次判定 + 到达时刻与新鲜度标记。
type livenessEntry struct {
	Report     livenessTargetPayload `json:"report"`
	SeenAt     int64                 `json:"seen_at"`
	AgeSeconds int                   `json:"age_seconds"`
	Stale      bool                  `json:"stale"`
}

// livenessStore 内存最新态（agentID → ip → entry）。
type livenessStore struct {
	mu     sync.RWMutex
	latest map[string]map[string]livenessEntry
	now    func() time.Time
	maxAge time.Duration
}

func newLivenessStore() *livenessStore {
	return &livenessStore{
		latest: map[string]map[string]livenessEntry{},
		now:    time.Now,
		maxAge: defaultLivenessMaxAge,
	}
}

// Record 幂等 upsert 一个 agent 的本轮报告（重复上报只刷新时刻）。
func (s *livenessStore) Record(agentID string, reports []livenessTargetPayload) {
	now := s.now()
	s.mu.Lock()
	defer s.mu.Unlock()
	targets, ok := s.latest[agentID]
	if !ok {
		targets = map[string]livenessEntry{}
		s.latest[agentID] = targets
	}
	for _, r := range reports {
		if r.IP == "" {
			continue
		}
		targets[r.IP] = livenessEntry{Report: r, SeenAt: now.Unix()}
	}
}

// Query 按 agent/ip 过滤返回最新态（未配置的过滤条件取全部）。
func (s *livenessStore) Query(agentID, ip string) []livenessEntry {
	now := s.now()
	s.mu.RLock()
	out := []livenessEntry{}
	for id, targets := range s.latest {
		if agentID != "" && id != agentID {
			continue
		}
		for targetIP, entry := range targets {
			if ip != "" && targetIP != ip {
				continue
			}
			cp := entry
			age := now.Sub(time.Unix(entry.SeenAt, 0))
			cp.AgeSeconds = int(age.Seconds())
			if cp.AgeSeconds < 0 {
				cp.AgeSeconds = 0
			}
			cp.Stale = age > s.maxAge
			out = append(out, cp)
		}
	}
	s.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].SeenAt != out[j].SeenAt {
			return out[i].SeenAt < out[j].SeenAt
		}
		return out[i].Report.IP < out[j].Report.IP
	})
	return out
}

// decodeLivenessReport 解码 liveness_report 负载（payload 缺失 = 空上报）。
func decodeLivenessReport(msg *protocol.Message) (livenessReportPayload, error) {
	var p livenessReportPayload
	if msg.Payload == nil {
		return p, nil
	}
	raw, err := json.Marshal(msg.Payload)
	if err != nil {
		return p, err
	}
	return p, json.Unmarshal(raw, &p)
}

// handleLivenessReport 处理带外探活上报：内存最新态 upsert。
func (s *Server) handleLivenessReport(agent *Agent, msg *protocol.Message) {
	if s.livenessStore == nil {
		return
	}
	p, err := decodeLivenessReport(msg)
	if err != nil {
		log.Printf("liveness_report decode failed from agent %s: %v", agent.ID, err)
		return
	}
	s.livenessStore.Record(agent.ID, p.Reports)
}

// handleOobLiveness GET /api/oob/liveness?agent_id=&ip= 查内存最新态。
// 不校验 agent 在线状态：断连后最近一轮判定仍有价值，新鲜度由 stale 给出。
func (s *Server) handleOobLiveness(w http.ResponseWriter, r *http.Request) {
	if s.livenessStore == nil {
		s.handleError(w, r, http.StatusServiceUnavailable, "liveness store unavailable")
		return
	}
	entries := s.livenessStore.Query(r.URL.Query().Get("agent_id"), r.URL.Query().Get("ip"))
	s.writeJSON(w, http.StatusOK, map[string]interface{}{
		"entries": entries,
	})
}
