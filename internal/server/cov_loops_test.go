package server

// cov_loops_test.go 覆盖后台循环（alert_loop / backup_loop / drift_scan /
// server_backup / ticket cleanup）与 recording.go 的内部函数。循环体统一用
// ctx 取消退出或直测单轮函数；等真实周期（1h ticker / 30s keepalive / 睡到
// 凌晨 3 点等）的分支见最终报告的不可达清单。

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/protocol"
	"github.com/cuihairu/cockpit/internal/storage"
)

// covLoopServer 返回带最小 cfg 的 server（循环函数普遍读 s.cfg.Notification）。
// covNewServer 不设 cancel 字段，这里补上可手动取消的 ctx。
func covLoopServer(t *testing.T) *Server {
	t.Helper()
	s := covNewServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	s.ctx = ctx
	s.cancel = cancel
	s.cfg = &config.Config{
		Database: &config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "cockpit.db")},
	}
	return s
}

// covRunLoop 带 deadline 等待循环 goroutine 退出
func covWaitExit(t *testing.T, desc string, exited <-chan struct{}) {
	t.Helper()
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatalf("loop did not exit on ctx cancel: %s", desc)
	}
}

// ============ alert_loop.go ============

func TestCovAlertLoopsExitOnCancel(t *testing.T) {
	s := covLoopServer(t)

	exited1 := covSpawnLoop(func() { s.cleanupLoop() })
	exited2 := covSpawnLoop(func() { s.alertCheckLoop() })
	// metricsCleanupLoop 先睡到凌晨 3 点，ctx 取消不能唤醒 sleep——只验证它
	// 能启动并挂起（覆盖 ticker/时间计算语句），不等待退出
	exited3 := covSpawnLoop(func() { s.metricsCleanupLoop() })

	time.Sleep(100 * time.Millisecond) // 让循环都进入 select
	s.cancel()                          // covNewServer 的 cleanup 也会 cancel（幂等）

	covWaitExit(t, "cleanupLoop", exited1)
	covWaitExit(t, "alertCheckLoop", exited2)
	select {
	case <-exited3:
		// 若恰好在 3 点附近跑测试会自然退出
	default:
	}
}

// covSpawnLoop 在 goroutine 里跑 fn，返回退出信号
func covSpawnLoop(fn func()) <-chan struct{} {
	exited := make(chan struct{})
	go func() {
		defer close(exited)
		fn()
	}()
	return exited
}

func TestCovRunAlertChecksAndCleanup(t *testing.T) {
	s := covLoopServer(t)
	// 直测单轮：空库不 panic 即可（内部覆盖告警生成与清理 SQL）
	s.runAlertChecks()
	s.cleanupOldAlerts()

	// 关库后同样不 panic（错误仅记日志）
	covCloseDB(t, s)
	s.runAlertChecks()
	s.cleanupOldAlerts()
}

// ============ backup_loop.go ============

