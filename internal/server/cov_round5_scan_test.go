package server

// cov_round5_scan_test.go 第五轮覆盖率：巡检链路带假 agent 的异常分支
// （NAS/SMART 快照 status=error / 解析失败 / 不可用 / 离线跳过）、
// DDNS fetchAgentIP 异常族 + DNS 记录写失败、ACME 巡检状态分支与
// Get*Interval/阈值解析的越界回退。

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/alert"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// covScanOKPayload 成功响应（data 自定）
func covScanOKPayload(data map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"status": "success", "data": data}
}

// TestCovScanSettingParseFallbacks Get*Interval/阈值对库里非法值的回退
// （Set* 已拦越界，这里直写 Setting 模拟历史脏数据）
func TestCovScanSettingParseFallbacks(t *testing.T) {
	s := covNewServer(t)
	set := func(key, v string) {
		t.Helper()
		if err := s.db.SetSetting(key, v); err != nil {
			t.Fatal(err)
		}
	}
	set(ACMEIntervalSettingKey, "bogus")
	if got := s.GetAcmeScanInterval(); got != acmeDefaultInterval {
		t.Fatalf("acme bogus: %d", got)
	}
	set(ACMEIntervalSettingKey, "5") // 低于下限
	if got := s.GetAcmeScanInterval(); got != acmeDefaultInterval {
		t.Fatalf("acme 5: %d", got)
	}
	set(ACMEIntervalSettingKey, "0") // 0 = 关闭，合法
	if got := s.GetAcmeScanInterval(); got != 0 {
		t.Fatalf("acme 0: %d", got)
	}
	set(NASIntervalSettingKey, "bogus")
	if got := s.GetNASScanInterval(); got != nasDefaultInterval {
		t.Fatalf("nas bogus: %d", got)
	}
	set(NASUsageWarnSettingKey, "bogus")
	if got := s.GetNASUsageWarnPercent(); got != nasDefaultUsageWarn {
		t.Fatalf("nas warn bogus: %d", got)
	}
	set(NASUsageWarnSettingKey, "3") // 低于下限
	if got := s.GetNASUsageWarnPercent(); got != nasDefaultUsageWarn {
		t.Fatalf("nas warn 3: %d", got)
	}
	set(SmartIntervalSettingKey, "bogus")
	if got := s.GetSmartScanInterval(); got != smartDefaultInterval {
		t.Fatalf("smart bogus: %d", got)
	}
	set(DDNSIntervalSettingKey, "bogus")
	if got := s.GetDDNSScanInterval(); got != ddnsDefaultInterval {
		t.Fatalf("ddns bogus: %d", got)
	}
}

// TestCovNASScanAgentBranches NAS 巡检异常族：status=error / 解析失败 /
// 不可用 / 离线跳过 / 正常快照的告警消费（pool 异常 + 容量阈值 + 空总量跳过）
func TestCovNASScanAgentBranches(t *testing.T) {
	s := covLoopServer(t) // cfg 非 nil → notifCfg 分支
	s.cfg.Notification = &config.NotificationConfig{}
	ok := covScanOKPayload
	covFakeAgent(t, s, "nas-err", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("zpool boom")
	})
	covFakeAgent(t, s, "nas-badjson", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return ok(map[string]interface{}{"available": true, "pools": 123}) // pools 非数组
	})
	covFakeAgent(t, s, "nas-unavail", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return ok(map[string]interface{}{"available": false})
	})
	covFakeAgent(t, s, "nas-good", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return ok(map[string]interface{}{
			"available": true,
			"pools": []map[string]interface{}{
				{"name": "tank", "state": "failed", "detail": "dev offline"},
				{"name": "mirror", "state": "degraded"},
			},
			"mounts": []map[string]interface{}{
				{"mountPath": "/mnt/zero", "totalGB": 0.0, "usedGB": 10.0}, // TotalGB<=0 跳过
				{"mountPath": "/srv", "totalGB": 100.0, "usedGB": 95.0},    // 超阈值告警
			},
		})
	})
	noHost := covFakeAgent(t, s, "nas-nohost", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return ok(map[string]interface{}{"available": true})
	})
	noHost.Hostname = "" // 触发 hostname 回退 agent.ID
	offline := covFakeAgent(t, s, "nas-offline", []string{"nas"}, nil)
	offline.LastSeen = time.Now().Add(-2 * time.Hour) // 心跳过期 → 离线跳过

	s.scanNASOnce()
}

