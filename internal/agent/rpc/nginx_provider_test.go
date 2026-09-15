package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// mockNginxRunner 记录调用序列，按命令分派预设错误。
// nginxTestErr → `nginx -t`；systemctlErr → `systemctl reload nginx`；
// reloadErr → `nginx -s reload`。nil 表示成功。
type mockNginxRunner struct {
	calls        []string
	nginxTestErr error
	systemctlErr error
	reloadErr    error
}

func (m *mockNginxRunner) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	m.calls = append(m.calls, name+" "+strings.Join(args, " "))
	switch {
	case name == "nginx" && len(args) > 0 && args[0] == "-t":
		if m.nginxTestErr != nil {
			return nil, []byte("emerg: invalid condition in /etc/nginx/nginx.conf:99"), m.nginxTestErr
		}
		return []byte("syntax is ok"), nil, nil
	case name == "systemctl":
		if m.systemctlErr != nil {
			return nil, []byte(m.systemctlErr.Error()), m.systemctlErr
		}
		return nil, nil, nil
	case name == "nginx" && len(args) > 1 && args[0] == "-s":
		if m.reloadErr != nil {
			return nil, []byte("reload failed: connection refused"), m.reloadErr
		}
		return nil, nil, nil
	}
	return nil, nil, nil
}

func newNginxTestProvider(t *testing.T, runner *mockNginxRunner) *NginxProvider {
	t.Helper()
	return NewNginxProvider(NginxConfig{ConfDir: t.TempDir(), Run: runner.run})
}

// siteParams 构造真实 JSON RPC 反序列化后的 params 形状（site 为 map 而非结构体）
func siteParams(t *testing.T, site *ProxySite) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{"site": site})
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]interface{}
	if err := json.Unmarshal(b, &params); err != nil {
		t.Fatal(err)
	}
	return params
}

func TestRenderSiteHTTP(t *testing.T) {
	site := &ProxySite{
		Name: "blog", ServerNames: []string{"blog.example.com"},
		Upstream: "127.0.0.1:3000", Scheme: "http", Websocket: true,
		Extra: "client_max_body_size 50m;",
	}
	out := renderSite(site)
	for _, want := range []string{
		nginxMetaPrefix, "listen 80;", "server_name blog.example.com;",
		"proxy_pass http://127.0.0.1:3000;", "proxy_http_version 1.1;",
		`proxy_set_header Connection "upgrade";`, "client_max_body_size 50m;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "listen 443") {
		t.Error("http site must not listen 443")
	}
	// meta 行必须是第一行且单行 JSON
	first := out[:strings.Index(out, "\n")]
	if !strings.HasPrefix(first, nginxMetaPrefix) {
		t.Fatalf("first line = %q", first)
	}
	if strings.Contains(first, "\n") {
		t.Fatal("meta must be single line")
	}
}

func TestRenderSiteHTTPS(t *testing.T) {
	site := &ProxySite{
		Name: "secure", ServerNames: []string{"a.example.com", "*.example.org"},
		Upstream: "10.0.0.2:8080", Scheme: "https",
		TLSCert: "/etc/ssl/a.crt", TLSKey: "/etc/ssl/a.key",
	}
	out := renderSite(site)
	for _, want := range []string{
		"listen 443 ssl;", "ssl_certificate /etc/ssl/a.crt;",
		"ssl_certificate_key /etc/ssl/a.key;", "server_name a.example.com;",
		"server_name *.example.org;", "return 301 https://$host$request_uri;",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered missing %q:\n%s", want, out)
		}
	}
	// 不开 websocket 不应有 upgrade 头
	if strings.Contains(out, "Upgrade") {
		t.Error("websocket disabled but Upgrade header rendered")
	}
}