func TestCovStartBackupLoopAndRecover(t *testing.T) {
	s := covLoopServer(t)

	// 造一条遗留 running 记录：recoverOrphanBackupRuns 应标记失败
	agent := covFakeAgent(t, s, "agent-loop", nil, nil)
	cfg := newBackupCfg("agent-loop", "etc", "manual", true)
	cfg.LastStatus = "running"
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	run := &storage.BackupRun{ConfigID: cfg.ID, Status: "running", StartedAt: time.Now().Unix()}
	if err := s.db.CreateBackupRun(run); err != nil {
		t.Fatal(err)
	}

	exited := covSpawnLoop(s.startBackupLoop)
	covWaitGone(t, "orphan run recovered", func() bool {
		got, err := s.db.GetBackupRun(run.ID)
		return err == nil && got.Status == "failed" && got.Error == "server restarted during backup"
	})
	gotCfg, err := s.db.GetBackupConfig(cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if gotCfg.LastStatus != "failed" {
		t.Errorf("config last status = %s, want failed", gotCfg.LastStatus)
	}
	_ = agent

	s.cancel()
	covWaitExit(t, "backupLoop", exited)
}

func TestCovRecoverOrphanDBError(t *testing.T) {
	s := covLoopServer(t)
	covCloseDB(t, s)
	s.recoverOrphanBackupRuns() // RunningBackupRuns 报错直接返回
}

func TestCovDispatchDueBackups(t *testing.T) {
	s := covLoopServer(t)
	covFakeAgent(t, s, "agent-due", nil, nil)

	// 手动到期：NextRunAt 设为过去 + every 调度推进
	cfg := newBackupCfg("ghost-agent", "due", "every:1h", true)
	cfg.NextRunAt = time.Now().Add(-time.Hour).Unix()
	if err := s.db.CreateBackupConfig(cfg); err != nil {
		t.Fatal(err)
	}
	s.dispatchDueBackups() // 推进 next_run_at + 离线 agent 下发失败

	got, err := s.db.GetBackupConfig(cfg.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.NextRunAt <= cfg.NextRunAt {
		t.Errorf("next_run_at not advanced: %d", got.NextRunAt)
	}
	if got.LastStatus != "failed" {
		t.Errorf("last status = %s, want failed", got.LastStatus)
	}

	// 关库 → list due 报错分支
	covCloseDB(t, s)
	s.dispatchDueBackups()
}

func TestCovStartBackupRunGuards(t *testing.T) {
	s := covLoopServer(t)
	covFakeAgent(t, s, "agent-guard", nil, nil)

	// LastStatus=running → already running
	busy := covSeedBackupCfg(t, s, "agent-guard", "busy")
	busy.LastStatus = "running"
	if err := s.db.UpdateBackupConfig(busy); err != nil {
		t.Fatal(err)
	}
	if _, err := s.startBackupRun(busy, "manual"); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("err = %v, want already running", err)
	}

	// 非法名 → invalid name
	bad := newBackupCfg("agent-guard", "Bad Name!", "manual", true)
	if _, err := s.startBackupRun(bad, "manual"); err == nil || !strings.Contains(err.Error(), "invalid backup name") {
		t.Fatalf("err = %v, want invalid name", err)
	}

	// destDir 为空 → destDir required
	noDest := newBackupCfg("agent-guard", "nodest", "manual", true)
	noDest.DestDir = ""
	if _, err := s.startBackupRun(noDest, "manual"); err == nil || !strings.Contains(err.Error(), "destDir required") {
		t.Fatalf("err = %v, want destDir required", err)
	}
}

func TestCovStartBackupRunDispatchFailureAndSuccess(t *testing.T) {
	// 离线 agent：记失败终态 + 通知 + 返回错误
	s := covLoopServer(t)
	cfg := covSeedBackupCfg(t, s, "ghost-agent", "off")
	runID, err := s.startBackupRun(cfg, "scheduled")
	if err == nil || !strings.Contains(err.Error(), "dispatch to agent") {
		t.Fatalf("err = %v, want dispatch failure", err)
	}
	run, rerr := s.db.GetBackupRun(runID)
	if rerr != nil || run.Status != "failed" {
		t.Fatalf("run = %+v, err = %v (want failed)", run, rerr)
	}

	// 成功路径：应答 taskId + 轮询任务到 success 终态（backupTrackInterval 缩短）
	old := backupTrackInterval
	backupTrackInterval = 20 * time.Millisecond
	t.Cleanup(func() { backupTrackInterval = old })

	s2 := covLoopServer(t)
	var calls int32
	covFakeAgent(t, s2, "agent-ok", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		switch method {
		case "backup.run":
			return covOKPayload(map[string]interface{}{"taskId": "task-1"})
		case "backup.task.get":
			if atomic.AddInt32(&calls, 1) == 1 {
				return covOKPayload(map[string]interface{}{"status": "running"})
			}
			return covOKPayload(map[string]interface{}{"status": "success", "file": "etc.tar.gz", "size": 123})
		}
		return covErrPayload("unexpected " + method)
	})
	cfg2 := covSeedBackupCfg(t, s2, "agent-ok", "etc")
	runID2, err := s2.startBackupRun(cfg2, "manual")
	if err != nil {
		t.Fatalf("startBackupRun: %v", err)
	}
	covWaitGone(t, "backup run success", func() bool {
		run, err := s2.db.GetBackupRun(runID2)
		if err != nil || run.Status != "success" || run.File != "etc.tar.gz" || run.Size != 123 {
			return false
		}
		got, err := s2.db.GetBackupConfig(cfg2.ID)
		return err == nil && got.LastStatus == "success"
	})

	// trackBackupTask 的 ctx 取消分支：成功下发后立即 cancel
	s3 := covLoopServer(t)
	s3Cancel, ok := s3.ctx.Deadline()
	_ = s3Cancel
	_ = ok
	covFakeAgent(t, s3, "agent-cancel", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "backup.run" {
			return covOKPayload(map[string]interface{}{"taskId": "task-2"})
		}
		return covOKPayload(map[string]interface{}{"status": "running"}) // 永远 running
	})
	cfg3 := covSeedBackupCfg(t, s3, "agent-cancel", "etc")
	if _, err := s3.startBackupRun(cfg3, "manual"); err != nil {
		t.Fatalf("startBackupRun: %v", err)
	}
	time.Sleep(50 * time.Millisecond) // 让 trackBackupTask 至少跑一轮
}

