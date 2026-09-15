package rpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// fakeRecorder 捕获挂钩调用（注入 provider 测试写路径挂钩）
type fakeRecorder struct {
	records map[string][]byte
	forgot  []string
}

func newFakeRecorder() *fakeRecorder {
	return &fakeRecorder{records: map[string][]byte{}}
}

func (f *fakeRecorder) Record(kind, name string, content []byte) {
	f.records[kind+"/"+name] = content
}

func (f *fakeRecorder) Forget(kind, name string) {
	f.forgot = append(f.forgot, kind+"/"+name)
}

// driftFind 从 check 结果里取指定条目；absent=true 时断言不存在
func driftFind(t *testing.T, result interface{}, kind, name string) driftItem {
	t.Helper()
	m, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type %T", result)
	}
	raw, _ := json.Marshal(m["items"])
	var items []driftItem
	if err := json.Unmarshal(raw, &items); err != nil {
		t.Fatal(err)
	}
	for _, it := range items {
		if it.Kind == kind && it.Name == name {
			return it
		}
	}
	t.Fatalf("item %s/%s not found in %+v", kind, name, items)
	return driftItem{}
}

func TestDriftBaselinePersist(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "baseline.json")
	b := NewDriftBaseline(path)
	b.Record("nginx", "web", []byte("server_name a"))
	b.Record("nginx", "web", []byte("server_name b")) // 覆盖更新
	b.Record("stack", "api/compose.yml", []byte("services: {}"))

	// 重新打开同一文件验证持久化
	b2 := NewDriftBaseline(path)
	snap := b2.snapshot()
	if len(snap) != 2 {
		t.Fatalf("entries = %v", snap)
	}
	e := snap["nginx/web"]
	if len(e.SHA256) != 64 || e.UpdatedAt == 0 {
		t.Fatalf("nginx/web entry = %+v", e)
	}
	if _, ok := snap["stack/api/compose.yml"]; !ok {
		t.Fatalf("stack entry missing: %v", snap)
	}

	// Forget 幂等且持久化
	b2.Forget("nginx", "web")
	b2.Forget("nginx", "web") // 再删一次不报错
	if got := NewDriftBaseline(path).snapshot(); len(got) != 1 {
		t.Fatalf("after forget = %v", got)
	}
}

