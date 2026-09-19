package rpc

// 多用户 crontab 测试（cron-design.md M4 D21-D22）：user 校验矩阵、
// -u argv 拼装、getent 解析与 /etc/passwd fallback、指定用户全链路。

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestValidateCronUserMatrix(t *testing.T) {
	valid := []string{"", "root", "www-data", "_postgres", "a-b_c", strings.Repeat("a", 32)}
	for _, u := range valid {
		if err := validateCronUser(u); err != nil {
			t.Errorf("%q should pass: %v", u, err)
		}
	}
	invalid := []string{
		"Root",                  // 大写
		"1abc",                  // 数字开头
		"-abc",                  // 连字符开头
		"a b",                   // 空格
		"a;b",                   // 注入形态
		"../etc",                // 路径形态
		"123",                   // 纯数字（uid 形态不收，D21）
		strings.Repeat("a", 33), // 超长
	}
	for _, u := range invalid {
		if err := validateCronUser(u); err == nil {
			t.Errorf("%q should fail", u)
		}
	}
}

func TestCronUserFromParams(t *testing.T) {
	if u, err := cronUserFromParams(nil); err != nil || u != "" {
		t.Errorf("nil params = %q %v", u, err)
	}
	if u, err := cronUserFromParams(map[string]interface{}{"user": " postgres "}); err != nil || u != "postgres" {
		t.Errorf("trim = %q %v", u, err)
	}
	if _, err := cronUserFromParams(map[string]interface{}{"user": "Bad User"}); err == nil {
		t.Error("invalid user should fail")
	}
}

// recordingCronRunner 记录全部 argv 并模拟 crontab 读/写语义（含 -u 形态）
type recordingCronRunner struct {
	mu          sync.Mutex
	calls       [][]string
	listOut     string
	listErr     error
	fileContent string
	wrote       bool
}

func (r *recordingCronRunner) run(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, append([]string{name}, args...))
	if name != "crontab" {
		return nil, nil, nil
	}
	switch {
	case len(args) >= 1 && args[len(args)-1] == "-l":
		return []byte(r.listOut), nil, r.listErr
	case len(args) >= 1 && !strings.HasPrefix(args[len(args)-1], "-"):
		// crontab [-u user] <file>：读回临时文件内容存档
		b, err := os.ReadFile(args[len(args)-1])
		if err != nil {
			return nil, []byte(err.Error()), err
		}
		r.fileContent = string(b)
		r.listOut = r.fileContent // 真实语义：写回后 -l 读到新内容
		r.wrote = true
		return nil, nil, nil
	}
	return nil, nil, nil
}

func TestCronUserArgvWiring(t *testing.T) {
	r := &recordingCronRunner{listOut: "0 3 * * * keep\n"}
	p := NewCronProvider(r.run)

	if _, err := p.readCrontab("postgres"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.calls[0], " "); got != "crontab -u postgres -l" {
		t.Errorf("read -u argv = %q", got)
	}
	if _, err := p.readCrontab(""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.calls[1], " "); got != "crontab -l" {
		t.Errorf("read default argv = %q", got)
	}
	if err := p.writeViaFile("x\n", "www"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.calls[2], " "); !strings.HasPrefix(got, "crontab -u www ") {
		t.Errorf("write -u argv = %q", got)
	}
	if err := p.writeViaFile("y\n", ""); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.calls[3], " "); !strings.HasPrefix(got, "crontab ") || strings.Contains(got, " -u ") {
		t.Errorf("write default argv = %q", got)
	}
}

