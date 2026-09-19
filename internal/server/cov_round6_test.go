package server

// cov_round6_test.go 第六轮覆盖率：handler 的错误响应族——RPC 响应解析失败
// （status 类型非法 → DecodeRPCResponse err）与 status=error 透传（nas/
// smart/overlay/service/logs.search/logs.follow）、service 路由守卫、ddns
// handler 的 405/404/400 与 check 端点、files chown 校验与认证审计、
// drift record 的 405 与审计、stack history 空列表归一化、logs.follow 的
// 客户端断开写帧失败与缓冲背压。
// CallAgent 的 30s 超时路径与 forward 的 CallAgent err 分支仍不在目标内。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/storage"
)

// covBadRespPayload 非法 RPC 响应：status 非字符串 → DecodeRPCResponse 失败
func covBadRespPayload(string, map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{"status": 123}
}

// TestCovServiceAPIRouteGuards handleAgentServiceAPI 的路由守卫：rest 缺
// /services 后缀（agentID=="" 的第二守卫在 idx>0 时结构上不可达，不追）
func TestCovServiceAPIRouteGuards(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleAgentServiceAPI(rec, covReq(http.MethodGet, "/api/agents/x", nil), "/agents")
	covWantCode(t, "no /services suffix", rec, http.StatusNotFound)
}

// TestCovAgentStatusAPIErrorFamilies nas/smart/overlay 三族 status API 的
// error 响应透传与非法 RPC 响应分支
func TestCovAgentStatusAPIErrorFamilies(t *testing.T) {
	s := covNewServer(t)
	reject := func(string, map[string]interface{}) map[string]interface{} {
		return covErrPayload("backend exploded")
	}
	covFakeAgent(t, s, "fam2-nas-reject", []string{"nas"}, reject)
	covFakeAgent(t, s, "fam2-nas-bad", []string{"nas"}, covBadRespPayload)
	covFakeAgent(t, s, "fam2-sm-reject", nil, reject)
	covFakeAgent(t, s, "fam2-sm-bad", nil, covBadRespPayload)
	covFakeAgent(t, s, "fam2-ov-reject", nil, reject)
	covFakeAgent(t, s, "fam2-ov-bad", nil, covBadRespPayload)

	cases := []struct {
		desc string
		call func(rec *httptest.ResponseRecorder, id string)
	}{
		{"nas reject", func(rec *httptest.ResponseRecorder, id string) {
			s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/"+id+"/nas/status", nil), id+"/nas/status")
		}},
		{"nas badresp", func(rec *httptest.ResponseRecorder, id string) {
			s.handleAgentNASAPI(rec, covReq(http.MethodGet, "/api/agents/"+id+"/nas/status", nil), id+"/nas/status")
		}},
		{"smart reject", func(rec *httptest.ResponseRecorder, id string) {
			s.handleAgentSmartAPI(rec, covReq(http.MethodGet, "/api/agents/"+id+"/smart/status", nil), id+"/smart/status")
		}},
		{"smart badresp", func(rec *httptest.ResponseRecorder, id string) {
			s.handleAgentSmartAPI(rec, covReq(http.MethodGet, "/api/agents/"+id+"/smart/status", nil), id+"/smart/status")
		}},
		{"overlay reject", func(rec *httptest.ResponseRecorder, id string) {
			s.handleAgentOverlayAPI(rec, covReq(http.MethodGet, "/api/agents/"+id+"/overlay/status", nil), id+"/overlay/status")
		}},
		{"overlay badresp", func(rec *httptest.ResponseRecorder, id string) {
			s.handleAgentOverlayAPI(rec, covReq(http.MethodGet, "/api/agents/"+id+"/overlay/status", nil), id+"/overlay/status")
		}},
	}
	ids := []string{"fam2-nas-reject", "fam2-nas-bad", "fam2-sm-reject", "fam2-sm-bad", "fam2-ov-reject", "fam2-ov-bad"}
	for i, tc := range cases {
		rec := covRec()
		tc.call(rec, ids[i])
		if rec.Code < http.StatusInternalServerError {
			t.Errorf("%s: code = %d body %s", tc.desc, rec.Code, rec.Body.String())
		}
	}
}

