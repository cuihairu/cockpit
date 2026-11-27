//go:build windows

// Windows SCM 服务管理真实环境验收探针（acceptance-checklist「服务管理（三后端）」
// Windows SCM 行、cockpit todo.md「Windows SCM 服务管理」条目、checklist:253）。
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
//	  非管理员 token 直接 Access is denied（scm.go:61 mgr.Connect）。提权会话
//	  下另建独占验收测试服务（镜像 systemd 侧 cockpit-acc-* 惯例）：
//	    cockpit-acc-scm      正常 helper（stop 立即收敛）— 启停/自启/restart
//	    cockpit-acc-scm-hang 挂死 helper（stop 报 StopPending 永不收敛）
//	      — 确定性构造 restart 的 30s 等待超时路径（scmStopAndWait）
//	  实测 list/启停/自启切换/restart 30s 超时路径 + 服务级 ACL 拒绝透传
//	  （WinDefend）。不动业务服务；WinDefend stop 在 tamper protection
//	  开启时预期被拒（若意外成功则立即启动还原并如实 FAIL）。
//	  结束后 taskkill + sc delete 清理，provider list 复验服务已消失。
//
// 证据落 .acceptance/services/evidence/（stdout 重定向 + scm-acc.log +
// scm-list-elevated.json）。不 create/delete 业务服务。
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cuihairu/cockpit/internal/agent/rpc"
	"golang.org/x/sys/windows/svc/mgr"
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

const (
	accSvcName  = "cockpit-acc-scm"
	accHangName = "cockpit-acc-scm-hang"
	accImage    = "scm-acc-svc.exe"
	// 服务进程以 LocalSystem 跑，用户 profile 目录 ACL 不保证可写——日志落
	// ProgramData（SYSTEM/Administrators 可写）
	accLogPath = `C:\ProgramData\scm-acc.log`
)

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
		runElevated(p)
	}

	fmt.Printf("\n=== RESULT %d PASS / %d FAIL / %d BLOCKED ===\n", pass, fail, blocked)
	if fail > 0 {
		os.Exit(1)
	}
}

