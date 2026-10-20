package rpc

// oob_provider_redirect_test.go 兜底转发编排 RPC（片C）：注入 fake executor，
// 断言参数解析、规则名生成（过期时刻进名字）、凭据不进错误文案、
// 拆除只接受 cockpit 前缀规则名与列表透传。无真实 ubus 调用。

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/oob/redirect"
	"github.com/cuihairu/cockpit/internal/openwrt"
)

var errBoom = errors.New("boom")

// fakeExecutor 记录调用并回放错误。rules 非空时列表返回它（缺省两条），
// removeErr 只作用于拆除（清扫单条失败分支用）。
type fakeExecutor struct {
	added     []map[string]interface{}
	removed   []string
	listed    bool
	err       error
	rules     []map[string]interface{}
	removeErr error
}

func (f *fakeExecutor) AddFirewallRedirect(p map[string]interface{}) error {
	if f.err != nil {
		return f.err
	}
	f.added = append(f.added, p)
	return nil
}
func (f *fakeExecutor) RemoveFirewallRedirect(name string) error {
	if f.removeErr != nil {
		return f.removeErr
	}
	if f.err != nil {
		return f.err
	}
	f.removed = append(f.removed, name)
	return nil
}
func (f *fakeExecutor) ListFirewallRedirects() ([]map[string]interface{}, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.listed = true
	if f.rules != nil {
		return f.rules, nil
	}
	return []map[string]interface{}{{"name": "cockpit_a_1"}, {"name": "user_rule"}}, nil
}

func withFakeRedirectExecutor(t *testing.T, f *fakeExecutor) {
	t.Helper()
	orig := newRedirectExecutor
	t.Cleanup(func() { newRedirectExecutor = orig })
	newRedirectExecutor = func(cfg openwrt.Config) redirectExecutor { return f }
}

func redirectSpecTestParams() map[string]interface{} {
	return map[string]interface{}{
		"host": "10.0.0.9", "label": "a", "proto": "tcp", "src_zone": "wan",
		"dst_zone": "lan", "dst_ip": "1.2.3.4", "dst_port": 22,
	}
}

// ============ 下发 ============

func TestOobRedirectForwards(t *testing.T) {
	f := &fakeExecutor{}
	withFakeRedirectExecutor(t, f)

	params := redirectSpecTestParams()
	for k, v := range map[string]interface{}{
		"port": 8443, "username": "root", "password": "pw", "insecure_tls": true,
		"label": "amtbmc", "src_ip": "10.0.0.5", "src_port": 51000, "ttl_seconds": 60,
	} {
		params[k] = v
	}

	out, err := NewOobProvider(nil, nil, nil).ApplyRedirect(params)
	if err != nil {
		t.Fatalf("ApplyRedirect: %v", err)
	}
	m := out.(map[string]interface{})
	name, ok := m["name"].(string)
	if !ok || !redirect.Owned(name) {
		t.Fatalf("name = %v", m["name"])
	}
	_, expiry, ok := redirect.SplitRuleName(name)
	if !ok || expiry.Unix()-time.Now().Unix() < 55 {
		t.Fatalf("expiry = %v (name %s)", expiry, name)
	}
	if m["ttl_seconds"] != int64(60) {
		t.Fatalf("ttl = %v", m["ttl_seconds"])
	}
	if len(f.added) != 1 {
		t.Fatalf("added = %v", f.added)
	}
	if f.added[0]["name"] != name || f.added[0]["dest_port"] != "22" ||
		f.added[0]["src_ip"] != "10.0.0.5" || f.added[0]["src_port"] != "51000" {
		t.Fatalf("payload = %v", f.added[0])
	}
}