func TestProxySiteValidate(t *testing.T) {
	base := &ProxySite{
		Name: "ok", ServerNames: []string{"a.com"}, Upstream: "127.0.0.1:80", Scheme: "http",
	}
	if err := base.validate(); err != nil {
		t.Fatalf("base should pass: %v", err)
	}
	cases := []struct {
		desc string
		mut  func(*ProxySite)
	}{
		{"bad name", func(s *ProxySite) { s.Name = "Bad Name" }},
		{"empty serverNames", func(s *ProxySite) { s.ServerNames = nil }},
		{"too many serverNames", func(s *ProxySite) {
			s.ServerNames = make([]string, 17)
			for i := range s.ServerNames {
				s.ServerNames[i] = "a.com"
			}
		}},
		{"bad domain", func(s *ProxySite) { s.ServerNames = []string{"a b.com"} }},
		{"bad upstream", func(s *ProxySite) { s.Upstream = "host port" }},
		{"bad scheme", func(s *ProxySite) { s.Scheme = "ftp" }},
		{"https without cert", func(s *ProxySite) { s.Scheme = "https" }},
		{"https relative cert", func(s *ProxySite) {
			s.Scheme = "https"
			s.TLSCert = "ssl/a.crt"
			s.TLSKey = "/ssl/a.key"
		}},
	}
	for _, c := range cases {
		s := *base
		c.mut(&s)
		if err := s.validate(); err == nil {
			t.Errorf("%s: should be rejected", c.desc)
		}
	}
}

func TestProxyMetaRoundtrip(t *testing.T) {
	site := &ProxySite{
		Name: "rt", ServerNames: []string{"rt.example.com"},
		Upstream: "127.0.0.1:9000", Scheme: "https",
		TLSCert: "/c.pem", TLSKey: "/k.pem", Websocket: true, Extra: "# note\nkeepalive 8;",
	}
	parsed, err := parseMeta([]byte(renderSite(site)))
	if err != nil {
		t.Fatalf("parseMeta: %v", err)
	}
	if !reflect.DeepEqual(parsed, site) {
		t.Fatalf("roundtrip mismatch:\n got %+v\nwant %+v", parsed, site)
	}
	// 缺 meta 头拒绝
	if _, err := parseMeta([]byte("server {\n}\n")); err == nil {
		t.Error("content without meta should be rejected")
	}
}

func TestProxyApplyTestFailNotWritten(t *testing.T) {
	runner := &mockNginxRunner{nginxTestErr: errors.New("exit 1")}
	p := newNginxTestProvider(t, runner)
	site := &ProxySite{Name: "x", ServerNames: []string{"x.com"}, Upstream: "127.0.0.1:80", Scheme: "http"}

	_, err := p.Call("site.apply", siteParams(t, site))
	if err == nil || !strings.Contains(err.Error(), "nginx -t failed") {
		t.Fatalf("err = %v, want nginx -t failure with stderr", err)
	}
	if _, err := os.Stat(filepath.Join(p.confDir, "cockpit-site-x.conf")); !os.IsNotExist(err) {
		t.Fatal("config must not be written when nginx -t fails")
	}
}

