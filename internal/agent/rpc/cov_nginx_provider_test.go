package rpc

// 覆盖率补充测试：nginx_provider.go 错误分支与辅助函数。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func covValidSite() *ProxySite {
	return &ProxySite{
		Name: "blog", ServerNames: []string{"blog.example.com"},
		Upstream: "127.0.0.1:3000", Scheme: "http",
	}
}

func TestCovNginxExtraTooLarge(t *testing.T) {
	s := covValidSite()
	s.Extra = strings.Repeat("x", nginxMaxExtra+1)
	if err := s.validate(); err == nil || !strings.Contains(err.Error(), "extra directives too large") {
		t.Errorf("oversized extra err = %v", err)
	}
}

func TestCovDefaultCommander(t *testing.T) {
	out, stderr, err := defaultCommander(context.Background(), "true")
	if err != nil || len(out) != 0 || len(stderr) != 0 {
		t.Errorf("true: out=%q stderr=%q err=%v", out, stderr, err)
	}
	_, _, err = defaultCommander(context.Background(), "sh", "-c", "echo hi; echo oops >&2; exit 7")
	if err == nil {
		t.Error("failing command should return err")
	}
}

func TestCovNewNginxProviderDirSources(t *testing.T) {
	t.Setenv("COCKPIT_NGINX_CONF_DIR", "/from/env")
	if p := NewNginxProvider(NginxConfig{}); p.confDir != "/from/env" {
		t.Errorf("env confDir = %q", p.confDir)
	}
	t.Setenv("COCKPIT_NGINX_CONF_DIR", "")
	p := NewNginxProvider(NginxConfig{})
	if p.confDir != "/etc/nginx/conf.d" {
		t.Errorf("default confDir = %q", p.confDir)
	}
	if p.run == nil {
		t.Error("run should default to defaultCommander")
	}
	if p.Type() != "nginx" {
		t.Errorf("type = %q", p.Type())
	}
}

func TestCovNginxCallDispatch(t *testing.T) {
	p := newNginxTestProvider(t, &mockNginxRunner{})
	// 写一个合法站点供 get/list/delete 使用
	if _, err := p.Call("site.apply", siteParams(t, covValidSite())); err != nil {
		t.Fatalf("apply: %v", err)
	}
	for _, action := range []string{"status", "sites"} {
		if _, err := p.Call(action, nil); err != nil {
			t.Errorf("%s: %v", action, err)
		}
	}
	if _, err := p.Call("site.get", map[string]interface{}{"name": "blog"}); err != nil {
		t.Errorf("site.get: %v", err)
	}
	if _, err := p.Call("site.delete", map[string]interface{}{"name": "blog"}); err != nil {
		t.Errorf("site.delete: %v", err)
	}
	// site.apply：无 site 对象 / 坏 payload
	if _, err := p.Call("site.apply", nil); err == nil || !strings.Contains(err.Error(), "site object required") {
		t.Errorf("missing site err = %v", err)
	}
	if _, err := p.Call("site.apply", map[string]interface{}{
		"site": map[string]interface{}{"name": 1},
	}); err == nil || !strings.Contains(err.Error(), "bad site payload") {
		t.Errorf("bad payload err = %v", err)
	}
	// unknown action
	if _, err := p.Call("bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown nginx action") {
		t.Errorf("unknown action err = %v", err)
	}
}

