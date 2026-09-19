package rpc

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// okTraefikRunner 恒成功桩：traefik version 返回 v2 形态输出
func okTraefikRunner(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	if name == "traefik" && len(args) > 0 && args[0] == "version" {
		return []byte("Version:      v2.10.7\nCodename:     cheddar\n"), nil, nil
	}
	return nil, nil, nil
}

func newTraefikTestProvider(t *testing.T, dir string, run Commander) *TraefikProvider {
	t.Helper()
	p := NewTraefikProvider(dir, run)
	p.SetBaseline(NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")))
	return p
}

// TestRenderTraefikSiteHTTP http 站点：单 router 挂 web entrypoint、
// service 引用自洽、无 tls/middlewares 段；meta 首行可 roundtrip
func TestRenderTraefikSiteHTTP(t *testing.T) {
	site := &ProxySite{Name: "blog", ServerNames: []string{"blog.example.com"},
		Upstream: "127.0.0.1:3000", Scheme: "http"}
	out, err := renderTraefikSite(site)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := checkTraefikYAML(out); err != nil {
		t.Fatalf("self-check: %v", err)
	}
	text := string(out)
	if !strings.HasPrefix(text, nginxMetaPrefix) {
		t.Errorf("missing meta header: %q", text[:40])
	}
	if meta, err := parseMeta(out); err != nil || meta.Name != "blog" {
		t.Fatalf("meta roundtrip: %v %+v", err, meta)
	}
	for _, want := range []string{
		"cockpit-blog:", "rule: Host(`blog.example.com`)", "entryPoints:",
		"- web", "service: cockpit-blog", "url: http://127.0.0.1:3000",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered output missing %q:\n%s", want, text)
		}
	}
	for _, banned := range []string{"tls:", "middlewares:", "websecure", "redirectScheme"} {
		if strings.Contains(text, banned) {
			t.Errorf("http site should not contain %q:\n%s", banned, text)
		}
	}
}

// TestRenderTraefikSiteHTTPS https 站点：双 router + redirectScheme 永久
// 跳转 + 顶层 tls.certificates 文件引用；多域名 rule 全覆盖
func TestRenderTraefikSiteHTTPS(t *testing.T) {
	site := &ProxySite{Name: "panel", ServerNames: []string{"a.example.com", "b.example.com"},
		Upstream: "10.0.0.5:8080", Scheme: "https",
		TLSCert: "/etc/cockpit/certs/fullchain.pem", TLSKey: "/etc/cockpit/certs/privkey.pem"}
	out, err := renderTraefikSite(site)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if err := checkTraefikYAML(out); err != nil {
		t.Fatalf("self-check: %v", err)
	}
	text := string(out)
	for _, want := range []string{
		"cockpit-panel-web:", "cockpit-panel-websecure:",
		"Host(`a.example.com`) || Host(`b.example.com`)",
		"cockpit-panel-redirect:", "scheme: https", "permanent: true",
		"certFile: /etc/cockpit/certs/fullchain.pem", "keyFile: /etc/cockpit/certs/privkey.pem",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("rendered output missing %q:\n%s", want, text)
		}
	}
	// 443 router 的 tls 开关在 router 段内出现两次（web/websecure 各自 tls 语义）
	if strings.Count(text, "tls:") != 2 {
		t.Errorf("want 2 tls keys (router + top-level certificates), got:\n%s", text)
	}
}

// TestCheckTraefikYAMLSelfCheck 自检拒绝：router 引用缺失 service / 无 http 段
func TestCheckTraefikYAMLSelfCheck(t *testing.T) {
	meta := nginxMetaPrefix + `{"name":"x"}` + "\n"
	if err := checkTraefikYAML([]byte(meta + "http:\n  routers:\n    r:\n      rule: Host(`a`)\n      service: gone\n  services:\n    s: {}\n")); err == nil {
		t.Error("missing service reference should fail self-check")
	}
	if err := checkTraefikYAML([]byte(meta + "tls: {}\n")); err == nil {
		t.Error("no http section should fail self-check")
	}
	if err := checkTraefikYAML([]byte(meta + "http:\n  routers:\n    r:\n      rule: Host(`a`)\n      service: s\n  services:\n    s: {}\n")); err != nil {
		t.Errorf("valid config should pass: %v", err)
	}
}