func TestOobRedirectParamErrors(t *testing.T) {
	mutate := func(kv map[string]interface{}) map[string]interface{} {
		out := redirectSpecTestParams()
		for k, v := range kv {
			out[k] = v
		}
		return out
	}
	bad := map[string]map[string]interface{}{
		"nil params":  nil,
		"no host":     mutate(map[string]interface{}{"host": ""}),
		"blank host":  mutate(map[string]interface{}{"host": "  "}),
		"bad label":   mutate(map[string]interface{}{"label": "A B"}),
		"bad proto":   mutate(map[string]interface{}{"proto": "icmp"}),
		"no src zone": mutate(map[string]interface{}{"src_zone": ""}),
		"no dst zone": mutate(map[string]interface{}{"dst_zone": ""}),
		"bad dst ip":  mutate(map[string]interface{}{"dst_ip": "not-an-ip"}),
		"bad src ip":  mutate(map[string]interface{}{"src_ip": "1.2.3.999"}),
		"dst port 0":  mutate(map[string]interface{}{"dst_port": 0}),
		"dst port hi": mutate(map[string]interface{}{"dst_port": 70000}),
	}
	withFakeRedirectExecutor(t, &fakeExecutor{})
	for name, params := range bad {
		if _, err := NewOobProvider(nil, nil, nil).ApplyRedirect(params); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}

func TestOobRedirectOptionalParams(t *testing.T) {
	f := &fakeExecutor{}
	withFakeRedirectExecutor(t, f)

	// 空标签合法：规则名按 proto/目标推导；缺省 TTL + 非法端口回落
	// （paramInt 拒绝越界，等价于未提供）
	out, err := NewOobProvider(nil, nil, nil).ApplyRedirect(mutate(redirectSpecTestParams(), map[string]interface{}{
		"label": "", "src_port": 70000, "port": 70000,
	}))
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]interface{})
	if got := m["ttl_seconds"]; got != int64(redirect.DefaultTTL/time.Second) {
		t.Fatalf("ttl = %v", got)
	}
	if name := m["name"].(string); !strings.HasPrefix(name, redirect.RulePrefix+"tcp.1.2.3.4.22_") {
		t.Fatalf("derived name = %s", name)
	}
	if _, has := f.added[0]["src_port"]; has {
		t.Fatalf("zero src_port must be omitted: %v", f.added[0])
	}
}

func mutate(params, kv map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range params {
		out[k] = v
	}
	for k, v := range kv {
		out[k] = v
	}
	return out
}

func TestOobRedirectExecError(t *testing.T) {
	f := &fakeExecutor{err: errBoom}
	withFakeRedirectExecutor(t, f)
	params := redirectSpecTestParams()
	params["password"] = "secretpw"
	_, err := NewOobProvider(nil, nil, nil).ApplyRedirect(params)
	if err == nil {
		t.Fatal("want error")
	}
	if strings.Contains(err.Error(), "secretpw") {
		t.Fatalf("credential leaked into error: %v", err)
	}
}

// ============ 拆除 ============

