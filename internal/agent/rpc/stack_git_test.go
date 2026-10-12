package rpc

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ============ Stack Git 化测试 ============
//
// 全部用真实 git + t.TempDir() 下的本地 bare 仓当远端（不依赖外网凭据）。
// 覆盖：首次接入落地文件、二次 pull 快进、drift 识别、.cockpit 过滤、
// 本地改动冲突、远端校验、分支校验、未接入状态、git 缺失。

// gitTestProvider 建 stacks 目录 provider，git 指向系统 git
func gitTestProvider(t *testing.T) *StackProvider {
	t.Helper()
	return NewStackProvider(StackConfig{Dir: t.TempDir(), GitBin: []string{"git"}})
}

// gitInitRemote 建本地 bare 远端并推入一版初始内容（branch 分支）
func gitInitRemote(t *testing.T, branch string, files map[string]string) string {
	t.Helper()
	work := t.TempDir()
	remote := filepath.Join(t.TempDir(), "stacks-remote.git")
	run := func(dir string, args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return string(out)
	}
	run("", "init", "--bare", "-b", branch, remote)
	run("", "init", "-b", branch, work)
	for name, content := range files {
		p := filepath.Join(work, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run(work, "add", ".")
	run(work, "commit", "-m", "init")
	run(work, "remote", "add", "origin", remote)
	run(work, "push", "-u", "origin", branch)
	return remote
}

func gitWrite(t *testing.T, dir, name, content string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestStackGitAttachAndPull(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{
		"web/compose.yml": "services:\n  web:\n    image: nginx\n",
	})
	p := gitTestProvider(t)

	// 首次 pull = 接入：落地远端文件
	res, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"})
	if err != nil {
		t.Fatalf("attach pull: %v", err)
	}
	if res["enabled"] != true || res["branch"] != "main" {
		t.Fatalf("attach result = %v", res)
	}
	if res["ahead"].(int) != 0 || res["behind"].(int) != 0 {
		t.Fatalf("attach ahead/behind = %v/%v", res["ahead"], res["behind"])
	}
	data, err := os.ReadFile(filepath.Join(p.dir, "web", "compose.yml"))
	if err != nil || !strings.Contains(string(data), "nginx") {
		t.Fatalf("remote file not materialized: %v", err)
	}
	if res["head"] == "" || res["head"] != res["remoteHead"] {
		t.Fatalf("head/remoteHead mismatch: %v vs %v", res["head"], res["remoteHead"])
	}

	// 无本地改动 → drift 为空
	st := p.GitStatus()
	if len(st.Changes) != 0 {
		t.Fatalf("clean tree changes = %v", st.Changes)
	}
	if st.Remote == "" {
		t.Fatal("remote not reported")
	}

	// 远端新提交：status 不联网 fetch（设计：behind 反映上次 fetch 时点），
	// pull 才做 fetch + 快进
	pushNewCommit(t, remote, "web/compose.yml", "services:\n  web:\n    image: nginx:1.27\n")
	st = p.GitStatus()
	if st.Behind != 0 {
		t.Fatalf("unfetched behind = %d, want 0 (status is fetchless)", st.Behind)
	}
	res, err = p.GitPull(map[string]interface{}{})
	if err != nil {
		t.Fatalf("second pull: %v", err)
	}
	if res["behind"].(int) != 0 {
		t.Fatalf("after pull behind = %v", res["behind"])
	}
	data, _ = os.ReadFile(filepath.Join(p.dir, "web", "compose.yml"))
	if !strings.Contains(string(data), "1.27") {
		t.Fatalf("fast-forward content = %s", data)
	}
}

// pushNewCommit 往 bare 远端推一个新提交（借一个一次性 work 目录）
func pushNewCommit(t *testing.T, remote, file, content string) {
	t.Helper()
	work := t.TempDir()
	run := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(work, "clone", "--quiet", remote, ".")
	gitWrite(t, work, file, content)
	run(work, "add", ".")
	run(work, "commit", "-m", "update")
	run(work, "push", "origin", "HEAD")
}

func TestStackGitDriftAndFilter(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{
		"web/compose.yml": "services: {}\n",
		"db/compose.yml":  "services: {}\n",
	})
	p := gitTestProvider(t)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}

	// 改 tracked 文件 + 加 untracked 文件 + 动 .cockpit 保留目录
	gitWrite(t, p.dir, "web/compose.yml", "services:\n  web:\n    image: changed\n")
	gitWrite(t, p.dir, "newstack/compose.yml", "services: {}\n")
	gitWrite(t, p.dir, ".cockpit/x/tmp.log", "noise")

	st := p.GitStatus()
	got := map[string]string{}
	for _, c := range st.Changes {
		got[c.Path] = c.Code
	}
	if got["web/compose.yml"] != "M" {
		t.Fatalf("modified drift missing: %v", got)
	}
	if _, ok := got["newstack/compose.yml"]; !ok {
		t.Fatalf("untracked drift missing: %v", got)
	}
	for path := range got {
		if strings.Contains(path, stackReservedDir) {
			t.Fatalf(".cockpit leaked into drift: %v", got)
		}
	}

	// drift（tracked 改动）+ 远端同文件新提交 → pull 被拒（git 拒绝覆盖本地改动）
	pushNewCommit(t, remote, "web/compose.yml", "services:\n  web:\n    image: remote-new\n")
	_, err := p.GitPull(map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "conflict") {
		t.Fatalf("dirty tree pull err = %v, want conflict", err)
	}
}

