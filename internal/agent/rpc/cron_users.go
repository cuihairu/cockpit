package rpc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

// 多用户 crontab（cron-design.md M4 D21-D22）：user 参数校验与系统用户枚举。

// cronUserRe 用户名白名单：POSIX 惯用形态，大小写敏感、拒怪形态与超长；
// 不收 uid 数字形态（getent 名字才是稳定键）。argv 转发下注入面本为零，
// 正则是第二道闸（server 同规则）。
var cronUserRe = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)

// cronLookPath 可注入（测试替换）
var cronLookPath = exec.LookPath

// osReadFile 可注入（测试覆盖 /etc/passwd 不可读的兜底失败分支）
var osReadFile = os.ReadFile

// validateCronUser 空放行（缺省 = 当前运行用户），非空按白名单校验
func validateCronUser(user string) error {
	if user == "" {
		return nil
	}
	if !cronUserRe.MatchString(user) {
		return fmt.Errorf("invalid user name %q", user)
	}
	return nil
}

// cronUserFromParams 提取可选 user 参数并校验（M4 D21）
func cronUserFromParams(params map[string]interface{}) (string, error) {
	user := ""
	if params != nil {
		user = strings.TrimSpace(paramString(params, "user"))
	}
	if err := validateCronUser(user); err != nil {
		return "", err
	}
	return user, nil
}

// CronUserEntry 系统用户条目（枚举供下拉选择，M4 D22）
type CronUserEntry struct {
	Name  string `json:"name"`
	UID   int    `json:"uid"`
	Shell string `json:"shell,omitempty"`
}

// Users 枚举系统用户：getent passwd 优先（NSS 兼容，覆盖 LDAP/SSSD），
// 命令缺失或失败 fallback 解析 /etc/passwd（行格式同构）。全量返回不做
// nologin 过滤——服务用户常 nologin 却恰是 crontab 的主人（D22）。
func (p *CronProvider) Users() (interface{}, error) {
	users, err := listCronUsers(p.run)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"users": users}, nil
}

// listCronUsers 枚举实现；getent 不可用或报错时 fallback /etc/passwd
func listCronUsers(run Commander) ([]CronUserEntry, error) {
	if _, err := cronLookPath("getent"); err == nil {
		ctx, cancel := context.WithTimeout(context.Background(), nginxTestTimeout)
		defer cancel()
		out, _, err := run(ctx, "getent", "passwd")
		if err == nil {
			return parsePasswdLines(string(out)), nil
		}
		// getent 失败（如 NSS 后端异常）不致命，落 fallback
	}
	data, err := osReadFile("/etc/passwd")
	if err != nil {
		return nil, fmt.Errorf("enumerate users: %w", err)
	}
	return parsePasswdLines(string(data)), nil
}

// parsePasswdLines 解析 passwd 行：name:x:uid:gid:gecos:home:shell；
// 字段不足或 uid 非数字的行跳过（不致命）
func parsePasswdLines(data string) []CronUserEntry {
	users := []CronUserEntry{}
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		f := strings.Split(line, ":")
		if len(f) < 3 || f[0] == "" {
			continue
		}
		uid, err := strconv.Atoi(f[2])
		if err != nil {
			continue
		}
		entry := CronUserEntry{Name: f[0], UID: uid}
		if len(f) >= 7 {
			entry.Shell = f[6]
		}
		users = append(users, entry)
	}
	return users
}