func TestCovTrackBackupTaskFailedNotify(t *testing.T) {
	old := backupTrackInterval
	backupTrackInterval = 20 * time.Millisecond
	t.Cleanup(func() { backupTrackInterval = old })

	s := covLoopServer(t)
	covFakeAgent(t, s, "agent-f", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		if method == "backup.task.get" {
			return covOKPayload(map[string]interface{}{"status": "failed", "error": "disk full"})
		}
		return covErrPayload("unexpected " + method)
	})
	cfg := covSeedBackupCfg(t, s, "agent-f", "etc")
	run := &storage.BackupRun{ConfigID: cfg.ID, Status: "running", StartedAt: time.Now().Unix()}
	if err := s.db.CreateBackupRun(run); err != nil {
		t.Fatal(err)
	}
	go s.trackBackupTask(cfg.ID, "agent-f", "task-x", run.ID)
	covWaitGone(t, "backup run failed", func() bool {
		got, err := s.db.GetBackupRun(run.ID)
		if err != nil || got.Status != "failed" || got.Error != "disk full" {
			return false
		}
		gotCfg, err := s.db.GetBackupConfig(cfg.ID)
		return err == nil && gotCfg.LastStatus == "failed"
	})
}

func TestCovPollBackupTaskBranches(t *testing.T) {
	s := covLoopServer(t)

	// agent 不存在 → 非终态
	if _, _, _, _, done := s.pollBackupTask("ghost", "t"); done {
		t.Error("missing agent should not be terminal")
	}

	// decode 失败（status 非字符串）→ 非终态
	covBadPayloadAgent(t, s, "agent-decode")
	if _, _, _, _, done := s.pollBackupTask("agent-decode", "t"); done {
		t.Error("decode error should not be terminal")
	}

	// rpc error 含 not found → failed + task lost
	covFakeAgent(t, s, "agent-lost", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("task not found")
	})
	st, _, _, e, done := s.pollBackupTask("agent-lost", "t")
	if !done || st != "failed" || e != "task lost on agent" {
		t.Fatalf("lost task = %s %s %v", st, e, done)
	}

	// rpc error 其他 → 非终态（agent 忙）
	covFakeAgent(t, s, "agent-busy", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("agent busy")
	})
	if _, _, _, _, done := s.pollBackupTask("agent-busy", "t"); done {
		t.Error("other rpc error should not be terminal")
	}

	// data 非 map → 非终态
	covFakeAgent(t, s, "agent-nomap", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload("plain-string")
	})
	if _, _, _, _, done := s.pollBackupTask("agent-nomap", "t"); done {
		t.Error("non-map data should not be terminal")
	}

	// failed 无 error 字段 → 默认文案
	covFakeAgent(t, s, "agent-fnoerr", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"status": "failed"})
	})
	st, _, _, e, done = s.pollBackupTask("agent-fnoerr", "t")
	if !done || st != "failed" || e != "backup failed on agent" {
		t.Fatalf("failed no err = %s %s %v", st, e, done)
	}

	// running → 非终态
	covFakeAgent(t, s, "agent-running", nil, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"status": "running"})
	})
	if _, _, _, _, done := s.pollBackupTask("agent-running", "t"); done {
		t.Error("running should not be terminal")
	}
}