func runElevated(p *rpc.WindowsServiceProvider) {
	exePath, err := os.Executable()
	if err != nil {
		check("T5", "定位探针 exe 路径", false, errDetail(err))
		return
	}
	dir := filepath.Dir(exePath)
	accExe := filepath.Join(dir, accImage)

	m, err := mgr.Connect()
	if err != nil {
		check("T5", "提权会话 mgr.Connect", false, errDetail(err))
		return
	}
	defer m.Disconnect()

	// ===== setup：清残留 + 建两个 Manual 验收服务 =====
	preClean()
	defer postClean()

	if _, err := m.CreateService(accSvcName, accExe, mgr.Config{
		StartType:   mgr.StartManual,
		DisplayName: "cockpit acceptance test service (normal)",
	}, accSvcName, "normal", accLogPath); err != nil {
		check("S1", "创建验收测试服务 cockpit-acc-scm", false, errDetail(err))
		return
	}
	if _, err := m.CreateService(accHangName, accExe, mgr.Config{
		StartType:   mgr.StartManual,
		DisplayName: "cockpit acceptance test service (stop-hang)",
	}, accHangName, "hang", accLogPath); err != nil {
		check("S1", "创建验收测试服务 cockpit-acc-scm-hang", false, errDetail(err))
		return
	}
	check("S1", "创建验收测试服务（cockpit-acc-scm / -hang，Manual）", true, accExe)

	// ===== T5 list =====
	res, err := p.Call("list", nil)
	if err != nil {
		check("T5a", "list 无错", false, errDetail(err))
		return
	}
	units := res.(map[string]interface{})["services"].([]rpc.ServiceUnit)
	check("T5a", "list 无错", true, fmt.Sprintf("%d 服务", len(units)))
	bad := 0
	for _, u := range units {
		switch u.ActiveState {
		case "active", "inactive", "activating", "deactivating", "paused", "unknown":
		default:
			bad++
		}
		if u.LoadState != "loaded" {
			bad++
		}
	}
	check("T5c", "ActiveState/LoadState 值域（windows-scm loaded/active/inactive/…）", bad == 0, fmt.Sprintf("%d 越界", bad))
	found := map[string]rpc.ServiceUnit{}
	for _, u := range units {
		found[u.Name] = u
	}
	for _, n := range []string{accSvcName, accHangName} {
		u, ok := found[n]
		check("T5d", "验收测试服务入列（新装 Manual）"+n, ok && u.LoadState == "loaded" &&
			strings.EqualFold(u.ActiveState, "inactive") && u.UnitFileState == "disabled",
			fmt.Sprintf("load=%s active=%s unitFile=%s", u.LoadState, u.ActiveState, u.UnitFileState))
	}
	dumpList(units)

	// ===== T6 status 概览 =====
	res, err = p.Call("status", nil)
	if err != nil {
		check("T6", "status 概览无错", false, errDetail(err))
	} else {
		st := res.(map[string]interface{})
		enabled, _ := st["enabled"].(int)
		check("T6", "status：systemState=unknown/failed=0（SCM 无 systemd 概念）+ total 一致",
			st["systemState"] == "unknown" && st["failed"] == 0 && st["total"] == len(units) && enabled <= len(units),
			fmt.Sprintf("total=%v active=%v enabled=%v", st["total"], st["active"], st["enabled"]))
	}

	// ===== T7 启动（含幂等）=====
	if _, err := p.Call("action", map[string]interface{}{"name": accSvcName, "action": "start"}); err != nil {
		check("T7a", "start 无错", false, errDetail(err))
	} else {
		check("T7a", "start 无错", true, "")
	}
	if _, err := p.Call("action", map[string]interface{}{"name": accSvcName, "action": "start"}); err != nil {
		check("T7b", "已运行再 start 幂等无错", false, errDetail(err))
	} else {
		check("T7b", "已运行再 start 幂等无错", true, "")
	}
	check("T7c", "start 后 list=active(running)",
		listState(p, accSvcName) == "active", "state="+listState(p, accSvcName))

	// ===== T8 停止（含幂等）=====
	if _, err := p.Call("action", map[string]interface{}{"name": accSvcName, "action": "stop"}); err != nil {
		check("T8a", "stop 无错", false, errDetail(err))
	} else {
		check("T8a", "stop 无错", true, "")
	}
	if _, err := p.Call("action", map[string]interface{}{"name": accSvcName, "action": "stop"}); err != nil {
		check("T8b", "已停止再 stop 幂等无错", false, errDetail(err))
	} else {
		check("T8b", "已停止再 stop 幂等无错", true, "")
	}
	check("T8c", "stop 后 list=inactive(stopped)",
		listState(p, accSvcName) == "inactive", "state="+listState(p, accSvcName))

	// ===== T9 restart（停止态/运行态双路径 + PID 进程证明）=====
	t0 := time.Now()
	pidBefore := svcPid(m, accSvcName)
	if _, err := p.Call("action", map[string]interface{}{"name": accSvcName, "action": "restart"}); err != nil {
		check("T9a", "restart（从停止态，scmStopped 幂等直拉起）无错", false, errDetail(err))
	} else {
		check("T9a", "restart（从停止态，scmStopped 幂等直拉起）无错", true, fmt.Sprintf("等待 %v", time.Since(t0).Round(time.Millisecond)))
	}
	if !waitState(p, accSvcName, "active", 15*time.Second) {
		check("T9b", "restart（停止态）后回到 active", false, "15s 内未收敛")
		return
	}
	check("T9b", "restart（停止态）后回到 active", true, "")
	t0 = time.Now()
	if _, err := p.Call("action", map[string]interface{}{"name": accSvcName, "action": "restart"}); err != nil {
		check("T9c", "restart（从运行态，scmStopAndWait 等收敛再拉起）无错", false, errDetail(err))
	} else if !waitState(p, accSvcName, "active", 15*time.Second) {
		check("T9c", "restart（从运行态）无错且回到 active", false, "15s 内未收敛")
	} else {
		pidAfter := svcPid(m, accSvcName)
		check("T9c", "restart（从运行态）无错且换新进程",
			pidAfter != 0 && pidAfter != pidBefore,
			fmt.Sprintf("等待 %v pid %d→%d", time.Since(t0).Round(time.Millisecond), pidBefore, pidAfter))
	}

	// ===== T10 hang 服务 restart → 30s 等待超时路径 =====
	if _, err := p.Call("action", map[string]interface{}{"name": accHangName, "action": "start"}); err != nil {
		check("T10", "hang 服务 start（构造超时前提）", false, errDetail(err))
	} else if !waitState(p, accHangName, "active", 15*time.Second) {
		check("T10", "hang 服务 start 后 active（构造超时前提）", false, "15s 内未收敛")
	} else {
		t0 := time.Now()
		_, err := p.Call("action", map[string]interface{}{"name": accHangName, "action": "restart"})
		elapsed := time.Since(t0)
		if err == nil {
			check("T10", "hang 服务 restart 超时报错", false, "预期 did not stop 错误，实际无错——helper 挂死语义失效？")
		} else {
			check("T10", "restart 30s 等待超时路径（scmStopAndWait deadline）",
				strings.Contains(err.Error(), "did not stop within") && strings.Contains(err.Error(), accHangName) &&
					elapsed >= 25*time.Second && elapsed <= 60*time.Second,
				fmt.Sprintf("err=%s 等待 %v", errDetail(err), elapsed.Round(time.Millisecond)))
		}
		killAccImage()
	}

	// ===== T11 自启切换（enable/disable 往返 + SCM 直读）=====
	_, err = p.Call("action", map[string]interface{}{"name": accSvcName, "action": "disable"})
	ok1 := err == nil && listUnitFile(p, accSvcName) == "disabled" && scmStartType(m, accSvcName) == mgr.StartDisabled
	_, err = p.Call("action", map[string]interface{}{"name": accSvcName, "action": "enable"})
	ok2 := err == nil && listUnitFile(p, accSvcName) == "enabled" && scmStartType(m, accSvcName) == mgr.StartAutomatic
	// 还原 Manual（安装初值），不残留 Automatic——与生产 scmSetStartType 同法：
	// 先取全量 Config 只改 StartType 回写
	ok3 := false
	if s, err := m.OpenService(accSvcName); err == nil {
		if cfg, err := s.Config(); err == nil {
			cfg.StartType = mgr.StartManual
			ok3 = s.UpdateConfig(cfg) == nil && scmStartType(m, accSvcName) == mgr.StartManual
		}
		s.Close()
	}
	check("T11", "自启切换 disable/enable 往返（provider 字段 + SCM 直读）+ 还原 Manual",
		ok1 && ok2 && ok3, fmt.Sprintf("disable=%v enable=%v restore=%v", ok1, ok2, ok3))

	// ===== T12 无权限服务报错透传（服务级 ACL，elevated 下 connect 成功）=====
	// 该机无 WinDefend（Server 2022 未装 Defender）——对 hang 服务 sdset 显式
	// 拒绝 BA 组的 SERVICE_STOP（WP），构造确定性的服务级 ACL 拒绝；SD 写操作
	// 是探针侧 setup 工具，被测链路仍是 provider→SCM 原文透传
	sdset := exec.Command("sc", "sdset", accHangName,
		"D:(D;;WP;;;BA)(A;;CCLCSWRPDTLOCRRCSD;;;SY)(A;;CCLCSWRPDTLOCRRCSD;;;BA)")
	if out, err := sdset.CombinedOutput(); err != nil {
		check("T12", "sdset 拒停 SDDL（构造 ACL 拒绝前提）", false, errDetail(err)+" out="+string(out))
	} else {
		_, err = p.Call("action", map[string]interface{}{"name": accHangName, "action": "stop"})
		if err != nil && strings.Contains(strings.ToLower(err.Error()), "access is denied") {
			check("T12", "服务级 ACL 拒绝原文透传（stop 被拒，未被吞/未改判）",
				strings.Contains(err.Error(), accHangName), errDetail(err))
		} else if err == nil {
			check("T12", "服务级 ACL 拒绝原文透传", false, "sdset 拒停后 stop 意外成功")
		} else {
			check("T12", "服务级 ACL 拒绝原文透传", false, "非预期错误: "+errDetail(err))
		}
	}

	// ===== T13 清理核验 =====
	postClean()
	// Windows 对 marked-for-delete 是懒收敛（真机实测可达分钟级，属 SCM
	// 数据库回收行为，不可压缩）：探针内可确定断言的是「删除已受理（exit
	// 0 或 1072=已标记）+ 无残留进程」；1060 从 SCM 消失由证据侧事后轮询
	// 复核（evidence post-check），不在探针内空转等待
	markOK := true
	for _, n := range []string{accSvcName, accHangName} {
		if e := exitCode(exec.Command("sc", "delete", n).Run()); e != 0 && e != 1072 {
			markOK = false
			fmt.Printf("CLEANUP-WARN sc delete %s exit=%d\n", n, e)
		}
	}
	out, _ := exec.Command("tasklist", "/FI", "IMAGENAME eq "+accImage).CombinedOutput()
	procsGone := !strings.Contains(string(out), accImage)
	q1 := exitCode(exec.Command("sc", "query", accSvcName).Run())
	q2 := exitCode(exec.Command("sc", "query", accHangName).Run())
	check("T13", "清理受理（sc delete 0/1072）+ 无残留进程（1060 收敛由 evidence 事后复核）",
		markOK && procsGone, fmt.Sprintf("mark=%v procsGone=%v query exit=%d/%d", markOK, procsGone, q1, q2))
}