// TestCovServiceForwardErrorFamilies forwardServiceRPC：非法响应 / error
// 透传（无 error 字段走兜底文案）/ 带认证的审计分支
func TestCovServiceForwardErrorFamilies(t *testing.T) {
	s := covNewServer(t)
	emptyErr := func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error"}
	}
	covFakeAgent(t, s, "svc6-reject", nil, emptyErr)
	covFakeAgent(t, s, "svc6-bad", nil, covBadRespPayload)
	covFakeAgent(t, s, "svc6-ok", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covScanOKPayload(map[string]interface{}{})
	})

	save := func(id string) *httptest.ResponseRecorder {
		rec := covRec()
		s.handleAgentServiceAPI(rec, covReq(http.MethodPut, "/api/agents/"+id+"/services/a.service/file",
			strings.NewReader(`{"content":"unit body"}`)), id+"/services/a.service/file")
		return rec
	}
	if rec := save("svc6-reject"); rec.Code < http.StatusInternalServerError {
		t.Errorf("reject: %d %s", rec.Code, rec.Body.String())
	}
	if rec := save("svc6-bad"); rec.Code < http.StatusInternalServerError {
		t.Errorf("badresp: %d %s", rec.Code, rec.Body.String())
	}
	// 合法 action 带认证 → 审计记录用户名（forwardServiceRPC 的 user 分支）
	rec := covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleAgentServiceAPI(w, r, "svc6-ok/services/a.service/restart")
	}, covAuthReq(http.MethodPost, "/api/agents/svc6-ok/services/a.service/restart", nil, "u1", "admin", "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("audit action: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCovDDNSHandlerValidationFamily ddns handler：PATCH 405、更新缺失
// 404、坏 body 400、类型非法 400、注入 provider 后 check 端点完整链路
func TestCovDDNSHandlerValidationFamily(t *testing.T) {
	s := covLoopServer(t)
	s.cfg.Notification = &config.NotificationConfig{}
	s.dns = &fakeDNSProvider{}
	cfg := &storage.DDNSConfig{
		Enabled: true, AgentID: "ddns6-ip", Type: "A",
		ZoneID: "zone6", RecordName: "home6.example.com",
	}
	if err := s.db.CreateDDNSConfig(cfg); err != nil {
		t.Fatal(err)
	}
	covFakeAgent(t, s, "ddns6-ip", []string{"ddns"}, func(string, map[string]interface{}) map[string]interface{} {
		return covScanOKPayload(map[string]interface{}{"ipv4": "203.0.113.9"})
	})

	rec := covRec()
	s.handleDDNS(rec, covReq(http.MethodPatch, "/ddns/"+strconv.Itoa(int(cfg.ID)), nil))
	covWantCode(t, "patch 405", rec, http.StatusMethodNotAllowed)

	rec = covRec()
	s.handleDDNS(rec, covReq(http.MethodPut, "/ddns/999999", strings.NewReader(`{}`)))
	covWantCode(t, "update missing", rec, http.StatusNotFound)

	rec = covRec()
	s.handleDDNS(rec, covReq(http.MethodPut, "/ddns/"+strconv.Itoa(int(cfg.ID)), strings.NewReader(`{bad`)))
	covWantCode(t, "update bad json", rec, http.StatusBadRequest)

	rec = covRec()
	s.handleDDNS(rec, covReq(http.MethodPut, "/ddns/"+strconv.Itoa(int(cfg.ID)),
		strings.NewReader(`{"type":"bogus"}`)))
	covWantCode(t, "update bad type", rec, http.StatusBadRequest)

	// check 端点：provider 已注入 + agent 可应答 → 200 且带回 IP
	rec = covRec()
	s.handleDDNS(rec, covReq(http.MethodPost, "/ddns/"+strconv.Itoa(int(cfg.ID))+"/check", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "203.0.113.9") {
		t.Errorf("check: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCovScanFetchDecodeErrors 三个 fetch* 的非法 RPC 响应分支
func TestCovScanFetchDecodeErrors(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "sc6-ddns-bad", []string{"ddns"}, covBadRespPayload)
	covFakeAgent(t, s, "sc6-nas-bad", []string{"nas"}, covBadRespPayload)
	covFakeAgent(t, s, "sc6-sm-bad", nil, covBadRespPayload)
	if _, err := s.fetchAgentIP("sc6-ddns-bad", "A"); err == nil {
		t.Fatal("expect ddns decode error")
	}
	if _, err := s.fetchNASSnapshot("sc6-nas-bad"); err == nil {
		t.Fatal("expect nas decode error")
	}
	if _, err := s.fetchSmartDevices("sc6-sm-bad"); err == nil {
		t.Fatal("expect smart decode error")
	}
}

// TestCovFilesChownValidation chown：相对路径 400、uid 越界 400、认证成功
// 请求走 auditFile 的用户名分支
func TestCovFilesChownValidation(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "files6-ok", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covScanOKPayload(map[string]interface{}{})
	})
	call := func(id, body string, authed bool) *httptest.ResponseRecorder {
		req := covReq(http.MethodPost, "/api/agents/"+id+"/files/chown", strings.NewReader(body))
		if authed {
			return covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
				s.handleAgentFilesAPI(w, r, id+"/files/chown")
			}, covAuthReq(http.MethodPost, "/api/agents/"+id+"/files/chown", strings.NewReader(body), "u1", "admin", "admin"))
		}
		rec := covRec()
		s.handleAgentFilesAPI(rec, req, id+"/files/chown")
		return rec
	}
	if rec := call("files6-ok", `{"path":"rel/path","uid":1,"gid":1}`, false); rec.Code != http.StatusBadRequest {
		t.Errorf("relative path: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call("files6-ok", `{"path":"/tmp/x","uid":-5,"gid":1}`, false); rec.Code != http.StatusBadRequest {
		t.Errorf("bad uid: %d %s", rec.Code, rec.Body.String())
	}
	if rec := call("files6-ok", `{"path":"/tmp/x","uid":1,"gid":1}`, true); rec.Code != http.StatusOK {
		t.Errorf("chown ok: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCovDriftRecordHandler record：非 POST 405、认证成功链路的审计分支
func TestCovDriftRecordHandler(t *testing.T) {
	s := covNewServer(t)
	covFakeAgent(t, s, "drift6-ok", nil, func(string, map[string]interface{}) map[string]interface{} {
		return covScanOKPayload(map[string]interface{}{})
	})
	rec := covRec()
	s.handleDriftRecord(rec, covReq(http.MethodPatch, "/api/agents/drift6-ok/drift/record", nil), "drift6-ok")
	covWantCode(t, "patch 405", rec, http.StatusMethodNotAllowed)

	rec = covCallAuth(s, func(w http.ResponseWriter, r *http.Request) {
		s.handleDriftRecord(w, r, "drift6-ok")
	}, covAuthReq(http.MethodPost, "/api/agents/drift6-ok/drift/record",
		strings.NewReader(`{"kind":"nginx","name":"site1"}`), "u1", "admin", "admin"))
	if rec.Code != http.StatusOK {
		t.Errorf("record: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCovStackHistoryEmptyList 部署历史空结果归一化为空数组
func TestCovStackHistoryEmptyList(t *testing.T) {
	s := covNewServer(t)
	rec := covRec()
	s.handleStackHistory(rec, covReq(http.MethodGet, "/api/agents/ghost/stacks/app/history", nil), "ghost", "app")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "deployments") {
		t.Errorf("history: %d %s", rec.Code, rec.Body.String())
	}
}

// TestCovLogsSearchAgentFamilies searchOneAgent：非法响应降级与 error 兜底
func TestCovLogsSearchAgentFamilies(t *testing.T) {
	s := covNewServer(t)
	bad := covFakeAgent(t, s, "ls6-bad", []string{"logs"}, covBadRespPayload)
	rej := covFakeAgent(t, s, "ls6-reject", []string{"logs"}, func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error"}
	})
	if res := s.searchOneAgent(bad, map[string]interface{}{"query": "x"}); res.Error != "agent returned invalid response" {
		t.Errorf("badresp: %+v", res)
	}
	if res := s.searchOneAgent(rej, map[string]interface{}{"query": "x"}); res.Error != "agent rejected the operation" {
		t.Errorf("reject: %+v", res)
	}
}

// errWriteOnce 写入即失败的 ResponseWriter（嵌入具体类型保留 Flush，
// 接口嵌入不会提升 Flusher——streaming 探测会失败）
type errWriteOnce struct{ *httptest.ResponseRecorder }

func (w *errWriteOnce) Write([]byte) (int, error) { return 0, errors.New("client gone") }

// TestCovLogsFollowDisconnectAndBackpressure 客户端断开的写帧失败路径
// （数据帧 / done 排干帧）、error 应答无 error 字段的兜底文案、以及
// follower 缓冲打满后的背压丢弃
func TestCovLogsFollowDisconnectAndBackpressure(t *testing.T) {
	s := covNewServer(t)
	okResp := func(string, map[string]interface{}) map[string]interface{} {
		return covScanOKPayload(map[string]interface{}{})
	}
	covFakeAgent(t, s, "flw6-err", []string{"logs"}, okResp)
	covFakeAgent(t, s, "flw6-reject", []string{"logs"}, func(string, map[string]interface{}) map[string]interface{} {
		return map[string]interface{}{"status": "error"}
	})

	// agent 应答 error 且无 error 字段 → 兜底文案
	rec := covRec()
	s.handleAgentLogsFollow(rec, covReq(http.MethodPost, "/api/agents/flw6-reject/logs/follow",
		strings.NewReader(`{"type":"systemd","source":"x.service","tail":50}`)), "flw6-reject")
	if rec.Code != http.StatusBadGateway || !strings.Contains(rec.Body.String(), "agent rejected the follow") {
		t.Errorf("reject follow: %d %s", rec.Code, rec.Body.String())
	}

	// 背压：自建 follower 塞满 256 行缓冲，后续推送走 default 丢弃
	fbp, ok := registerLogsFollower("covbp", "agent-any")
	if !ok {
		t.Fatal("register backpressure follower")
	}
	for i := 0; i < 300; i++ {
		s.HandleLogsFollowData("logs:covbp", []byte("x"))
	}
	removeLogsFollower("covbp")
	_ = fbp

	// 断开场景：errWriter 使首帧写入失败 → handler 从数据帧或 done 排干
	// 帧两条路径返回（select 竞争，跑多次覆盖两分支）
	waitFollower := func(id string) *logsFollower {
		t.Helper()
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			logsFollowersMu.Lock()
			for _, v := range logsFollowers {
				if v.agentID == id {
					logsFollowersMu.Unlock()
					return v
				}
			}
			logsFollowersMu.Unlock()
			time.Sleep(10 * time.Millisecond)
		}
		return nil
	}
	runSession := func(id string, closeIt bool) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, "/api/agents/"+id+"/logs/follow",
			strings.NewReader(`{"type":"systemd","source":"x.service","tail":50}`))
		rec := covRec()
		fin := make(chan struct{})
		go func() {
			s.handleAgentLogsFollow(&errWriteOnce{rec}, req, id)
			close(fin)
		}()
		f := waitFollower(id)
		if f == nil {
			select {
			case <-fin:
				t.Fatalf("handler %s exited early with code %d body %s", id, rec.Code, rec.Body.String())
			case <-time.After(300 * time.Millisecond):
				t.Fatalf("follower %s never appeared and handler still running", id)
			}
		}
		f.ch <- "a line"
		if closeIt {
			s.HandleLogsFollowClose("logs:"+id, "agent-exit")
		}
		select {
		case <-fin:
		case <-time.After(3 * time.Second):
			t.Fatal("handler did not return after write failure: " + id)
		}
	}
	runSession("flw6-err", false)
	runSession("flw6-err", true)
	runSession("flw6-err", true)
}
