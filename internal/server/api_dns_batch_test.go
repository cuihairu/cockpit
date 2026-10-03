package server

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/dns"
)

// M4 批量导入/导出（dns-design.md D21-D25）：
// export 多页聚合/便携形态/错误路径；import 四分类/校验/超限/失败不中断/
// 非 CF proxied 分支；审计动作与台账联动；RBAC 权限点钉死。

// auditActions 收集库里的审计动作集合（resourceID 子串可选过滤）
func auditActions(t *testing.T, s *Server, resourceIDContains string) map[string]string {
	t.Helper()
	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]string{}
	for _, l := range logs {
		if resourceIDContains == "" || strings.Contains(l.ResourceID, resourceIDContains) {
			actions[l.Action] = l.ResourceID
		}
	}
	return actions
}

func TestDNSRecordsExport(t *testing.T) {
	pageCalls := 0
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/zones/z1/dns_records") {
			pageCalls++
			if pageCalls == 1 {
				return 200, `{"success":true,"errors":[],"result":[
					{"id":"r1","type":"A","name":"www.example.com","content":"1.2.3.4","ttl":1,"proxied":true},
					{"id":"r2","type":"CNAME","name":"app.example.com","content":"target.com","ttl":300,"proxied":false}
				],"result_info":{"page":1,"total_pages":2}}`
			}
			return 200, `{"success":true,"errors":[],"result":[
				{"id":"r3","type":"TXT","name":"example.com","content":"v=spf1 -all","ttl":1,"proxied":false}
			],"result_info":{"page":2,"total_pages":2}}`
		}
		return 200, `{"success":true,"errors":[],"result":[]}`
	})

	w := doDNS(s, http.MethodGet, "/dns/zones/z1/records/export", "")
	if w.Code != http.StatusOK {
		t.Fatalf("export = %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"zone_id":"z1"`, `"provider":"cloudflare"`, `"count":3`,
		`"name":"www.example.com"`, `"name":"app.example.com"`, `"name":"example.com"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("export body missing %s: %s", want, body)
		}
	}
	// 便携 RecordInput 形态：不含 provider id / locked 字段（可直接作导入输入）
	if strings.Contains(body, `"locked"`) || strings.Contains(body, `"id":"r1"`) {
		t.Errorf("export should be portable form: %s", body)
	}
	if pageCalls != 2 {
		t.Errorf("list calls = %d, want 2 (翻页聚合)", pageCalls)
	}
	// GET 不经审计中间件：手动 dns_export 落库（取内容审计先例，D22）
	actions := auditActions(t, s, "z1/export")
	if _, ok := actions["dns_export"]; !ok {
		t.Fatalf("dns_export audit missing: %v", actions)
	}
}

func TestDNSRecordsExportErrors(t *testing.T) {
	// 未配置 503
	s := newBackupTestServer(t)
	w := doDNS(s, http.MethodGet, "/dns/zones/z1/records/export", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("not configured = %d, want 503", w.Code)
	}

	// 上游错误 502 透传
	s2 := newDNSTestServer(t, func(r *http.Request) (int, string) {
		return 500, `{"success":false,"errors":[{"code":1000,"message":"boom up"}],"result":null}`
	})
	w = doDNS(s2, http.MethodGet, "/dns/zones/z1/records/export", "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "boom up") {
		t.Fatalf("upstream error = %d %s, want 502 with message", w.Code, w.Body.String())
	}

	// 方法不符 405（export 仅 GET）
	w = doDNS(s2, http.MethodPost, "/dns/zones/z1/records/export", "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("post export = %d, want 405", w.Code)
	}
	// 多余路径段 404
	w = doDNS(s2, http.MethodGet, "/dns/zones/z1/records/export/x", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("export/x = %d, want 404", w.Code)
	}
}

func TestDNSRecordsImportFlow(t *testing.T) {
	creates, updates := 0, 0
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		switch {
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/zones/z1/dns_records"):
			return 200, `{"success":true,"errors":[],"result":[
				{"id":"r1","type":"A","name":"www.example.com","content":"1.2.3.4","ttl":300,"proxied":false}
			],"result_info":{"page":1,"total_pages":1}}`
		case r.Method == http.MethodGet: // /zones：M3 联动探测 zone 名
			return 200, `{"success":true,"errors":[],"result":[
				{"id":"z1","name":"example.com","status":"active","name_servers":[]}
			],"result_info":{"page":1,"total_pages":1}}`
		case r.Method == http.MethodPost:
			creates++
			return 200, `{"success":true,"errors":[],"result":{"id":"r9","type":"A","name":"new.example.com","content":"9.9.9.9","ttl":300,"proxied":false}}`
		case r.Method == http.MethodPut:
			updates++
			return 200, `{"success":true,"errors":[],"result":{"id":"r1","type":"A","name":"www.example.com","content":"1.2.3.4","ttl":600,"proxied":false}}`
		}
		return 500, `{"success":false}`
	})

	// [0] 与现状完全一致 → skipped；[1] TTL 差异 → updated；
	// [2] 未命中 → created；[3] 与 [2] 同键（同请求内重复）→ skipped
	body := `{"records":[
		{"type":"A","name":"www.example.com","content":"1.2.3.4","ttl":300,"proxied":false},
		{"type":"A","name":"www.example.com","content":"1.2.3.4","ttl":600,"proxied":false},
		{"type":"A","name":"new.example.com","content":"9.9.9.9","ttl":300,"proxied":false},
		{"type":"A","name":"new.example.com","content":"9.9.9.9","ttl":300,"proxied":false}
	]}`
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", body)
	if w.Code != http.StatusOK {
		t.Fatalf("import = %d %s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	for _, want := range []string{`"total":4`, `"created":1`, `"updated":1`, `"skipped":2`, `"failed":null`} {
		if !strings.Contains(resp, want) {
			t.Errorf("import resp missing %s: %s", want, resp)
		}
	}
	if creates != 1 || updates != 1 {
		t.Errorf("upstream writes = create %d update %d, want 1/1", creates, updates)
	}

	// M3 台账联动：created/updated 的 A 记录行落「资源 → 域名」
	for _, name := range []string{"www.example.com", "new.example.com"} {
		if d, err := s.db.GetDomainByName(name); err != nil || d == nil {
			t.Errorf("domain %s should be linked: %v", name, err)
		}
	}

	// 审计 dns_import：resourceID={zid}/batch，details 只含计数
	actions := auditActions(t, s, "z1/batch")
	if _, ok := actions["dns_import"]; !ok {
		t.Fatalf("dns_import audit missing: %v", actions)
	}
	logs, _, _ := s.db.GetAuditLogs(0, 50, nil)
	for _, l := range logs {
		if l.Action == "dns_import" &&
			(!strings.Contains(l.Details, `"created":1`) || strings.Contains(l.Details, "9.9.9.9")) {
			t.Errorf("dns_import details should carry counts only: %s", l.Details)
		}
	}
}

func TestDNSRecordsImportFailuresContinue(t *testing.T) {
	creates := 0
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		switch {
		case r.Method == http.MethodGet:
			return 200, `{"success":true,"errors":[],"result":[],"result_info":{"page":1,"total_pages":1}}`
		case r.Method == http.MethodPost:
			creates++
			if creates == 2 {
				return 400, `{"success":false,"errors":[{"code":1002,"message":"record exists"}],"result":null}`
			}
			return 200, `{"success":true,"errors":[],"result":{"id":"r9","type":"A","name":"a.example.com","content":"1.1.1.1","ttl":300,"proxied":false}}`
		}
		return 500, `{"success":false}`
	})

	// 单条上游失败不中断：第一条照常 created，第二条进 failed 明细（D24）
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", `{"records":[
		{"type":"A","name":"a.example.com","content":"1.1.1.1","ttl":300,"proxied":false},
		{"type":"A","name":"b.example.com","content":"2.2.2.2","ttl":300,"proxied":false}
	]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("import = %d %s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	for _, want := range []string{`"created":1`, `"failed":[{"index":1,"name":"b.example.com","error":`, `record exists`} {
		if !strings.Contains(resp, want) {
			t.Errorf("import resp missing %s: %s", want, resp)
		}
	}
}

// TestDNSRecordsImportUpdateFailure 命中且需更新但上游 UpdateRecord 失败：
// 进 failed 明细且不中断同批 create（D24 失败容错的 update 路径）
func TestDNSRecordsImportUpdateFailure(t *testing.T) {
	s := newBackupTestServer(t)
	fake := &fakeDNSProvider{
		zones:     []dns.Zone{{ID: "z1", Name: "example.com", Status: "active"}},
		records:   []dns.Record{{ID: "r1", Type: "A", Name: "www.example.com", Content: "1.2.3.4", TTL: 600}},
		updateErr: errors.New("update boom"),
	}
	s.dns = fake
	// [0] 命中但 TTL 600→300 有差异 → UpdateRecord 失败进 failed；
	// [1] 未命中 → 照常 created（失败不中断）
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", `{"records":[
		{"type":"A","name":"www.example.com","content":"1.2.3.4","ttl":300,"proxied":false},
		{"type":"A","name":"new.example.com","content":"9.9.9.9","ttl":300,"proxied":false}
	]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("import = %d %s", w.Code, w.Body.String())
	}
	resp := w.Body.String()
	if !strings.Contains(resp, `"failed":[{"index":0,"name":"www.example.com","error":"update boom"}`) {
		t.Errorf("update failure should land in failed detail: %s", resp)
	}
	if !strings.Contains(resp, `"created":1`) || !strings.Contains(resp, `"updated":0`) {
		t.Errorf("update failure must not block create: %s", resp)
	}
	if len(fake.created) != 1 {
		t.Errorf("create should still reach upstream: %v", fake.created)
	}
}