func TestDriftCheckNginxStates(t *testing.T) {
	confDir := t.TempDir()
	writeSite := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(confDir, "cockpit-site-"+name+".conf"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeSite("a", "# cockpit meta a\nserver block a")
	writeSite("b", "# 手改过的存量站点")

	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	b.Record("nginx", "a", []byte("# cockpit meta a\nserver block a"))
	b.Record("nginx", "gone", []byte("已被手删的站点"))

	p := NewDriftProvider(b, DriftConfig{ConfDir: confDir})
	res, err := p.Check()
	if err != nil {
		t.Fatal(err)
	}

	if got := driftFind(t, res, "nginx", "a"); got.Status != "ok" || got.BaselineSHA == "" || got.CurrentSHA == "" {
		t.Fatalf("a = %+v", got)
	}
	if got := driftFind(t, res, "nginx", "b"); got.Status != "no_baseline" || got.BaselineSHA != "" {
		t.Fatalf("b = %+v", got)
	}
	if got := driftFind(t, res, "nginx", "gone"); got.Status != "missing" || got.CurrentSHA != "" {
		t.Fatalf("gone = %+v", got)
	}

	// 手改后 drifted（字节级比对，meta 注释改动同样算漂移）
	writeSite("a", "# 被人改过的配置")
	if got := driftFind(t, mustCheck(t, p), "nginx", "a"); got.Status != "drifted" {
		t.Fatalf("a after edit = %+v", got)
	}
}

func TestDriftCheckCronSection(t *testing.T) {
	meta := `# cockpit:job {"name":"backup","schedule":"0 3 * * *","command":"/opt/b.sh","enabled":true}`
	runner := &mockCronRunner{hasFile: true, content: meta + "\n0 * * * * /usr/bin/legacy.sh\n"}

	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	p := NewDriftProvider(b, DriftConfig{CronRun: runner.run})

	// 无基线（从未面板管理过）→ no_baseline
	if got := driftFind(t, mustCheck(t, p), "cron", "cockpit"); got.Status != "no_baseline" {
		t.Fatalf("initial = %+v", got)
	}

	// 经 CronProvider 面板写路径 ApplyJob → 基线登记 → ok
	cp := NewCronProvider(runner.run)
	cp.SetBaseline(b)
	if _, err := cp.ApplyJob(&CronJob{Name: "backup", Schedule: "0 3 * * *", Command: "/opt/b.sh", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	if got := driftFind(t, mustCheck(t, p), "cron", "cockpit"); got.Status != "ok" {
		t.Fatalf("after apply = %+v", got)
	}

	// 用户改外部条目（非 cockpit 段）→ 不算漂移（D2）
	runner.mu.Lock()
	runner.content = meta + "\n30 * * * * /usr/bin/legacy-edited.sh\n"
	runner.mu.Unlock()
	if got := driftFind(t, mustCheck(t, p), "cron", "cockpit"); got.Status != "ok" {
		t.Fatalf("after external edit = %+v", got)
	}

	// 手改 cockpit 命令行 → drifted
	runner.mu.Lock()
	runner.content = `# cockpit:job {"name":"backup","schedule":"0 3 * * *","command":"/tmp/evil.sh","enabled":true}` + "\n"
	runner.mu.Unlock()
	if got := driftFind(t, mustCheck(t, p), "cron", "cockpit"); got.Status != "drifted" {
		t.Fatalf("after cockpit edit = %+v", got)
	}

	// 任务被整体删除 → drifted（基线有任务，现在 cockpit 段空了）
	runner.mu.Lock()
	runner.content = "0 * * * * /usr/bin/legacy.sh\n"
	runner.mu.Unlock()
	if got := driftFind(t, mustCheck(t, p), "cron", "cockpit"); got.Status != "drifted" {
		t.Fatalf("after delete = %+v", got)
	}
}

func TestDriftCheckStackFiles(t *testing.T) {
	stacksDir := t.TempDir()
	mk := func(rel, content string) {
		t.Helper()
		path := filepath.Join(stacksDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	mk("web/compose.yml", "services:\n  nginx:\n    image: nginx")
	mk("web/.env", "TAG=1.0")
	mk(".cockpit/junk", "保留目录不检测")

	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	b.Record("stack", "web/compose.yml", []byte("services:\n  nginx:\n    image: nginx"))
	b.Record("stack", "web/.env", []byte("TAG=1.0"))
	b.Record("stack", "old/compose.yml", []byte("已删除的 stack"))

	p := NewDriftProvider(b, DriftConfig{StacksDir: stacksDir})
	res := mustCheck(t, p)

	if got := driftFind(t, res, "stack", "web/compose.yml"); got.Status != "ok" {
		t.Fatalf("compose = %+v", got)
	}
	if got := driftFind(t, res, "stack", "web/.env"); got.Status != "ok" {
		t.Fatalf("env = %+v", got)
	}
	if got := driftFind(t, res, "stack", "old/compose.yml"); got.Status != "missing" {
		t.Fatalf("old = %+v", got)
	}

	// .env 单独被改 → 仅 .env drifted，compose 不受影响
	mk("web/.env", "TAG=2.0-hacked")
	res = mustCheck(t, p)
	if got := driftFind(t, res, "stack", "web/.env"); got.Status != "drifted" {
		t.Fatalf("env after edit = %+v", got)
	}
	if got := driftFind(t, res, "stack", "web/compose.yml"); got.Status != "ok" {
		t.Fatalf("compose after env edit = %+v", got)
	}
}

func TestStackSaveAndRemoveUpdateBaseline(t *testing.T) {
	rec := newFakeRecorder()
	p := NewStackProvider(StackConfig{Dir: t.TempDir(), ComposeBin: []string{"/bin/sh", "-c", "exit 0"}})
	p.SetBaseline(rec)

	if _, err := p.SaveStackFile(map[string]interface{}{
		"name":    "web",
		"compose": "services:\n  nginx:\n    image: nginx",
		"env":     "TAG=1.0",
	}); err != nil {
		t.Fatal(err)
	}
	if got := rec.records["stack/web/compose.yml"]; string(got) != "services:\n  nginx:\n    image: nginx" {
		t.Fatalf("compose baseline = %q", got)
	}
	if got := rec.records["stack/web/.env"]; string(got) != "TAG=1.0" {
		t.Fatalf("env baseline = %q", got)
	}

	// 保存校验失败（compose 非法）不应登记基线
	rec2 := newFakeRecorder()
	p2 := NewStackProvider(StackConfig{Dir: t.TempDir(), ComposeBin: []string{"false"}})
	p2.SetBaseline(rec2)
	_, _ = p2.SaveStackFile(map[string]interface{}{"name": "bad", "compose": "not: [valid"})
	if len(rec2.records) != 0 {
		t.Fatalf("invalid save recorded baselines: %v", rec2.records)
	}
}

func TestNginxApplyAndDeleteUpdateBaseline(t *testing.T) {
	rec := newFakeRecorder()
	p := newNginxTestProvider(t, &mockNginxRunner{})
	p.SetBaseline(rec)

	if _, err := p.ApplySite(&ProxySite{Name: "app", ServerNames: []string{"a.example.com"}, Upstream: "127.0.0.1:3000", Scheme: "http"}); err != nil {
		t.Fatal(err)
	}
	path := p.sitePath("app")
	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := rec.records["nginx/app"]; string(got) != string(onDisk) {
		t.Fatalf("baseline != on-disk content")
	}

	if _, err := p.DeleteSite("app"); err != nil {
		t.Fatal(err)
	}
	if len(rec.forgot) != 1 || rec.forgot[0] != "nginx/app" {
		t.Fatalf("forgot = %v", rec.forgot)
	}
}

func TestDriftCronUnavailableSkipsSection(t *testing.T) {
	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	// CronRun 为 nil（cron 命令不可用）→ cron 类不产生条目
	p := NewDriftProvider(b, DriftConfig{ConfDir: t.TempDir(), StacksDir: filepath.Join(t.TempDir(), "absent")})
	res, err := p.Check()
	if err != nil {
		t.Fatal(err)
	}
	m, _ := res.(map[string]interface{})
	if items, _ := m["items"].([]driftItem); len(items) != 0 {
		// json roundtrip 后是 []driftItem；直接断言空
		t.Fatalf("items = %+v", m["items"])
	}
}

func mustCheck(t *testing.T, p *DriftProvider) interface{} {
	t.Helper()
	res, err := p.Check()
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// driftItem JSON 形状核对（字段名与 server/Web 契约一致）
func TestDriftItemJSONShape(t *testing.T) {
	raw, err := json.Marshal(driftItem{Kind: "nginx", Name: "web", Status: "drifted", BaselineSHA: "a", CurrentSHA: "b"})
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kind", "name", "status", "baseline_sha", "current_sha"} {
		if _, ok := m[key]; !ok {
			t.Fatalf("missing key %q in %s", key, raw)
		}
	}
}
