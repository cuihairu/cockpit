package probe

// cov_binding_ref_test.go D7 probe 域名引用覆盖：binding://{agentID}/{target}
// 解析、失效可见（down + error）、类型限制、换域名跟随。

import (
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/health"
	"github.com/cuihairu/cockpit/internal/storage"
)

// recordingHTTPChecker 记录 CheckHTTP 收到的 target（断言引用解析结果）
type recordingHTTPChecker struct{ gotHTTP, gotTCP string }

func (c *recordingHTTPChecker) CheckDNS(service, target string) *health.Result {
	return &health.Result{Service: service, Type: "dns", Target: target, Status: health.StatusHealthy}
}
func (c *recordingHTTPChecker) CheckHTTP(service, target string, expectedStatus int) *health.Result {
	c.gotHTTP = target
	return &health.Result{Service: service, Type: "http", Target: target, Status: health.StatusHealthy}
}
func (c *recordingHTTPChecker) CheckTCP(service, addr string) *health.Result {
	c.gotTCP = addr
	return &health.Result{Service: service, Type: "tcp", Target: addr, Status: health.StatusHealthy}
}

func seedBindingRef(t *testing.T, db *storage.DB, domain, agentID, target string, enabled bool) {
	t.Helper()
	if !enabled {
		_ = db.DeleteDomainBinding(domain)
	}
	b := &storage.DomainBinding{Domain: domain, AgentID: agentID, Target: target, Enabled: enabled,
		AutoDNS: true, AutoProxy: true, AutoCert: true}
	if err := db.SaveDomainBinding(b); err != nil {
		t.Fatalf("seed binding %s: %v", domain, err)
	}
}

func TestResolveBindingTarget(t *testing.T) {
	r, db := newTestRunner(t)
	seedBindingRef(t, db, "blog.example.com", "a1", "127.0.0.1:8080", true)
	seedBindingRef(t, db, "nas.example.com", "a1", "docker://nas", true)

	// 命中（含 docker:// 整串 target）
	if got, err := r.resolveBindingTarget("binding://a1/127.0.0.1:8080"); err != nil || got != "https://blog.example.com" {
		t.Errorf("host ref = %q (%v)", got, err)
	}
	if got, err := r.resolveBindingTarget("binding://a1/docker://nas"); err != nil || got != "https://nas.example.com" {
		t.Errorf("docker ref = %q (%v)", got, err)
	}

	// 语法错：无 target / 空 agent
	if _, err := r.resolveBindingTarget("binding://a1"); err == nil || !strings.Contains(err.Error(), "invalid binding reference") {
		t.Errorf("no-target ref err = %v", err)
	}
	if _, err := r.resolveBindingTarget("binding:///127.0.0.1:8080"); err == nil || !strings.Contains(err.Error(), "invalid binding reference") {
		t.Errorf("empty-agent ref err = %v", err)
	}

	// 未命中
	if _, err := r.resolveBindingTarget("binding://a1/10.0.0.1:9999"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("miss ref err = %v", err)
	}

	// 停用
	seedBindingRef(t, db, "blog.example.com", "a1", "127.0.0.1:8080", false)
	if _, err := r.resolveBindingTarget("binding://a1/127.0.0.1:8080"); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Errorf("disabled ref err = %v", err)
	}
}

func TestProbeServiceBindingReference(t *testing.T) {
	r, db := newTestRunner(t)
	fake := &recordingHTTPChecker{}
	r.healthChecker = fake
	seedBindingRef(t, db, "blog.example.com", "a1", "127.0.0.1:8080", true)

	// http + 有效引用 → 以解析后的 https://{域名} 探测
	pr := r.probeService(&storage.Service{ID: "s1", Name: "blog", Type: "http",
		URL: "binding://a1/127.0.0.1:8080"})
	if pr.Status != "up" || fake.gotHTTP != "https://blog.example.com" {
		t.Errorf("ref probe = %q target=%q, want up + https://blog.example.com", pr.Status, fake.gotHTTP)
	}

	// 引用失效 → down 且错误可见
	pr = r.probeService(&storage.Service{ID: "s2", Name: "dead", Type: "http",
		URL: "binding://a1/10.0.0.1:9999"})
	if pr.Status != "down" || !strings.Contains(pr.Error, "not found") {
		t.Errorf("stale ref = %+v, want down + error", pr)
	}

	// tcp 类型不支持引用
	pr = r.probeService(&storage.Service{ID: "s3", Name: "db", Type: "tcp",
		URL: "binding://a1/127.0.0.1:8080"})
	if pr.Status != "down" || !strings.Contains(pr.Error, "requires http/https") {
		t.Errorf("tcp ref = %+v", pr)
	}

	// 普通 URL 直通不受影响
	pr = r.probeService(&storage.Service{ID: "s4", Name: "plain", Type: "http",
		URL: "http://127.0.0.1:9999"})
	if fake.gotHTTP != "http://127.0.0.1:9999" {
		t.Errorf("plain url target = %q", fake.gotHTTP)
	}
}

// TestProbeBindingRefFollowsRename D7 闭环演示：binding 换域名（删旧建新
// 同 agent+target）后，引用型探测目标自动跟随，无需改 service 配置
func TestProbeBindingRefFollowsRename(t *testing.T) {
	r, db := newTestRunner(t)
	fake := &recordingHTTPChecker{}
	r.healthChecker = fake

	seedBindingRef(t, db, "blog.old.com", "a1", "127.0.0.1:8080", true)
	svc := &storage.Service{ID: "s1", Name: "blog", Type: "http", URL: "binding://a1/127.0.0.1:8080"}
	r.probeService(svc)
	if fake.gotHTTP != "https://blog.old.com" {
		t.Fatalf("before rename = %q", fake.gotHTTP)
	}

	// 换域名：删旧建新，agent+target 不变
	if err := db.DeleteDomainBinding("blog.old.com"); err != nil {
		t.Fatal(err)
	}
	seedBindingRef(t, db, "blog.new.com", "a1", "127.0.0.1:8080", true)
	r.probeService(svc)
	if fake.gotHTTP != "https://blog.new.com" {
		t.Fatalf("after rename = %q, want https://blog.new.com", fake.gotHTTP)
	}
}