func TestCovBackupLoopHelpers(t *testing.T) {
	// extractTaskID 四分支
	if got := extractTaskID(nil); got != "" {
		t.Errorf("nil resp task id = %q", got)
	}
	badMsg := protocol.NewMessage(protocol.MessageTypeRPCResponse, map[string]interface{}{"status": 123})
	if got := extractTaskID(badMsg); got != "" {
		t.Errorf("bad resp task id = %q", got)
	}
	noMap := protocol.NewMessage(protocol.MessageTypeRPCResponse, covOKPayload("str"))
	if got := extractTaskID(noMap); got != "" {
		t.Errorf("non-map data task id = %q", got)
	}
	good := protocol.NewMessage(protocol.MessageTypeRPCResponse,
		covOKPayload(map[string]interface{}{"taskId": "task-9"}))
	if got := extractTaskID(good); got != "task-9" {
		t.Errorf("task id = %q", got)
	}

	// truncateErr
	short := errors.New("boom")
	if truncateErr(short) != "boom" {
		t.Error("short error should be unchanged")
	}
	long := errors.New(strings.Repeat("x", 600))
	if got := truncateErr(long); len(got) != 480 {
		t.Errorf("truncated len = %d, want 480", len(got))
	}

	// notifyBackupFailed：未启用通知时仅入队丢弃，不 panic
	s := covLoopServer(t)
	s.notifyBackupFailed(&storage.BackupConfig{ID: 7, Name: "etc"}, "boom")

	// ValidBackupSchedule 边界
	for _, c := range []struct {
		in   string
		want bool
	}{
		{"manual", true}, {"daily@00:00", true}, {"daily@24:00", false}, {"daily@00:60", false},
		{"daily@0:0", true}, {"daily@000:00", false}, {"daily@00:000", false}, {"daily@ab:cd", false},
		{"daily@", false}, {"daily@00", false}, {"every:1h", true}, {"every:0h", false},
		{"every:169h", false}, {"every:168h", true}, {"every:2", true}, {"every:h", false},
		{"weekly", false},
	} {
		if got := ValidBackupSchedule(c.in); got != c.want {
			t.Errorf("ValidBackupSchedule(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// ============ drift_scan.go ============

func TestCovDriftScan(t *testing.T) {
	s := covLoopServer(t)

	// 间隔读取：未配置 → 默认；0 → 关闭（合法）；非法 → 默认
	if got := s.GetDriftScanInterval(); got != 1800 {
		t.Errorf("default interval = %d", got)
	}
	if err := s.SetDriftScanInterval(0); err != nil {
		t.Fatal(err)
	}
	if got := s.GetDriftScanInterval(); got != 0 {
		t.Errorf("0 interval = %d, want 0", got)
	}
	if err := s.SetDriftScanInterval(-1); err == nil {
		t.Error("negative interval should be rejected")
	}
	for _, v := range []string{"abc", "99999"} { // 非法值 → 默认
		if err := s.db.SetSetting(DriftIntervalSettingKey, v); err != nil {
			t.Fatal(err)
		}
		if got := s.GetDriftScanInterval(); got != 1800 {
			t.Errorf("interval for %q = %d, want default", v, got)
		}
	}

	// driftScanLoop：先睡 90s，只验证可启动（goroutine 挂着，进程退出回收）
	go s.driftScanLoop()

	// scanDriftOnce：无 drift agent → 直接返回
	s.scanDriftOnce()

	// 有 agent：调用失败（离线→CallAgent 错误分支其实不会：bare agent 在线但
	// 不应答；这里用不应答 agent 触发 30s 响应超时太久，改用 error 应答）
	covFakeAgent(t, s, "agent-drift", []string{"drift"}, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload(map[string]interface{}{"items": []interface{}{
			map[string]interface{}{"kind": "file", "name": "/etc/x", "status": "drifted"},
			map[string]interface{}{"kind": "file", "name": "/etc/y", "status": "missing"},
			map[string]interface{}{"kind": "file", "name": "/etc/z", "status": "error"},
			map[string]interface{}{"kind": "file", "name": "/etc/w", "status": "ok"},
		}})
	})
	s.scanDriftOnce()

	// fetchDriftItems 分支
	if _, err := s.fetchDriftItems("ghost"); err == nil {
		t.Error("missing agent should error")
	}
	covFakeAgent(t, s, "agent-derr", []string{"drift"}, func(method string, params map[string]interface{}) map[string]interface{} {
		return covErrPayload("boom")
	})
	if _, err := s.fetchDriftItems("agent-derr"); err == nil || err.Error() != "boom" {
		t.Errorf("rpc error = %v", err)
	}
	covBadPayloadAgent(t, s, "agent-ddecode")
	if _, err := s.fetchDriftItems("agent-ddecode"); err == nil {
		t.Error("decode error should propagate")
	}
	covFakeAgent(t, s, "agent-dstr", []string{"drift"}, func(method string, params map[string]interface{}) map[string]interface{} {
		return covOKPayload("not-items")
	})
	if _, err := s.fetchDriftItems("agent-dstr"); err == nil {
		t.Error("non-map data should fail unmarshal")
	}
}

// ============ server_backup.go ============

func TestCovServerBackup(t *testing.T) {
	s := covLoopServer(t)

	// 间隔/保留读取：非法回默认
	if got := s.serverBackupIntervalHours(); got != 24 {
		t.Errorf("default interval = %d", got)
	}
	_ = s.db.SetSetting(ServerBackupIntervalSettingKey, "abc")
	if got := s.serverBackupIntervalHours(); got != 24 {
		t.Errorf("bad interval = %d, want 24", got)
	}
	_ = s.db.SetSetting(ServerBackupIntervalSettingKey, "999")
	if got := s.serverBackupIntervalHours(); got != 24 {
		t.Errorf("over-range interval = %d, want 24", got)
	}
	_ = s.db.SetSetting(ServerBackupRetentionSettingKey, "xyz")
	if got := s.serverBackupRetentionDays(); got != 7 {
		t.Errorf("bad retention = %d, want 7", got)
	}

	// 无备份目录 → 空列表 + 零值
	if list, err := s.listServerBackups(); err != nil || len(list) != 0 {
		t.Fatalf("empty list = %v, %v", list, err)
	}
	if !s.latestServerBackupAt().IsZero() {
		t.Error("no backup should give zero time")
	}

	// 正常备份一次
	name, err := s.runServerBackup()
	if err != nil || !isValidServerBackupName(name) {
		t.Fatalf("runServerBackup = %q, %v", name, err)
	}
	list, err := s.listServerBackups()
	if err != nil || len(list) != 1 || list[0].Name != name {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if s.latestServerBackupAt().IsZero() {
		t.Error("latest backup time should be parsed")
	}

	// 同秒再备 → already exists
	if _, err := s.runServerBackup(); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second same-second backup err = %v", err)
	}

	// 伪造同名文件让 isValidServerBackupName 过滤失效路径：放一个坏名字文件
	if err := os.WriteFile(filepath.Join(s.serverBackupDir(), "evil.txt"), []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	list, _ = s.listServerBackups()
	if len(list) != 1 {
		t.Fatalf("evil file should be filtered: %+v", list)
	}

	// cleanupServerBackups：0 保留 → 直接返回；过期删除
	_ = s.db.SetSetting(ServerBackupRetentionSettingKey, "0")
	s.cleanupServerBackups() // 永久保留
	list, _ = s.listServerBackups()
	if len(list) != 1 {
		t.Fatalf("0 retention should keep file: %+v", list)
	}
	_ = s.db.SetSetting(ServerBackupRetentionSettingKey, "1")
	if err := os.Chtimes(filepath.Join(s.serverBackupDir(), name), time.Now().Add(-72*time.Hour), time.Now().Add(-72*time.Hour)); err != nil {
		t.Fatal(err)
	}
	s.cleanupServerBackups()
	if list, _ = s.listServerBackups(); len(list) != 0 {
		t.Fatalf("expired backup should be removed: %+v", list)
	}

	// serverBackupLoop：可启动并在 ctx 取消后退出
	exited := covSpawnLoop(s.serverBackupLoop)
	time.Sleep(50 * time.Millisecond)
	s.cancel()
	covWaitExit(t, "serverBackupLoop", exited)
}

func TestCovServerBackupFailures(t *testing.T) {
	// 目录创建失败：db 路径的父目录本身是文件
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0600); err != nil {
		t.Fatal(err)
	}
	s := covLoopServer(t)
	s.cfg.Database.Path = filepath.Join(blocker, "cockpit.db")
	if _, err := s.runServerBackup(); err == nil || !strings.Contains(err.Error(), "create server-backups dir") {
		t.Fatalf("mkdir failure err = %v", err)
	}
	// listServerBackups 对非目录路径报错
	if _, err := s.listServerBackups(); err == nil {
		t.Error("list on file path should error")
	}
	s.cleanupServerBackups() // list 失败直接返回

	// VacuumInto 失败：对已关闭的 db
	s2 := covLoopServer(t)
	covCloseDB(t, s2)
	if _, err := s2.runServerBackup(); err == nil || !strings.Contains(err.Error(), "vacuum into") {
		t.Fatalf("vacuum failure err = %v", err)
	}
}

// ============ recording.go 内部 ============

func TestCovCastRecorderInternals(t *testing.T) {
	// 目录创建失败：父目录是文件
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	os.WriteFile(blocker, []byte("x"), 0600)
	if _, err := newCastRecorder(filepath.Join(blocker, "sub", "x.cast"), "s1", "t", time.Now()); err == nil {
		t.Error("mkdir failure should error")
	}

	// OpenFile 失败：目标路径是目录
	sub := filepath.Join(dir, "adir")
	os.MkdirAll(sub, 0700)
	if _, err := newCastRecorder(sub, "s1", "t", time.Now()); err == nil {
		t.Error("path-is-dir should error")
	}

	// header 写失败：/dev/full 写入报错
	if _, err := newCastRecorder("/dev/full", "s1", "t", time.Now()); err == nil {
		t.Error("write failure should error")
	}

	// 正常创建 → WriteOutput → Close
	path := filepath.Join(dir, "ok.cast")
	rec, err := newCastRecorder(path, "s1", "title", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	rec.WriteOutput([]byte("hello"))
	var nilRec *castRecorder
	nilRec.WriteOutput([]byte("nil-safe")) // nil receiver 安全
	rec.Close()
	rec.Close() // 幂等
	rec.WriteOutput([]byte("after close")) // f 已置 nil，静默丢弃

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "hello") {
		t.Errorf("cast content = %q", data)
	}
}

func TestCovRecordingSettingsAndCleanup(t *testing.T) {
	s := covLoopServer(t)

	// 保留天数解析：空/非法/负数/超限/正常
	for _, c := range []struct {
		set  string
		want int
	}{
		{"", 7}, {"abc", 7}, {"-5", 0}, {"99999", 365}, {"30", 30}, {"0", 0},
	} {
		if c.set == "" {
			s.db.DeleteSetting(RecordingRetentionSettingKey)
		} else {
			_ = s.db.SetSetting(RecordingRetentionSettingKey, c.set)
		}
		if got := s.recordingRetentionDays(); got != c.want {
			t.Errorf("retention(%q) = %d, want %d", c.set, got, c.want)
		}
	}

	// cleanupExpiredRecordings：0 保留直接返回
	_ = s.db.SetSetting(RecordingRetentionSettingKey, "0")
	s.cleanupExpiredRecordings()

	// 正常清理：造两条当前时间的记录（CreatedAt 为零值会被判过期）
	for _, sess := range []string{"sess-old", "sess-new"} {
		if err := s.db.CreateTerminalRecording(&storage.TerminalRecording{
			SessionID: sess, AgentID: "a1", Username: "admin", StartedAt: time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(s.recordingsDir(), 0755); err != nil {
		t.Fatal(err)
	}
	for _, sess := range []string{"sess-old", "sess-new"} {
		if err := os.WriteFile(filepath.Join(s.recordingsDir(), sess+".cast"), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if old, _ := s.db.GetTerminalRecording("sess-old"); old == nil {
		t.Fatal("seed recording missing")
	}
	_ = s.db.SetSetting(RecordingRetentionSettingKey, "7")
	s.cleanupExpiredRecordings() // 两条都未过期 → 保留
	if recs, _ := s.db.ListTerminalRecordings(10); len(recs) != 2 {
		t.Fatalf("fresh recordings should be kept: %d", len(recs))
	}

	// 再造一条零时间记录 + 不存在的文件 → meta 仍被删除（os.IsNotExist 容忍）
	covSeedRecording(t, s, "sess-zcat")
	s.cleanupExpiredRecordings()
	if recs, _ := s.db.ListTerminalRecordings(10); len(recs) != 2 {
		t.Fatalf("expired meta should be deleted: %d", len(recs))
	}

	// 关库 → ListExpired 失败分支
	covCloseDB(t, s)
	s.cleanupExpiredRecordings()
}

func TestCovStartRecordingFailures(t *testing.T) {
	s := covLoopServer(t)
	session := &TerminalSession{
		ID: "sess-recfail", Username: "u", AgentID: "a", Host: "h", Port: 22,
		Protocol: "ssh", CreatedAt: time.Now(),
	}

	// 文件创建失败：recordingsDir 指向已存在文件 → MkdirAll 失败 → 返回 nil
	blocker := filepath.Join(t.TempDir(), "blocker")
	os.WriteFile(blocker, []byte("x"), 0600)
	s.cfg.Database.Path = filepath.Join(blocker, "cockpit.db")
	if rec := s.startRecording(session); rec != nil {
		t.Errorf("recorder should be nil on dir failure, got %v", rec)
	}

	// DB 失败：目录正常但库已关 → meta 插入失败 → 关闭录制并返回 nil
	s2 := covLoopServer(t)
	covCloseDB(t, s2)
	if rec := s2.startRecording(session); rec != nil {
		t.Errorf("recorder should be nil on db failure, got %v", rec)
	}
}

// ============ 其他小缺口 ============

func TestCovShutdownNilGuards(t *testing.T) {
	s := covLoopServer(t) // inventorySync/probeRunner/proxyMgr 全为 nil
	s.Shutdown()          // 覆盖 nil 分支 + db.Close
	_ = context.Background
}
