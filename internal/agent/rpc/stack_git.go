package rpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ============ Stack Git 化（P2 ① Git-to-deploy） ============
//
// stacks 根目录整体作为一个 git 仓库接入远端（bare 仓/任意 git remote），
// pull 用 fetch + --ff-only 快进（不自动 commit、不 merge 本地分叉），
// drift = 工作区相对 HEAD 的改动（stack.file.save 的 UI 编辑即本地改动，
// 不悄悄替用户提交——compose-file-first 原则的延伸）。
// 设计决策见 docs/guide/stack-deploy-design.md P2 章节与 decision-log。

var (
	// gitBranchRe 分支名约束：拒绝以 - 开头（防被当作 git 选项）、拒绝 ..（防
	// 跨 ref 路径）；git 自身对分支字符还有校验，这里是入口侧的最小防线
	gitBranchRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]{0,99}$`)

	// gitCredLikeRe URL 内 userinfo 凭据（user:pass@）——任何回显前抹除，
	// 凭据不进日志/审计/错误消息
	gitCredLikeRe = regexp.MustCompile(`(//)[^/@\s]+@`)
)

// gitOutputTimeout 单条 git 命令的墙钟上限；fetch 走网络给足余量
const gitOutputTimeout = 60 * time.Second

// StackGitChange 工作区相对仓库的一条改动（porcelain XY 码 + 路径）
type StackGitChange struct {
	Code string `json:"code"`
	Path string `json:"path"`
}

// StackGitStatus stacks 目录的 git 接入状态（未接入不是错误，是常态）
type StackGitStatus struct {
	Enabled bool   `json:"enabled"`
	Reason  string `json:"reason,omitempty"` // git-unavailable / not-a-repo
	Remote  string `json:"remote,omitempty"`
	Branch  string `json:"branch,omitempty"`
	Head    string `json:"head,omitempty"` // 本地 HEAD（短 SHA；未提交为空）
	// RemoteHead origin/<branch> 的短 SHA（从未 fetch 到为空）；
	// head != remoteHead 且 behind>0 → 有可拉取更新
	RemoteHead string           `json:"remoteHead,omitempty"`
	Ahead      int              `json:"ahead"`
	Behind     int              `json:"behind"`
	Changes    []StackGitChange `json:"changes,omitempty"` // drift：本地改动 vs 仓库
}

// gitBin 返回 git 可执行（StackConfig.GitBin 可注入；nil/空 = 系统 git）
func (p *StackProvider) gitBin() []string {
	if len(p.gitCmd) > 0 {
		return p.gitCmd
	}
	return []string{"git"}
}

// gitAvailable git 二进制是否可执行
func (p *StackProvider) gitAvailable() bool {
	bin := p.gitBin()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, bin[0], append(bin[1:], "--version")...).Run() == nil
}