func covFakeBin(t *testing.T, name, script string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestCovDetectNginx(t *testing.T) {
	// 正常：stderr 输出版本（nginx -v 语义）
	t.Setenv("PATH", covFakeBin(t, "nginx", "#!/bin/sh\necho 'nginx version: nginx/1.24.0' >&2\nexit 0\n"))
	if ver, ok := DetectNginx(); !ok || ver != "nginx/1.24.0" {
		t.Errorf("detect = %q %v", ver, ok)
	}
	// 输出不含 nginx/ → 泛化版本名
	t.Setenv("PATH", covFakeBin(t, "nginx", "#!/bin/sh\necho 'weird output' >&2\nexit 0\n"))
	if ver, ok := DetectNginx(); !ok || ver != "nginx" {
		t.Errorf("detect fallback = %q %v", ver, ok)
	}
	// 失败且无版本输出 → 空行不含 nginx/，退化为泛化版本名
	t.Setenv("PATH", covFakeBin(t, "nginx", "#!/bin/sh\nexit 1\n"))
	if ver, ok := DetectNginx(); !ok || ver != "nginx" {
		t.Errorf("detect silent failure = %q %v", ver, ok)
	}
	// 无 nginx → 不可用
	t.Setenv("PATH", t.TempDir())
	if _, ok := DetectNginx(); ok {
		t.Error("detect without nginx should be false")
	}
}

func TestCovNginxStatusReloadModes(t *testing.T) {
	p := newNginxTestProvider(t, &mockNginxRunner{})
	// PATH 含 systemctl → reloadMode=systemctl（DetectNginx 返回不可用，字段照常返回）
	t.Setenv("PATH", covFakeBin(t, "systemctl", "#!/bin/sh\nexit 0\n"))
	raw, err := p.Call("status", nil)
	if err != nil {
		t.Fatalf("status systemctl: %v", err)
	}
	if raw.(map[string]interface{})["reloadMode"] != "systemctl" {
		t.Errorf("reloadMode = %v", raw.(map[string]interface{})["reloadMode"])
	}
	// PATH 为空 → signal 分支
	t.Setenv("PATH", t.TempDir())
	raw, err = p.Call("status", nil)
	if err != nil {
		t.Fatalf("status signal: %v", err)
	}
	m := raw.(map[string]interface{})
	if m["reloadMode"] != "signal" || m["installed"] != false {
		t.Errorf("status = %v", m)
	}
}

func TestCovNginxGlobBadPattern(t *testing.T) {
	p := NewNginxProvider(NginxConfig{ConfDir: "/tmp/cov[bad", Run: (&mockNginxRunner{}).run})
	if _, err := p.Sites(); err == nil {
		t.Error("sites with bad glob pattern should fail")
	}
	if _, err := p.Status(); err == nil {
		t.Error("status with bad glob pattern should fail")
	}
}

func TestCovNginxGetSiteErrors(t *testing.T) {
	p := newNginxTestProvider(t, &mockNginxRunner{})
	// 非法名
	if _, err := p.Call("site.get", map[string]interface{}{"name": "../bad"}); err == nil ||
		!strings.Contains(err.Error(), "invalid site name") {
		t.Errorf("get invalid name err = %v", err)
	}
	// 不存在
	if _, err := p.Call("site.get", map[string]interface{}{"name": "ghost"}); err == nil ||
		!strings.Contains(err.Error(), "site not found") {
		t.Errorf("get missing err = %v", err)
	}
	// meta 损坏：首行非 meta 注释
	os.WriteFile(p.sitePath("broken"), []byte("server {\n}\n"), 0o644)
	if _, err := p.Call("site.get", map[string]interface{}{"name": "broken"}); err == nil ||
		!strings.Contains(err.Error(), "missing meta header") {
		t.Errorf("get no-meta err = %v", err)
	}
	// meta 损坏：JSON 坏
	os.WriteFile(p.sitePath("badjson"), []byte(nginxMetaPrefix+"{oops\n}\n"), 0o644)
	if _, err := p.Call("site.get", map[string]interface{}{"name": "badjson"}); err == nil ||
		!strings.Contains(err.Error(), "corrupted meta") {
		t.Errorf("get bad-json err = %v", err)
	}
}

func TestCovNginxApplyErrors(t *testing.T) {
	// 直调 validate 失败（Call 会先走 siteFromParams，这里直接构造坏站点）
	p := newNginxTestProvider(t, &mockNginxRunner{})
	if _, err := p.ApplySite(&ProxySite{Name: "BAD NAME"}); err == nil ||
		!strings.Contains(err.Error(), "invalid site name") {
		t.Errorf("apply invalid err = %v", err)
	}
	// confDir 不存在 → 写片段失败
	p2 := NewNginxProvider(NginxConfig{ConfDir: filepath.Join(t.TempDir(), "missing"), Run: (&mockNginxRunner{}).run})
	if _, err := p2.ApplySite(covValidSite()); err == nil ||
		!strings.Contains(err.Error(), "write config") {
		t.Errorf("apply write err = %v", err)
	}
}

func TestCovNginxDeleteErrors(t *testing.T) {
	p := newNginxTestProvider(t, &mockNginxRunner{})
	// 非法名 / 不存在
	if _, err := p.Call("site.delete", map[string]interface{}{"name": "../bad"}); err == nil ||
		!strings.Contains(err.Error(), "invalid site name") {
		t.Errorf("delete invalid name err = %v", err)
	}
	if _, err := p.Call("site.delete", map[string]interface{}{"name": "ghost"}); err == nil ||
		!strings.Contains(err.Error(), "site not found") {
		t.Errorf("delete missing err = %v", err)
	}
	// Remove 失败：confDir 只读（非 root）
	if os.Geteuid() != 0 {
		p3 := newNginxTestProvider(t, &mockNginxRunner{})
		os.WriteFile(p3.sitePath("stuck"), []byte("x"), 0o644)
		os.Chmod(p3.confDir, 0o500)
		t.Cleanup(func() { os.Chmod(p3.confDir, 0o700) })
		if _, err := p3.Call("site.delete", map[string]interface{}{"name": "stuck"}); err == nil ||
			!strings.Contains(err.Error(), "remove config") {
			t.Errorf("delete remove err = %v", err)
		}
	}
}

func TestCovNginxLoadSitesSkips(t *testing.T) {
	p := newNginxTestProvider(t, &mockNginxRunner{})
	// 空名文件：Glob 命中但 proxyFileRe 不匹配 → 跳过
	os.WriteFile(filepath.Join(p.confDir, "cockpit-site-.conf"), []byte("x"), 0o644)
	// 目录伪装片段：ReadFile 失败 → 跳过
	os.Mkdir(filepath.Join(p.confDir, "cockpit-site-dir.conf"), 0o700)
	// 合法片段 + 损坏 meta 片段
	os.WriteFile(p.sitePath("good"), []byte(renderSite(covValidSite())), 0o644)
	os.WriteFile(p.sitePath("ugly"), []byte("no meta here\n"), 0o644)

	sites, err := p.loadSites()
	if err != nil {
		t.Fatalf("loadSites: %v", err)
	}
	if len(sites) != 1 || sites[0].Name != "blog" {
		t.Fatalf("sites = %+v, want only the rendered good site", sites)
	}
}

func TestCovCommandErrSummary(t *testing.T) {
	long := strings.Repeat("e", 3000)
	if got := commandErrSummary([]byte(long), errors.New("x")); len(got) != 2048 {
		t.Errorf("truncated len = %d", len(got))
	}
	if got := commandErrSummary(nil, errors.New("boom")); got != "boom" {
		t.Errorf("empty stderr fallback = %q", got)
	}
}
