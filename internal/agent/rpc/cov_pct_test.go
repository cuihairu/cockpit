package rpc

// cov_pct_test.go 覆盖率冲刺（backup / cron / logs / ddns / smart / overlay）：
// 只用既有注入点（rcloneBin、backupHookTimeout、recordingCronRunner、PATH env、
// ddns 源列表 var、私有函数直调）补齐分支，不改任何生产代码。
// 经分析不可达/不可稳定触发的行（rand.Read 失败、f.Stat/f.Close 在已打开 fd 上
// 失败等）见任务报告，不在本文件强测。

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// pctRT 可编程 RoundTripper：直接返回预设的响应或错误
type pctRT func(*http.Request) (*http.Response, error)

func (f pctRT) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// pctErrBody 永远读失败的响应体（触发 fetchOne 的 io.ReadAll 错误分支）
type pctErrBody struct{}

func (pctErrBody) Read([]byte) (int, error) { return 0, errors.New("body broken") }
func (pctErrBody) Close() error             { return nil }

// TestCovPctBackupListFilesInfoErr backup.list 条目 Info() 失败分支：
// 目录去 x 位（保留 r）：ReadDir 仍可列名，但 lstat(dir/entry) EACCES
// → e.Info() 报错 → continue 跳过（L297）。root 不理会权限位，跳过。
func TestCovPctBackupListFilesInfoErr(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root 不受目录 x 位限制，无法触发 lstat 失败")
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.tar.gz"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o400); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(dir, 0o700) }() // 恢复 x 位，t.TempDir 清理需要

	p := newBackupTestProvider(t)
	res, err := p.ListFiles(dir)
	if err != nil {
		t.Fatalf("ListFiles: %v", err)
	}
	files, _ := res["files"].([]BackupFile)
	if len(files) != 0 {
		t.Fatalf("Info 失败的条目应被跳过, got %d", len(files))
	}
}

// TestCovPctPushRemoteTruncate pushRemote 截断分支：fake rclone 输出 3000 字节
// 并以 9 退出 → summary>2048 截断加省略号（L316）+ detail>512 截断进错误（L323）
func TestCovPctPushRemoteTruncate(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "fake-rclone")
	script := "#!/bin/sh\nhead -c 3000 /dev/zero | tr '\\0' 'x'\nexit 9\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	p := newBackupTestProvider(t)
	p.rcloneBin = bin
	task := &backupTask{Log: &cappedBuffer{limit: backupLogLimit}}

	_, err := p.pushRemote(task, filepath.Join(dir, "f.tar.gz"), "remote:bucket")
	if err == nil || !strings.Contains(err.Error(), "rclone copy") {
		t.Fatalf("期望 rclone 失败错误, got %v", err)
	}
	if !strings.Contains(task.Log.String(), "…") {
		t.Fatalf("summary >2048 应截断加省略号, log=%q", task.Log.String())
	}
	// detail 截断是硬截断（不加省略号）：错误里不应出现完整 3000 字节
	if strings.Contains(err.Error(), "…") || len(err.Error()) > 600 {
		t.Fatalf("detail >512 应截断, err 长度=%d", len(err.Error()))
	}
}

// TestCovPctRunHookOutputTruncations runHook 输出截断与失败摘要分支：
// 成功 hook 输出 >2048 → summary 截断（L364）；失败 hook 输出 >512 →
// detail 截断（L371）；失败 hook 无输出 → 纯 exit status 摘要（L377）
func TestCovPctRunHookOutputTruncations(t *testing.T) {
	p := newBackupTestProvider(t)
	newTask := func() *backupTask { return &backupTask{Log: &cappedBuffer{limit: backupLogLimit}} }

	// L364：hook 成功但输出超长
	task := newTask()
	if err := p.runHook(task, "head -c 3000 /dev/zero | tr '\\0' 'x'"); err != nil {
		t.Fatalf("成功 hook 不应报错: %v", err)
	}
	if !strings.Contains(task.Log.String(), "…") {
		t.Fatalf("hook summary >2048 应截断, log=%q", task.Log.String())
	}

	// L371：hook 非零退出且输出超长
	task = newTask()
	err := p.runHook(task, "head -c 600 /dev/zero | tr '\\0' 'y'; exit 3")
	if err == nil || !strings.Contains(err.Error(), "pre-hook failed") {
		t.Fatalf("期望 pre-hook failed, got %v", err)
	}
	// detail 硬截断（不加省略号）：错误长度应远小于完整 600 字节输出
	if len(err.Error()) > 600 {
		t.Fatalf("hook detail >512 应截断, err 长度=%d", len(err.Error()))
	}

	// L377：hook 非零退出且无输出
	task = newTask()
	err = p.runHook(task, "exit 7")
	if err == nil || !strings.Contains(err.Error(), "exit status 7") {
		t.Fatalf("期望 exit status 7 摘要, got %v", err)
	}
}