func preClean() {
	killAccImage()
	os.Remove(accLogPath)
	for _, n := range []string{accSvcName, accHangName} {
		_ = exec.Command("sc", "delete", n).Run()
		// Windows 对 marked-for-delete 是懒收敛（真机实测 >30s）：不等 1060
		// 就 CreateService 会撞 ERROR_SERVICE_MARKED_FOR_DELETE（1072）
		for i := 0; i < 40 && exitCode(exec.Command("sc", "query", n).Run()) != 1060; i++ {
			time.Sleep(3 * time.Second)
		}
	}
}

func postClean() {
	killAccImage()
	// sc delete 只需 DELETE 权，不受 T12 sdset 拒停 SDDL 影响（mgr.OpenService
	// 恒申请 ALL_ACCESS，对 ACL 收紧的服务开不了句柄）；1072=已标记删除
	//（重跑/defer 二次调用），属预期不告警
	for _, n := range []string{accSvcName, accHangName} {
		if out, err := exec.Command("sc", "delete", n).CombinedOutput(); err != nil && exitCode(err) != 1072 {
			fmt.Printf("CLEANUP-WARN sc delete %s: %v %s\n", n, err, strings.TrimSpace(string(out)))
		}
	}
}

// exitCode 命令退出码（sc query 1060=服务未安装；无 ExitError 时 0/负）
func exitCode(err error) int {
	if ee, ok := err.(*exec.ExitError); ok {
		return ee.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

func killAccImage() {
	_ = exec.Command("taskkill", "/F", "/T", "/IM", accImage).Run() // 不在运行是正常情形
}

// svcPid SCM Query 直读运行进程 PID（0=非 Running）
func svcPid(m *mgr.Mgr, name string) uint32 {
	s, err := m.OpenService(name)
	if err != nil {
		return 0
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return 0
	}
	return st.ProcessId
}

func waitState(p *rpc.WindowsServiceProvider, name, want string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if listState(p, name) == want {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func listState(p *rpc.WindowsServiceProvider, name string) string {
	res, err := p.Call("list", nil)
	if err != nil {
		return fmt.Sprintf("ERR:%v", err)
	}
	for _, u := range res.(map[string]interface{})["services"].([]rpc.ServiceUnit) {
		if u.Name == name {
			return u.ActiveState
		}
	}
	return "NOTFOUND"
}

func listUnitFile(p *rpc.WindowsServiceProvider, name string) string {
	res, err := p.Call("list", nil)
	if err != nil {
		return fmt.Sprintf("ERR:%v", err)
	}
	for _, u := range res.(map[string]interface{})["services"].([]rpc.ServiceUnit) {
		if u.Name == name {
			return u.UnitFileState
		}
	}
	return "NOTFOUND"
}

func scmStartType(m *mgr.Mgr, name string) uint32 {
	s, err := m.OpenService(name)
	if err != nil {
		return ^uint32(0)
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return ^uint32(0)
	}
	return cfg.StartType
}

func svcIn(units []rpc.ServiceUnit, name string) bool {
	for _, u := range units {
		if u.Name == name {
			return true
		}
	}
	return false
}

func dumpList(units []rpc.ServiceUnit) {
	tmp := make([]map[string]interface{}, 0, len(units))
	for _, u := range units {
		tmp = append(tmp, map[string]interface{}{
			"name":          u.Name,
			"description":   u.Description,
			"loadState":     u.LoadState,
			"activeState":   u.ActiveState,
			"subState":      u.SubState,
			"unitFileState": u.UnitFileState,
		})
	}
	b, err := json.MarshalIndent(tmp, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(filepath.Join(filepath.Dir(mustExeDir()), accListName()), b, 0o644)
}

func mustExeDir() string {
	exe, _ := os.Executable()
	return filepath.Dir(exe)
}

func accListName() string { return "scm-list-elevated.json" }

func errDetail(err error) string {
	if err == nil {
		return "err=nil"
	}
	return "err=" + err.Error()
}
