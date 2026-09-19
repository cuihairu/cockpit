package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/overlay"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// Overlay 云管理面端点测试（M2 D13/D14/D19）：httptest 假云端 + 直构
// &Server{}，断言校验、managed 对照、降级与审计。

// newOverlayCloudTestServer 直构两 provider 就绪的测试 server
func newOverlayCloudTestServer(t *testing.T, ztHandle, tsHandle func(r *http.Request) (int, string)) *Server {
	t.Helper()
	s := newBackupTestServer(t)
	if ztHandle != nil {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code, body := ztHandle(r)
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		s.overlayZT = overlay.NewZeroTierWithBase("tok-zt", srv.URL)
	}
	if tsHandle != nil {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			code, body := tsHandle(r)
			w.WriteHeader(code)
			_, _ = w.Write([]byte(body))
		}))
		t.Cleanup(srv.Close)
		s.overlayTS = overlay.NewTailscaleWithBase("tok-ts", "-", srv.URL)
	}
	return s
}

// withOverlayIdentityAgent 注册携带 overlay identity metadata 的假 agent
func withOverlayIdentityAgent(t *testing.T, s *Server, id string, ztNodeID, tsDeviceID string) {
	t.Helper()
	metadata := map[string]any{
		"zerotier": true, "tailscale": true,
	}
	identity := map[string]any{}
	if ztNodeID != "" {
		identity["zerotier"] = map[string]any{"nodeId": ztNodeID}
	}
	if tsDeviceID != "" {
		identity["tailscale"] = map[string]any{"id": tsDeviceID}
	}
	if len(identity) > 0 {
		metadata["identity"] = identity
	}
	agent := NewAgent(id, nil)
	agent.Capabilities = []protocol.Capability{{Type: "overlay", Metadata: metadata}}
	if err := s.registry.Register(agent); err != nil {
		t.Fatal(err)
	}
}

func doOverlayCloud(s *Server, method, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleOverlayCloud(w, r)
	return w
}

func TestOverlayCloudListManagedFlags(t *testing.T) {
	s := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) {
			switch r.URL.Path {
			case "/network":
				return 200, `[{"id":"8056c2e21c000000","config":{"name":"home"}}]`
			case "/network/8056c2e21c000000/member":
				return 200, `[
					{"id":"7f3d0a9b12","name":"nas","authorized":true,"online":true},
					{"id":"a1b2c3d4e5","name":"stray","authorized":false,"online":false}
				]`
			}
			return 404, `{}`
		},
		func(r *http.Request) (int, string) {
			if r.URL.Path == "/tailnet/-/devices" {
				return 200, `{"devices":[
					{"id":"1234567890","name":"nas.tail.ts.net.","authorized":true},
					{"id":"9876543210","name":"phone","authorized":false}
				]}`
			}
			return 404, `{}`
		})
	withOverlayIdentityAgent(t, s, "agent-1", "7f3d0a9b12", "9876543210")

	w := doOverlayCloud(s, http.MethodGet, "/api/overlay/cloud", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list = %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	for _, want := range []string{
		`"configured":true`,
		// ZT：面板 agent nodeId 命中 → managed
		`{"id":"7f3d0a9b12","name":"nas","authorized":true,"online":true,"managed":true}`,
		// ZT：云端多出的成员 → unmanaged（本设计核心卖点）
		`{"id":"a1b2c3d4e5","name":"stray","authorized":false,"online":false,"managed":false}`,
		// TS：device id 命中（agent 报的是 9876543210）
		`{"id":"1234567890","name":"nas.tail.ts.net.","authorized":true,"online":false,"managed":false}`,
		`{"id":"9876543210","name":"phone","authorized":false,"online":false,"managed":true}`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}
}