// TestCovPctRunHookStartErr runHook cmd.Start() 失败分支：PATH 指向空目录，
// exec.Command("sh") 查找不到可执行文件 → Start 报错 → "pre-hook start"（L350）
func TestCovPctRunHookStartErr(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	p := newBackupTestProvider(t)
	task := &backupTask{Log: &cappedBuffer{limit: backupLogLimit}}
	err := p.runHook(task, "true")
	if err == nil || !strings.Contains(err.Error(), "pre-hook start") {
		t.Fatalf("期望 pre-hook start 错误, got %v", err)
	}
}

// TestCovPctCronNextEdgeFields parseCronSchedule 边缘分支：
// 非法字段片段 → m == nil 报错（L106）；`*-5` 星号带范围 → m[1]=="*"
// 时 start 收敛到域下限（L123）
func TestCovPctCronNextEdgeFields(t *testing.T) {
	if _, err := parseCronSchedule("bad * * * *"); err == nil {
		t.Fatal("非法字段应报错")
	}
	s, err := parseCronSchedule("*-5 * * * *")
	if err != nil || s == nil {
		t.Fatalf("*-5 应解析成功, err=%v", err)
	}
	if s.min&(1<<0) == 0 {
		t.Fatal("星号范围应从域下限 0 展开")
	}
	if s.min&(1<<5) == 0 {
		t.Fatal("范围上界 5 应在位集合内")
	}
}

// TestCovPctCronTimersAndNoArgMethods Call 分派 timers（L168）+ 空 unit 列表
// 分支（cron_timers L33）；Status()/DeleteJob() 无参公共方法（M4 保留变体，
// L197/L291）经假执行器直接覆盖
func TestCovPctCronTimersAndNoArgMethods(t *testing.T) {
	r := &recordingCronRunner{}
	p := NewCronProvider(r.run)

	res, err := p.Call("timers", nil)
	if err != nil {
		t.Fatalf("timers: %v", err)
	}
	m, _ := res.(map[string]interface{})
	timers, _ := m["timers"].([]interface{})
	if len(timers) != 0 {
		t.Fatalf("空列表输出应得空 timers, got %d", len(timers))
	}

	if _, err := p.Status(); err != nil {
		t.Fatalf("Status: %v", err)
	}

	pair := renderJobPair(&CronJob{Name: "demo", Schedule: "0 3 * * *", Command: "run.sh", Enabled: true})
	r.listOut = pair[0] + "\n" + pair[1] + "\n"
	if _, err := p.DeleteJob("demo"); err != nil {
		t.Fatalf("DeleteJob: %v", err)
	}
	if !r.wrote {
		t.Fatal("删除任务应写回 crontab")
	}
}

// TestCovPctDDNSFetchOneErrors fetchOne 错误分支（私有方法直调 + 可编程传输）：
// Transport 失败 → client.Do 错误（L106）；200 但响应体读失败 →
// io.ReadAll 错误（L114）
func TestCovPctDDNSFetchOneErrors(t *testing.T) {
	ctx := context.Background()

	pErr := NewDDNSProvider(&http.Client{Transport: pctRT(func(*http.Request) (*http.Response, error) {
		return nil, errors.New("dial broken")
	})})
	if got := pErr.fetchOne(ctx, "https://ip.example", false); got != "" {
		t.Fatalf("Do 失败应返回空串, got %q", got)
	}

	pBody := NewDDNSProvider(&http.Client{Transport: pctRT(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(pctErrBody{}),
			Header:     http.Header{},
		}, nil
	})})
	if got := pBody.fetchOne(ctx, "https://ip.example", false); got != "" {
		t.Fatalf("响应体读失败应返回空串, got %q", got)
	}
}

