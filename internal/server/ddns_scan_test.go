package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// fakeDNSProvider dns.Provider 测试桩：记录写调用、返回预置记录表
type fakeDNSProvider struct {
	records []dns.Record
	created []dns.RecordInput
	updated map[string]dns.RecordInput
	listErr error
}

func (f *fakeDNSProvider) ListZones(ctx context.Context) ([]dns.Zone, error) { return nil, nil }

func (f *fakeDNSProvider) ListRecords(ctx context.Context, zoneID, recordType string, page int) (*dns.RecordsPage, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &dns.RecordsPage{Records: f.records, Page: 1, TotalPage: 1}, nil
}

func (f *fakeDNSProvider) CreateRecord(ctx context.Context, zoneID string, input dns.RecordInput) (*dns.Record, error) {
	f.created = append(f.created, input)
	// 回填记录表：与真实 provider 行为一致，后续轮次 ListRecords 能查到
	rec := dns.Record{ID: "rec-" + strconv.Itoa(len(f.records)+1), Type: input.Type, Name: input.Name, Content: input.Content}
	f.records = append(f.records, rec)
	return &rec, nil
}

func (f *fakeDNSProvider) UpdateRecord(ctx context.Context, zoneID, recordID string, input dns.RecordInput) (*dns.Record, error) {
	if f.updated == nil {
		f.updated = map[string]dns.RecordInput{}
	}
	f.updated[recordID] = input
	return &dns.Record{ID: recordID, Type: input.Type, Name: input.Name, Content: input.Content}, nil
}

func (f *fakeDNSProvider) DeleteRecord(ctx context.Context, zoneID, recordID string) error {
	return nil
}

// withFakeDDNSAgent 注册带 ddns capability 的假 agent，应答 ddns.ip。
// IPv4/IPv6 用指针承载：调用方改值即改变后续应答（无需重复注册）。
func withFakeDDNSAgent(t *testing.T, s *Server, agentID string, ipv4, ipv6 *string) {
	t.Helper()
	agent := NewAgent(agentID, nil)
	agent.Hostname = "host-" + agentID
	agent.Capabilities = []protocol.Capability{{Type: "ddns"}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatalf("register agent: %v", err)
	}
	go func() {
		for reqMsg := range agent.Send {
			if reqMsg.Type != protocol.MessageTypeRPCRequest {
				continue
			}
			data := map[string]interface{}{}
			if ipv4 != nil {
				data["ipv4"] = *ipv4
			}
			if ipv6 != nil {
				data["ipv6"] = *ipv6
			}
			payload := map[string]interface{}{"status": "success", "data": data}
			resp := protocol.NewMessage(protocol.MessageTypeRPCResponse, payload)
			resp.ID = reqMsg.ID
			s.handleRPCResponse(resp)
		}
	}()
	t.Cleanup(func() { close(agent.Send) })
}

// newDDNSTestServer 测试 server + 注入 fake provider
func newDDNSTestServer(t *testing.T) (*Server, *fakeDNSProvider) {
	s := newBackupTestServer(t)
	p := &fakeDNSProvider{}
	s.dns = p
	return s, p
}

func mkDDNSConfig(t *testing.T, s *Server, mutate func(*storage.DDNSConfig)) *storage.DDNSConfig {
	t.Helper()
	cfg := &storage.DDNSConfig{
		AgentID:    "a1",
		ZoneID:     "zone-1",
		ZoneName:   "example.com",
		RecordName: "home.example.com",
		Type:       "A",
		Enabled:    true,
		LastStatus: "never",
	}
	if mutate != nil {
		mutate(cfg)
	}
	if err := s.db.CreateDDNSConfig(cfg); err != nil {
		t.Fatalf("create config: %v", err)
	}
	return cfg
}