func TestStackGitAttachConflictOnUntracked(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{
		"web/compose.yml": "services:\n  web:\n    image: nginx\n",
	})
	p := gitTestProvider(t)
	// 既有同名本地文件 → 接入不得静默覆盖，报冲突
	gitWrite(t, p.dir, "web/compose.yml", "services: {}\n")
	_, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"})
	if err == nil {
		t.Fatal("attach over untracked file should fail")
	}
	data, _ := os.ReadFile(filepath.Join(p.dir, "web", "compose.yml"))
	if string(data) != "services: {}\n" {
		t.Fatalf("local file overwritten: %q", data)
	}
}

func TestStackGitValidationAndStatus(t *testing.T) {
	p := gitTestProvider(t)

	// 未接入状态：reason=not-a-repo，不是错误
	st := p.GitStatus()
	if st.Enabled || st.Reason != "not-a-repo" {
		t.Fatalf("fresh dir status = %+v", st)
	}

	// remote 校验：ext:: 助手语法 / - 开头 / 空
	if err := validateGitRemote("ext::sh -c touch"); err == nil {
		t.Fatal("ext:: transport accepted")
	}
	if err := validateGitRemote("-oops"); err == nil {
		t.Fatal("leading dash accepted")
	}
	if err := validateGitRemote(""); err == nil {
		t.Fatal("empty remote accepted")
	}
	if err := validateGitRemote("https://github.com/x/y.git"); err != nil {
		t.Fatalf("https remote rejected: %v", err)
	}

	// 分支校验：- 开头、..、非法字符
	for _, b := range []string{"-main", "a..b", "main drop", "main;rm"} {
		if _, err := p.GitPull(map[string]interface{}{"remote": "/tmp/x", "branch": b}); err == nil {
			t.Fatalf("branch %q accepted", b)
		}
	}

	// 非 attach 的 pull 缺 remote 且未接入 → attach 路径校验兜住
	if _, err := p.GitPull(map[string]interface{}{}); err == nil {
		t.Fatal("pull without remote on fresh dir should fail")
	}

	// git 不可用的 provider：status 报 git-unavailable
	broken := NewStackProvider(StackConfig{Dir: t.TempDir(), GitBin: []string{"git-definitely-missing-xyz"}})
	st = broken.GitStatus()
	if st.Enabled || st.Reason != "git-unavailable" {
		t.Fatalf("broken git status = %+v", st)
	}
	if _, err := broken.GitPull(map[string]interface{}{"remote": "/tmp/x"}); err == nil {
		t.Fatal("pull without git should fail")
	}
}