func TestDNSRecordsImportValidation(t *testing.T) {
	writes := 0
	s := newDNSTestServer(t, func(r *http.Request) (int, string) {
		if r.Method == http.MethodPost || r.Method == http.MethodPut {
			writes++
		}
		return 500, `{"success":false}`
	})

	// 坏 JSON
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", `{"records":`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid JSON") {
		t.Fatalf("bad json = %d %s", w.Code, w.Body.String())
	}
	// 空 records
	w = doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", `{"records":[]}`)
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "must not be empty") {
		t.Fatalf("empty = %d %s", w.Code, w.Body.String())
	}
	// 超限 500
	var sb strings.Builder
	sb.WriteString(`{"records":[`)
	for i := 0; i < dnsImportMaxRecords+1; i++ {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(`{"type":"A","name":"a","content":"1.1.1.1","ttl":1}`)
	}
	sb.WriteString(`]}`)
	w = doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", sb.String())
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "exceeds limit") {
		t.Fatalf("over limit = %d %s", w.Code, w.Body.String())
	}
	// 非法条目：带下标、最多 5 条
	w = doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", `{"records":[
		{"type":"PTR","name":"x","content":"y","ttl":1},
		{"type":"A","name":"","content":"1.1.1.1","ttl":1},
		{"type":"A","name":"ok","content":"1.1.1.1","ttl":30}
	]}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("invalid entries = %d %s, want 400", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "records[0]") || !strings.Contains(w.Body.String(), "records[2]") {
		t.Errorf("invalid entries should carry indexes: %s", w.Body.String())
	}
	if writes != 0 {
		t.Errorf("validation failure must not reach upstream, writes = %d", writes)
	}

	// 校验通过但现状拉取失败 → 502（上游错误透传）
	s2 := newDNSTestServer(t, func(r *http.Request) (int, string) {
		return 500, `{"success":false,"errors":[{"code":1000,"message":"list boom"}],"result":null}`
	})
	w = doDNS(s2, http.MethodPost, "/dns/zones/z1/records/import",
		`{"records":[{"type":"A","name":"a","content":"1.1.1.1","ttl":1}]}`)
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "list boom") {
		t.Fatalf("list failure = %d %s, want 502", w.Code, w.Body.String())
	}

	// 方法不符 405（import 仅 POST）
	w = doDNS(s2, http.MethodGet, "/dns/zones/z1/records/import", "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("get import = %d, want 405", w.Code)
	}
	// 未配置 503
	s3 := newBackupTestServer(t)
	w = doDNS(s3, http.MethodPost, "/dns/zones/z1/records/import", `{"records":[{"type":"A","name":"a","content":"1.1.1.1","ttl":1}]}`)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("not configured = %d, want 503", w.Code)
	}
}

