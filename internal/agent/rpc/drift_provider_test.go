package rpc

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

// ---------- M3：基线存原文 + drift.diff（D19/D20） ----------

func TestDriftRecordStoresContent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "baseline.json")
	b := NewDriftBaseline(path)
	b.Record("nginx", "web", []byte("server block"))

	snap := NewDriftBaseline(path).snapshot() // 重新打开验证持久化
	if got := snap["nginx/web"].Content; got != "server block" {
		t.Fatalf("content = %q", got)
	}

	// 超上限只存 hash 不存原文（基线文件不膨胀）
	huge := strings.Repeat("x", driftBaselineContentMax+1)
	b.Record("nginx", "big", []byte(huge))
	if got := NewDriftBaseline(path).snapshot()["nginx/big"].Content; got != "" {
		t.Fatalf("oversized content should be empty, got %d bytes", len(got))
	}

	// 恰好等于上限仍存原文
	b.Record("nginx", "edge", []byte(strings.Repeat("y", driftBaselineContentMax)))
	if got := NewDriftBaseline(path).snapshot()["nginx/edge"].Content; len(got) != driftBaselineContentMax {
		t.Fatalf("edge content len = %d", len(got))
	}
}

func TestDriftDiffNginx(t *testing.T) {
	confDir := t.TempDir()
	sitePath := filepath.Join(confDir, "cockpit-site-web.conf")
	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	b.Record("nginx", "web", []byte("# meta\nserver block a"))
	if err := os.WriteFile(sitePath, []byte("# meta\nserver block a"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewDriftProvider(b, DriftConfig{ConfDir: confDir})

	// 一致：两侧全文相等
	res, err := p.Diff("nginx", "web")
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	if m["expected"] != "# meta\nserver block a" || m["current"] != "# meta\nserver block a" {
		t.Fatalf("equal diff = %+v", m)
	}
	if m["baseline_updated_at"].(int64) == 0 {
		t.Fatalf("baseline_updated_at = %v", m["baseline_updated_at"])
	}

	// 手改（含 meta 注释）→ diff 可见当前内容
	if err := os.WriteFile(sitePath, []byte("# 被改的 meta\nserver block EVIL"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err = p.Diff("nginx", "web")
	if err != nil {
		t.Fatal(err)
	}
	m = res.(map[string]interface{})
	if m["expected"] != "# meta\nserver block a" || m["current"] != "# 被改的 meta\nserver block EVIL" {
		t.Fatalf("drifted diff = %+v", m)
	}

	// 文件被删 → 报错（current 缺失）
	if err := os.Remove(sitePath); err != nil {
		t.Fatal(err)
	}
	if _, err := p.Diff("nginx", "web"); err == nil {
		t.Fatal("missing file should error")
	}
}

func TestDriftDiffCronPrettyAndSameSource(t *testing.T) {
	runner := &mockCronRunner{hasFile: true}
	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	p := NewDriftProvider(b, DriftConfig{CronRun: runner.run})

	// 经 CronProvider 面板写路径登记基线（compact marshal 存入）
	cp := NewCronProvider(runner.run)
	cp.SetBaseline(b)
	if _, err := cp.ApplyJob(&CronJob{Name: "backup", Schedule: "0 3 * * *", Command: "/opt/b.sh", Enabled: true}); err != nil {
		t.Fatal(err)
	}

	// diff 返回两侧均为美化后 JSON（多行），且与 check 判定同源（此刻 ok）
	res, err := p.Diff("cron", "cockpit")
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	expected, current := m["expected"].(string), m["current"].(string)
	if !strings.Contains(expected, "\n") || !strings.Contains(current, "\n") {
		t.Fatalf("cron diff should be pretty-printed:\nexpected=%q\ncurrent=%q", expected, current)
	}
	if expected != current {
		t.Fatalf("expected != current before drift:\n%q\n%q", expected, current)
	}
	if got := driftFind(t, mustCheck(t, p), "cron", "cockpit"); got.Status != "ok" {
		t.Fatalf("check after record = %+v", got)
	}

	// 手改 cockpit 段 → drifted 且 diff 两侧不同（展示层美化不破坏判定）
	runner.mu.Lock()
	runner.content = `# cockpit:job {"name":"backup","schedule":"0 3 * * *","command":"/tmp/evil.sh","enabled":true}` + "\n"
	runner.mu.Unlock()
	if got := driftFind(t, mustCheck(t, p), "cron", "cockpit"); got.Status != "drifted" {
		t.Fatalf("after edit = %+v", got)
	}
	res, err = p.Diff("cron", "cockpit")
	if err != nil {
		t.Fatal(err)
	}
	m = res.(map[string]interface{})
	if m["expected"] == m["current"] {
		t.Fatal("diff sides should differ after drift")
	}
	if !strings.Contains(m["current"].(string), "/tmp/evil.sh") {
		t.Fatalf("current should show drifted command: %q", m["current"])
	}
}

func TestDriftDiffStack(t *testing.T) {
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
	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	b.Record("stack", "web/compose.yml", []byte("services: {}"))
	b.Record("stack", "web/.env", []byte("TAG=1.0"))
	mk("web/compose.yml", "services:\n  nginx:\n    image: nginx") // 手改
	mk("web/.env", "TAG=1.0")
	p := NewDriftProvider(b, DriftConfig{StacksDir: stacksDir})

	res, err := p.Diff("stack", "web/compose.yml")
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]interface{})["current"] != "services:\n  nginx:\n    image: nginx" {
		t.Fatalf("compose diff = %+v", res)
	}
	if _, err := p.Diff("stack", "web/.env"); err != nil {
		t.Fatalf("env diff: %v", err)
	}
}

func TestDriftDiffLegacyBaselineNoContent(t *testing.T) {
	// 旧版本基线只有 hash 无原文（Content 空）→ 明确报错而非失败
	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	if err := b.update("nginx/old", BaselineEntry{
		SHA256:    strings.Repeat("a", 64),
		UpdatedAt: time.Now().Unix(),
	}); err != nil {
		t.Fatal(err)
	}
	p := NewDriftProvider(b, DriftConfig{ConfDir: t.TempDir()})
	_, err := p.Diff("nginx", "old")
	if err == nil || !strings.Contains(err.Error(), "no content") {
		t.Fatalf("legacy baseline err = %v", err)
	}
}

func TestDriftDiffTooLarge(t *testing.T) {
	confDir := t.TempDir()
	sitePath := filepath.Join(confDir, "cockpit-site-web.conf")
	b := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	b.Record("nginx", "web", []byte("small baseline"))
	if err := os.WriteFile(sitePath, []byte(strings.Repeat("x", driftDiffMaxBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewDriftProvider(b, DriftConfig{ConfDir: confDir})
	_, err := p.Diff("nginx", "web")
	if err == nil || !strings.Contains(err.Error(), "current content too large") {
		t.Fatalf("oversized current err = %v", err)
	}

	// 基线侧过大（手工构造超限原文的旧基线）→ 报 baseline too large
	b2 := NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json"))
	if err := b2.update("nginx/big", BaselineEntry{
		SHA256: strings.Repeat("b", 64), UpdatedAt: 1,
		Content: strings.Repeat("y", driftDiffMaxBytes+1),
	}); err != nil {
		t.Fatal(err)
	}
	bigPath := filepath.Join(confDir, "cockpit-site-big.conf")
	if err := os.WriteFile(bigPath, []byte("anything"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = NewDriftProvider(b2, DriftConfig{ConfDir: confDir}).Diff("nginx", "big")
	if err == nil || !strings.Contains(err.Error(), "baseline content too large") {
		t.Fatalf("oversized baseline err = %v", err)
	}
}

func TestDriftDiffValidation(t *testing.T) {
	stacksDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(stacksDir, "web"), 0o700); err != nil {
		t.Fatal(err)
	}
	p := NewDriftProvider(
		NewDriftBaseline(filepath.Join(t.TempDir(), "baseline.json")),
		DriftConfig{ConfDir: t.TempDir(), StacksDir: stacksDir},
	)

	cases := []struct {
		kind, name, wantErr string
	}{
		{"kubernetes", "x", "unknown kind"},
		{"cron", "other", "unknown cron object"},
		{"nginx", "", "name required"},
		{"nginx", strings.Repeat("n", driftMaxNameLen+1), "name too long"},
		{"nginx", "../etc/passwd", "invalid nginx site name"},
		{"nginx", `a\b`, "invalid nginx site name"},
		{"stack", "compose.yml", "invalid stack name"}, // 缺目录段
		{"stack", "../escape/compose.yml", "invalid stack name"},
		{"stack", `we\b/compose.yml`, "invalid stack name"},
		{"stack", "web/other.txt", "invalid stack file name"},
		{"stack", "web/nested/compose.yml", "invalid stack file name"}, // 文件名白名单拒绝多级
		{"nginx", "ghost", "no baseline"},                              // 合法形态但无基线
		{"stack", "web/compose.yml", "no baseline"},
	}
	for _, tc := range cases {
		_, err := p.Diff(tc.kind, tc.name)
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("Diff(%q, %q) err = %v, want contains %q", tc.kind, tc.name, err, tc.wantErr)
		}
	}
}