// TestTraefikApplyGetDeleteLifecycle 全生命周期：apply 落盘+基线登记 →
// get 回读 meta/content → delete 删文件+基线清除
func TestTraefikApplyGetDeleteLifecycle(t *testing.T) {
	dir := t.TempDir()
	baseline := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	p := NewTraefikProvider(dir, okTraefikRunner)
	p.SetBaseline(baseline)

	site := &ProxySite{Name: "grafana", ServerNames: []string{"g.example.com"},
		Upstream: "127.0.0.1:3001", Scheme: "http"}
	if _, err := p.Call("site.apply", siteParams(t, site)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	path := filepath.Join(dir, "cockpit-site-grafana.yml")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("fragment not written: %v", err)
	}
	// 无 .cockpit-tmp 残留（原子写 rename 完成）
	if entries, _ := os.ReadDir(dir); len(entries) != 1 {
		t.Errorf("dir should only hold the fragment, got %d entries", len(entries))
	}
	// 基线已登记（traefik kind，diff 可用）
	if entry := baseline.snapshot()["traefik/grafana"]; entry.Content == "" {
		t.Errorf("baseline should record traefik/grafana with content, got %+v", entry)
	}

	got, err := p.Call("site.get", map[string]interface{}{"name": "grafana"})
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if content, _ := got.(map[string]interface{})["content"].(string); !strings.Contains(content, "g.example.com") {
		t.Errorf("get content = %q", content)
	}

	if _, err := p.Call("site.delete", map[string]interface{}{"name": "grafana"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("fragment should be gone, stat err = %v", err)
	}
	if _, has := baseline.snapshot()["traefik/grafana"]; has {
		t.Error("baseline entry should be forgotten after delete")
	}
	// 再删：不存在报错
	if _, err := p.Call("site.delete", map[string]interface{}{"name": "grafana"}); err == nil {
		t.Error("delete of missing site should fail")
	}
}

// TestTraefikSitesListing sites 返回结构与字段
func TestTraefikSitesListing(t *testing.T) {
	dir := t.TempDir()
	p := newTraefikTestProvider(t, dir, okTraefikRunner)
	site := &ProxySite{Name: "s1", ServerNames: []string{"s1.example.com"},
		Upstream: "127.0.0.1:80", Scheme: "https",
		TLSCert: "/c.pem", TLSKey: "/k.pem", Websocket: true}
	if _, err := p.ApplySite(site); err != nil {
		t.Fatalf("apply: %v", err)
	}
	// 非 cockpit 片段与损坏 meta 均跳过
	os.WriteFile(filepath.Join(dir, "user-own.yml"), []byte("http: {}\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "cockpit-site-broken.yml"), []byte("garbage"), 0o644)

	resp, err := p.Sites()
	if err != nil {
		t.Fatalf("sites: %v", err)
	}
	list := resp.(map[string]interface{})["sites"].([]map[string]interface{})
	if len(list) != 1 {
		t.Fatalf("sites = %d entries, want 1", len(list))
	}
	if list[0]["name"] != "s1" || list[0]["scheme"] != "https" || list[0]["websocket"] != true {
		t.Errorf("site entry = %+v", list[0])
	}
}

// TestTraefikApplyRejectsExtra extra 直通口仅 nginx 特有：traefik 拒绝且不落盘
func TestTraefikApplyRejectsExtra(t *testing.T) {
	dir := t.TempDir()
	p := newTraefikTestProvider(t, dir, okTraefikRunner)
	site := &ProxySite{Name: "x", ServerNames: []string{"x.example.com"},
		Upstream: "127.0.0.1:80", Scheme: "http", Extra: "deny all;"}
	if _, err := p.ApplySite(site); err == nil {
		t.Fatal("extra should be rejected by traefik backend")
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("rejected apply must not write files, got %d", len(entries))
	}
	// 非法站点名同样拒绝
	site = &ProxySite{Name: "Bad Name", ServerNames: []string{"x.example.com"},
		Upstream: "127.0.0.1:80", Scheme: "http"}
	if _, err := p.ApplySite(site); err == nil {
		t.Error("invalid site name should be rejected")
	}
}

// TestTraefikStatus Status 概览：backend/confDir/siteCount 与 version 探测
func TestTraefikStatus(t *testing.T) {
	dir := t.TempDir()
	p := newTraefikTestProvider(t, dir, okTraefikRunner)
	site := &ProxySite{Name: "s", ServerNames: []string{"s.example.com"},
		Upstream: "127.0.0.1:80", Scheme: "http"}
	if _, err := p.ApplySite(site); err != nil {
		t.Fatalf("apply: %v", err)
	}
	resp, err := p.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	st := resp.(map[string]interface{})
	if st["backend"] != "traefik" || st["installed"] != true || st["reloadMode"] != "hot" {
		t.Errorf("status = %+v", st)
	}
	if st["confDir"] != dir || st["siteCount"] != 1 {
		t.Errorf("confDir/siteCount = %v/%v", st["confDir"], st["siteCount"])
	}
	if st["version"] != "v2.10.7" {
		t.Errorf("version = %v, want v2.10.7 (v2 output first line)", st["version"])
	}

	// 无 traefik 二进制（命令失败）：version 为空而非报错
	p2 := newTraefikTestProvider(t, dir, func(_ context.Context, name string, _ ...string) ([]byte, []byte, error) {
		return nil, nil, errors.New("exec: not found")
	})
	resp, _ = p2.Status()
	if v := resp.(map[string]interface{})["version"]; v != "" {
		t.Errorf("version should be empty without binary, got %v", v)
	}
}

// TestTraefikCallDispatch Call 分发：未知 action / 非法 get 名
func TestTraefikCallDispatch(t *testing.T) {
	p := newTraefikTestProvider(t, t.TempDir(), okTraefikRunner)
	if _, err := p.Call("nope", nil); err == nil {
		t.Error("unknown action should fail")
	}
	if _, err := p.Call("site.get", map[string]interface{}{"name": "BAD"}); err == nil {
		t.Error("invalid name should fail")
	}
	if _, err := p.Call("site.apply", map[string]interface{}{}); err == nil {
		t.Error("missing site object should fail")
	}
}

// TestTraefikDirFromStatic 静态配置解析：providers.file.directory 取值、
// 缺失回空串、.yaml 拼写、解析失败忽略
func TestTraefikDirFromStatic(t *testing.T) {
	yml := filepath.Join(t.TempDir(), "traefik.yml")
	os.WriteFile(yml, []byte("providers:\n  file:\n    directory: /data/traefik/dyn\n"), 0o644)
	if got := traefikDirFromStatic([]string{yml}); got != "/data/traefik/dyn" {
		t.Errorf("directory = %q", got)
	}

	// 无 file 段 → 空串
	empty := filepath.Join(t.TempDir(), "traefik.yml")
	os.WriteFile(empty, []byte("entryPoints:\n  web:\n    address: :80\n"), 0o644)
	if got := traefikDirFromStatic([]string{empty}); got != "" {
		t.Errorf("empty config should yield \"\", got %q", got)
	}
	// 文件不存在 → 空串
	if got := traefikDirFromStatic([]string{filepath.Join(t.TempDir(), "absent.yml")}); got != "" {
		t.Errorf("missing file should yield \"\", got %q", got)
	}

	// 首个候选缺失时读第二个（.yaml 拼写）
	yamlAlt := filepath.Join(t.TempDir(), "traefik.yaml")
	os.WriteFile(yamlAlt, []byte("providers:\n  file:\n    directory: /alt\n"), 0o644)
	if got := traefikDirFromStatic([]string{filepath.Join(t.TempDir(), "absent.yml"), yamlAlt}); got != "/alt" {
		t.Errorf("fallback to .yaml candidate = %q", got)
	}
}

// TestDetectTraefikEnvOverride env 覆盖：目录存在返回自身；不存在探测失败
func TestDetectTraefikEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("COCKPIT_TRAEFIK_DIR", dir)
	got, ok := DetectTraefik()
	if !ok || got != dir {
		t.Fatalf("DetectTraefik = %q %v, want %q true", got, ok, dir)
	}

	t.Setenv("COCKPIT_TRAEFIK_DIR", filepath.Join(dir, "nope"))
	if _, ok := DetectTraefik(); ok {
		t.Error("env pointing to missing dir should fail detection")
	}
}

// TestDriftTraefikSection drift 检查扩 traefik kind（M2 D15）：四态与
// nginx 同语义，diff 目标校验放行 traefik 并拒绝穿越
func TestDriftTraefikSection(t *testing.T) {
	dynDir := t.TempDir()
	writeFrag := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dynDir, "cockpit-site-"+name+".yml"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeFrag("a", "# cockpit:meta {\"name\":\"a\"}\nhttp: {}")
	writeFrag("b", "# 手改前的片段")

	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	b.Record("traefik", "a", []byte("# cockpit:meta {\"name\":\"a\"}\nhttp: {}"))
	b.Record("traefik", "gone", []byte("已被手删的片段"))

	p := NewDriftProvider(b, DriftConfig{DynamicDir: dynDir})
	res, err := p.Check()
	if err != nil {
		t.Fatal(err)
	}
	if got := driftFind(t, res, "traefik", "a"); got.Status != "ok" {
		t.Fatalf("a = %+v", got)
	}
	if got := driftFind(t, res, "traefik", "b"); got.Status != "no_baseline" {
		t.Fatalf("b = %+v", got)
	}
	if got := driftFind(t, res, "traefik", "gone"); got.Status != "missing" {
		t.Fatalf("gone = %+v", got)
	}
	// 手改 → drifted
	writeFrag("a", "# 被人改过的片段")
	if got := driftFind(t, mustCheck(t, p), "traefik", "a"); got.Status != "drifted" {
		t.Fatalf("a after edit = %+v", got)
	}

	// diff：基线有原文，drifted 条目两侧全文可取（expected=基线原文，current=实时读盘）
	diff, err := p.Diff("traefik", "a")
	if err != nil {
		t.Fatalf("diff traefik/a: %v", err)
	}
	dm := diff.(map[string]interface{})
	if !strings.Contains(dm["expected"].(string), "cockpit:meta") ||
		!strings.Contains(dm["current"].(string), "被人改过的片段") {
		t.Errorf("diff sides unexpected: %+v", dm)
	}

	// 目标校验：合法名放行，穿越/路径分隔拒绝
	if err := validDriftTarget("traefik", "a"); err != nil {
		t.Errorf("valid traefik target rejected: %v", err)
	}
	for _, bad := range []string{"../evil", "a/b", ".", ""} {
		if err := validDriftTarget("traefik", bad); err == nil {
			t.Errorf("traefik target %q should be rejected", bad)
		}
	}
}

// TestTraefikProviderType provider 注册键（providers map 以 Type() 为 key）
func TestTraefikProviderType(t *testing.T) {
	p := NewTraefikProvider("", nil)
	if p.Type() != "traefik" {
		t.Errorf("Type = %q, want traefik", p.Type())
	}
	if NewTraefikProvider("", nil).dir != traefikDynamicDirDefault {
		t.Errorf("empty dir should fall back to default")
	}
}

// TestDetectTraefikStaticConfig 静态配置路径分支（注入候选路径，真实
// /etc/traefik 不可写）：解析出 directory → 目录存在即探测通过；目录
// 不存在则失败；静态配置无 file 段回缺省目录
func TestDetectTraefikStaticConfig(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "traefik.yml")
	dynDir := filepath.Join(root, "dyn")
	os.WriteFile(cfgPath, []byte("providers:\n  file:\n    directory: "+dynDir+"\n"), 0o644)

	t.Setenv("COCKPIT_TRAEFIK_DIR", "")
	orig := traefikStaticConfigs
	traefikStaticConfigs = []string{cfgPath}
	t.Cleanup(func() { traefikStaticConfigs = orig })

	// 目录不存在 → 探测失败（静态配置有值但落空）
	if _, ok := DetectTraefik(); ok {
		t.Error("missing dynamic dir should fail detection")
	}

	// 目录存在 → 通过且返回该目录
	os.Mkdir(dynDir, 0o755)
	got, ok := DetectTraefik()
	if !ok || got != dynDir {
		t.Fatalf("DetectTraefik = %q %v, want %q true", got, ok, dynDir)
	}

	// 静态配置无 file 段 → 回缺省目录（不存在即失败，仅验证不解析出 dynDir）
	os.WriteFile(cfgPath, []byte("log:\n  level: INFO\n"), 0o644)
	os.Remove(dynDir)
	if _, ok := DetectTraefik(); ok {
		t.Error("default dir should not exist in test root")
	}
}