func TestOverlayCloudListUnconfiguredAndDegraded(t *testing.T) {
	// 两 provider 都未配置：读不报错，段级 configured:false（D13）
	s := newOverlayCloudTestServer(t, nil, nil)
	w := doOverlayCloud(s, http.MethodGet, "/api/overlay/cloud", "")
	if w.Code != http.StatusOK {
		t.Fatalf("unconfigured list = %d %s", w.Code, w.Body.String())
	}
	if strings.Count(w.Body.String(), `"configured":false`) != 2 {
		t.Errorf("want both segments configured:false: %s", w.Body.String())
	}

	// 单 provider 失败降级：ZT 500 → 该段 error，TS 正常返回（不整体 5xx）
	s2 := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) { return 500, `{"error":"zt down"}` },
		func(r *http.Request) (int, string) {
			return 200, `{"devices":[{"id":"1","name":"d","authorized":true}]}`
		})
	withOverlayIdentityAgent(t, s2, "agent-1", "", "1")
	w = doOverlayCloud(s2, http.MethodGet, "/api/overlay/cloud", "")
	if w.Code != http.StatusOK {
		t.Fatalf("degraded list = %d %s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error":"overlay cloud api status 500`) {
		t.Errorf("zt segment should carry error: %s", body)
	}
	if !strings.Contains(body, `"name":"d"`) {
		t.Errorf("tailscale segment should still return devices: %s", body)
	}
	// agent 身份命中 device "1" → managed:true 恰一枚
	if strings.Count(body, `"managed":true`) != 1 {
		t.Errorf("managed flags = %s", body)
	}
}

func TestOverlayCloudMutationsAndAudit(t *testing.T) {
	var ztMethod, ztPath string
	s := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) {
			ztMethod, ztPath = r.Method, r.URL.Path
			return 200, `{}`
		},
		func(r *http.Request) (int, string) {
			return 200, `{}`
		})
	withOverlayIdentityAgent(t, s, "agent-1", "7f3d0a9b12", "")

	// ZT 授权
	w := doOverlayCloud(s, http.MethodPost,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12",
		`{"authorized":true}`)
	if w.Code != http.StatusOK {
		t.Fatalf("authorize = %d %s", w.Code, w.Body.String())
	}
	if ztMethod != http.MethodPost || ztPath != "/network/8056c2e21c000000/member/7f3d0a9b12" {
		t.Errorf("cloud request = %s %s", ztMethod, ztPath)
	}

	// ZT 取消授权
	w = doOverlayCloud(s, http.MethodPost,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12",
		`{"authorized":false}`)
	if w.Code != http.StatusOK {
		t.Fatalf("deauthorize = %d %s", w.Code, w.Body.String())
	}

	// ZT 除名
	w = doOverlayCloud(s, http.MethodDelete,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12", "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete = %d %s", w.Code, w.Body.String())
	}

	// TS 授权与除名
	w = doOverlayCloud(s, http.MethodPost, "/api/overlay/cloud/tailscale/devices/123/authorize", "")
	if w.Code != http.StatusOK {
		t.Fatalf("ts authorize = %d %s", w.Code, w.Body.String())
	}
	w = doOverlayCloud(s, http.MethodDelete, "/api/overlay/cloud/tailscale/devices/123", "")
	if w.Code != http.StatusOK {
		t.Fatalf("ts delete = %d %s", w.Code, w.Body.String())
	}

	// 审计落库（D14）
	logs, _, err := s.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	actions := map[string]int{}
	for _, l := range logs {
		actions[l.Action]++
	}
	if actions["overlay_authz"] != 3 || actions["overlay_remove"] != 2 {
		t.Fatalf("audit actions = %v, want authz=3 remove=2", actions)
	}
	// 审计详情不含 token
	for _, l := range logs {
		if strings.Contains(l.Details, "tok-zt") || strings.Contains(l.Details, "tok-ts") {
			t.Errorf("audit leaked token: %s", l.Details)
		}
	}
}