// TestCovPctDDNSFamilyDeadline probeFamily 族总超时分支（L88）：
// 挂住的回显服务耗尽 15s 族预算 → client.Do 随 ctx 超时返回 → 下一轮
// select 命中 ctx.Done → 直接返回空串，第二源不再尝试（handler 只被调一次）。
// 硬超时非睡眠时序，确定性成立；short 模式跳过省时。
func TestCovPctDDNSFamilyDeadline(t *testing.T) {
	if testing.Short() {
		t.Skip("short 模式跳过 15s 族超时用例")
	}
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-r.Context().Done() // 挂住直到客户端放弃
	}))
	defer srv.Close()

	p := NewDDNSProvider(&http.Client{}) // 不设客户端超时：只由族 15s 硬超时截断
	sources := []string{srv.URL, srv.URL}
	if got := p.probeFamily(sources, false); got != "" {
		t.Fatalf("族超时应返回空串, got %q", got)
	}
	if n := atomic.LoadInt32(&hits); n != 1 {
		t.Fatalf("第二源不应被尝试, hits=%d", n)
	}
}

// TestCovPctLogsCallFollowDispatch logs Call 分派 follow.start / follow.stop
// 分支（L117/L119）：未注入 sender → start 报 not available；不存在的
// followId stop 幂等成功
func TestCovPctLogsCallFollowDispatch(t *testing.T) {
	p := NewLogsProvider(nil)
	if _, err := p.Call("follow.start", map[string]interface{}{"followId": "f1"}); err == nil ||
		!strings.Contains(err.Error(), "log follow not available") {
		t.Fatalf("follow.start 无 sender 应报错, got %v", err)
	}
	if _, err := p.Call("follow.stop", map[string]interface{}{"followId": "ghost"}); err != nil {
		t.Fatalf("follow.stop 不存在 id 应幂等成功, got %v", err)
	}
}

// TestCovPctStartFollowCmdOK startFollowCmd 成功路径（L170）：PATH 注入可执行的
// 假 journalctl（sleep 挂住），Start 成功后返回 cmd 与行读取器；用例结束经
// ctx cancel（CommandContext 杀进程）+ Wait 回收
func TestCovPctStartFollowCmdOK(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "journalctl")
	if err := os.WriteFile(bin, []byte("#!/bin/sh\nexec sleep 30\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cmd, reader, err := startFollowCmd(ctx, &LogsQuery{Type: "systemd", Source: "nginx.service", Tail: 5})
	if err != nil {
		t.Fatalf("startFollowCmd: %v", err)
	}
	if cmd == nil || cmd.Process == nil || reader == nil {
		t.Fatal("成功路径应返回 cmd 与 reader")
	}
	cancel()
	_ = cmd.Wait()
}

// TestCovPctSmartParseEdges smart 解析边缘分支（私有函数直调）：
// 表项非对象 → 跳过（L94）；id 匹配但 raw 非对象 → 跳过（L101）；
// stderr 为空且 err 非空 → 用 err 文本作摘要（L112）
func TestCovPctSmartParseEdges(t *testing.T) {
	if ataAttrByID([]interface{}{"junk"}, 5) != nil {
		t.Fatal("非对象表项应被跳过")
	}
	bad := []interface{}{map[string]interface{}{"id": float64(5), "raw": "junk"}}
	if ataAttrByID(bad, 5) != nil {
		t.Fatal("raw 非对象应被跳过")
	}
	if got := smartErrSummary(nil, errors.New("boom")); got != "boom" {
		t.Fatalf("空 stderr 应回落 err 文本, got %q", got)
	}
}

// TestCovPctWGDumpShortLine parseWGDump 字段数 <3 的行直接跳过（L214）
func TestCovPctWGDumpShortLine(t *testing.T) {
	if out := parseWGDump([]byte("only\ttwo\n"), time.Now()); len(out) != 0 {
		t.Fatalf("字段不足的行应被跳过, got %d", len(out))
	}
}
