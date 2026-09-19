package rpc

// cov_round5_test.go 第五轮覆盖率：rpc 侧可注入的分支——logs.follow 参数
// 校验失败与 followCmdFn 启动失败、startFollowCmd 双二进制缺失（PATH 注入）、
// atomicWriteFile 成功与 rename 冲突、overlay toolUnavailable 判定、launchd
// plist 解析杂支、traefik Call 分发/站点查询/删除与 yaml 自检、file Search
// 的上限与跳过族、drift/backup 名校验、DetectSystemd 的 stat 分支。
// 真实 CA（acme）、TOTP、二进制存在但失败的 degrade 路径不在覆盖目标内。

import (
	"bufio"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestCovStartFollowCmdMissingBinaries PATH 注入空目录：journalctl/docker
// 均不可寻址，覆盖两分支的 Start 失败包装
func TestCovStartFollowCmdMissingBinaries(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	q := &LogsQuery{Type: "systemd", Source: "nginx.service", Tail: 10}
	if _, _, err := startFollowCmd(context.Background(), q); err == nil || !strings.Contains(err.Error(), "start systemd follow") {
		t.Fatalf("systemd: %v", err)
	}
	q.Type = "docker"
	if _, _, err := startFollowCmd(context.Background(), q); err == nil || !strings.Contains(err.Error(), "start docker follow") {
		t.Fatalf("docker: %v", err)
	}
}

// TestCovFollowStartParamAndCmdErrors FollowStart 前置校验失败族：
// 参数不可 JSON 化 / followId 类型错 / followCmdFn 启动失败
func TestCovFollowStartParamAndCmdErrors(t *testing.T) {
	p := newFollowTestProvider()
	if _, err := p.FollowStart(map[string]interface{}{"followId": make(chan int)}); err == nil {
		t.Fatal("expect marshal error")
	}
	if _, err := p.FollowStart(map[string]interface{}{"followId": 123}); err == nil {
		t.Fatal("expect followId type error")
	}
	p2 := newFollowTestProvider()
	p2.followCmdFn = func(context.Context, *LogsQuery) (*exec.Cmd, *bufio.Reader, error) {
		return nil, nil, errors.New("journalctl missing")
	}
	if _, err := p2.FollowStart(followParams("f-err", "")); err == nil || !strings.Contains(err.Error(), "journalctl missing") {
		t.Fatalf("cmd fn: %v", err)
	}
}

// TestCovAtomicWriteFile 原子写成功回读 + rename 冲突（目标位被目录占用，
// 文件→目录 rename 必失败）分支
func TestCovAtomicWriteFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "unit.conf")
	if err := atomicWriteFile(path, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "hello" {
		t.Fatalf("read back: %q %v", data, err)
	}
	blocked := filepath.Join(dir, "blocked")
	if err := os.Mkdir(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWriteFile(blocked, []byte("x"), 0o600); err == nil {
		t.Fatal("expect rename error")
	}
}

// TestCovToolUnavailableClassification 不可用判定：exec.ErrNotFound 直传 /
// exec.Error 包装形态 → unavailable；其余错误 → degrade（error）
func TestCovToolUnavailableClassification(t *testing.T) {
	if got := toolUnavailable(&overlayTool{Tool: "zerotier"}, exec.ErrNotFound); got.Status != "unavailable" {
		t.Fatalf("ErrNotFound: %+v", got)
	}
	wrapped := &exec.Error{Name: "wg", Err: exec.ErrNotFound}
	if got := toolUnavailable(&overlayTool{Tool: "wg"}, wrapped); got.Status != "unavailable" {
		t.Fatalf("exec.Error: %+v", got)
	}
	if got := toolUnavailable(&overlayTool{Tool: "wg"}, errors.New("exit status 1")); got.Status != "error" || got.Error == "" {
		t.Fatalf("degrade: %+v", got)
	}
}

// TestCovParsePlistDaemonBranches plist 解析杂支：<false/> 值、array/dict
// 整树跳过、key 内嵌元素报错、空标量值元素的 EndElement 复位
func TestCovParsePlistDaemonBranches(t *testing.T) {
	svc, err := parsePlistDaemon([]byte(`<plist><dict>
		<key>Label</key><string>com.example.x</string>
		<key>RunAtLoad</key><false/>
		<key>Disabled</key><true/>
	</dict></plist>`))
	if err != nil || svc.Label != "com.example.x" || svc.RunAtLoad || !svc.Disabled {
		t.Fatalf("false branch: %+v %v", svc, err)
	}
	svc, err = parsePlistDaemon([]byte(`<plist><dict>
		<key>Label</key><string>a</string>
		<key>ProgramArguments</key><array><string>/bin/x</string></array>
		<key>Disabled</key><false/>
	</dict></plist>`))
	if err != nil || svc.Label != "a" || svc.Disabled {
		t.Fatalf("array skip: %+v %v", svc, err)
	}
	if _, err := parsePlistDaemon([]byte(`<plist><dict><key><dict/></key></dict></plist>`)); err == nil || !strings.Contains(err.Error(), "inside key") {
		t.Fatalf("nested key: %v", err)
	}
	if _, err := parsePlistDaemon([]byte(`<plist><dict><key>Label</key><string></string><key>RunAtLoad</key><true/></dict></plist>`)); err != nil {
		t.Fatalf("empty scalar: %v", err)
	}
}

// TestCovCronTZAndDriftPureHelpers 纯函数：isAlphaTZ 字符白名单、
// driftIndentJSON 非法 JSON 回退原文、validDriftTarget 的 stack/cron/未知
// kind 校验
func TestCovCronTZAndDriftPureHelpers(t *testing.T) {
	if !isAlphaTZ("UTC") || !isAlphaTZ("") || isAlphaTZ("UTC+8") || isAlphaTZ("As ia") {
		t.Fatal("isAlphaTZ misclassifies")
	}
	if got := driftIndentJSON("{bad json"); got != "{bad json" {
		t.Fatalf("indent fallback: %q", got)
	}
	if err := validDriftTarget("stack", ""); err == nil || err.Error() != "name required" {
		t.Fatalf("stack empty: %v", err)
	}
	if err := validDriftTarget("stack", strings.Repeat("d", driftMaxNameLen+1)+"/compose.yml"); err == nil || !strings.Contains(err.Error(), "too long") {
		t.Fatalf("stack long: %v", err)
	}
	if err := validDriftTarget("stack", "../escape"); err == nil {
		t.Fatal("stack traversal accepted")
	}
	if err := validDriftTarget("stack", "app/other.yml"); err == nil {
		t.Fatal("stack bad file accepted")
	}
	if err := validDriftTarget("cron", "other"); err == nil {
		t.Fatal("cron object accepted")
	}
	if err := validDriftTarget("bogus", "x"); err == nil {
		t.Fatal("unknown kind accepted")
	}
}

// TestCovDriftCurrentContentUnknownKind 内容分发对未知 kind 的兜底
func TestCovDriftCurrentContentUnknownKind(t *testing.T) {
	p := NewDriftProvider(nil, DriftConfig{})
	if _, err := p.currentContent("bogus", "x"); err == nil || !strings.Contains(err.Error(), "unknown kind") {
		t.Fatalf("unknown kind: %v", err)
	}
}

// TestCovTraefikDispatchAndSiteOps Call 的 status/sites 分发、动态目录不可
// 读、GetSite 的非法名/缺失/meta 损坏、DeleteSite 的非法名/缺失
func TestCovTraefikDispatchAndSiteOps(t *testing.T) {
	dir := t.TempDir()
	p := newTraefikTestProvider(t, dir, okTraefikRunner)
	if _, err := p.Call("status", nil); err != nil {
		t.Fatalf("call status: %v", err)
	}
	if _, err := p.Call("sites", nil); err != nil {
		t.Fatalf("call sites: %v", err)
	}
	// 未闭合 [ 使 Glob 模式非法（ErrBadPattern）→ 扫描失败
	broken := NewTraefikProvider(filepath.Join(dir, "bad["), okTraefikRunner)
	if _, err := broken.Call("status", nil); err == nil || !strings.Contains(err.Error(), "scan dynamic dir") {
		t.Fatalf("broken dir: %v", err)
	}
	if _, err := p.GetSite("bad/name"); err == nil {
		t.Fatal("invalid site name accepted")
	}
	if _, err := p.GetSite("ghost"); err == nil || !strings.Contains(err.Error(), "site not found") {
		t.Fatalf("ghost: %v", err)
	}
	if err := os.WriteFile(p.sitePath("broken"), []byte("not a meta line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := p.GetSite("broken"); err == nil || !strings.Contains(err.Error(), "corrupted meta") {
		t.Fatalf("broken meta: %v", err)
	}
	if _, err := p.DeleteSite("bad/name"); err == nil {
		t.Fatal("invalid delete name accepted")
	}
	if _, err := p.DeleteSite("ghost"); err == nil || !strings.Contains(err.Error(), "site not found") {
		t.Fatalf("delete ghost: %v", err)
	}
}

// TestCovCheckTraefikYAML 渲染自检的失败族：非法 YAML / 无 http 段 /
// 空 routers 或 services / router 引用缺失 service
func TestCovCheckTraefikYAML(t *testing.T) {
	meta := nginxMetaPrefix + ` {"name":"a"}` + "\n"
	if err := checkTraefikYAML([]byte(meta + "not: [valid\n  yaml\n")); err == nil {
		t.Fatal("expect yaml parse error")
	}
	if err := checkTraefikYAML([]byte(meta + "http: null\n")); err == nil || !strings.Contains(err.Error(), "no http section") {
		t.Fatalf("null http: %v", err)
	}
	if err := checkTraefikYAML([]byte(meta + "http:\n  routers: {}\n  services: {}\n")); err == nil || !strings.Contains(err.Error(), "no routers or services") {
		t.Fatalf("empty http: %v", err)
	}
	dangling := meta + "http:\n  routers:\n    r1:\n      service: missing-svc\n      rule: Host(`a.test`)\n  services:\n    other:\n      loadBalancer:\n        servers:\n          - url: http://127.0.0.1:80\n"
	if err := checkTraefikYAML([]byte(dangling)); err == nil || !strings.Contains(err.Error(), "missing service") {
		t.Fatalf("dangling router: %v", err)
	}
}

// TestCovFileSearchGuardBranches Search 的跳过与上限族：.git/node_modules
// 整枝跳过、超深截断、symlink 跳过、二进制探测、无权限文件 open 失败、
// 命中长行截断、maxResults 提前停
func TestCovFileSearchGuardBranches(t *testing.T) {
	p := NewFileProvider()
	root := t.TempDir()
	write := func(rel, content string) {
		t.Helper()
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("hit.txt", "has findme inside\n")
	write("bin.dat", "find\x00me binary\n")                         // 二进制探测跳过
	write("deep-line.txt", "findme "+strings.Repeat("x", 300)+"\n") // 命中且长行截断
	write("noperm.txt", "findme\n")                                 // 稍后去权限 → open 失败
	for _, skip := range []string{".git/a.txt", "node_modules/b.txt"} {
		write(skip, "findme in skipped dir\n")
	}
	if err := os.Symlink(filepath.Join(root, "hit.txt"), filepath.Join(root, "link.txt")); err != nil {
		t.Fatal(err)
	}
	rel := ""
	for i := 0; i <= fileSearchMaxDepth+1; i++ {
		rel = filepath.Join(rel, "d"+strconv.Itoa(i))
	}
	write(filepath.Join(rel, "leaf.txt"), "findme deep\n") // 超深 → 整枝跳过
	if err := os.Chmod(filepath.Join(root, "noperm.txt"), 0o000); err != nil {
		t.Fatal(err)
	}

	res, err := p.Search(map[string]interface{}{"dir": root, "query": "findme"})
	if err != nil {
		t.Fatal(err)
	}
	m := res.(map[string]interface{})
	if m["truncated"] != true { // 深度截断
		t.Fatalf("expect truncated: %v", m)
	}
	matches := m["matches"].([]map[string]interface{})
	if len(matches) != 2 { // hit.txt + deep-line.txt（其余被跳过）
		t.Fatalf("matches: %v", matches)
	}
	for _, match := range matches {
		text := match["text"].(string)
		if len(text) > 200 {
			t.Fatalf("long line not truncated: %q", text)
		}
	}
	if skipped := m["skipped"].(int); skipped < 1 { // bin.dat 二进制跳过
		t.Fatalf("skipped: %v", m)
	}

	// maxResults 提前停：首个命中后 SkipAll
	res, err = p.Search(map[string]interface{}{"dir": root, "query": "findme", "maxResults": float64(1)})
	if err != nil {
		t.Fatal(err)
	}
	m = res.(map[string]interface{})
	if got := len(m["matches"].([]map[string]interface{})); got != 1 || m["truncated"] != true {
		t.Fatalf("maxResults: %v", m)
	}
}

// TestCovBackupNameGuards 备份文件名校验族：非法名、删除不存在的文件、
// 恢复时归档缺失
func TestCovBackupNameGuards(t *testing.T) {
	p := newBackupTestProvider(t)
	if _, err := p.DeleteFile("/tmp", "bad name.tar.gz"); err == nil {
		t.Fatal("invalid name accepted")
	}
	if _, err := p.DeleteFile(t.TempDir(), "ghost.tar.gz"); err == nil || !strings.Contains(err.Error(), "remove") {
		t.Fatalf("remove missing: %v", err)
	}
	if _, err := p.RunRestore(map[string]interface{}{"dir": "/tmp", "name": "ghost.tar.gz", "destDir": t.TempDir()}); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("restore missing: %v", err)
	}
}

// TestCovDetectSystemdStatBranch 空 PATH 时 LookPath 失败返回 false；注入
// 假 systemctl 使 LookPath 命中，必经 /run/systemd/system 的 Stat（返回值
// 随宿主环境，不断言）
func TestCovDetectSystemdStatBranch(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if DetectSystemd() {
		t.Fatal("expect false with empty PATH")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "systemctl"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_ = DetectSystemd()
}