// TestCovSmartScanAgentBranches SMART 巡检异常族：agentHasSmart 过滤、
// status=error、解析失败、健康判定（failed / 重映射 / 待定 / 介质错误 / unknown）
func TestCovSmartScanAgentBranches(t *testing.T) {
	s := covLoopServer(t) // cfg 非 nil → notifCfg 分支
	s.cfg.Notification = &config.NotificationConfig{}
	ok := covScanOKPayload
	// payload 由调用方给定（成功体或错误体），capability 的 smart 标志独立控制
	mkSmart := func(id string, smartOK bool, payload map[string]interface{}) *Agent {
		a := covFakeAgent(t, s, id, nil, func(string, map[string]interface{}) map[string]interface{} {
			return payload
		})
		a.Capabilities = append(a.Capabilities, protocol.Capability{
			Type:     "hardware-monitor",
			Metadata: map[string]interface{}{"smart": smartOK},
		})
		return a
	}
	mkSmart("sm-err", true, covErrPayload("smartctl boom"))
	mkSmart("sm-badjson", true, ok(map[string]interface{}{"available": true, "devices": "no"})) // devices 非数组
	mkSmart("sm-good", true, ok(map[string]interface{}{
		"available": true,
		"devices": []map[string]interface{}{
			{"name": "sda", "health": "failed"},
			{"name": "sdb", "health": "passed", "reallocatedSectors": 8, "pendingSectors": 0, "mediaErrors": 3},
			{"name": "sdc", "health": "unknown"},
		},
	}))
	mkSmart("sm-nometadata", false, ok(map[string]interface{}{"available": true, "devices": []map[string]interface{}{}})) // 无 smart 标志 → 跳过
	mkSmart("sm-unavail", true, ok(map[string]interface{}{"available": false}))                                           // smartctl 不可用
	offline := mkSmart("sm-offline", true, ok(map[string]interface{}{"available": true}))
	offline.LastSeen = time.Now().Add(-2 * time.Hour)
	noHost, _ := s.registry.Get("sm-good")
	noHost.Hostname = "" // 告警时的 hostname 回退 agent.ID

	s.scanSmartOnce()
}

