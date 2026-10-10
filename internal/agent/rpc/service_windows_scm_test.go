package rpc

import (
	"errors"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/core/platform"
)

// Windows SCM provider 全链测试（P7b 后无 build tag）：挂约注入假
// ServiceManager，覆盖 list/status/action 三动作的映射、计数、校验与
// nil 挂约防御（原非 Windows stub 的等价语义）。

type fakeSvcMgr struct {
	svcs       []platform.Service
	listErr    error
	actionErr  error
	calls      []string // "name|action" 顺序记录
	lastAction time.Duration
}

func (f *fakeSvcMgr) List() ([]platform.Service, error) { return f.svcs, f.listErr }

func (f *fakeSvcMgr) Action(name, action string, timeout time.Duration) error {
	f.calls = append(f.calls, name+"|"+action)
	f.lastAction = timeout
	return f.actionErr
}

// withSvcMgr 注入假挂约并注册恢复
func withSvcMgr(t *testing.T, mgr platform.ServiceManager) *fakeSvcMgr {
	t.Helper()
	fake, _ := mgr.(*fakeSvcMgr)
	old := serviceWindowsMgr
	serviceWindowsMgr = func() platform.ServiceManager { return mgr }
	t.Cleanup(func() { serviceWindowsMgr = old })
	return fake
}

func TestWindowsServiceListMapsAndSorts(t *testing.T) {
	withSvcMgr(t, &fakeSvcMgr{svcs: []platform.Service{
		{Name: "wuauserv", DisplayName: "Windows Update", Status: "Stopped", StartType: "Manual"},
		{Name: "bits", DisplayName: "BITS", Status: "Running", StartType: "Automatic (Delayed)"},
	}})
	p := NewWindowsServiceProvider()
	res, err := p.Call("list", nil)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	m := res.(map[string]interface{})
	units := m["services"].([]ServiceUnit)
	if len(units) != 2 {
		t.Fatalf("units = %d, want 2", len(units))
	}
	// 名称升序：bits 在前
	if units[0].Name != "bits" || units[1].Name != "wuauserv" {
		t.Fatalf("sort wrong: %s, %s", units[0].Name, units[1].Name)
	}
	if units[0].Description != "BITS" || units[0].UnitFileState != "enabled" {
		t.Fatalf("bits mapping wrong: %+v", units[0])
	}
	if units[1].UnitFileState != "disabled" {
		t.Fatalf("manual start type should map disabled, got %q", units[1].UnitFileState)
	}
}

func TestWindowsServiceListErrorPassthrough(t *testing.T) {
	withSvcMgr(t, &fakeSvcMgr{listErr: errors.New("scm connect: boom")})
	p := NewWindowsServiceProvider()
	if _, err := p.List(); err == nil || err.Error() != "scm connect: boom" {
		t.Fatalf("err = %v", err)
	}
}

func TestWindowsServiceStatusCounts(t *testing.T) {
	withSvcMgr(t, &fakeSvcMgr{svcs: []platform.Service{
		{Name: "a", Status: "Running", StartType: "Automatic"},
		{Name: "b", Status: "Paused", StartType: "Disabled"},
		{Name: "c", Status: "Stopped", StartType: "Automatic"},
	}})
	p := NewWindowsServiceProvider()
	res, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	m := res.(map[string]interface{})
	if m["total"] != 3 || m["active"] != 2 || m["failed"] != 0 || m["enabled"] != 2 || m["systemState"] != "unknown" {
		t.Fatalf("status = %+v", m)
	}
}

func TestWindowsServiceActionValidatesBeforeManager(t *testing.T) {
	fake := withSvcMgr(t, &fakeSvcMgr{})
	p := NewWindowsServiceProvider()

	// 非法 unit 名 / 动词 / reload：拒绝且不达挂约
	if _, err := p.Call("action", map[string]interface{}{"name": "../evil", "action": "start"}); err == nil {
		t.Fatal("invalid name should reject")
	}
	if _, err := p.Call("action", map[string]interface{}{"name": "svc", "action": "reboot"}); err == nil {
		t.Fatal("invalid verb should reject")
	}
	if _, err := p.Call("action", map[string]interface{}{"name": "svc", "action": "reload"}); err == nil {
		t.Fatal("reload should reject on windows backend")
	}
	if len(fake.calls) != 0 {
		t.Fatalf("rejected actions must not reach manager: %v", fake.calls)
	}

	// 合法动作：透传 name/action 与统一超时
	if _, err := p.Call("action", map[string]interface{}{"name": "wuauserv", "action": "restart"}); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != "wuauserv|restart" {
		t.Fatalf("calls = %v", fake.calls)
	}
	if fake.lastAction != serviceActionTimeout {
		t.Fatalf("timeout = %v, want %v", fake.lastAction, serviceActionTimeout)
	}
}

func TestWindowsServiceActionErrorPassthrough(t *testing.T) {
	withSvcMgr(t, &fakeSvcMgr{actionErr: errors.New("start svc: access denied")})
	p := NewWindowsServiceProvider()
	if _, err := p.Call("action", map[string]interface{}{"name": "svc", "action": "start"}); err == nil ||
		err.Error() != "start svc: access denied" {
		t.Fatalf("err = %v", err)
	}
}

// TestWindowsServiceNilManager 防御层（原 stub 语义）：挂约 nil（非 windows
// 构建/未来 GOOS 收窄）时三动作显式报错，正常路径探测门控根本不会注册本
// provider。
func TestWindowsServiceNilManager(t *testing.T) {
	withSvcMgr(t, nil)
	p := NewWindowsServiceProvider()
	if p.Type() != "service" {
		t.Fatalf("Type() = %q", p.Type())
	}
	for _, action := range []string{"list", "status"} {
		if _, err := p.Call(action, nil); err == nil ||
			err.Error() != "windows-scm service backend requires a windows agent build" {
			t.Errorf("Call(%q) err = %v", action, err)
		}
	}
	if _, err := p.Call("action", map[string]interface{}{"name": "svc", "action": "start"}); err == nil ||
		err.Error() != "windows-scm service backend requires a windows agent build" {
		t.Errorf("action err = %v", err)
	}
}

func TestWindowsServiceUnknownAction(t *testing.T) {
	withSvcMgr(t, &fakeSvcMgr{})
	p := NewWindowsServiceProvider()
	if _, err := p.Call("reboot", nil); err == nil {
		t.Fatal("unknown action should reject")
	}
}
