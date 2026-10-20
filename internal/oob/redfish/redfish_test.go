package redfish

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/oob"
)

// fakeRedfish 按路径表回放的假 Redfish 服务（契约锁定：本片只做只读发现探测，
// 与 bmc-capabilities §6 的 gofish/bmclib 操作片边界见包注释）。
type fakeRedfish struct {
	status  int               // service root 状态码（默认 200）
	root    map[string]any    // service root 文档
	bodies  map[string]string // 其余路径 → 原文响应
	codes   map[string]int    // 其余路径 → 状态码（缺省 200）
	failAll bool              // 全路径 500（子资源降级）
}

func (f *fakeRedfish) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/redfish/v1" {
			if f.status == 0 {
				f.status = http.StatusOK
			}
			w.WriteHeader(f.status)
			if f.status != http.StatusOK {
				return
			}
			_ = json.NewEncoder(w).Encode(f.root)
			return
		}
		code := http.StatusOK
		if c, ok := f.codes[r.URL.Path]; ok {
			code = c
		}
		if f.failAll {
			code = http.StatusInternalServerError
		}
		w.WriteHeader(code)
		if body, ok := f.bodies[r.URL.Path]; ok {
			_, _ = w.Write([]byte(body))
		}
	}
}

func fullTree() map[string]any {
	return map[string]any{
		"RedfishVersion": "1.14.0",
		"Product":        "iDRAC 9",
		"Systems":        map[string]any{"@odata.id": "/redfish/v1/Systems"},
		"Managers":       map[string]any{"@odata.id": "/redfish/v1/Managers"},
	}
}

const (
	systemsCollection  = `{"Members":[{"@odata.id":"/redfish/v1/Systems/System.Embedded.1"}]}`
	managersCollection = `{"Members":[{"@odata.id":"/redfish/v1/Managers/iDRAC.Embedded.1"}]}`
	fullSystem         = `{"PowerState":"On","Actions":{"#ComputerSystem.Reset":{"target":"/redfish/v1/Systems/System.Embedded.1/Actions/ComputerSystem.Reset"}},"Bios":{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Bios"},"Boot":{"BootSourceOverrideTarget":"Pxe"}}`
	biosDoc            = `{"@odata.id":"/redfish/v1/Systems/System.Embedded.1/Bios","Attributes":{"BootMode":"Uefi"}}`
	managerDoc         = `{"Model":"iDRAC9","FirmwareVersion":"7.10.30.00"}`
)

func newServer(t *testing.T, f *fakeRedfish) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(f.handler())
	t.Cleanup(ts.Close)
	return ts
}

func TestProbeFullTree(t *testing.T) {
	ts := newServer(t, &fakeRedfish{
		root: fullTree(),
		bodies: map[string]string{
			"/redfish/v1/Systems":                        systemsCollection,
			"/redfish/v1/Managers":                       managersCollection,
			"/redfish/v1/Systems/System.Embedded.1":      fullSystem,
			"/redfish/v1/Systems/System.Embedded.1/Bios": biosDoc,
			"/redfish/v1/Managers/iDRAC.Embedded.1":      managerDoc,
		},
	})
	p := New(Options{})
	info, err := p.Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}
	if p.Name() != "redfish" || info.Provider != "redfish" {
		t.Fatalf("names: provider=%q info=%q", p.Name(), info.Provider)
	}
	for _, cap := range []oob.Capability{oob.CapabilityPower, oob.CapabilityInventory, oob.CapabilityBIOS, oob.CapabilityBoot} {
		st, ok := info.Capabilities[cap]
		if !ok {
			t.Errorf("capability %q missing: %+v", cap, info.Capabilities)
			continue
		}
		if !st.Usable() {
			t.Errorf("capability %q should be usable: %+v", cap, st)
		}
	}
	// console/recovery 是 OEM 面，标准探测不猜（评审：布尔仅示意，值由检测产生）。
	for _, cap := range []oob.Capability{oob.CapabilityConsole, oob.CapabilityRecovery} {
		if _, ok := info.Capabilities[cap]; ok {
			t.Errorf("capability %q must not be guessed", cap)
		}
	}
}

