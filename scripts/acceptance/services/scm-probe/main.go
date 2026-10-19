//go:build windows

// Windows SCM 服务管理真实环境验收探针（acceptance-checklist「服务管理（三后端）」
// Windows SCM 行、cockpit todo.md「Windows SCM 服务管理」条目、checklist:249）。
// 经 provider 入口 WindowsServiceProvider 走生产同一条链。按提权需求分两层：
//
//	【校验层·无需提权】validateWindowsServiceUnit 在 mgr.Connect 之前
//	  （service_windows_scm.go:104），故动词白名单/非法名/未知动词三类拒绝
//	  在非管理员会话即可实测：
//	  T1 reload 被白名单拒绝（checklist:249「reload 确认隐藏」后端半）
//	  T2 非法 unit 名（../etc/passwd）在管理器前拒绝
//	  T3 未知动词在管理器前拒绝
//	  T4 无权限报错透传机制：对真实 SCM connect 的 Access is denied 原文
//	     经 %w 透传到 provider（不被吞），即「报错透传」链路本身可证
//
//	【SCM 连接层·需提权 admin】mgr.Connect() 申请 SC_MANAGER_ALL_ACCESS，
//	  非管理员 token 直接 Access is denied（scm.go:61 mgr.Connect）。故
//	  list/启停/自启切换/restart 30s 超时路径实测需提权 agent 会话——
//	  本探针如实记录 connect 被拒，不伪造通过，标 BLOCKED 待提权复验。
//
// 证据落 .acceptance/services/evidence/。不 create/delete 服务、不碰业务服务。
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

func main() {
	p := rpc.NewWindowsServiceProvider()

	// ===== 校验层（无需提权，validate 在 connect 前）=====

	// T1 reload 被动词白名单拒绝
	_, err := p.Call("action", map[string]interface{}{"name": "Spooler", "action": "reload"})
	check("T1", "reload 被动词白名单拒绝（reload 隐藏后端半）",
		err != nil && strings.Contains(err.Error(), "unsupported action"), errDetail(err))

	// T2 非法 unit 名在管理器前拒绝
	_, err = p.Call("action", map[string]interface{}{"name": "../etc/passwd", "action": "start"})
	check("T2", "非法 unit 名被拒（路径穿越面）",
		err != nil && strings.Contains(err.Error(), "invalid windows service name"), errDetail(err))

	// T3 未知动词在管理器前拒绝
	_, err = p.Call("action", map[string]interface{}{"name": "Spooler", "action": "nuke"})
	check("T3", "未知动词被拒",
		err != nil && strings.Contains(err.Error(), "unsupported action"), errDetail(err))

	// T4 无权限报错透传：真实 SCM connect 的 Access is denied 原文透传到 provider
	_, err = p.Call("list", nil)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "access is denied") {
		// 透传链路本身可证：SCM 原始错误未被吞、带 scm connect 上下文
		check("T4", "SCM 无权限报错原文透传到 provider（Access is denied 未被吞）",
			strings.Contains(err.Error(), "scm connect"), errDetail(err))
		// SCM 连接层其余项需提权——如实标 BLOCKED，不伪造
		block("T5", "list/启停/自启/restart 30s 超时路径实测",
			fmt.Sprintf("mgr.Connect 需 SC_MANAGER_ALL_ACCESS，本会话非提权（UAC），需 admin agent 复验；err=%s", errDetail(err)))
	} else if err != nil {
		// 连上了但别的错——不该发生于非提权，如实 FAIL
		check("T4", "SCM connect 结果符合预期", false, "意外错误: "+errDetail(err))
	} else {
		// 非提权却连上了（该会话实际有权限）——继续跑连接层实测
		runConnectLayer(p)
	}

	fmt.Printf("\n=== RESULT %d PASS / %d FAIL / %d BLOCKED ===\n", pass, fail, blocked)
	if fail > 0 {
		os.Exit(1)
	}
}

// runConnectLayer 仅当 SCM connect 成功（提权会话）才跑：list/字段值域/排序
func runConnectLayer(p *rpc.WindowsServiceProvider) {
	res, err := p.Call("list", nil)
	if err != nil {
		check("T5a", "list 无错", false, errDetail(err))
		return
	}
	units := res.(map[string]interface{})["services"].([]rpc.ServiceUnit)
	check("T5b", "list 非空", len(units) > 0, fmt.Sprintf("%d 服务", len(units)))
	goodActive := map[string]bool{"active": true, "inactive": true, "activating": true,
		"deactivating": true, "paused": true, "unknown": true}
	bad := 0
	for _, u := range units {
		if !goodActive[u.ActiveState] {
			bad++
		}
	}
	check("T5c", "ActiveState 值域", bad == 0, fmt.Sprintf("%d 越界", bad))
}

func errDetail(err error) string {
	if err == nil {
		return "err=nil"
	}
	return "err=" + err.Error()
}