func TestOverlayCloudValidations(t *testing.T) {
	called := false
	s := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) { called = true; return 200, `{}` },
		func(r *http.Request) (int, string) { called = true; return 200, `{}` })

	cases := []struct {
		name, method, path, body string
		want                     int
	}{
		{"bad zt network id", http.MethodPost, "/api/overlay/cloud/zerotier/networks/ZZZZ/members/7f3d0a9b12", `{"authorized":true}`, 400},
		{"short zt network id", http.MethodPost, "/api/overlay/cloud/zerotier/networks/8056c2e21/members/7f3d0a9b12", `{"authorized":true}`, 400},
		{"bad zt member id", http.MethodPost, "/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/nope", `{"authorized":true}`, 400},
		{"missing authorized field", http.MethodPost, "/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12", `{}`, 400},
		{"bad json", http.MethodPost, "/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12", `{`, 400},
		{"bad ts device id", http.MethodPost, "/api/overlay/cloud/tailscale/devices/abc/authorize", "", 400},
		{"ts unknown suffix", http.MethodPost, "/api/overlay/cloud/tailscale/devices/123/rename", "", 404},
		{"unknown subpath", http.MethodGet, "/api/overlay/cloud/other", "", 404},
	}
	for _, tc := range cases {
		w := doOverlayCloud(s, tc.method, tc.path, tc.body)
		if w.Code != tc.want {
			t.Errorf("%s: code = %d, want %d (%s)", tc.name, w.Code, tc.want, w.Body.String())
		}
	}
	// 校验拒绝的请求绝不透传到云端（D19）
	if called {
		t.Error("validation failures must not reach the cloud API")
	}
}

func TestOverlayCloudMutationsNotConfiguredAndUpstream(t *testing.T) {
	// 未配置：变更 503 + 指名引导键（D12）
	s := newOverlayCloudTestServer(t, nil, nil)
	w := doOverlayCloud(s, http.MethodPost,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12", `{"authorized":true}`)
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "ZEROTIER_API_TOKEN") {
		t.Fatalf("zt unconfigured = %d %s", w.Code, w.Body.String())
	}
	w = doOverlayCloud(s, http.MethodDelete, "/api/overlay/cloud/tailscale/devices/123", "")
	if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), "TAILSCALE_API_TOKEN") {
		t.Fatalf("ts unconfigured = %d %s", w.Code, w.Body.String())
	}

	// 云端 401 → 502，摘要不含 token（D12）
	s2 := newOverlayCloudTestServer(t,
		func(r *http.Request) (int, string) { return 401, `{"error":"bad token"}` }, nil)
	w = doOverlayCloud(s2, http.MethodDelete,
		"/api/overlay/cloud/zerotier/networks/8056c2e21c000000/members/7f3d0a9b12", "")
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "401") {
		t.Fatalf("upstream 401 = %d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "tok-zt") {
		t.Errorf("error leaked token: %s", w.Body.String())
	}
	// 失败不记审计
	logs, _, err := s2.db.GetAuditLogs(0, 50, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range logs {
		if l.Action == "overlay_remove" {
			t.Errorf("failed mutation should not be audited: %+v", l)
		}
	}
}

func TestOverlayIdentityFromCapabilities(t *testing.T) {
	nodeID, deviceID := overlayIdentityFromCapabilities([]protocol.Capability{
		{Type: "hardware-monitor", Metadata: map[string]any{"smart": true}},
		{Type: "overlay", Metadata: map[string]any{
			"identity": map[string]any{
				"zerotier":  map[string]any{"nodeId": "7f3d0a9b12"},
				"tailscale": map[string]any{"id": "123"},
			},
		}},
	})
	if nodeID != "7f3d0a9b12" || deviceID != "123" {
		t.Errorf("identity = (%q, %q)", nodeID, deviceID)
	}

	// 形状畸形（JSON 往返后可能的形态）不 panic、返回空
	nodeID, deviceID = overlayIdentityFromCapabilities([]protocol.Capability{
		{Type: "overlay", Metadata: map[string]any{
			"identity": "not-a-map",
		}},
	})
	if nodeID != "" || deviceID != "" {
		t.Errorf("malformed identity = (%q, %q), want empty", nodeID, deviceID)
	}
}