// TestCovDDNSScanFetchAndProvider fetchAgentIP 异常族（status=error / 解析
// 失败 / AAAA 分支）与 checkDDNSConfig 的 DNS 写失败族（查询 / 建 / 改失败）
func TestCovDDNSScanFetchAndProvider(t *testing.T) {
	s := covNewServer(t)
	ok := covScanOKPayload
	covFakeAgent(t, s, "ddns-err", []string{"ddns"}, func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("ip probe failed")
	})
	covFakeAgent(t, s, "ddns-bad", []string{"ddns"}, func(string, map[string]interface{}) map[string]interface{} {
		return ok(map[string]interface{}{"ipv4": 42}) // 类型不符
	})
	covFakeAgent(t, s, "ddns-empty", []string{"ddns"}, func(string, map[string]interface{}) map[string]interface{} {
		return ok(map[string]interface{}{}) // 无任何地址
	})
	covFakeAgent(t, s, "ddns-ip", []string{"ddns"}, func(string, map[string]interface{}) map[string]interface{} {
		return ok(map[string]interface{}{"ipv4": "1.2.3.4", "ipv6": "fe80::1"})
	})

	// fetchAgentIP：status=error / 解析失败 / AAAA / A
	if _, err := s.fetchAgentIP("ddns-err", "A"); err == nil {
		t.Fatal("expect error for error-status agent")
	}
	if _, err := s.fetchAgentIP("ddns-bad", "A"); err == nil {
		t.Fatal("expect unmarshal error")
	}
	if ip, err := s.fetchAgentIP("ddns-ip", "AAAA"); err != nil || ip != "fe80::1" {
		t.Fatalf("AAAA: %q %v", ip, err)
	}
	if ip, err := s.fetchAgentIP("ddns-ip", "A"); err != nil || ip != "1.2.3.4" {
		t.Fatalf("A: %q %v", ip, err)
	}

	baseCfg := func(agent, typ string) *storage.DDNSConfig {
		return &storage.DDNSConfig{
			Enabled: true, AgentID: agent,
			Type: typ, ZoneID: "zone1", RecordName: "home.example.com",
		}
	}
	cache := func() map[string][]dns.Record { return map[string][]dns.Record{} } // 每场景独立缓存

	// fetchAgentIP err / 空 ip / 查询失败
	if _, _, err := s.checkDDNSConfig(baseCfg("ddns-err", "A"), &fakeDNSProvider{}, cache()); err == nil {
		t.Fatal("expect fetch error")
	}
	if _, _, err := s.checkDDNSConfig(baseCfg("ddns-empty", "A"), &fakeDNSProvider{}, cache()); err == nil {
		t.Fatal("expect empty-ip error")
	}
	if _, _, err := s.checkDDNSConfig(baseCfg("ddns-ip", "A"), &fakeDNSProvider{listErr: errors.New("list fail")}, cache()); err == nil {
		t.Fatal("expect list error")
	}

	// 建记录失败 / 成功建（A 与 AAAA 两分支）
	pCreateFail := &fakeDNSProvider{createErr: errors.New("create fail")}
	if _, _, err := s.checkDDNSConfig(baseCfg("ddns-ip", "A"), pCreateFail, cache()); err == nil {
		t.Fatal("expect create error")
	}
	for _, typ := range []string{"A", "AAAA"} {
		p := &fakeDNSProvider{}
		ip, changed, err := s.checkDDNSConfig(baseCfg("ddns-ip", typ), p, cache())
		if err != nil || !changed || ip != "1.2.3.4" && typ == "A" {
			t.Fatalf("create %s: ip=%q changed=%v err=%v", typ, ip, changed, err)
		}
		if len(p.created) != 1 {
			t.Fatalf("create %s: created=%v", typ, p.created)
		}
	}

	// 改记录失败 / 成功改 / 无变化安静轮
	cfgA := baseCfg("ddns-ip", "A")
	if _, _, err := s.checkDDNSConfig(cfgA, &fakeDNSProvider{
		records:   []dns.Record{{ID: "r1", Type: "A", Name: "home.example.com", Content: "9.9.9.9"}},
		updateErr: errors.New("update fail"),
	}, cache()); err == nil {
		t.Fatal("expect update error")
	}
	pUpd := &fakeDNSProvider{records: []dns.Record{{ID: "r1", Type: "A", Name: "home.example.com", Content: "9.9.9.9"}}}
	if _, changed, err := s.checkDDNSConfig(cfgA, pUpd, cache()); err != nil || !changed {
		t.Fatalf("update: changed=%v err=%v", changed, err)
	}
	pQuiet := &fakeDNSProvider{records: []dns.Record{{ID: "r1", Type: "A", Name: "home.example.com", Content: "1.2.3.4"}}}
	if _, changed, err := s.checkDDNSConfig(cfgA, pQuiet, cache()); err != nil || changed {
		t.Fatalf("quiet: changed=%v err=%v", changed, err)
	}
}

// TestCovDDNSScanOnceFailPath 巡检轮：库里有配置但 provider/agent 不可用 →
// runDDNSCheck 失败路径（状态回写 + 告警）
func TestCovDDNSScanOnceFailPath(t *testing.T) {
	s := covLoopServer(t) // cfg 非 nil → notifCfg 分支
	s.cfg.Notification = &config.NotificationConfig{}
	cfg := &storage.DDNSConfig{
		Enabled: true, AgentID: "ghost",
		Type: "A", ZoneID: "zone1", RecordName: "home.example.com",
	}
	if err := s.db.CreateDDNSConfig(cfg); err != nil {
		t.Fatal(err)
	}
	s.scanDDNSOnce() // s.dns 为 nil → "DNS provider not configured"
	got, _ := s.db.GetDDNSConfig(cfg.ID)
	if got.LastStatus != "failed" {
		t.Fatalf("last status: %q", got.LastStatus)
	}
}