func TestStackGitSwitchBranchAndCredSanitize(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"a/compose.yml": "services: {}\n"})
	// 再造一个 dev 分支
	work := t.TempDir()
	run := func(dir string, args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run(work, "clone", "--quiet", remote, ".")
	run(work, "checkout", "-b", "dev")
	gitWrite(t, work, "b/compose.yml", "services: {}\n")
	run(work, "add", ".")
	run(work, "commit", "-m", "dev")
	run(work, "push", "origin", "dev")

	p := gitTestProvider(t)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	// 切到 dev：落地 b/
	res, err := p.GitPull(map[string]interface{}{"branch": "dev"})
	if err != nil {
		t.Fatalf("switch branch: %v", err)
	}
	if res["branch"] != "dev" {
		t.Fatalf("branch = %v", res["branch"])
	}
	if _, err := os.Stat(filepath.Join(p.dir, "b", "compose.yml")); err != nil {
		t.Fatalf("dev file missing: %v", err)
	}

	// 凭据脱敏：URL 里 userinfo 不出现在回显
	if got := sanitizeGitSecret("https://user:secret@host/x.git"); strings.Contains(got, "secret") {
		t.Fatalf("credential leaked: %q", got)
	}
	remoteWithCred := strings.Replace(remote, "/stacks-remote.git", "/user:pass@stacks-remote.git", 1)
	_, err = p.GitPull(map[string]interface{}{"remote": remoteWithCred, "branch": "dev"})
	if err == nil || strings.Contains(err.Error(), "user:pass") {
		t.Fatalf("credential leaked in error: %v", err)
	}
}

func TestGitPathSkipped(t *testing.T) {
	cases := map[string]bool{
		"web/compose.yml":      false,
		".cockpit/x/log":       true,
		".git/index":           true,
		"web/.cockpit/tmp":     true,
		"web/.gitignore":       false, // 文件名 .gitignore 不误伤
		"somegit/x":            false,
	}
	for path, want := range cases {
		if got := gitPathSkipped(path); got != want {
			t.Errorf("gitPathSkipped(%q) = %v, want %v", path, got, want)
		}
	}
}

// gitRun 测试脚手架：在 dir 下执行一条真 git 命令
func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// gitFailBin 包一层假 git：指定子命令直接失败，其余转发真 git
//（模拟 git 二进制层故障，用于 attach 流程中间步骤的错误分支）
func gitFailBin(t *testing.T, failCmd string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "git-wrap")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"" + failCmd + "\" ]; then\n" +
		"  echo 'simulated failure: " + failCmd + "' >&2\n" +
		"  exit 9\n" +
		"fi\n" +
		"exec git \"$@\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// gitFakeStatusBin 包一层假 git：status 子命令输出固定文本，其余转发真 git
//（porcelain 解析器对异常/特殊输出的行为验证）
func gitFakeStatusBin(t *testing.T, statusOut string) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "git-fake-status")
	script := "#!/bin/sh\n" +
		"if [ \"$1\" = \"status\" ]; then\n" +
		"  printf '%s\\n' " + statusOut + "\n" +
		"  exit 0\n" +
		"fi\n" +
		"exec git \"$@\"\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin
}

// Call 分发 + gitBin 默认值（provider 未配 GitBin 时走系统 git）
func TestStackGitCallDispatch(t *testing.T) {
	p := NewStackProvider(StackConfig{Dir: t.TempDir()})
	res, err := p.Call("git.status", map[string]interface{}{})
	if err != nil {
		t.Fatalf("call git.status: %v", err)
	}
	st, ok := res.(*StackGitStatus)
	if !ok || st.Enabled || st.Reason != "not-a-repo" {
		t.Fatalf("call git.status = %#v", res)
	}
	if _, err := p.Call("git.pull", map[string]interface{}{}); err == nil {
		t.Fatal("call git.pull without remote should fail")
	}
}