// TestDNSRecordsImportNonCF 非 cloudflare provider：proxied 恒 false 不触发
// 更新；TTL auto（0/1）表示「provider 决定」同样不触发（跨 provider 迁移
// 幂等，D23）；CF 下 proxied 差异则更新
func TestDNSRecordsImportNonCF(t *testing.T) {
	newFake := func() *fakeDNSProvider {
		return &fakeDNSProvider{
			zones:   []dns.Zone{{ID: "z1", Name: "example.com", Status: "active"}},
			records: []dns.Record{{ID: "r1", Type: "A", Name: "www.example.com", Content: "1.2.3.4", TTL: 600}},
		}
	}
	s := newBackupTestServer(t)
	s.cfg = &config.Config{DNS: &config.DNSConfig{Provider: "dnspod"}}
	fake := newFake()
	s.dns = fake
	w := doDNS(s, http.MethodPost, "/dns/zones/z1/records/import", `{"records":[
		{"type":"A","name":"www.example.com","content":"1.2.3.4","ttl":1,"proxied":true}
	]}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"skipped":1`) {
		t.Fatalf("dnspod import = %d %s, want skipped 1", w.Code, w.Body.String())
	}
	if len(fake.updated) != 0 || len(fake.created) != 0 {
		t.Errorf("dnspod should not write for auto ttl + ignored proxied: updated=%v created=%v", fake.updated, fake.created)
	}

	// cloudflare：proxied 差异触发更新
	s2 := newBackupTestServer(t)
	fake2 := newFake()
	s2.dns = fake2
	w = doDNS(s2, http.MethodPost, "/dns/zones/z1/records/import", `{"records":[
		{"type":"A","name":"www.example.com","content":"1.2.3.4","ttl":600,"proxied":true}
	]}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"updated":1`) {
		t.Fatalf("cf import = %d %s, want updated 1", w.Code, w.Body.String())
	}
	if len(fake2.updated) != 1 {
		t.Errorf("cf proxied diff should update r1: %v", fake2.updated)
	}
}