func TestProbeIntegratesWithRegistryMerge(t *testing.T) {
	ts := newServer(t, &fakeRedfish{
		root: fullTree(),
		bodies: map[string]string{
			"/redfish/v1/Systems":                   systemsCollection,
			"/redfish/v1/Systems/System.Embedded.1": fullSystem,
		},
	})
	r := oob.NewRegistry()
	if err := r.Register(New(Options{})); err != nil {
		t.Fatal(err)
	}
	results := r.ProbeAll(context.Background(), oob.Target{Address: ts.URL})
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("ProbeAll: %+v", results)
	}
	view := oob.Merge(results)
	if len(view.Providers) != 1 || view.Providers[0] != "redfish" {
		t.Fatalf("view providers: %v", view.Providers)
	}
	if v, ok := view.Capabilities[oob.CapabilityPower]; !ok || !v.State.Usable() || v.Source != "redfish" {
		t.Fatalf("power in view: %+v", v)
	}
}

func TestProbeUnauthorizedRootIsReachableButUndetermined(t *testing.T) {
	// 401：服务在但凭据面未接（凭据随操作片经 vault 落位）——不算错误，
	// 三态不可判就不判，空能力集如实上账。
	ts := newServer(t, &fakeRedfish{status: http.StatusUnauthorized})
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("401 should not be a probe error: %v", err)
	}
	if info.Provider != "redfish" || len(info.Capabilities) != 0 {
		t.Fatalf("undetermined probe must carry empty capabilities: %+v", info)
	}
}

func TestProbeRejectsNonRedfish(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		wantMsg string
	}{
		{"404 root", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}, "404"},
		{"non-json root", func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("<html>not redfish</html>"))
		}, "redfish"},
		{"missing version", func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]any{"Product": "some appliance"})
		}, "redfish"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(tc.handler)
			defer ts.Close()
			_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
			if err == nil || !strings.Contains(err.Error(), tc.wantMsg) {
				t.Fatalf("want error containing %q, got %v", tc.wantMsg, err)
			}
		})
	}
}

func TestProbeTransportFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	ts.Close() // 立即关：连接层失败
	_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err == nil {
		t.Fatal("transport failure must surface as probe error")
	}
	var urlErr *neturl.Error
	if !errors.As(err, &urlErr) {
		t.Fatalf("transport error should stay wrapped (audit trail): %T %v", err, err)
	}
}

func TestProbeHonorsContextCancel(t *testing.T) {
	ts := newServer(t, &fakeRedfish{root: fullTree()})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New(Options{}).Probe(ctx, oob.Target{Address: ts.URL})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ctx cancel must surface: %v", err)
	}
}

func TestProbePartialTreeDowngrades(t *testing.T) {
	// 无 Reset 动作（只读老 BMC）、无 Bios 链接、root 无 Managers 链接：
	// power/bios/boot/inventory 全部不猜，探测本身仍成功。
	ts := newServer(t, &fakeRedfish{
		root: map[string]any{
			"RedfishVersion": "1.14.0",
			"Systems":        map[string]any{"@odata.id": "/redfish/v1/Systems"},
		},
		bodies: map[string]string{
			"/redfish/v1/Systems":   `{"Members":[{"@odata.id":"/redfish/v1/Systems/1"}]}`,
			"/redfish/v1/Systems/1": `{"PowerState":"On"}`,
		},
	})
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("partial tree is not an error: %v", err)
	}
	for _, cap := range []oob.Capability{oob.CapabilityPower, oob.CapabilityBIOS, oob.CapabilityBoot, oob.CapabilityInventory} {
		if _, ok := info.Capabilities[cap]; ok {
			t.Errorf("capability %q must be omitted without evidence: %+v", cap, info.Capabilities[cap])
		}
	}
}

func TestProbeSubResourceFailuresOnlyDropReachable(t *testing.T) {
	// Bios 链接在但 GET 500、Managers 成员 500：supported/enabled 保留，
	// reachable=false（子资源失败只降可达性，不炸探测）。
	ts := newServer(t, &fakeRedfish{
		root: fullTree(),
		bodies: map[string]string{
			"/redfish/v1/Systems":                   systemsCollection,
			"/redfish/v1/Systems/System.Embedded.1": fullSystem,
			"/redfish/v1/Managers":                  managersCollection,
			"/redfish/v1/Managers/iDRAC.Embedded.1": managerDoc,
		},
		codes: map[string]int{
			"/redfish/v1/Systems/System.Embedded.1/Bios": http.StatusInternalServerError,
			"/redfish/v1/Managers/iDRAC.Embedded.1":      http.StatusInternalServerError,
		},
	})
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("sub-resource failure must not fail probe: %v", err)
	}
	bios := info.Capabilities[oob.CapabilityBIOS]
	if bios.Supported != true || bios.Enabled != true || bios.Reachable {
		t.Fatalf("bios should degrade to supported+enabled, unreachable: %+v", bios)
	}
	inv := info.Capabilities[oob.CapabilityInventory]
	if inv.Reachable || !inv.Supported {
		t.Fatalf("inventory should degrade likewise: %+v", inv)
	}
	if power := info.Capabilities[oob.CapabilityPower]; !power.Usable() {
		t.Fatalf("power unaffected by bios/manager failures: %+v", power)
	}
}

