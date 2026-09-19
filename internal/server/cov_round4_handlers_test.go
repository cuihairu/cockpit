package server

// cov_round4_handlers_test.go 第四轮覆盖率：DDNS / ACME / NAS / Service
// 四族 handler 的校验与错误分支（httptest 直调，先例 cov_helpers_test.go）。

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// mustJSON 解析 recorder 响应体
func mustJSON(t *testing.T, rec *httptest.ResponseRecorder, out interface{}) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		t.Fatalf("decode response: %v (%s)", err, rec.Body.String())
	}
}

// ============ api_ddns.go ============

func TestCovDDNSHandlers(t *testing.T) {
	s := covLoopServer(t)

	// Create：bad json / 五种校验形态 / 合法
	cases := []struct {
		name, body string
		want       int
	}{
		{"bad json", `{`, 400},
		{"bad type", `{"type":"CNAME","agentId":"a","zoneId":"z","recordName":"r"}`, 400},
		{"no agent", `{"type":"A","zoneId":"z","recordName":"r"}`, 400},
		{"no zone", `{"type":"A","agentId":"a","recordName":"r"}`, 400},
		{"no name", `{"type":"A","agentId":"a","zoneId":"z"}`, 400},
		{"ok", `{"type":"a","agentId":"a1","zoneId":"z1","zoneName":"Z","recordName":"r1","enabled":true}`, 200},
	}
	var createdID uint
	for _, tc := range cases {
		rec := covRec()
		s.handleDDNSCreate(rec, covReq(http.MethodPost, "/api/ddns", strings.NewReader(tc.body)))
		if rec.Code != tc.want {
			t.Errorf("create %s: code = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
		if tc.name == "ok" {
			var out map[string]interface{}
			mustJSON(t, rec, &out)
			createdID = uint(out["id"].(float64))
			if out["type"] != "A" { // 归一化大写
				t.Errorf("type = %v, want A", out["type"])
			}
		}
	}

	// List
	rec := covRec()
	s.handleDDNSList(rec, covReq(http.MethodGet, "/api/ddns", nil))
	if rec.Code != 200 {
		t.Errorf("list: code = %d", rec.Code)
	}

	// Update：404 / 合法（含大小写归一化）
	rec = covRec()
	s.handleDDNSUpdate(rec, covReq(http.MethodPut, "/api/ddns/999", strings.NewReader(`{"type":"A","agentId":"a","zoneId":"z","recordName":"r"}`)), 999)
	if rec.Code != 404 {
		t.Errorf("update missing: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleDDNSUpdate(rec, covReq(http.MethodPut, "/api/ddns/1", strings.NewReader(`{"type":"aaaa","agentId":"a2","zoneId":"z2","recordName":"r2","enabled":false}`)), createdID)
	if rec.Code != 200 {
		t.Errorf("update ok: code = %d (%s)", rec.Code, rec.Body.String())
	}

	// Check：405 / 坏 id / missing / dns 未配置 503（在 Delete 之前，记录尚在）
	rec = covRec()
	s.handleDDNSCheck(rec, covReq(http.MethodGet, "/ddns/x/check", nil), "x")
	if rec.Code != 405 {
		t.Errorf("check GET: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleDDNSCheck(rec, covReq(http.MethodPost, "/ddns/999/check", nil), "999")
	if rec.Code != 404 {
		t.Errorf("check missing: code = %d", rec.Code)
	}
	rec = covRec()
	checkID := strconv.Itoa(int(createdID))
	s.handleDDNSCheck(rec, covReq(http.MethodPost, "/ddns/"+checkID+"/check", nil), checkID)
	if rec.Code != 503 {
		t.Errorf("check no dns provider: code = %d", rec.Code)
	}

	// Delete：404 / 200
	rec = covRec()
	s.handleDDNSDelete(rec, covReq(http.MethodDelete, "/api/ddns/999", nil), 999)
	if rec.Code != 404 {
		t.Errorf("delete missing: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleDDNSDelete(rec, covReq(http.MethodDelete, "/api/ddns/1", nil), createdID)
	if rec.Code != 200 {
		t.Errorf("delete ok: code = %d", rec.Code)
	}

	// Config API：GET / PUT 合法 / PUT 越界 / bad json
	rec = covRec()
	s.handleDDNSConfigAPI(rec, covReq(http.MethodGet, "/api/ddns/config", nil))
	if rec.Code != 200 {
		t.Errorf("config GET: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleDDNSConfigAPI(rec, covReq(http.MethodPut, "/api/ddns/config", strings.NewReader(`{"scan_interval_seconds":600}`)))
	if rec.Code != 200 {
		t.Errorf("config PUT ok: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleDDNSConfigAPI(rec, covReq(http.MethodPut, "/api/ddns/config", strings.NewReader(`{"scan_interval_seconds":1}`)))
	if rec.Code != 400 {
		t.Errorf("config PUT out of range: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleDDNSConfigAPI(rec, covReq(http.MethodPut, "/api/ddns/config", strings.NewReader(`{`)))
	if rec.Code != 400 {
		t.Errorf("config PUT bad json: code = %d", rec.Code)
	}

	// 分发层：坏 id 404 / 不允许的方法 405（直调路径以 /ddns 为前缀，
	// handler 内部 TrimPrefix 的是 "/ddns" 而非 "/api/ddns"）
	rec = covRec()
	s.handleDDNS(rec, covReq(http.MethodGet, "/ddns/abc", nil))
	if rec.Code != 404 {
		t.Errorf("dispatch bad id: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleDDNS(rec, covReq(http.MethodDelete, "/ddns", nil))
	if rec.Code != 405 {
		t.Errorf("dispatch bad method: code = %d", rec.Code)
	}
}

// ============ api_acme.go ============

func TestCovAcmeHandlers(t *testing.T) {
	s := covLoopServer(t)

	// Create：bad json / 域名系列 / ca 非法 / days 越界 / deploy 三态 / 合法
	cases := []struct {
		name, body string
		want       int
	}{
		{"bad json", `{`, 400},
		{"no domain", `{"caDirectory":"staging"}`, 400},
		{"bad domain", `{"domains":["bad_domain!"],"caDirectory":"staging"}`, 400},
		{"blank domain only", `{"domains":["  "],"caDirectory":"staging"}`, 400},
		{"bad ca", `{"domains":["x.com"],"caDirectory":"go daddy"}`, 400},
		{"days low", `{"domains":["x.com"],"renewBeforeDays":6}`, 400},
		{"days high", `{"domains":["x.com"],"renewBeforeDays":91}`, 400},
		{"deploy paths no agent", `{"domains":["x.com"],"deployCertPath":"/etc/x.pem"}`, 400},
		{"deploy relative path", `{"domains":["x.com"],"deployAgentId":"a","deployCertPath":"etc/x.pem","deployKeyPath":"/etc/x.key"}`, 400},
		{"ok", `{"domains":["*.X.com","y.com"],"caDirectory":" production ","renewBeforeDays":14}`, 200},
		{"ok with deploy", `{"domains":["z.com"],"deployAgentId":"a1","deployCertPath":"/etc/z.pem","deployKeyPath":"/etc/z.key"}`, 200},
	}
	ids := map[string]uint{}
	for _, tc := range cases {
		rec := covRec()
		s.handleAcmeCreate(rec, covReq(http.MethodPost, "/api/acme", strings.NewReader(tc.body)))
		if rec.Code != tc.want {
			t.Errorf("create %s: code = %d, want %d (%s)", tc.name, rec.Code, tc.want, rec.Body.String())
		}
		if strings.HasPrefix(tc.name, "ok") {
			var out map[string]interface{}
			mustJSON(t, rec, &out)
			ids[tc.name] = uint(out["id"].(float64))
		}
	}
	// 泛域名剥 *.：primary 保留原样带 *.
	if ids["ok"] == 0 {
		t.Fatal("ok create missing")
	}

	// Update：404 / 域名变更重置 pending
	rec := covRec()
	s.handleAcmeUpdate(rec, covReq(http.MethodPut, "/api/acme/999", strings.NewReader(`{"domains":["x.com"]}`)), 999)
	if rec.Code != 404 {
		t.Errorf("update missing: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeUpdate(rec, covReq(http.MethodPut, "/api/acme/1", strings.NewReader(`{"domains":["changed.com"],"caDirectory":"staging"}`)), ids["ok"])
	if rec.Code != 200 {
		t.Errorf("update ok: code = %d (%s)", rec.Code, rec.Body.String())
	}

	// Delete：404 / 200
	rec = covRec()
	s.handleAcmeDelete(rec, covReq(http.MethodDelete, "/api/acme/999", nil), 999)
	if rec.Code != 404 {
		t.Errorf("delete missing: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeDelete(rec, covReq(http.MethodDelete, "/api/acme/1", nil), ids["ok with deploy"])
	if rec.Code != 200 {
		t.Errorf("delete ok: code = %d", rec.Code)
	}

	// Issue：405 / 坏 id / missing / issuer 未配置 503
	rec = covRec()
	s.handleAcmeIssue(rec, covReq(http.MethodGet, "/api/acme/1/issue", nil), "1")
	if rec.Code != 405 {
		t.Errorf("issue GET: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeIssue(rec, covReq(http.MethodPost, "/api/acme/x/issue", nil), "x")
	if rec.Code != 404 {
		t.Errorf("issue bad id: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeIssue(rec, covReq(http.MethodPost, "/api/acme/999/issue", nil), "999")
	if rec.Code != 404 {
		t.Errorf("issue missing: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeIssue(rec, covReq(http.MethodPost, "/api/acme/1/issue", nil), strconv.Itoa(int(ids["ok"])))
	if rec.Code != 503 {
		t.Errorf("issue no issuer: code = %d", rec.Code)
	}

	// Deploy 端点：405 / 坏 id / missing / 无 target 400 / agent 离线 503
	// （直测 deployAcmeCert 会以拷贝主键回写库，必须放在端点断言之后）
	path := "/api/acme/" + strconv.Itoa(int(ids["ok"])) + "/deploy"
	idStr := strconv.Itoa(int(ids["ok"]))
	rec = covRec()
	s.handleAcmeDeploy(rec, covReq(http.MethodGet, path, nil), idStr)
	if rec.Code != 405 {
		t.Errorf("deploy GET: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeDeploy(rec, covReq(http.MethodPost, "/api/acme/x/deploy", nil), "x")
	if rec.Code != 404 {
		t.Errorf("deploy bad id: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeDeploy(rec, covReq(http.MethodPost, "/api/acme/999/deploy", nil), "999")
	if rec.Code != 404 {
		t.Errorf("deploy missing: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAcmeDeploy(rec, covReq(http.MethodPost, path, nil), idStr)
	if rec.Code != 400 {
		t.Errorf("deploy no target: code = %d", rec.Code)
	}

	// deployAcmeCert 直测（收尾执行）：无 target / 未签发 / agent 离线。
	// 注意拷贝共享主键，fail() 回写会落到库中该证书——放测试末尾不再有
	// 依赖库内 DeployAgentID 的断言
	base, err := s.db.GetAcmeCert(ids["ok"])
	if err != nil {
		t.Fatalf("get cert: %v", err)
	}
	c2 := *base
	if err := s.deployAcmeCert(&c2); err == nil || !strings.Contains(err.Error(), "no deploy target") {
		t.Errorf("no target: err = %v", err)
	}
	c3 := *base
	c3.DeployAgentID = "offline-agent"
	if err := s.deployAcmeCert(&c3); err == nil || !strings.Contains(err.Error(), "not issued yet") {
		t.Errorf("not issued: err = %v", err)
	}
	c4 := *base
	c4.DeployAgentID = "ghost"
	c4.CertificatePEM, c4.PrivateKeyPEM = "c", "k"
	if err := s.deployAcmeCert(&c4); err == nil {
		t.Error("offline agent should fail")
	}
}

// ============ api_nas.go ============

func TestCovNASHandlers(t *testing.T) {
	s := covLoopServer(t)

	// 分发：无后缀 404
	rec := covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/a/nas/", nil), "a/nas/")
	if rec.Code != 404 {
		t.Errorf("no suffix: code = %d", rec.Code)
	}
	// offline 503
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/ghost/nas/status", nil), "ghost/nas/status")
	if rec.Code != 503 {
		t.Errorf("offline: code = %d", rec.Code)
	}

	// 在线但非 status / 非 GET → 404
	covFakeAgent(t, s, "nas-a", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"disks": []interface{}{}})
	})
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/nas-a/nas/other", nil), "nas-a/nas/other")
	if rec.Code != 404 {
		t.Errorf("non status sub: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodPost, "/api/agents/nas-a/nas/status", nil), "nas-a/nas/status")
	if rec.Code != 404 {
		t.Errorf("POST status: code = %d", rec.Code)
	}

	// error 应答 → 502 透传 / 空数据 → {}
	covFakeAgent(t, s, "nas-err", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("omv down")
	})
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/nas-err/nas/status", nil), "nas-err/nas/status")
	if rec.Code != 502 || !strings.Contains(rec.Body.String(), "omv down") {
		t.Errorf("error passthrough: code = %d body = %s", rec.Code, rec.Body.String())
	}
	covFakeAgent(t, s, "nas-nil", []string{"nas"}, func(string, map[string]interface{}) map[string]interface{} {
		return covOKPayload(nil)
	})
	rec = covRec()
	s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/nas-nil/nas/status", nil), "nas-nil/nas/status")
	if rec.Code != 200 {
		t.Errorf("nil data: code = %d", rec.Code)
	}

	// config API：PUT 合法含 usage_warn / PUT 间隔越界 / PUT warn 越界 / 405
	rec = covRec()
	s.handleNASConfig(rec, covReq(http.MethodPut, "/api/nas/config", strings.NewReader(`{"scan_interval_seconds":900,"usage_warn_percent":85}`)))
	if rec.Code != 200 {
		t.Errorf("config PUT: code = %d (%s)", rec.Code, rec.Body.String())
	}
	rec = covRec()
	s.handleNASConfig(rec, covReq(http.MethodPut, "/api/nas/config", strings.NewReader(`{"scan_interval_seconds":1}`)))
	if rec.Code != 400 {
		t.Errorf("config PUT bad interval: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleNASConfig(rec, covReq(http.MethodPut, "/api/nas/config", strings.NewReader(`{"scan_interval_seconds":900,"usage_warn_percent":1000}`)))
	if rec.Code != 400 {
		t.Errorf("config PUT bad warn: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleNASConfig(rec, covReq(http.MethodDelete, "/api/nas/config", nil))
	if rec.Code != 405 {
		t.Errorf("config DELETE: code = %d", rec.Code)
	}
}

// ============ api_service.go ============

func TestCovServiceHandlers(t *testing.T) {
	s := covLoopServer(t)
	svcOK := covOKPayload(map[string]interface{}{"active": "inactive"})

	// offline 503
	rec := covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodGet, "/api/agents/ghost/services", nil), "ghost/services")
	if rec.Code != 503 {
		t.Errorf("offline: code = %d", rec.Code)
	}

	covFakeAgent(t, s, "svc-a", []string{"service"}, func(method string, _ map[string]interface{}) map[string]interface{} {
		if method == "service.error" {
			return covErrPayload("systemctl boom")
		}
		return svcOK
	})

	// 三段 sub 404
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPost, "/api/agents/svc-a/services/a/b/c", nil), "svc-a/services/a/b/c")
	if rec.Code != 404 {
		t.Errorf("three-segment sub: code = %d", rec.Code)
	}
	// 非法 action 400
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPost, "/api/agents/svc-a/services/nginx.service/kill", nil), "svc-a/services/nginx.service/kill")
	if rec.Code != 400 {
		t.Errorf("bad action: code = %d", rec.Code)
	}
	// 非法 unit 名 400：`!` 不在两 pattern 字符类内（windows 名允许字母数字
	// 与 ._- 空格）；unit 派生自 rest 参数，不经 URL 解析
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPost, "/api/agents/svc-a/services/x/restart", nil), "svc-a/services/bad!name/restart")
	if rec.Code != 400 {
		t.Errorf("bad unit: code = %d", rec.Code)
	}
	// daemon-reload（带审计分支）
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPost, "/api/agents/svc-a/services/daemon-reload", nil), "svc-a/services/daemon-reload")
	if rec.Code != 200 {
		t.Errorf("daemon-reload: code = %d (%s)", rec.Code, rec.Body.String())
	}
	// unit 文件 GET / PUT 合法 / PUT 超限 / PUT bad json
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodGet, "/api/agents/svc-a/services/a.service/file", nil), "svc-a/services/a.service/file")
	if rec.Code != 200 {
		t.Errorf("unitfile GET: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPut, "/api/agents/svc-a/services/a.service/file",
		strings.NewReader(`{"content":"[Unit]\n"}`)), "svc-a/services/a.service/file")
	if rec.Code != 200 {
		t.Errorf("unitfile PUT: code = %d (%s)", rec.Code, rec.Body.String())
	}
	big := `{"content":"` + strings.Repeat("x", unitFileBodyLimit+10) + `"}`
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPut, "/api/agents/svc-a/services/a.service/file",
		strings.NewReader(big)), "svc-a/services/a.service/file")
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("unitfile PUT too large: code = %d", rec.Code)
	}
	rec = covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodPut, "/api/agents/svc-a/services/a.service/file",
		strings.NewReader(`{`)), "svc-a/services/a.service/file")
	if rec.Code != 400 {
		t.Errorf("unitfile PUT bad json: code = %d", rec.Code)
	}

	// forwardServiceRPC 错误透传：error 应答 502 / agent 拒绝
	covFakeAgent(t, s, "svc-err", []string{"service"}, func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("")
	})
	rec = covRec()
	s.forwardServiceRPC(rec, covReq(http.MethodGet, "/api/agents/svc-err/services/status", nil), "svc-err", "service.status", nil, "", "")
	if rec.Code != 502 {
		t.Errorf("error passthrough: code = %d", rec.Code)
	}
}