func TestDDNSConfigAPI(t *testing.T) {
	s, _ := newDDNSTestServer(t)

	// 创建：合法
	rec := httptest.NewRecorder()
	body := `{"agentId":"a1","zoneId":"zone-1","zoneName":"example.com","recordName":"home.example.com","type":"a","enabled":true}`
	s.handleDDNS(rec, httptest.NewRequest(http.MethodPost, "/ddns", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("create: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created storage.DDNSConfig
	json.Unmarshal(rec.Body.Bytes(), &created)
	if created.ID == 0 || created.Type != "A" || created.LastStatus != "never" {
		t.Fatalf("created = %+v", created)
	}

	// 创建：类型拒绝
	rec = httptest.NewRecorder()
	bad := `{"agentId":"a1","zoneId":"zone-1","recordName":"x.example.com","type":"CNAME"}`
	s.handleDDNS(rec, httptest.NewRequest(http.MethodPost, "/ddns", strings.NewReader(bad)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("CNAME should be rejected: code = %d", rec.Code)
	}
	// 创建：缺名称
	rec = httptest.NewRecorder()
	s.handleDDNS(rec, httptest.NewRequest(http.MethodPost, "/ddns",
		strings.NewReader(`{"agentId":"a1","zoneId":"z","type":"A"}`)))
	if rec.Code != http.StatusBadRequest {
		t.Errorf("empty recordName should be rejected: code = %d", rec.Code)
	}

	// 列表
	rec = httptest.NewRecorder()
	s.handleDDNS(rec, httptest.NewRequest(http.MethodGet, "/ddns", nil))
	var list []*storage.DDNSConfig
	json.Unmarshal(rec.Body.Bytes(), &list)
	if len(list) != 1 {
		t.Fatalf("list = %d configs", len(list))
	}

	// 更新：关闭启用
	rec = httptest.NewRecorder()
	s.handleDDNS(rec, httptest.NewRequest(http.MethodPut, "/ddns/"+strconv.Itoa(int(created.ID)),
		strings.NewReader(`{"agentId":"a1","zoneId":"zone-1","recordName":"home.example.com","type":"A","enabled":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("update: code=%d", rec.Code)
	}
	got, _ := s.db.GetDDNSConfig(created.ID)
	if got.Enabled {
		t.Error("update should set enabled=false")
	}

	// 删除
	rec = httptest.NewRecorder()
	s.handleDDNS(rec, httptest.NewRequest(http.MethodDelete, "/ddns/"+strconv.Itoa(int(created.ID)), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: code=%d", rec.Code)
	}
	list, _ = s.db.ListDDNSConfigs()
	if len(list) != 0 {
		t.Errorf("after delete list = %d", len(list))
	}

	// 审计已记录
	logs, _, _ := s.db.GetAuditLogs(0, 10, nil)
	found := map[string]bool{}
	for _, l := range logs {
		found[l.Action] = true
	}
	for _, action := range []string{"ddns_create", "ddns_update", "ddns_delete"} {
		if !found[action] {
			t.Errorf("audit %s missing, logs = %v", action, found)
		}
	}
}

func TestDDNSScanCreateAndUpdate(t *testing.T) {
	s, p := newDDNSTestServer(t)
	agentIP := "203.0.113.7"
	withFakeDDNSAgent(t, s, "a1", &agentIP, nil)
	cfg := mkDDNSConfig(t, s, nil)

	// 记录缺失 → 创建
	s.scanDDNSOnce()
	if len(p.created) != 1 || p.created[0].Content != "203.0.113.7" || p.created[0].Name != "home.example.com" {
		t.Fatalf("created = %+v", p.created)
	}
	got, _ := s.db.GetDDNSConfig(cfg.ID)
	if got.LastStatus != "ok" || got.LastIP != "203.0.113.7" {
		t.Fatalf("after create: status=%s ip=%s", got.LastStatus, got.LastIP)
	}

	// 无变化 → 安静轮（不再写 API）
	p.created = nil
	s.scanDDNSOnce()
	if len(p.created) != 0 || len(p.updated) != 0 {
		t.Fatalf("quiet round should not write API, created=%d updated=%d", len(p.created), len(p.updated))
	}

	// IP 变化 → 更新且保留原 TTL/Proxied
	agentIP = "203.0.113.9"
	p.records = []dns.Record{{ID: "rec-1", Type: "A", Name: "home.example.com", Content: "203.0.113.7", TTL: 300, Proxied: true}}
	s.scanDDNSOnce()
	up, ok := p.updated["rec-1"]
	if !ok || up.Content != "203.0.113.9" || up.TTL != 300 || !up.Proxied {
		t.Fatalf("updated = %+v (want content 203.0.113.9 with original TTL/proxied)", p.updated)
	}
}

func TestDDNSScanFailureAlertAndDedup(t *testing.T) {
	s, p := newDDNSTestServer(t)
	// agent 离线 → failed + 告警
	mkDDNSConfig(t, s, nil)

	s.scanDDNSOnce()
	alerts, _ := s.db.ListAlerts(10)
	if len(alerts) != 1 || !strings.Contains(alerts[0].Title, "home.example.com") {
		t.Fatalf("alerts = %+v", alerts)
	}
	cfgs, _ := s.db.ListDDNSConfigs()
	if cfgs[0].LastStatus != "failed" || cfgs[0].LastError == "" {
		t.Fatalf("status = %s err = %s", cfgs[0].LastStatus, cfgs[0].LastError)
	}
	if len(p.created) != 0 {
		t.Error("agent offline should not touch DNS")
	}

	// 再次扫描：告警未读 → 去重仍 1 条
	s.scanDDNSOnce()
	alerts, _ = s.db.ListAlerts(10)
	if len(alerts) != 1 {
		t.Fatalf("alerts after rescan = %d, want 1", len(alerts))
	}
}

func TestDDNSScanSkipsDisabledAndUnconfigured(t *testing.T) {
	s, p := newDDNSTestServer(t)
	mkDDNSConfig(t, s, func(c *storage.DDNSConfig) { c.Enabled = false })

	// disabled 跳过
	s.scanDDNSOnce()
	if len(p.created) != 0 {
		t.Fatal("disabled config should be skipped")
	}

	// dns 未配置 → failed 不写
	s.dns = nil
	mkDDNSConfig(t, s, func(c *storage.DDNSConfig) { c.RecordName = "other.example.com" })
	var notifCfg *config.NotificationConfig
	generator := alert.NewGenerator(s.db, s.notifier, notifCfg)
	cfgs, _ := s.db.ListDDNSConfigs()
	ip, changed := s.runDDNSCheck(cfgs[1], nil, map[string][]dns.Record{}, generator)
	if ip != "" || changed {
		t.Error("nil provider should fail without write")
	}
	cfgs, _ = s.db.ListDDNSConfigs()
	if cfgs[1].LastStatus != "failed" {
		t.Errorf("status = %s, want failed", cfgs[1].LastStatus)
	}
}

func TestDDNSScanInterval(t *testing.T) {
	s, _ := newDDNSTestServer(t)
	if got := s.GetDDNSScanInterval(); got != 300 {
		t.Errorf("default interval = %d, want 300", got)
	}
	for _, v := range []int{0, 60, 86400} {
		if err := s.SetDDNSScanInterval(v); err != nil {
			t.Errorf("Set(%d): %v", v, err)
		}
		if got := s.GetDDNSScanInterval(); got != v {
			t.Errorf("Set(%d) then Get = %d", v, got)
		}
	}
	for _, bad := range []int{-1, 30, 90000} {
		if err := s.SetDDNSScanInterval(bad); err == nil {
			t.Errorf("Set(%d) should fail", bad)
		}
	}
}

func TestDDNSCheckEndpoint(t *testing.T) {
	s, p := newDDNSTestServer(t)
	withFakeDDNSAgent(t, s, "a1", &[]string{"198.51.100.2"}[0], nil)
	mkDDNSConfig(t, s, nil)

	rec := httptest.NewRecorder()
	s.handleDDNS(rec, httptest.NewRequest(http.MethodPost, "/ddns/1/check", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("check: code=%d body=%s", rec.Code, rec.Body.String())
	}
	var resp struct {
		IP      string `json:"ip"`
		Changed bool   `json:"changed"`
		Status  string `json:"status"`
	}
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp.IP != "198.51.100.2" || !resp.Changed || resp.Status != "ok" {
		t.Fatalf("resp = %+v", resp)
	}
	if len(p.created) != 1 {
		t.Fatalf("created = %d, want 1", len(p.created))
	}

	// 未知 id
	rec = httptest.NewRecorder()
	s.handleDDNS(rec, httptest.NewRequest(http.MethodPost, "/ddns/999/check", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown id: code = %d", rec.Code)
	}
}
