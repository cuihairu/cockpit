package rpc

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

// mockCronRunner 模拟 crontab 命令：-l 读内存内容，<file> 写回时读文件存档。
// listErr 注入读失败（"no crontab for" 视为空表由 provider 处理）；
// writeErr 注入写失败。writes 统计写回次数。
type mockCronRunner struct {
	mu       sync.Mutex
	content  string // 最近一次写回的内容（真实路径：临时文件 → 读回）
	hasFile  bool   // false 时 -l 报 no crontab for
	listErr  error
	writeErr error
	writes   int
}

func (m *mockCronRunner) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if name != "crontab" {
		return nil, nil, nil
	}
	switch {
	case len(args) == 1 && args[0] == "-l":
		if !m.hasFile {
			return nil, []byte("no crontab for root"), errors.New("no crontab for root")
		}
		if m.listErr != nil {
			return nil, []byte(m.listErr.Error()), m.listErr
		}
		return []byte(m.content), nil, nil
	case len(args) == 1 && !strings.HasPrefix(args[0], "-"):
		// crontab <file>：读回临时文件内容存档（写回语义的落点）
		if m.writeErr != nil {
			return nil, []byte(m.writeErr.Error()), m.writeErr
		}
		b, err := os.ReadFile(args[0])
		if err != nil {
			return nil, []byte(err.Error()), err
		}
		m.writes++
		m.hasFile = true
		m.content = string(b)
		return nil, nil, nil
	}
	return nil, nil, nil
}

// jobParams 构造真实 JSON RPC 反序列化后的 params 形状（job 为 map 而非结构体）
func jobParams(t *testing.T, job *CronJob) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{"job": job})
	if err != nil {
		t.Fatal(err)
	}
	var params map[string]interface{}
	if err := json.Unmarshal(b, &params); err != nil {
		t.Fatal(err)
	}
	return params
}

func TestValidateCronExpr(t *testing.T) {
	valid := []string{
		"0 3 * * *", "*/5 * * * *", "30 4 1,15 * 0", "0 0-6 * * *",
		"@reboot", "@daily", "@yearly", "59 23 31 12 7",
	}
	for _, e := range valid {
		if err := validateCronExpr(e); err != nil {
			t.Errorf("%q should pass: %v", e, err)
		}
	}
	invalid := []string{
		"", "* * * *", // 字段数
		"* * * * * *",   // 字段数
		"60 * * * *",    // 分钟越界
		"* 24 * * *",    // 小时越界
		"* * 0 * *",     // 日从 1 起
		"* * * 13 *",    // 月越界
		"* * * * 8",     // 周越界
		"@everyhour",    // 非法 @
		"a * * * *",     // 非数字
		"1-100 * * * *", // 范围越界
		"*/0 * * * *",   // 步长 0
	}
	for _, e := range invalid {
		if err := validateCronExpr(e); err == nil {
			t.Errorf("%q should be rejected", e)
		}
	}
}

func TestCronJobValidate(t *testing.T) {
	base := &CronJob{Name: "bk", Schedule: "0 3 * * *", Command: "/opt/bk.sh", Enabled: true}
	if err := base.validate(); err != nil {
		t.Fatalf("base should pass: %v", err)
	}
	for _, c := range []struct {
		desc string
		mut  func(*CronJob)
	}{
		{"bad name", func(j *CronJob) { j.Name = "Bad Name" }},
		{"bad expr", func(j *CronJob) { j.Schedule = "61 * * * *" }},
		{"empty command", func(j *CronJob) { j.Command = "" }},
		{"multiline command", func(j *CronJob) { j.Command = "a\nb" }},
	} {
		j := *base
		c.mut(&j)
		if err := j.validate(); err == nil {
			t.Errorf("%s: should be rejected", c.desc)
		}
	}
}

func TestCronMetaRoundtrip(t *testing.T) {
	job := CronJob{Name: "rt", Schedule: "@daily", Command: "echo 'hi' > /tmp/x", Enabled: false}
	pair := renderJobPair(&job)
	// 任务对回读：拼进空 crontab 再解析
	_, jobs, _ := splitCockpit(strings.Join(pair, "\n") + "\n")
	if len(jobs) != 1 || jobs[0] != job {
		t.Fatalf("roundtrip mismatch: %+v vs %+v", jobs, job)
	}
	// 停用任务的命令行整体被注释
	if !strings.HasPrefix(pair[1], "#@daily") {
		t.Fatalf("disabled command should be commented: %q", pair[1])
	}
}

func TestCronUpsertPreservesExternal(t *testing.T) {
	old := "# user comment\nMAILTO=admin@example.com\n0 * * * * /usr/bin/legacy.sh\n"
	job := &CronJob{Name: "new", Schedule: "*/10 * * * *", Command: "/opt/x.sh", Enabled: true}
	out := upsertJob(old, job)

	// 外部行逐行保留
	for _, want := range []string{"# user comment", "MAILTO=admin@example.com", "0 * * * * /usr/bin/legacy.sh"} {
		if !strings.Contains(out, want) {
			t.Errorf("external line lost: %q\nout:\n%s", want, out)
		}
	}
	// 新任务对在尾部
	if !strings.Contains(out, cronMetaPrefix) || !strings.Contains(out, "*/10 * * * * /opt/x.sh") {
		t.Errorf("job pair missing:\n%s", out)
	}

	// 更新同名任务：旧对被替换，不产生重复
	updated := &CronJob{Name: "new", Schedule: "@daily", Command: "/opt/y.sh", Enabled: true}
	out2 := upsertJob(out, updated)
	if strings.Count(out2, cronMetaPrefix) != 1 {
		t.Fatalf("expected exactly one cockpit pair:\n%s", out2)
	}
	if !strings.Contains(out2, "@daily /opt/y.sh") {
		t.Errorf("update not applied:\n%s", out2)
	}
}