// TestDNSBatchRBACPerms RBAC 权限点核对：批量端点沿用 /api/dns 前缀推导，
// rbac.go 零改动；export（GET）= dns:read、import（POST）= dns:write
func TestDNSBatchRBACPerms(t *testing.T) {
	perms, governed := requiredPerms("/api/dns/zones/z1/records/export", http.MethodGet)
	if !governed || len(perms) != 1 || perms[0] != "dns:read" {
		t.Fatalf("export GET = %v governed=%v, want [dns:read]", perms, governed)
	}
	perms, governed = requiredPerms("/api/dns/zones/z1/records/import", http.MethodPost)
	if !governed || len(perms) != 1 || perms[0] != "dns:write" {
		t.Fatalf("import POST = %v governed=%v, want [dns:write]", perms, governed)
	}
	// 单条 CRUD 与批量端点同口径（既有行为钉死，防回归）
	perms, _ = requiredPerms("/api/dns/zones/z1/records", http.MethodPost)
	if perms[0] != "dns:write" {
		t.Fatalf("create POST = %v, want dns:write", perms)
	}
	perms, _ = requiredPerms("/api/dns/zones/z1/records/r1", http.MethodDelete)
	if perms[0] != "dns:write" {
		t.Fatalf("delete = %v, want dns:write", perms)
	}
}
