package overlay

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// 云客户端测试（D20）：httptest 假控制面，断言白名单映射、请求形态与
// UpstreamError 降级。

func TestZeroTierNetworks(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotAuth == "" {
			gotAuth = r.Header.Get("Authorization")
		}
		switch {
		case r.URL.Path == "/network":
			_, _ = w.Write([]byte(`[
				{"id":"8056c2e21c","type":"Network","config":{"name":"home","private":true}},
				{"id":"0000000000000000","type":"Network","config":{}}
			]`))
		case r.URL.Path == "/network/8056c2e21c/member":
			_, _ = w.Write([]byte(`[
				{"id":"7f3d0a9b12","name":"nas","authorized":true,"online":true,
				 "lastSeen":1758265212000,"version":"1.14.0",
				 "config":{"ipAssignments":["10.147.20.2","fd80::2"]}},
				{"id":"a1b2c3d4e5","authorized":false,"online":false}
			]`))
		case r.URL.Path == "/network/0000000000000000/member":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := NewZeroTierWithBase("tok-zt", srv.URL)
	networks, err := c.Networks(context.Background())
	if err != nil {
		t.Fatalf("Networks() error = %v", err)
	}
	if gotAuth != "Bearer tok-zt" {
		t.Errorf("Authorization = %q, want bearer token", gotAuth)
	}
	if len(networks) != 2 {
		t.Fatalf("networks = %d, want 2", len(networks))
	}
	home := networks[0]
	if home.ID != "8056c2e21c" || home.Name != "home" {
		t.Errorf("network[0] = %+v", home)
	}
	if len(home.Members) != 2 {
		t.Fatalf("members = %d, want 2", len(home.Members))
	}
	m := home.Members[0]
	if m.ID != "7f3d0a9b12" || !m.Authorized || !m.Online || m.Version != "1.14.0" {
		t.Errorf("member = %+v", m)
	}
	if len(m.IPs) != 2 || m.IPs[0] != "10.147.20.2" {
		t.Errorf("ips = %v", m.IPs)
	}
	// lastSeen 毫秒 → RFC3339 UTC（与 M1 peers 口径一致）
	if m.LastSeen != "2025-09-19T07:00:12Z" {
		t.Errorf("lastSeen = %q", m.LastSeen)
	}
	// 单网成员拉取失败降级为空成员表，不影响其余网络（D5 同款哲学）
	if len(networks[1].Members) != 0 {
		t.Errorf("degraded network should have no members, got %v", networks[1].Members)
	}
}

func TestZeroTierMutations(t *testing.T) {
	var methods, paths, bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		buf := new(strings.Builder)
		if r.Body != nil {
			var b [512]byte
			n, _ := r.Body.Read(b[:])
			buf.Write(b[:n])
		}
		bodies = append(bodies, buf.String())
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewZeroTierWithBase("tok-zt", srv.URL)
	ctx := context.Background()
	if err := c.SetMemberAuthorized(ctx, "8056c2e21c", "7f3d0a9b12", true); err != nil {
		t.Fatalf("SetMemberAuthorized(true) error = %v", err)
	}
	if err := c.SetMemberAuthorized(ctx, "8056c2e21c", "7f3d0a9b12", false); err != nil {
		t.Fatalf("SetMemberAuthorized(false) error = %v", err)
	}
	if err := c.DeleteMember(ctx, "8056c2e21c", "7f3d0a9b12"); err != nil {
		t.Fatalf("DeleteMember error = %v", err)
	}

	if methods[0] != http.MethodPost || paths[0] != "/network/8056c2e21c/member/7f3d0a9b12" {
		t.Errorf("authorize request = %s %s", methods[0], paths[0])
	}
	var payload struct {
		Authorized bool `json:"authorized"`
	}
	if err := json.Unmarshal([]byte(bodies[0]), &payload); err != nil || !payload.Authorized {
		t.Errorf("authorize body = %q (err %v)", bodies[0], err)
	}
	if err := json.Unmarshal([]byte(bodies[1]), &payload); err != nil || payload.Authorized {
		t.Errorf("deauthorize body = %q (err %v)", bodies[1], err)
	}
	if methods[2] != http.MethodDelete || paths[2] != "/network/8056c2e21c/member/7f3d0a9b12" {
		t.Errorf("delete request = %s %s", methods[2], paths[2])
	}
}

func TestZeroTierUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"invalid token"}`))
	}))
	defer srv.Close()

	c := NewZeroTierWithBase("tok-zt", srv.URL)
	_, err := c.Networks(context.Background())
	ue, ok := err.(*UpstreamError)
	if !ok || ue.StatusCode != http.StatusUnauthorized {
		t.Fatalf("Networks() error = %v, want *UpstreamError 401", err)
	}
	if !strings.Contains(ue.Error(), "401") {
		t.Errorf("error summary = %q, want status in message", ue.Error())
	}
}

func TestTailscaleDevices(t *testing.T) {
	var gotAuth, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"devices":[
			{"id":"1234567890","name":"nas.tail0434.ts.net.","addresses":["100.64.0.2"],
			 "user":"me@example.com","os":"linux","authorized":true,"online":true,
			 "keyExpiry":"2027-01-01T00:00:00Z","lastSeen":"2026-09-19T07:00:00Z"},
			{"id":"9876543210","name":"phone","os":"ios","authorized":false,"online":false}
		]}`))
	}))
	defer srv.Close()

	c := NewTailscaleWithBase("tok-ts", "", srv.URL)
	if c.TailnetName() != "-" {
		t.Errorf("TailnetName() = %q, want default \"-\"", c.TailnetName())
	}
	devices, err := c.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices() error = %v", err)
	}
	if gotPath != "/tailnet/-/devices" {
		t.Errorf("path = %q, want /tailnet/-/devices", gotPath)
	}
	if gotAuth != "Bearer tok-ts" {
		t.Errorf("Authorization = %q", gotAuth)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %d, want 2", len(devices))
	}
	d := devices[0]
	if d.ID != "1234567890" || !d.Authorized || !d.Online || d.User != "me@example.com" || d.OS != "linux" {
		t.Errorf("device = %+v", d)
	}
	if d.KeyExpiry != "2027-01-01T00:00:00Z" {
		t.Errorf("keyExpiry = %q", d.KeyExpiry)
	}
	if devices[1].Authorized {
		t.Errorf("device[1] should be unauthorized")
	}
}

func TestTailscaleMutations(t *testing.T) {
	var methods, paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := NewTailscaleWithBase("tok-ts", "example.ts.net", srv.URL)
	ctx := context.Background()
	if err := c.AuthorizeDevice(ctx, "1234567890"); err != nil {
		t.Fatalf("AuthorizeDevice error = %v", err)
	}
	if err := c.DeleteDevice(ctx, "1234567890"); err != nil {
		t.Fatalf("DeleteDevice error = %v", err)
	}
	if methods[0] != http.MethodPost || paths[0] != "/device/1234567890/authorize" {
		t.Errorf("authorize = %s %s", methods[0], paths[0])
	}
	if methods[1] != http.MethodDelete || paths[1] != "/device/1234567890" {
		t.Errorf("delete = %s %s", methods[1], paths[1])
	}
}

func TestTailscaleUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusForbidden)
	}))
	defer srv.Close()

	c := NewTailscaleWithBase("tok-ts", "-", srv.URL)
	_, err := c.Devices(context.Background())
	ue, ok := err.(*UpstreamError)
	if !ok || ue.StatusCode != http.StatusForbidden {
		t.Fatalf("Devices() error = %v, want *UpstreamError 403", err)
	}
}