func TestProbeToleratesGarbageSubResourceBody(t *testing.T) {
	// 200 但 body 非法 JSON：该能力降级，探测不失败（fail-open 只对子资源，
	// service root 的 JSON 仍是硬门槛，见 TestProbeRejectsNonRedfish）。
	ts := newServer(t, &fakeRedfish{
		root: fullTree(),
		bodies: map[string]string{
			"/redfish/v1/Systems":                   systemsCollection,
			"/redfish/v1/Systems/System.Embedded.1": "{not json",
		},
	})
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("garbage system body must not fail probe: %v", err)
	}
	for _, cap := range []oob.Capability{oob.CapabilityPower, oob.CapabilityBIOS, oob.CapabilityBoot} {
		if _, ok := info.Capabilities[cap]; ok {
			t.Errorf("capability %q from unparseable resource must be omitted: %+v", cap, info.Capabilities[cap])
		}
	}
}

func TestProbeDefaultsToHTTPSWithoutScheme(t *testing.T) {
	// 地址不带 scheme → 默认按 https 走（§5.2 生产口径）；对只讲 http 的
	// 测试服务器表现为「不是 redfish 服务」而非 panic/挂死。
	ts := newServer(t, &fakeRedfish{root: fullTree()})
	addr := strings.TrimPrefix(ts.URL, "http://") // host:port，无 scheme
	_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: addr})
	if err == nil {
		t.Fatal("https against plain-http server must fail, not silently pass")
	}
}

func TestProbeEmptyCollectionOmitsCapabilities(t *testing.T) {
	// Systems 集合合法但 Members 为空：不猜能力（无成员 = 无证据）。
	ts := newServer(t, &fakeRedfish{
		root: map[string]any{
			"RedfishVersion": "1.14.0",
			"Systems":        map[string]any{"@odata.id": "/redfish/v1/Systems"},
		},
		bodies: map[string]string{"/redfish/v1/Systems": `{"Members":[]}`},
	})
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("empty collection is not an error: %v", err)
	}
	if _, ok := info.Capabilities[oob.CapabilityPower]; ok {
		t.Fatalf("power must be omitted on empty collection: %+v", info.Capabilities)
	}
}

func TestProbeFollowsAbsoluteMemberLink(t *testing.T) {
	// 部分 BMC 在集合成员里回完整绝对 URL（而非根起路径）：abs 须原样沿用。
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/redfish/v1":
			_ = json.NewEncoder(w).Encode(fullTree())
		case "/redfish/v1/Systems":
			_, _ = w.Write([]byte(`{"Members":[{"@odata.id":"` + ts.URL + `/redfish/v1/Systems/Abs"}]}`))
		case "/redfish/v1/Systems/Abs":
			_, _ = w.Write([]byte(fullSystem))
		}
	}))
	t.Cleanup(ts.Close)
	info, err := New(Options{}).Probe(context.Background(), oob.Target{Address: ts.URL})
	if err != nil {
		t.Fatalf("absolute member link must resolve: %v", err)
	}
	if st, ok := info.Capabilities[oob.CapabilityPower]; !ok || !st.Usable() {
		t.Fatalf("power via absolute member link: %+v", info.Capabilities)
	}
}

func TestProbeRejectsMalformedURL(t *testing.T) {
	// 带控制字符的地址：请求构造层失败，须上抛（status==0 传输分支），
	// 不许 panic 也不许吞。
	_, err := New(Options{}).Probe(context.Background(), oob.Target{Address: "http://127.0.0.1\x7f:1"})
	if err == nil || !strings.Contains(err.Error(), "redfish") {
		t.Fatalf("malformed URL must surface wrapped error: %v", err)
	}
}