func TestCronApplyViaMockCommand(t *testing.T) {
	runner := &mockCronRunner{hasFile: true, content: "0 * * * * /usr/bin/legacy.sh\n"}
	p := NewCronProvider(runner.run)

	job := &CronJob{Name: "bk", Schedule: "0 3 * * *", Command: "/opt/bk.sh", Enabled: true}
	if _, err := p.Call("job.apply", jobParams(t, job)); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(runner.content, "0 * * * * /usr/bin/legacy.sh") {
		t.Errorf("external line lost:\n%s", runner.content)
	}
	if !strings.Contains(runner.content, "0 3 * * * /opt/bk.sh") {
		t.Errorf("job not written:\n%s", runner.content)
	}
	// 列表可见
	list, err := p.Call("jobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	jobs := list.(map[string]interface{})["jobs"].([]map[string]interface{})
	if len(jobs) != 1 || jobs[0]["name"] != "bk" {
		t.Fatalf("jobs = %v", jobs)
	}
}

func TestCronDelete(t *testing.T) {
	job := &CronJob{Name: "gone", Schedule: "@hourly", Command: "x", Enabled: true}
	content := strings.Join(renderJobPair(job), "\n") + "\n0 0 * * * keepme.sh\n"
	runner := &mockCronRunner{hasFile: true, content: content}
	p := NewCronProvider(runner.run)

	if _, err := p.Call("job.delete", map[string]interface{}{"name": "gone"}); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if strings.Contains(runner.content, cronMetaPrefix) || strings.Contains(runner.content, "@hourly x") {
		t.Errorf("job pair not removed:\n%s", runner.content)
	}
	if !strings.Contains(runner.content, "keepme.sh") {
		t.Errorf("external line lost:\n%s", runner.content)
	}

	// 删除不存在的任务报错
	if _, err := p.Call("job.delete", map[string]interface{}{"name": "gone"}); err == nil {
		t.Error("deleting missing job should fail")
	}
}

func TestCronJobsListAndStatus(t *testing.T) {
	j1 := &CronJob{Name: "a", Schedule: "0 3 * * *", Command: "/a.sh", Enabled: true}
	j2 := &CronJob{Name: "b", Schedule: "@reboot", Command: "/b.sh", Enabled: false}
	content := "# head\n" + strings.Join(renderJobPair(j1), "\n") + "\n" + strings.Join(renderJobPair(j2), "\n") + "\n"
	runner := &mockCronRunner{hasFile: true, content: content}
	p := NewCronProvider(runner.run)

	list, err := p.Call("jobs", nil)
	if err != nil {
		t.Fatal(err)
	}
	m := list.(map[string]interface{})
	jobs := m["jobs"].([]map[string]interface{})
	if len(jobs) != 2 || jobs[0]["name"] != "a" || jobs[1]["enabled"] != false {
		t.Fatalf("jobs = %v", jobs)
	}
	ext := m["external"].(string)
	if !strings.Contains(ext, "# head") {
		t.Errorf("external = %q", ext)
	}

	st, err := p.Call("status", nil)
	if err != nil {
		t.Fatal(err)
	}
	sm := st.(map[string]interface{})
	if sm["cockpitCount"] != 2 || sm["externalCount"] != 1 {
		t.Fatalf("status = %v", sm)
	}
}

func TestCronApplyValidatesBeforeCommand(t *testing.T) {
	runner := &mockCronRunner{}
	p := NewCronProvider(runner.run)

	bad := &CronJob{Name: "x", Schedule: "61 * * * *", Command: "y", Enabled: true}
	if _, err := p.Call("job.apply", jobParams(t, bad)); err == nil {
		t.Fatal("invalid expression should be rejected")
	}
	if runner.writes != 0 {
		t.Fatal("should not write crontab on validation failure")
	}
}

func TestCronWriteFailureSurfacesError(t *testing.T) {
	runner := &mockCronRunner{hasFile: true, content: "", writeErr: errors.New("permission denied")}
	p := NewCronProvider(runner.run)
	job := &CronJob{Name: "w", Schedule: "@daily", Command: "x", Enabled: true}
	if _, err := p.Call("job.apply", jobParams(t, job)); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v, want write failure surfaced", err)
	}
}

func TestCronEmptyCrontabFirstUse(t *testing.T) {
	runner := &mockCronRunner{hasFile: false} // -l 报 no crontab for
	p := NewCronProvider(runner.run)

	job := &CronJob{Name: "first", Schedule: "@daily", Command: "x", Enabled: true}
	if _, err := p.Call("job.apply", jobParams(t, job)); err != nil {
		t.Fatalf("first apply on empty crontab: %v", err)
	}
	if !strings.Contains(runner.content, "first") {
		t.Errorf("job not written:\n%s", runner.content)
	}
}

func TestCronReadFailureSurfacesError(t *testing.T) {
	runner := &mockCronRunner{hasFile: true, listErr: errors.New("permission denied")}
	p := NewCronProvider(runner.run)
	if _, err := p.Call("jobs", nil); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("err = %v, want read failure surfaced", err)
	}
}