func TestOobRedirectRemoveForwards(t *testing.T) {
	f := &fakeExecutor{}
	withFakeRedirectExecutor(t, f)
	out, err := NewOobProvider(nil, nil, nil).RemoveRedirect(map[string]interface{}{
		"host": "10.0.0.9", "name": "cockpit_a_1", "port": 8443,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.(map[string]interface{})["name"] != "cockpit_a_1" || len(f.removed) != 1 {
		t.Fatalf("out = %v removed = %v", out, f.removed)
	}
}

func TestOobRedirectRemoveParamErrors(t *testing.T) {
	bad := map[string]map[string]interface{}{
		"nil params": nil,
		"no name":    map[string]interface{}{"host": "10.0.0.9"},
		"empty name": map[string]interface{}{"host": "10.0.0.9", "name": "  "},
		"foreign":    map[string]interface{}{"host": "10.0.0.9", "name": "user_rule"},
		"no host":    map[string]interface{}{"name": "cockpit_a_1"},
		"blank host": map[string]interface{}{"name": "cockpit_a_1", "host": "  "},
	}
	for name, params := range bad {
		if _, err := NewOobProvider(nil, nil, nil).RemoveRedirect(params); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}

func TestOobRedirectRemoveExecError(t *testing.T) {
	f := &fakeExecutor{err: errBoom}
	withFakeRedirectExecutor(t, f)
	_, err := NewOobProvider(nil, nil, nil).RemoveRedirect(map[string]interface{}{
		"host": "10.0.0.9", "name": "cockpit_a_1",
	})
	if err == nil || !strings.Contains(err.Error(), "failed to remove redirect") {
		t.Fatalf("err = %v", err)
	}
}

// ============ 列表 ============

func TestOobRedirectListForwards(t *testing.T) {
	f := &fakeExecutor{}
	withFakeRedirectExecutor(t, f)
	out, err := NewOobProvider(nil, nil, nil).ListRedirects(map[string]interface{}{"host": "10.0.0.9"})
	if err != nil {
		t.Fatal(err)
	}
	if !f.listed {
		t.Fatal("list must be called")
	}
	if out.(map[string]interface{})["redirects"] == nil {
		t.Fatalf("out = %v", out)
	}
}

func TestOobRedirectListErrors(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]interface{}
		err    error
	}{
		{"nil params", nil, nil},
		{"no host", map[string]interface{}{}, nil},
		{"exec error", map[string]interface{}{"host": "10.0.0.9"}, errBoom},
	}
	for _, c := range cases {
		withFakeRedirectExecutor(t, &fakeExecutor{err: c.err})
		if _, err := NewOobProvider(nil, nil, nil).ListRedirects(c.params); err == nil {
			t.Fatalf("%s: want error", c.name)
		}
	}
}

// ============ 清扫 ============

// TestOobRedirectSweepForwards 只拆过期且 cockpit 前缀的规则：未到期、
// 用户自建、名字字段残缺的一律不动。
func TestOobRedirectSweepForwards(t *testing.T) {
	f := &fakeExecutor{}
	withFakeRedirectExecutor(t, f)
	f.rules = []map[string]interface{}{
		{"name": "cockpit_expired_1"}, // 过期时刻 unix 1
		{"name": fmt.Sprintf("cockpit_fresh_%d", time.Now().Add(time.Hour).Unix())}, // 未到期
		{"name": "user_rule"}, // 用户自建
		{"name": 42},          // 名字非字符串
		{"src": "wan"},        // 无名字
	}
	out, err := NewOobProvider(nil, nil, nil).SweepRedirects(map[string]interface{}{"host": "10.0.0.9"})
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]interface{})
	if m["count"] != 1 {
		t.Fatalf("count = %v (removed %v, exec %v)", m["count"], m["removed"], f.removed)
	}
	if removed := m["removed"].([]string); len(removed) != 1 || removed[0] != "cockpit_expired_1" {
		t.Fatalf("removed = %v", removed)
	}
	if len(f.removed) != 1 || f.removed[0] != "cockpit_expired_1" {
		t.Fatalf("exec removed = %v", f.removed)
	}
}

func TestOobRedirectSweepErrors(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]interface{}
		f      *fakeExecutor
	}{
		{"nil params", nil, &fakeExecutor{}},
		{"no host", map[string]interface{}{}, &fakeExecutor{}},
		{"list error", map[string]interface{}{"host": "10.0.0.9"}, &fakeExecutor{err: errBoom}},
	}
	for _, c := range cases {
		withFakeRedirectExecutor(t, c.f)
		if _, err := NewOobProvider(nil, nil, nil).SweepRedirects(c.params); err == nil {
			t.Fatalf("%s: want error", c.name)
		}
	}
}

// TestOobRedirectSweepRemoveError 单条拆除失败即中止；错误文案带规则名不带凭据。
func TestOobRedirectSweepRemoveError(t *testing.T) {
	f := &fakeExecutor{rules: []map[string]interface{}{{"name": "cockpit_expired_1"}}, removeErr: errBoom}
	withFakeRedirectExecutor(t, f)
	params := map[string]interface{}{"host": "10.0.0.9", "password": "secretpw"}
	_, err := NewOobProvider(nil, nil, nil).SweepRedirects(params)
	if err == nil || !strings.Contains(err.Error(), `failed to remove "cockpit_expired_1"`) {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "secretpw") {
		t.Fatalf("credential leaked into error: %v", err)
	}
}

// ============ 分发 ============

func TestOobRedirectDispatch(t *testing.T) {
	p := NewOobProvider(nil, nil, nil)
	for _, action := range []string{"redirect", "redirect.remove", "redirect.list", "redirect.sweep"} {
		if _, err := p.Call(action, nil); err == nil {
			t.Fatalf("%s must error on nil params", action)
		}
	}
	if _, err := p.Call("redirect.unknown", nil); err == nil {
		t.Fatal("unknown redirect action must error")
	}
}

// TestRedirectExecutorDefaults 缺省注入点装配真实客户端且不报错。
func TestRedirectExecutorDefaults(t *testing.T) {
	if c := newRedirectExecutor(openwrt.Config{Host: "127.0.0.1", Port: 80}); c == nil {
		t.Fatal("default executor must be non-nil")
	}
}
