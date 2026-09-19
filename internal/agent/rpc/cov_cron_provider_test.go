package rpc

// 覆盖率补充测试：cron_provider.go 错误分支与辅助函数。

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCovCronNewDefaultsAndType(t *testing.T) {
	p := NewCronProvider(nil)
	if p == nil || p.Type() != "cron" {
		t.Fatalf("provider = %v type = %q", p, p.Type())
	}
	if p.run == nil {
		t.Error("nil runner should fall back to defaultCommander")
	}
}

func TestCovCronCommandTooLarge(t *testing.T) {
	job := &CronJob{Name: "big", Schedule: "@daily", Command: strings.Repeat("x", cronMaxCommand+1)}
	if err := job.validate(); err == nil || !strings.Contains(err.Error(), "command too large") {
		t.Errorf("oversized command err = %v", err)
	}
}

func TestCovCronCallParamErrors(t *testing.T) {
	p := NewCronProvider((&mockCronRunner{}).run)
	// job.apply 无 job 对象
	if _, err := p.Call("job.apply", map[string]interface{}{"name": "x"}); err == nil ||
		!strings.Contains(err.Error(), "job object required") {
		t.Errorf("missing job object err = %v", err)
	}
	// job 对象类型错误
	if _, err := p.Call("job.apply", map[string]interface{}{"job": "not-a-map"}); err == nil {
		t.Error("non-map job should fail")
	}
	// 坏 payload：name 为数字无法反序列化
	if _, err := p.Call("job.apply", map[string]interface{}{
		"job": map[string]interface{}{"name": 1, "schedule": "@daily", "command": "true"},
	}); err == nil || !strings.Contains(err.Error(), "bad job payload") {
		t.Errorf("bad payload err = %v", err)
	}
	// unknown action
	if _, err := p.Call("bogus", nil); err == nil || !strings.Contains(err.Error(), "unknown cron action") {
		t.Errorf("unknown action err = %v", err)
	}
}

func TestCovCronDetect(t *testing.T) {
	// PATH 含假 crontab → true
	bin := t.TempDir()
	fake := filepath.Join(bin, "crontab")
	if err := os.WriteFile(fake, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if !DetectCron() {
		t.Error("DetectCron should be true with crontab in PATH")
	}
	// PATH 为空目录 → false
	t.Setenv("PATH", t.TempDir())
	if DetectCron() {
		t.Error("DetectCron should be false without crontab in PATH")
	}
}

func TestCovCronReadErrorsSurface(t *testing.T) {
	listErr := errors.New("crontab exploded")
	p := NewCronProvider((&mockCronRunner{hasFile: true, listErr: listErr}).run)
	if _, err := p.Call("status", nil); err == nil || !strings.Contains(err.Error(), "crontab -l") {
		t.Errorf("status list err = %v", err)
	}
	if _, err := p.Call("jobs", nil); err == nil {
		t.Error("jobs list err should fail")
	}
	if _, err := p.Call("job.apply", jobParams(t, &CronJob{Name: "j", Schedule: "@daily", Command: "true", Enabled: true})); err == nil {
		t.Error("apply list err should fail")
	}
	// DeleteJob：非法名 / 读失败
	if _, err := p.Call("job.delete", map[string]interface{}{"name": "../bad"}); err == nil ||
		!strings.Contains(err.Error(), "invalid job name") {
		t.Errorf("delete invalid name err = %v", err)
	}
	if _, err := p.Call("job.delete", map[string]interface{}{"name": "j"}); err == nil {
		t.Error("delete list err should fail")
	}
}

func TestCovCronDeleteNotFoundWriteErrAndBaseline(t *testing.T) {
	// not found
	p := NewCronProvider((&mockCronRunner{hasFile: true, content: "# external line\n"}).run)
	if _, err := p.Call("job.delete", map[string]interface{}{"name": "ghost"}); err == nil ||
		!strings.Contains(err.Error(), "job not found") {
		t.Errorf("delete missing err = %v", err)
	}

	// 写回失败
	m := &mockCronRunner{hasFile: true, writeErr: errors.New("disk full"),
		content: cronMetaPrefix + `{"name":"j","schedule":"@daily","command":"true","enabled":true}` + "\n@daily true\n"}
	p2 := NewCronProvider(m.run)
	if _, err := p2.Call("job.delete", map[string]interface{}{"name": "j"}); err == nil ||
		!strings.Contains(err.Error(), "crontab write") {
		t.Errorf("delete write err = %v", err)
	}

	// 成功删除 + 基线记录
	rec := newFakeRecorder()
	m2 := &mockCronRunner{hasFile: true,
		content: cronMetaPrefix + `{"name":"j","schedule":"@daily","command":"true","enabled":true}` + "\n@daily true\nkeep me\n"}
	p3 := NewCronProvider(m2.run)
	p3.SetBaseline(rec)
	if _, err := p3.Call("job.delete", map[string]interface{}{"name": "j"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(rec.records) == 0 {
		t.Error("baseline should record after delete")
	}
	if !strings.Contains(m2.content, "keep me") {
		t.Errorf("external line must survive: %q", m2.content)
	}
}

func TestCovCronWriteSafetyCheck(t *testing.T) {
	p := NewCronProvider((&mockCronRunner{}).run)
	// 直调：old 与 new 的非 cockpit 段不一致 → 拒绝写回
	if err := p.writeCrontab("line-a\n", "line-b\n", ""); err == nil ||
		!strings.Contains(err.Error(), "safety check failed") {
		t.Errorf("safety check err = %v", err)
	}
}

func TestCovCronWriteViaFileCreateTempFails(t *testing.T) {
	// TMPDIR 指向不存在的目录 → CreateTemp 必败（与 uid 无关）
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	p := NewCronProvider((&mockCronRunner{}).run)
	if err := p.writeViaFile("x\n", ""); err == nil || !strings.Contains(err.Error(), "create temp file") {
		t.Errorf("create temp err = %v", err)
	}
}

func TestCovCronSplitCockpitCorruptedMeta(t *testing.T) {
	content := "# keep external\n\n" + cronMetaPrefix + "{bad json\n@daily orphan-cmd\n"
	kept, jobs, external := splitCockpit(content)
	if len(jobs) != 0 {
		t.Errorf("corrupted meta must yield no jobs, got %v", jobs)
	}
	// 损坏 meta 行与其后命令行都按普通行保留
	if !strings.Contains(kept, "{bad json") || !strings.Contains(kept, "orphan-cmd") {
		t.Errorf("kept = %q", kept)
	}
	// 空行不计入 external，损坏对计入
	if len(external) != 3 {
		t.Errorf("external = %q", external)
	}
}