// TestCovDDNSCheckEndpoint check 端点在 cfg 非 nil server 上的分发与失败
func TestCovDDNSCheckEndpoint(t *testing.T) {
	s := covLoopServer(t)
	cfg := &storage.DDNSConfig{
		Enabled: true, AgentID: "ghost",
		Type: "A", ZoneID: "zone1", RecordName: "home.example.com",
	}
	if err := s.db.CreateDDNSConfig(cfg); err != nil {
		t.Fatal(err)
	}
	// 非法 id
	rec := covRec()
	s.handleDDNS(rec, covReq(http.MethodPost, "/ddns/abc/check", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("bad id: %d", rec.Code)
	}
	// provider 未配置 → 5xx
	rec = covRec()
	s.handleDDNS(rec, covReq(http.MethodPost, "/ddns/"+strconv.Itoa(int(cfg.ID))+"/check", nil))
	if rec.Code == http.StatusOK {
		t.Fatalf("check should fail without provider: %d", rec.Code)
	}
}

// TestCovACMEScanStatusBranches ACME 巡检各证书状态的跳过分支 + 签发器缺失
func TestCovACMEScanStatusBranches(t *testing.T) {
	s := covNewServer(t)
	now := time.Now()
	mk := func(domain, status string, autoRenew bool, lastRenewAt int64) {
		t.Helper()
		c := &storage.AcmeCert{
			Domains: []string{domain}, PrimaryDomain: domain, CADirectory: "staging",
			Status: status, AutoRenew: autoRenew, RenewBeforeDays: 30,
			ExpiresAt: now.Add(48 * time.Hour), LastRenewAt: lastRenewAt,
		}
		if err := s.db.CreateAcmeCert(c); err != nil {
			t.Fatal(err)
		}
	}
	mk("near.test", "issued", true, 0)                 // 未临期 → 跳过
	mk("manual.test", "issued", false, 0)              // 关闭自动续期 → 跳过
	mk("throttle.test", "failed", true, now.Unix()-60) // 失败节流（<1h）→ 跳过
	mk("pending.test", "pending", true, 0)             // pending 手动首签 → 跳过
	s.scanACMEOnce()

	// 签发器未配置（acme==nil）
	gen := alert.NewGenerator(s.db, s.notifier, nil)
	if _, err := s.runACMEIssue(&storage.AcmeCert{PrimaryDomain: "x.test"}, gen, now); err == nil {
		t.Fatal("expect acme issuer not available")
	}
}

// TestCovAcmeObservationUpsert 证书观测表联动：新建（ErrNotFound 分支）与
// 已存在记录（Labels nil 补齐分支）
func TestCovAcmeObservationUpsert(t *testing.T) {
	s := covNewServer(t)
	obs := func(id uint, domain string) *storage.AcmeCert {
		return &storage.AcmeCert{
			ID: id, Domains: []string{domain}, PrimaryDomain: domain,
			CADirectory: "staging", Status: "issued", AutoRenew: true,
			RenewBeforeDays: 21, ExpiresAt: time.Now().Add(72 * time.Hour),
		}
	}
	s.upsertAcmeCertObservation(obs(77, "new.test")) // 无既有记录 → 新建
	got, err := s.db.GetCertificate("acme-77")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "valid" || got.Labels["source"] != "acme" {
		t.Fatalf("new observation: %+v", got)
	}
	// 预置一条 Labels 为 nil 的既有记录，覆盖 Labels 补齐分支
	//（UpsertCertificate 对已存在记录不回写字段——Assign 先取到旧行，
	// 这里只验证分支执行不 panic，不验证落库值）
	if err := s.db.UpsertCertificate(&storage.Certificate{ID: "acme-78", DomainName: "old.test"}); err != nil {
		t.Fatal(err)
	}
	s.upsertAcmeCertObservation(obs(78, "old.test"))
}