// runGit 在 dir 下执行一条 git 命令，返回合并后的 stdout+stderr。
// 错误文本统一过 sanitizeGitSecret（git 会把带凭据的 URL 原样回显）。
func (p *StackProvider) runGit(dir string, args ...string) (string, error) {
	bin := p.gitBin()
	ctx, cancel := context.WithTimeout(context.Background(), gitOutputTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin[0], append(bin[1:], args...)...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return sanitizeGitSecret(string(out)), err
}

// sanitizeGitSecret 抹除输出里的 userinfo 凭据
func sanitizeGitSecret(s string) string {
	return gitCredLikeRe.ReplaceAllString(s, "$1***@")
}

// isGitRepo stacks 目录是否已是 git 仓库（worktree 根或子目录均算）
func isGitRepo(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// GitStatus 查 stacks 目录的 git 接入与 drift 状态
func (p *StackProvider) GitStatus() *StackGitStatus {
	st := &StackGitStatus{}
	if !p.gitAvailable() {
		st.Reason = "git-unavailable"
		return st
	}
	if !isGitRepo(p.dir) {
		st.Reason = "not-a-repo"
		return st
	}

	out, err := p.runGit(p.dir, "config", "--get", "remote.origin.url")
	if err == nil {
		st.Remote = sanitizeGitSecret(strings.TrimSpace(out))
	}
	out, err = p.runGit(p.dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		st.Branch = strings.TrimSpace(out)
	}
	if st.Branch == "" || st.Branch == "HEAD" {
		st.Branch = "main"
	}

	// 本地 HEAD：unborn（空仓库）时 rev-parse 失败，Head 留空
	out, err = p.runGit(p.dir, "rev-parse", "--short", "HEAD")
	if err == nil {
		st.Head = strings.TrimSpace(out)
	}

	// origin/<branch>：fetch 过才有
	remoteRef := "refs/remotes/origin/" + st.Branch
	out, err = p.runGit(p.dir, "rev-parse", "--short", remoteRef)
	if err == nil {
		st.RemoteHead = strings.TrimSpace(out)
	}
	if st.Head != "" && st.RemoteHead != "" {
		out, err = p.runGit(p.dir, "rev-list", "--left-right", "--count", "HEAD..."+remoteRef)
		if err == nil {
			fmt.Sscanf(strings.TrimSpace(out), "%d\t%d", &st.Ahead, &st.Behind)
		}
	}

	st.Changes = p.gitDriftChanges()
	st.Enabled = true
	return st
}

// gitDriftChanges 工作区相对 HEAD 的改动清单（porcelain v1，逐行 "XY path"）。
// 过滤：.cockpit/ 保留目录（部署日志等机器产物，永不属于 drift）、.git/。
// 重命名行取 -> 后的新路径。
func (p *StackProvider) gitDriftChanges() []StackGitChange {
	// porcelain -uall：untracked 逐文件展开（drift 提示精确到文件而非目录）
	out, err := p.runGit(p.dir, "status", "--porcelain", "-uall")
	if err != nil {
		return nil
	}
	changes := make([]StackGitChange, 0, 8)
	for _, line := range strings.Split(out, "\n") {
		if len(line) < 4 {
			continue
		}
		code := strings.TrimSpace(line[:2])
		path := strings.TrimSpace(line[3:])
		if idx := strings.LastIndex(path, " -> "); idx >= 0 {
			path = path[idx+4:]
		}
		path = strings.Trim(path, `"`)
		if path == "" {
			continue
		}
		if gitPathSkipped(path) {
			continue
		}
		changes = append(changes, StackGitChange{Code: code, Path: path})
	}
	if len(changes) == 0 {
		return nil
	}
	return changes
}

// gitPathSkipped drift 过滤规则：路径任一段为 .cockpit / .git 即跳过
func gitPathSkipped(path string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(path), "/") {
		if seg == stackReservedDir || seg == ".git" {
			return true
		}
	}
	return false
}

// validateGitRemote 远端地址最小校验：拒绝 git 传输助手语法（ext:: 等可
// 执行任意命令）、拒绝 - 开头（防被当 git 选项）、拒绝控制字符。
// 允许：本地路径 / file: / http(s): / ssh: / git: / scp 语法（user@host:path）。
func validateGitRemote(remote string) error {
	if remote == "" {
		return fmt.Errorf("remote required")
	}
	if strings.Contains(remote, "::") {
		return fmt.Errorf("remote transport helpers (::) not allowed")
	}
	if strings.HasPrefix(remote, "-") {
		return fmt.Errorf("remote must not start with '-'")
	}
	for _, r := range remote {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("remote contains control characters")
		}
	}
	return nil
}

