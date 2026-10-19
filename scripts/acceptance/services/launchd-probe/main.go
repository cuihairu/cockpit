//go:build darwin

// macOS launchd 服务管理真实环境验收探针（acceptance-checklist「服务管理（三后端）」
// macOS launchd 行、cockpit todo.md「macOS launchd 服务管理」条目、checklist:250）。
// 经 provider 入口 LaunchdServiceProvider 走生产同一条链。按提权需求分两层：
//
//	【校验层·无需提权】validateLaunchdUnit 在任何 launchctl 执行之前
//	  （service_launchd_darwin.go:125），故 label 白名单/动词白名单两类拒绝
//	  在非管理员会话即可实测：
//	  T1 非法 label（../etc/passwd 含 /）被拒（路径穿越面，label 正则
//	     service_launchd_model.go:35 不含 / 与空格）
//	  T2 未知动词 nuke 被白名单拒
//	  T3 reload 被白名单拒（launchd 无 reload，D10.3；
//	     前端半：Services/index.tsx 仅 isSystemd 渲染重载按钮）
//	  T4 报错透传机制：合法动词打到不存在的 label → launchctl 错误原文
//	     带 `launchctl <args>` 上下文返回不被吞（kickstart 失败后
//	     findLaunchdPlist 找不到 plist 时保留 kickstart 原错误，
//	     service_launchd_darwin.go:149）——非 root 即可证透传链
//
//	【launchctl 执行层·需 root + 专用测试 daemon】kickstart/bootout/enable/
//	  disable 作用 system 域需 root；真机启停/自启切换/kickstart 失败回退
//	  bootstrap 路径（D10.3）需临时 LaunchDaemon（不碰系统服务）——本探针
//	  如实标 BLOCKED 待 root 会话 + 测试 daemon 复验，不伪造通过。
//	  root 会话（euid==0）下另跑 list 观测层（launchctl list + plist 扫描
//	  合并、ActiveState 值域），仍不执行动作动词。
//
// 证据落 .acceptance/services/evidence/。不安装/删除 LaunchDaemon、不碰系统服务。
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/cuihairu/cockpit/internal/agent/rpc"
)

var pass, fail, blocked int

func check(id, cond string, ok bool, detail string) {
	if ok {
		pass++
		fmt.Printf("PASS  %s  %s — %s\n", id, cond, detail)
	} else {
		fail++
		fmt.Printf("FAIL  %s  %s — %s\n", id, cond, detail)
	}
}

func block(id, cond, detail string) {
	blocked++
	fmt.Printf("BLOCK %s  %s — %s\n", id, cond, detail)
}

func errDetail(err error) string {
	if err == nil {
		return "err=nil"
	}
	return "err=" + err.Error()
}

func main() {
	p := rpc.NewLaunchdServiceProvider()

	// ===== 校验层（无需提权，validate 在 launchctl 前）=====

	// T1 非法 label（路径穿越面）在 launchctl 前被拒
	_, err := p.Call("action", map[string]interface{}{"name": "../etc/passwd", "action": "start"})
	check("T1", "非法 label 被拒（路径穿越面，label 正则不含 /）",
		err != nil && strings.Contains(err.Error(), "invalid launchd label"), errDetail(err))

	// T2 未知动词在 launchctl 前被拒
	_, err = p.Call("action", map[string]interface{}{"name": "com.example.probe", "action": "nuke"})
	check("T2", "未知动词被拒",
		err != nil && strings.Contains(err.Error(), "unsupported action"), errDetail(err))

	// T3 reload 被白名单拒（launchd 无 reload，D10.3）
	_, err = p.Call("action", map[string]interface{}{"name": "com.example.probe", "action": "reload"})
	check("T3", "reload 被动词白名单拒（launchd 后端无 reload）",
		err != nil && strings.Contains(err.Error(), "unsupported action"), errDetail(err))

	// T4 报错透传：合法动词打到不存在的 label，launchctl 错误原文带上下文透传
	_, err = p.Call("action", map[string]interface{}{"name": "com.cockpit.probe.nonexistent", "action": "start"})
	check("T4", "launchctl 错误原文带上下文透传到 provider（不被吞）",
		err != nil && strings.Contains(err.Error(), "launchctl kickstart"), errDetail(err))

	// ===== launchctl 执行层（需 root + 专用测试 daemon）=====

	if os.Geteuid() == 0 {
		runListLayer(p)
	} else {
		block("T5", "list 观测层（launchctl list + plist 扫描合并、ActiveState 值域）",
			"非 root 会话（euid!=0）launchctl list 仅用户域，system 域观测需 root agent 复验")
	}
	block("T6", "启停/自启切换 + kickstart 失败回退 bootstrap 路径实测（checklist:250）",
		"需 root 会话 + 临时 LaunchDaemon（不碰系统服务），真机复验")

	fmt.Printf("\n=== RESULT %d PASS / %d FAIL / %d BLOCKED ===\n", pass, fail, blocked)
	if fail > 0 {
		os.Exit(1)
	}
}

// runListLayer 仅 root 会话跑：list 观测全链，仍不执行动作动词
func runListLayer(p *rpc.LaunchdServiceProvider) {
	res, err := p.Call("list", nil)
	if err != nil {
		check("T5a", "list 无错", false, errDetail(err))
		return
	}
	units := res.(map[string]interface{})["services"].([]rpc.ServiceUnit)
	check("T5b", "list 非空", len(units) > 0, fmt.Sprintf("%d 服务", len(units)))
	goodActive := map[string]bool{"active": true, "inactive": true, "failed": true}
	bad := 0
	for _, u := range units {
		if !goodActive[u.ActiveState] {
			bad++
		}
	}
	check("T5c", "ActiveState 值域", bad == 0, fmt.Sprintf("%d 越界", bad))
}