func TestCronUserCallLayer(t *testing.T) {
	meta := renderJobPair(&CronJob{Name: "pg-backup", Schedule: "0 3 * * *", Command: "/opt/pg.sh", Enabled: true})
	r := &recordingCronRunner{listOut: strings.Join(meta, "\n") + "\n0 0 * * * external.sh\n"}
	p := NewCronProvider(r.run)

	// jobs 带指定用户
	res, err := p.Call("jobs", map[string]interface{}{"user": "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	jobs := res.(map[string]interface{})["jobs"].([]map[string]interface{})
	if len(jobs) != 1 || jobs[0]["name"] != "pg-backup" {
		t.Fatalf("jobs = %v", jobs)
	}
	if got := strings.Join(r.calls[0], " "); got != "crontab -u postgres -l" {
		t.Errorf("call jobs argv = %q", got)
	}

	// 非法 user 在 Call 入口即拒（不触达 crontab）
	if _, err := p.Call("jobs", map[string]interface{}{"user": "NO PE"}); err == nil ||
		!strings.Contains(err.Error(), "invalid user name") {
		t.Errorf("bad user err = %v", err)
	}
	if len(r.calls) != 1 {
		t.Errorf("bad user should not exec, calls = %v", r.calls)
	}

	// status 指定用户：user 字段即目标用户（不 whoami）
	st, err := p.Call("status", map[string]interface{}{"user": "www-data"})
	if err != nil {
		t.Fatal(err)
	}
	if st.(map[string]interface{})["user"] != "www-data" {
		t.Errorf("status user = %v", st)
	}
}

func TestCronUserApplyDeleteFlow(t *testing.T) {
	r := &recordingCronRunner{listOut: "0 0 * * * external.sh\n"}
	p := NewCronProvider(r.run)

	params := jobParams(t, &CronJob{Name: "clean", Schedule: "@daily", Command: "/usr/bin/clean.sh", Enabled: true})
	params["user"] = "postgres"
	if _, err := p.Call("job.apply", params); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(r.calls[0], " "); !strings.HasPrefix(got, "crontab -u postgres ") {
		t.Errorf("apply argv = %q", got)
	}
	if !strings.Contains(r.fileContent, "cockpit:job") || !strings.Contains(r.fileContent, "/usr/bin/clean.sh") {
		t.Errorf("applied content = %q", r.fileContent)
	}
	// 外部条目保持（D25：自检按用户视角成立）
	if !strings.Contains(r.fileContent, "external.sh") {
		t.Errorf("external entries lost: %q", r.fileContent)
	}

	del := map[string]interface{}{"name": "clean", "user": "postgres"}
	if _, err := p.Call("job.delete", del); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(r.fileContent, "cockpit:job") || !strings.Contains(r.fileContent, "external.sh") {
		t.Errorf("after delete content = %q", r.fileContent)
	}
}

func TestCronUsersViaGetent(t *testing.T) {
	orig := cronLookPath
	cronLookPath = func(string) (string, error) { return "/usr/bin/getent", nil }
	t.Cleanup(func() { cronLookPath = orig })

	out := "root:x:0:0:root:/root:/bin/bash\nwww-data:x:33:33:www:/var/www:/usr/sbin/nologin\nbad:x:abc:x\n"
	r := &recordingCronRunner{}
	// getent 走 run 但 name != crontab，直接回空——需要能注入输出；换用自定义 commander
	p := NewCronProvider(func(_ context.Context, name string, args ...string) ([]byte, []byte, error) {
		if name == "getent" {
			return []byte(out), nil, nil
		}
		return r.run(context.Background(), name, args...)
	})
	res, err := p.Call("users", nil)
	if err != nil {
		t.Fatal(err)
	}
	users := res.(map[string]interface{})["users"].([]CronUserEntry)
	if len(users) != 2 {
		t.Fatalf("users = %+v", users)
	}
	if users[0].Name != "root" || users[0].UID != 0 || users[0].Shell != "/bin/bash" {
		t.Errorf("root = %+v", users[0])
	}
	if users[1].Name != "www-data" || users[1].Shell != "/usr/sbin/nologin" {
		t.Errorf("www-data = %+v", users[1])
	}
}

func TestCronUsersFallbackEtcPasswd(t *testing.T) {
	orig := cronLookPath
	cronLookPath = func(string) (string, error) { return "", errors.New("no getent") }
	t.Cleanup(func() { cronLookPath = orig })

	users, err := listCronUsers(func(_ context.Context, _ string, _ ...string) ([]byte, []byte, error) {
		return nil, nil, errors.New("should not exec")
	})
	if err != nil {
		t.Fatal(err)
	}
	// CI/Linux 机器 /etc/passwd 必有 root
	found := false
	for _, u := range users {
		if u.Name == "root" && u.UID == 0 {
			found = true
		}
	}
	if !found {
		t.Errorf("root not found in fallback: %d users", len(users))
	}
}

func TestParsePasswdLines(t *testing.T) {
	users := parsePasswdLines("root:x:0:0:root:/root:/bin/sh\nshort:x:5\nbad:x:xx\n\n:x:9:9::/home:\n")
	if len(users) != 2 {
		t.Fatalf("users = %+v", users)
	}
	if users[0].Shell != "/bin/sh" {
		t.Errorf("shell = %q", users[0].Shell)
	}
	if users[1].Shell != "" {
		t.Errorf("short line shell should be empty, got %q", users[1].Shell)
	}
}