// GitPull 从远端拉取 stacks 仓库。
//
// 首次接入（目录还不是仓库）：init → remote add → fetch → checkout -B
// 到 origin/<branch>（要求远端分支已存在——空仓库先在别处推首版）。
// 已接入：fetch 后 --ff-only 快进（本地有提交分叉时明确报错，不代做
// rebase/merge 决策；工作区 tracked 文件被改时 git 拒绝合并，错误原文
// 含冲突文件清单，正好是 drift 提示的素材）。
func (p *StackProvider) GitPull(params map[string]interface{}) (map[string]interface{}, error) {
	remote := paramString(params, "remote")
	branch := paramString(params, "branch")
	if branch == "" {
		branch = "main"
	}
	if !gitBranchRe.MatchString(branch) || strings.Contains(branch, "..") {
		return nil, fmt.Errorf("invalid branch name: %q", branch)
	}
	if !p.gitAvailable() {
		return nil, fmt.Errorf("git unavailable on agent host")
	}
	if err := os.MkdirAll(p.dir, 0o700); err != nil {
		return nil, fmt.Errorf("create stacks dir: %w", err)
	}

	if !isGitRepo(p.dir) {
		if err := validateGitRemote(remote); err != nil {
			return nil, err
		}
		return p.gitAttach(remote, branch)
	}
	if remote != "" {
		if err := validateGitRemote(remote); err != nil {
			return nil, err
		}
		if _, err := p.runGit(p.dir, "remote", "set-url", "origin", remote); err != nil {
			// origin 尚不存在（init 过但没配过远端）
			if _, err2 := p.runGit(p.dir, "remote", "add", "origin", remote); err2 != nil {
				return nil, fmt.Errorf("set remote: %s", err2)
			}
		}
	}

	if _, err := p.runGit(p.dir, "fetch", "origin", "--prune"); err != nil {
		return nil, fmt.Errorf("fetch: %s", err)
	}

	// 目标分支：显式指定则切换（脏树冲突时 git 自行拒绝），否则用当前分支
	out, err := p.runGit(p.dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("read branch: %s", err)
	}
	current := strings.TrimSpace(out)
	if current != branch {
		if _, err := p.runGit(p.dir, "checkout", branch); err != nil {
			return nil, fmt.Errorf("checkout %s: %s", branch, err)
		}
	}

	remoteRef := "refs/remotes/origin/" + branch
	if _, err := p.runGit(p.dir, "rev-parse", "--verify", "--quiet", remoteRef); err != nil {
		return nil, fmt.Errorf("remote branch %s not found (empty repository? push a commit first)", branch)
	}
	if _, err := p.runGit(p.dir, "merge", "--ff-only", remoteRef); err != nil {
		// 本地改动 / 本地提交分叉——git 的拒绝理由就是给用户的 drift 提示
		return nil, fmt.Errorf("merge conflict, resolve local changes first: %s", err)
	}

	st := p.GitStatus()
	return map[string]interface{}{
		"enabled":    st.Enabled,
		"remote":     st.Remote,
		"branch":     st.Branch,
		"head":       st.Head,
		"remoteHead": st.RemoteHead,
		"ahead":      st.Ahead,
		"behind":     st.Behind,
		"changes":    st.Changes,
	}, nil
}

// gitAttach 首次接入：init + remote + fetch + checkout 落地远端内容
func (p *StackProvider) gitAttach(remote, branch string) (map[string]interface{}, error) {
	if _, err := p.runGit(p.dir, "init"); err != nil {
		return nil, fmt.Errorf("git init: %s", err)
	}
	// 显式把 HEAD 指到目标分支（git init -b 需 2.28+，symbolic-ref 全版本可用）
	if _, err := p.runGit(p.dir, "symbolic-ref", "HEAD", "refs/heads/"+branch); err != nil {
		return nil, fmt.Errorf("set branch %s: %s", branch, err)
	}
	if _, err := p.runGit(p.dir, "remote", "add", "origin", remote); err != nil {
		return nil, fmt.Errorf("remote add: %s", err)
	}
	if _, err := p.runGit(p.dir, "fetch", "origin", "--prune"); err != nil {
		return nil, fmt.Errorf("fetch: %s", err)
	}
	remoteRef := "refs/remotes/origin/" + branch
	if _, err := p.runGit(p.dir, "rev-parse", "--verify", "--quiet", remoteRef); err != nil {
		return nil, fmt.Errorf("remote branch %s not found (empty repository? push a commit first)", branch)
	}
	// -B：unborn HEAD 下等价 checkout；若远端文件与既有本地栈重名，git 拒绝
	// 覆盖 untracked 并报冲突清单（不静默覆盖本地文件）
	if _, err := p.runGit(p.dir, "checkout", "-B", branch, remoteRef); err != nil {
		return nil, fmt.Errorf("checkout: %s", err)
	}
	st := p.GitStatus()
	return map[string]interface{}{
		"enabled":    st.Enabled,
		"remote":     st.Remote,
		"branch":     st.Branch,
		"head":       st.Head,
		"remoteHead": st.RemoteHead,
		"ahead":      st.Ahead,
		"behind":     st.Behind,
		"changes":    st.Changes,
	}, nil
}