func TestProxyApplyReloadFailRollback(t *testing.T) {
	// systemctl 直接失败（非「单元不存在」）→ 不走 signal fallback，进入回滚分支；
	// reloadErr 兜底无 systemctl 的环境（走 nginx -s reload 也失败）
	runner := &mockNginxRunner{systemctlErr: errors.New("Job for nginx.service failed"), reloadErr: errors.New("exit 1")}
	p := newNginxTestProvider(t, runner)
	path := filepath.Join(p.confDir, "cockpit-site-y.conf")
	if err := os.WriteFile(path, []byte("# old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	site := &ProxySite{Name: "y", ServerNames: []string{"y.com"}, Upstream: "127.0.0.1:80", Scheme: "http"}

	if _, err := p.Call("site.apply", siteParams(t, site)); err == nil {
		t.Fatal("apply should fail on reload error")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "# old\n" {
		t.Fatalf("old content not restored: %q err=%v", got, err)
	}

	// 原本不存在的站点：reload 失败后新文件应被删除
	site2 := &ProxySite{Name: "z", ServerNames: []string{"z.com"}, Upstream: "127.0.0.1:80", Scheme: "http"}
	if _, err := p.Call("site.apply", siteParams(t, site2)); err == nil {
		t.Fatal("apply should fail on reload error")
	}
	if _, err := os.Stat(filepath.Join(p.confDir, "cockpit-site-z.conf")); !os.IsNotExist(err) {
		t.Fatal("new file must be removed on rollback")
	}
}

func TestProxyApplySuccessAndRoundtrip(t *testing.T) {
	runner := &mockNginxRunner{}
	p := newNginxTestProvider(t, runner)
	site := &ProxySite{
		Name: "app", ServerNames: []string{"app.example.com"},
		Upstream: "127.0.0.1:3000", Scheme: "http", Websocket: true,
	}

	res, err := p.Call("site.apply", siteParams(t, site))
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if res.(map[string]interface{})["file"] != "cockpit-site-app.conf" {
		t.Fatalf("res = %v", res)
	}
	// systemctl 存在（CI linux 有）→ reload 走 systemctl；本 mock 不注入 systemctl 错误
	// 落盘内容可 roundtrip
	content, err := os.ReadFile(filepath.Join(p.confDir, "cockpit-site-app.conf"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := parseMeta(content)
	if err != nil || parsed.Name != "app" || !parsed.Websocket {
		t.Fatalf("roundtrip: %v %+v", err, parsed)
	}
	// sites 列表可见
	list, err := p.Call("sites", nil)
	if err != nil {
		t.Fatal(err)
	}
	sites := list.(map[string]interface{})["sites"].([]map[string]interface{})
	if len(sites) != 1 || sites[0]["name"] != "app" {
		t.Fatalf("sites = %v", sites)
	}
}

func TestProxyDeleteReloadFailRestores(t *testing.T) {
	runner := &mockNginxRunner{systemctlErr: errors.New("Failed to reload nginx.service: Unit not found"), reloadErr: errors.New("exit 1")}
	p := newNginxTestProvider(t, runner)
	path := filepath.Join(p.confDir, "cockpit-site-d.conf")
	if err := os.WriteFile(path, []byte("# keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := p.Call("site.delete", map[string]interface{}{"name": "d"}); err == nil {
		t.Fatal("delete should fail when both reload paths fail")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != "# keep\n" {
		t.Fatalf("file not restored: %q err=%v", got, err)
	}
}

func TestProxySitesSkipsCorruptedMeta(t *testing.T) {
	runner := &mockNginxRunner{}
	p := newNginxTestProvider(t, runner)
	if err := os.WriteFile(filepath.Join(p.confDir, "cockpit-site-good.conf"),
		[]byte(renderSite(&ProxySite{Name: "good", ServerNames: []string{"g.com"}, Upstream: "1.2.3.4:80", Scheme: "http"})), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.confDir, "cockpit-site-broken.conf"), []byte("no meta here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// 非 cockpit 命名的文件不出现在列表
	if err := os.WriteFile(filepath.Join(p.confDir, "user-own.conf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	list, err := p.Call("sites", nil)
	if err != nil {
		t.Fatal(err)
	}
	sites := list.(map[string]interface{})["sites"].([]map[string]interface{})
	if len(sites) != 1 || sites[0]["name"] != "good" {
		t.Fatalf("sites = %v, want only good", sites)
	}
}

func TestProxyReloadFallsBackToSignal(t *testing.T) {
	// systemctl 报「单元不存在」→ 应 fallback 到 nginx -s reload 且成功
	runner := &mockNginxRunner{systemctlErr: errors.New("Failed to reload nginx.service: Unit nginx.service not found.")}
	p := newNginxTestProvider(t, runner)
	site := &ProxySite{Name: "fb", ServerNames: []string{"fb.com"}, Upstream: "1.1.1.1:80", Scheme: "http"}
	if _, err := p.Call("site.apply", siteParams(t, site)); err != nil {
		t.Fatalf("apply should succeed via fallback: %v", err)
	}
	joined := strings.Join(runner.calls, " | ")
	if !strings.Contains(joined, "nginx -s reload") {
		t.Fatalf("expected signal fallback, calls = %s", joined)
	}

	// systemctl 报其他错误 → 不 fallback，直接失败
	runner2 := &mockNginxRunner{systemctlErr: errors.New("Job for nginx.service failed because the control process exited with error code.")}
	p2 := newNginxTestProvider(t, runner2)
	site2 := &ProxySite{Name: "nf", ServerNames: []string{"nf.com"}, Upstream: "1.1.1.1:80", Scheme: "http"}
	if _, err := p2.Call("site.apply", siteParams(t, site2)); err == nil {
		t.Fatal("apply should fail without fallback")
	}
	for _, c := range runner2.calls {
		if strings.HasPrefix(c, "nginx -s") {
			t.Fatalf("should not fall back to signal, calls = %v", runner2.calls)
		}
	}
}