// 状态查询边界：unborn 仓库（无提交）分支回退 main；.git 是普通文件时
// status 失败 → enabled 但 drift 为空
func TestStackGitStatusUnbornAndBroken(t *testing.T) {
	p := gitTestProvider(t)
	gitRun(t, "", "init", p.dir)
	st := p.GitStatus()
	if !st.Enabled || st.Branch != "main" || st.Head != "" {
		t.Fatalf("unborn repo status = %+v", st)
	}

	// .git 是普通文件：isGitRepo 过，但 git 全部报错
	p2 := gitTestProvider(t)
	if err := os.WriteFile(filepath.Join(p2.dir, ".git"), []byte("garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	st = p2.GitStatus()
	if !st.Enabled || st.Branch != "main" || st.Remote != "" || len(st.Changes) != 0 {
		t.Fatalf("broken .git status = %+v", st)
	}
}

// porcelain 解析：重命名行取 -> 后新路径、带引号路径去引号、
// 空路径行跳过（借假 git 注入构造的 status 输出）
func TestStackGitDriftParserForms(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"a.txt": "x\n"})
	p := gitTestProvider(t)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	// 注入的 porcelain：重命名行 + 引号转义路径 + 空路径残行
	fake := NewStackProvider(StackConfig{Dir: p.dir, GitBin: []string{
		gitFakeStatusBin(t, `'R  a.txt -> b.txt' '?? "\346\226\207.txt"' 'M  "'`),
	}})
	st := fake.GitStatus()
	got := map[string]string{}
	for _, c := range st.Changes {
		got[c.Path] = c.Code
	}
	if got["b.txt"] != "R" {
		t.Fatalf("rename path missing: %v", got)
	}
	if got[`\346\226\207.txt`] != "??" {
		t.Fatalf("quoted path missing: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("unexpected parsed changes: %v", got)
	}
}

// 已接入仓库的 pull：更新 remote 走校验拒绝；control char 校验
func TestStackGitValidationUpdatePath(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"a.txt": "x\n"})
	p := gitTestProvider(t)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	// 已接入 + 非法 remote → 更新路径侧校验拒绝
	if _, err := p.GitPull(map[string]interface{}{"remote": "ext::x"}); err == nil {
		t.Fatal("bad remote on attached repo accepted")
	}
	if err := validateGitRemote("https://h\x01/x"); err == nil {
		t.Fatal("control char remote accepted")
	}
}

// init 过但没配过 origin 的仓库：set-url 失败 → remote add 兜住；
// unborn HEAD 下 checkout 目标分支失败明确报错
func TestStackGitPullUpdateRemoteAndCheckout(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"a.txt": "x\n"})
	p := gitTestProvider(t)
	gitRun(t, "", "init", p.dir)
	// unborn HEAD 下 rev-parse --abbrev-ref HEAD 退出 128：read branch 失败
	// 先于 checkout/merge 暴露（真实 git 行为，非防御分支）
	_, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"})
	if err == nil || !strings.Contains(err.Error(), "read branch") {
		t.Fatalf("unborn pull err = %v, want read branch", err)
	}
}

// 脏树切换分支：本地改动文件在两分支间有差异 → checkout 被 git 拒绝
func TestStackGitCheckoutConflict(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"a.txt": "x\n"})
	work := t.TempDir()
	gitRun(t, work, "clone", "--quiet", remote, ".")
	gitWrite(t, work, "a.txt", "dev version\n")
	gitRun(t, work, "commit", "-am", "dev")
	gitRun(t, work, "push", "origin", "HEAD:refs/heads/dev")

	p := gitTestProvider(t)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	gitWrite(t, p.dir, "a.txt", "local edit\n")
	_, err := p.GitPull(map[string]interface{}{"branch": "dev"})
	if err == nil || !strings.Contains(err.Error(), "checkout dev") {
		t.Fatalf("dirty switch err = %v, want checkout dev", err)
	}
}

// .git/config 只读：set-url 与 remote add 双双失败 → 明确报错（skip root）
func TestStackGitRemoteAddFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file perms")
	}
	remote := gitInitRemote(t, "main", map[string]string{"a.txt": "x\n"})
	p := gitTestProvider(t)
	gitRun(t, "", "init", p.dir)
	// git 写 config 走 .git/ 下 lockfile+rename，文件只读拦不住；
	// 只读 .git 目录才能同时挡住 set-url 与 remote add
	gitDir := filepath.Join(p.dir, ".git")
	if err := os.Chmod(gitDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(gitDir, 0o700) })
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err == nil ||
		!strings.Contains(err.Error(), "set remote") {
		t.Fatalf("readonly config err = %v, want set remote", err)
	}
}

// stacks 目录路径上有普通文件挡路：MkdirAll 失败明确报错
func TestStackGitDirBlocked(t *testing.T) {
	parent := t.TempDir()
	if err := os.WriteFile(filepath.Join(parent, "blocker"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := NewStackProvider(StackConfig{Dir: filepath.Join(parent, "blocker", "stacks"), GitBin: []string{"git"}})
	_, err := p.GitPull(map[string]interface{}{"remote": "/tmp/x", "branch": "main"})
	if err == nil || !strings.Contains(err.Error(), "create stacks dir") {
		t.Fatalf("blocked dir err = %v", err)
	}
}

// 远端分支被删（fetch --prune 后引用消失）：pull 明确报分支不存在
func TestStackGitRemoteBranchDeleted(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"a.txt": "x\n"})
	p := gitTestProvider(t)
	if _, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"}); err != nil {
		t.Fatal(err)
	}
	// bare 仓 HEAD 指着 main，先推 dev 并把 bare HEAD 改指 dev 才删得掉 main
	work := t.TempDir()
	gitRun(t, work, "clone", "--quiet", remote, ".")
	gitRun(t, work, "checkout", "-b", "dev")
	gitRun(t, work, "commit", "--allow-empty", "-m", "dev")
	gitRun(t, work, "push", "origin", "dev")
	gitRun(t, remote, "symbolic-ref", "HEAD", "refs/heads/dev")
	gitRun(t, work, "push", "origin", "--delete", "main")
	_, err := p.GitPull(map[string]interface{}{})
	if err == nil || !strings.Contains(err.Error(), "remote branch main not found") {
		t.Fatalf("deleted branch err = %v", err)
	}
}

// attach 流程中间步骤失败：init / symbolic-ref / fetch（借假 git 定点失败）
func TestStackGitAttachStepFailures(t *testing.T) {
	remote := gitInitRemote(t, "main", map[string]string{"a.txt": "x\n"})
	cases := []struct{ failCmd, wantErr string }{
		{"init", "git init"},
		{"symbolic-ref", "set branch"},
		{"remote", "remote add"},
		{"fetch", "fetch:"},
	}
	for _, c := range cases {
		p := NewStackProvider(StackConfig{Dir: t.TempDir(), GitBin: []string{gitFailBin(t, c.failCmd)}})
		_, err := p.GitPull(map[string]interface{}{"remote": remote, "branch": "main"})
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Fatalf("%s failure err = %v, want %s", c.failCmd, err, c.wantErr)
		}
	}

	// 空仓远端（无提交）：fetch 成功但远端分支不存在
	empty := filepath.Join(t.TempDir(), "empty.git")
	gitRun(t, "", "init", "--bare", "-b", "main", empty)
	p := gitTestProvider(t)
	_, err := p.GitPull(map[string]interface{}{"remote": empty, "branch": "main"})
	if err == nil || !strings.Contains(err.Error(), "remote branch main not found") {
		t.Fatalf("empty remote err = %v", err)
	}
}
