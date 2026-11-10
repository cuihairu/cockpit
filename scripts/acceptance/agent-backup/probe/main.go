// Agent 文件备份真机探针（acceptance-checklist「备份与恢复（agent 侧）」
// K0-K11，对应 backup-design.md D1-D29）。
//
// 依赖 run-server.sh 起的本地实例（server :19998 + agent abk-acc-a1 +
// webhook 接收器 :9701），全部场景走真实 REST/调度循环/agent 打包恢复/分块
// 下载；夹具与产物落 .acceptance/agent-backup/work/，证据落 evidence/。
//
//	K0 基线（登录/agent 在线/夹具树）
//	K1 创建合法配置（manual/retention=2）
//	K2 校验面（命名/源/目标/调度/retention/ghost agent/rclone 门/恢复双确认）
//	K3 手动运行 → tar.gz 完整（下载与盘上 sha256 对照 + 逐条目比对源树）
//	K4 符号链接不跟随（归档内 symlink 条目 + Linkname 保留）
//	K5 retention 自动清理（retention=2 跑三次 → 产物两件、最旧被清）
//	K6 分块下载（256MB 样本 Content-Length/sha256 对照 + server VmHWM 不炸）
//	K7 恢复双阶段（独立空目录、源树零改动、非空目录拒绝、任务日志可读）
//	K8 Zip Slip 构造样例被拒（越界条目跳过且不落盘）
//	K9 backup.failed 通知送达（webhook 接收器实收 + 运行历史 failed 透传）
//	K10 定时调度到点执行一次（daily@HH:mm，探针全程未调 /run）
//	K11 停机补跑只补一次（停机跨过到点时刻 → 恢复后恰一次，下一窗不重放）
package main

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

var (
	apiBase   = flag.String("api", "http://127.0.0.1:19998", "cockpit API base URL")
	apiUser   = flag.String("user", "admin", "登录用户名")
	apiPass   = flag.String("pass", "e2e-strong-pass-1", "登录密码")
	agentID   = flag.String("agent", "abk-acc-a1", "备份目标 agent ID")
	evDir     = flag.String("ev", ".acceptance/agent-backup/evidence", "证据目录")
	workDir   = flag.String("work", ".acceptance/agent-backup/work", "夹具与产物工作目录")
	hookLog   = flag.String("hook-log", ".acceptance/agent-backup/evidence/webhooks.jsonl", "webhook 接收器收包 JSONL")
	pidFile   = flag.String("server-pid", ".acceptance/agent-backup/instance/server.pid", "server pid 文件（读 VmHWM）")
	restartCm = flag.String("restart-cmd", "scripts/acceptance/agent-backup/restart-server.sh 110", "停机重启命令（sh -c 执行，含停机时长）")
	pollStep  = flag.Duration("poll-step", 2*time.Second, "轮询步长")
	maxWait   = flag.Duration("max-wait", 5*time.Minute, "单条件等待上限")
)

// bigFileSize 分块下载样本（随机内容不可压缩，归档体积≈256MB → 千级分块）
const bigFileSize = 256 << 20

// hwmLimitKB server VmHWM 上限（256MB 缓冲整文件必然越过，正常流式基线远低）
const hwmLimitKB = 300 * 1024

const (
	evFileEvidence = "summary.json"
)

// ---------- 场景编排 ----------

func main() {
	flag.Parse()
	// work 目录进配置的 sources/dest_dir，必须是绝对路径（server/agent 双端校验）
	if !filepath.IsAbs(*workDir) {
		if abs, aerr := filepath.Abs(*workDir); aerr == nil {
			*workDir = abs
		}
	}
	if err := os.MkdirAll(*evDir, 0o700); err != nil {
		fmt.Println("mkdir evidence:", err)
		osExit(1)
	}
	f, err := os.Create(filepath.Join(*evDir, "probe.log"))
	if err != nil {
		fmt.Println("create evidence:", err)
		osExit(1)
	}
	evMu.Lock()
	evF = f
	evMu.Unlock()
	defer f.Close()
	ev("== Agent 文件备份真机验收 %s ==", time.Now().Format(time.RFC3339))

	// ---------- K0 基线 ----------
	deadline := time.Now().Add(*maxWait)
	for {
		code, _ := req(http.MethodGet, "/health", nil, "")
		if code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			fatal(2, "K0 health 不可达")
		}
		time.Sleep(*pollStep)
	}
	tk, err := loginAs(*apiUser, *apiPass)
	if err != nil {
		fatal(2, "K0 登录失败: %v", err)
	}
	evMu.Lock()
	token = tk
	evMu.Unlock()

	online := false
	deadline = time.Now().Add(90 * time.Second)
	for !online && time.Now().Before(deadline) {
		code, raw := req(http.MethodGet, "/api/agents", nil, token)
		if code == http.StatusOK && bytes.Contains(raw, []byte(`"`+*agentID+`"`)) &&
			bytes.Contains(raw, []byte(`"online"`)) {
			online = true
			break
		}
		time.Sleep(*pollStep)
	}
	check("K0 agent 在线", online, *agentID)
	if !online {
		fatal(2, "K0 agent 未在线，后续场景无意义")
	}
	code, raw := req(http.MethodGet, "/api/backups/configs", nil, token)
	check("K0 配置列表基线为空", code == http.StatusOK && len(mustList(raw)) == 0,
		fmt.Sprintf("HTTP %d", code))

	// 夹具树：普通文件 + 嵌套 + 两个符号链接（相对 + 绝对）
	if err := os.RemoveAll(*workDir); err != nil {
		fatal(2, "清 work 目录: %v", err)
	}
	src := filepath.Join(*workDir, "src")
	nested := filepath.Join(src, "nested", "sub")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		fatal(2, "建夹具目录: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "readme.txt"), []byte("cockpit-agent-backup fixture\n"), 0o644); err != nil {
		fatal(2, "写 readme: %v", err)
	}
	data := make([]byte, 4096)
	if _, err := rand.Read(data); err != nil {
		fatal(2, "写 data.bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(src, "data.bin"), data, 0o644); err != nil {
		fatal(2, "写 data.bin: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "deep.txt"), []byte("deep\n"), 0o644); err != nil {
		fatal(2, "写 deep.txt: %v", err)
	}
	if err := os.Symlink("readme.txt", filepath.Join(src, "link-to-readme")); err != nil {
		fatal(2, "建相对链接: %v", err)
	}
	if err := os.Symlink("/etc/hostname", filepath.Join(src, "link-to-abs")); err != nil {
		fatal(2, "建绝对链接: %v", err)
	}
	srcSnap, err := buildTree(src, "src")
	if err != nil {
		fatal(2, "源树快照: %v", err)
	}
	check("K0 夹具树就绪", len(srcSnap) == 8, fmt.Sprintf("entries=%d", len(srcSnap)))

	destDir := filepath.Join(*workDir, "dest")

	// ---------- K1 创建合法配置 ----------
	cfgOK, err := newConfig("okset", []string{src}, destDir, "manual", 2)
	if err != nil {
		fatal(2, "K1 创建配置: %v", err)
	}
	check("K1 创建配置（manual/retention=2）", cfgOK.ID > 0 && cfgOK.NextRunAt == 0 && cfgOK.Enabled,
		fmt.Sprintf("id=%d next_run_at=%d retention=%d", cfgOK.ID, cfgOK.NextRunAt, cfgOK.Retention))

	// ---------- K2 校验面 ----------
	type badReq struct {
		label string
		body  map[string]interface{}
	}
	matrix := []badReq{
		{"大写名拒绝", map[string]interface{}{"name": "BadName", "sources": []string{src}, "dest_dir": destDir, "schedule": "manual"}},
		{"空 sources 拒绝", map[string]interface{}{"name": "t2", "sources": []string{}, "dest_dir": destDir, "schedule": "manual"}},
		{"相对 source 拒绝", map[string]interface{}{"name": "t3", "sources": []string{"rel/path"}, "dest_dir": destDir, "schedule": "manual"}},
		{"相对 dest 拒绝", map[string]interface{}{"name": "t4", "sources": []string{src}, "dest_dir": "dest", "schedule": "manual"}},
		{"未知调度拒绝", map[string]interface{}{"name": "t5", "sources": []string{src}, "dest_dir": destDir, "schedule": "weekly@1"}},
		{"every:0h 拒绝", map[string]interface{}{"name": "t6", "sources": []string{src}, "dest_dir": destDir, "schedule": "every:0h"}},
		{"every:169h 拒绝", map[string]interface{}{"name": "t7", "sources": []string{src}, "dest_dir": destDir, "schedule": "every:169h"}},
		{"retention 负值拒绝", map[string]interface{}{"name": "t8", "sources": []string{src}, "dest_dir": destDir, "schedule": "manual", "retention": -1}},
		{"retention 366 拒绝", map[string]interface{}{"name": "t9", "sources": []string{src}, "dest_dir": destDir, "schedule": "manual", "retention": 366}},
		{"ghost agent 拒绝", map[string]interface{}{"agent_id": "abk-ghost", "name": "t10", "sources": []string{src}, "dest_dir": destDir, "schedule": "manual"}},
		{"remote_dest 语法拒绝", map[string]interface{}{"name": "t11", "sources": []string{src}, "dest_dir": destDir, "schedule": "manual", "remote_dest": "bad dest"}},
		{"remote_dest 缺 rclone 配置期拦截", map[string]interface{}{"name": "t12", "sources": []string{src}, "dest_dir": destDir, "schedule": "manual", "remote_dest": "fake-remote:cockpit/back"}},
	}
	k2ok := 0
	for _, c := range matrix {
		body := c.body
		if _, hasAgent := body["agent_id"]; !hasAgent {
			body["agent_id"] = *agentID
		}
		if body["name"] == nil {
			body["name"] = "x"
		}
		code, _ := req(http.MethodPost, "/api/backups/configs", body, token)
		if code == http.StatusBadRequest {
			k2ok++
		} else {
			ev("  K2 %s → HTTP %d（期望 400）", c.label, code)
		}
	}
	check("K2 配置校验矩阵 12/12", k2ok == len(matrix), fmt.Sprintf("%d/%d", k2ok, len(matrix)))

	// 恢复/删除/下载的参数校验（先于 agent 调用）
	code, _ = req(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/restore", cfgOK.ID),
		map[string]string{"file": "x.tar.gz", "dest_dir": filepath.Join(*workDir, "r1"), "confirm_name": "y.tar.gz"}, token)
	check("K2 恢复 confirm 不匹配 400", code == http.StatusBadRequest, fmt.Sprintf("HTTP %d", code))
	code, _ = req(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/restore", cfgOK.ID),
		map[string]string{"file": "x.tar.gz", "dest_dir": "relative", "confirm_name": "x.tar.gz"}, token)
	check("K2 恢复相对 dest 400", code == http.StatusBadRequest, fmt.Sprintf("HTTP %d", code))
	code, _ = req(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/restore", cfgOK.ID),
		map[string]string{"file": "x.tar.gz", "dest_dir": "/", "confirm_name": "x.tar.gz"}, token)
	check("K2 恢复 / 拒绝", code == http.StatusBadRequest, fmt.Sprintf("HTTP %d", code))
	code, _ = req(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/files/delete", cfgOK.ID),
		map[string]string{"name": "a/b.tar.gz"}, token)
	check("K2 删除路径穿越名 400", code == http.StatusBadRequest, fmt.Sprintf("HTTP %d", code))
	code, _ = req(http.MethodGet,
		fmt.Sprintf("/api/backups/configs/%d/files/download?name=%s", cfgOK.ID, url.QueryEscape("../evil.tar.gz")), nil, token)
	check("K2 下载穿越名 400", code == http.StatusBadRequest, fmt.Sprintf("HTTP %d", code))

	// ---------- K3 手动运行 + tar.gz 完整 ----------
	if err := runConfig(cfgOK.ID); err != nil {
		fatal(2, "K3 运行: %v", err)
	}
	run1, err := waitTerminalRun(cfgOK.ID, time.Now().Add(*maxWait))
	check("K3 运行 success", err == nil && run1.Status == "success",
		fmt.Sprintf("status=%s err=%v", run1.Status, err))
	if run1.Status != "success" {
		fatal(2, "K3 运行失败: %s", run1.Error)
	}
	files, err := filesOf(cfgOK.ID)
	fname := ""
	if err == nil && len(files) == 1 {
		fname = files[0].Name
	}
	check("K3 产物一件且与历史一致", err == nil && len(files) == 1 && fname == run1.File,
		fmt.Sprintf("files=%d name=%s run.file=%s", len(files), fname, run1.File))
	diskPath := filepath.Join(destDir, fname)
	diskSHA, err := sha256File(diskPath)
	check("K3 盘上产物可读", err == nil, truncate(diskPath, 80))

	var arcBuf bytes.Buffer
	status, clen, dlSHA, err := downloadBackup(cfgOK.ID, fname, &arcBuf)
	check("K3 下载成功", err == nil && status == http.StatusOK && clen == int64(arcBuf.Len()),
		fmt.Sprintf("HTTP %d len=%d", status, clen))
	check("K3 下载 sha256 与盘上一致", err == nil && dlSHA == diskSHA && diskSHA != "",
		truncate(diskSHA, 20))

	arc, err := extractArchive(arcBuf.Bytes())
	var probs []string
	if err != nil {
		probs = []string{err.Error()}
	} else {
		probs = diffEntries(srcSnap, arc)
	}
	check("K3 归档逐条目与源树一致", err == nil && len(probs) == 0,
		func() string {
			if len(probs) > 0 {
				return truncate(strings.Join(probs, "; "), 200)
			}
			return fmt.Sprintf("%d entries", len(srcSnap))
		}())

	// ---------- K4 符号链接不跟随 ----------
	if arc == nil {
		check("K4 符号链接条目", false, "归档解析失败")
	} else {
		l1, ok1 := arc["src/link-to-readme"]
		l2, ok2 := arc["src/link-to-abs"]
		check("K4 相对链接记为 symlink", ok1 && l1.Kind == "sym" && l1.Link == "readme.txt",
			fmt.Sprintf("%+v", l1))
		check("K4 绝对链接 Linkname 原样保留", ok2 && l2.Kind == "sym" && l2.Link == "/etc/hostname",
			fmt.Sprintf("%+v", l2))
		rm, ok := arc["src/readme.txt"]
		check("K4 链接目标内容仅存一份", ok && rm.Kind == "reg" && rm.SHA == srcSnap["src/readme.txt"].SHA,
			"readme 唯一 reg 条目且内容一致")
	}

	// ---------- K5 retention 自动清理 ----------
	if err := runConfig(cfgOK.ID); err != nil {
		fatal(2, "K5 运行 #2: %v", err)
	}
	run2, err := waitTerminalRun(cfgOK.ID, time.Now().Add(*maxWait))
	check("K5 第二次运行 success", err == nil && run2.Status == "success" && run2.File != run1.File,
		fmt.Sprintf("file=%s", run2.File))
	if err := runConfig(cfgOK.ID); err != nil {
		fatal(2, "K5 运行 #3: %v", err)
	}
	run3, err := waitTerminalRun(cfgOK.ID, time.Now().Add(*maxWait))
	check("K5 第三次运行 success", err == nil && run3.Status == "success",
		fmt.Sprintf("file=%s", run3.File))
	files, err = filesOf(cfgOK.ID)
	inList := func(n string) bool {
		for _, fl := range files {
			if fl.Name == n {
				return true
			}
		}
		return false
	}
	prunedGone := false
	if _, serr := os.Stat(filepath.Join(destDir, run1.File)); serr != nil {
		prunedGone = true
	}
	check("K5 retention=2 只留两件", err == nil && len(files) == 2,
		fmt.Sprintf("files=%d", len(files)))
	check("K5 最旧一件被清、最新保留", err == nil && inList(run3.File) && !inList(run1.File) && prunedGone,
		fmt.Sprintf("run1在盘=%v run3在列表=%v", !prunedGone, inList(run3.File)))

	// ---------- K6 分块下载（256MB 样本） ----------
	bigPath := filepath.Join(*workDir, "big.bin")
	bf, err := os.Create(bigPath)
	if err != nil {
		fatal(2, "K6 建大样本: %v", err)
	}
	if _, err := io.CopyN(bf, rand.Reader, bigFileSize); err != nil {
		bf.Close()
		fatal(2, "K6 写大样本: %v", err)
	}
	bf.Close()
	cfgBig, err := newConfig("bigset", []string{bigPath}, filepath.Join(*workDir, "dest-big"), "manual", 0)
	if err != nil {
		fatal(2, "K6 创建配置: %v", err)
	}
	if err := runConfig(cfgBig.ID); err != nil {
		fatal(2, "K6 运行: %v", err)
	}
	runBig, err := waitTerminalRun(cfgBig.ID, time.Now().Add(5*time.Minute))
	check("K6 大包运行 success", err == nil && runBig.Status == "success",
		fmt.Sprintf("status=%s err=%v size=%d", runBig.Status, err, runBig.Size))
	hwmBefore, errB := readHWM(*pidFile)
	status, clen, bigSHA, err := downloadBackup(cfgBig.ID, runBig.File, nil)
	diskBig, _ := sha256File(filepath.Join(filepath.Join(*workDir, "dest-big"), runBig.File))
	hwmAfter, errA := readHWM(*pidFile)
	check("K6 Content-Length=文件真长", err == nil && status == http.StatusOK && clen == runBig.Size,
		fmt.Sprintf("len=%d size=%d", clen, runBig.Size))
	check("K6 下载 sha256 与盘上一致", err == nil && bigSHA == diskBig && diskBig != "",
		truncate(bigSHA, 20))
	check("K6 server VmHWM 未随大包增长", errB == nil && errA == nil && hwmBefore < hwmLimitKB && hwmAfter < hwmLimitKB,
		fmt.Sprintf("before=%dkB after=%dkB limit=%dkB chunks=%d",
			hwmBefore, hwmAfter, hwmLimitKB, (runBig.Size+262143)/262144))

	// ---------- K7 恢复双阶段 ----------
	// 用最新产物（run1 已被 K5 retention 清掉——恢复对象必须在盘上）
	restoreDir := filepath.Join(*workDir, "restore-7")
	taskID, err := startRestore(cfgOK.ID, run3.File, restoreDir, run3.File)
	check("K7 恢复任务受理", err == nil, truncate(taskID, 24))
	if err != nil {
		fatal(2, "K7 恢复启动: %v", err)
	}
	t, err := waitTerminalTask(cfgOK.ID, taskID, time.Now().Add(*maxWait))
	check("K7 恢复任务 success 且日志可读", err == nil && t.Status == "success" &&
		strings.Contains(t.Log, "[restore] done"),
		truncate(strings.ReplaceAll(t.Log, "\n", " | "), 200))
	restored, err := buildTree(restoreDir, "")
	if err != nil {
		probs = []string{err.Error()}
	} else {
		probs = diffEntries(srcSnap, restored)
	}
	check("K7 独立目录恢复与源树一致", err == nil && len(probs) == 0,
		func() string {
			if len(probs) > 0 {
				return truncate(strings.Join(probs, "; "), 200)
			}
			return "restored=" + filepath.Base(restoreDir)
		}())
	srcSnap2, err := buildTree(src, "src")
	check("K7 源目录零改动", err == nil && len(diffEntries(srcSnap, srcSnap2)) == 0,
		fmt.Sprintf("entries=%d", len(srcSnap2)))
	nonEmpty := filepath.Join(*workDir, "restore-nonempty")
	_ = os.MkdirAll(nonEmpty, 0o700)
	_ = os.WriteFile(filepath.Join(nonEmpty, "keep.txt"), []byte("x"), 0o600)
	code, raw = req(http.MethodPost, fmt.Sprintf("/api/backups/configs/%d/restore", cfgOK.ID),
		map[string]string{"file": run3.File, "dest_dir": nonEmpty, "confirm_name": run3.File}, token)
	check("K7 非空目录拒绝且原因透传", code == http.StatusBadGateway && strings.Contains(string(raw), "not empty"),
		fmt.Sprintf("HTTP %d body=%s", code, truncate(string(raw), 80)))

	// ---------- K8 Zip Slip ----------
	evilPath := filepath.Join(destDir, "evil.tar.gz")
	if err := writeEvilTar(evilPath); err != nil {
		fatal(2, "K8 构造恶意归档: %v", err)
	}
	restoreEvil := filepath.Join(*workDir, "restore-evil")
	esc1 := escapeTarget(restoreEvil, evilEscapeEntries[0])
	esc2 := escapeTarget(restoreEvil, evilEscapeEntries[1])
	taskID, err = startRestore(cfgOK.ID, "evil.tar.gz", restoreEvil, "evil.tar.gz")
	if err != nil {
		fatal(2, "K8 恢复启动: %v", err)
	}
	t, err = waitTerminalTask(cfgOK.ID, taskID, time.Now().Add(*maxWait))
	_, okOK := os.Stat(filepath.Join(restoreEvil, evilOKEntry))
	okContent, _ := os.ReadFile(filepath.Join(restoreEvil, evilOKEntry))
	_, e1err := os.Stat(esc1)
	_, e2err := os.Stat(esc2)
	check("K8 恶意归档恢复完成（越界跳过不中断）", err == nil && t.Status == "success" &&
		strings.Contains(t.Log, "skip unsafe entry"),
		truncate(strings.ReplaceAll(t.Log, "\n", " | "), 200))
	check("K8 正常条目照常恢复", okOK == nil && string(okContent) == "safe-content",
		fmt.Sprintf("content=%q", truncate(string(okContent), 30)))
	check("K8 越界条目未落盘", os.IsNotExist(e1err) && os.IsNotExist(e2err),
		fmt.Sprintf("t1=%s t2=%s", truncate(esc1, 60), truncate(esc2, 60)))
	_ = os.Remove(evilPath)

	// ---------- K9 backup.failed 通知 ----------
	cfgBad, err := newConfig("badset", []string{"/nonexistent-agent-backup-src-xyz"},
		filepath.Join(*workDir, "dest-bad"), "manual", 0)
	if err != nil {
		fatal(2, "K9 创建配置: %v", err)
	}
	if err := runConfig(cfgBad.ID); err != nil {
		fatal(2, "K9 运行: %v", err)
	}
	runBad, err := waitTerminalRun(cfgBad.ID, time.Now().Add(*maxWait))
	check("K9 失败运行透传历史", err == nil && runBad.Status == "failed" &&
		strings.Contains(runBad.Error, "no source could be archived"),
		truncate(runBad.Error, 120))
	wantRes := fmt.Sprintf(`"resource_id":"%d"`, cfgBad.ID)
	gotHook := false
	var hookDetail string
	deadline = time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		hooks, herr := readJSONL(*hookLog)
		if herr == nil {
			for _, h := range hooks {
				if strings.Contains(h.Body, `"event_type":"backup.failed"`) &&
					strings.Contains(h.Body, wantRes) &&
					strings.Contains(h.Body, `"level":"error"`) {
					gotHook = true
					hookDetail = h.Body
				}
			}
		}
		if gotHook {
			break
		}
		time.Sleep(*pollStep)
	}
	check("K9 webhook 实收 backup.failed", gotHook, truncate(hookDetail, 200))

	// ---------- K10 定时调度到点执行一次 ----------
	tCreate := time.Now()
	due := dueOf(tCreate, 2)
	cfgSched, err := newConfig("sched", []string{src}, destDir, dailyAt(tCreate, 2), 1)
	if err != nil {
		fatal(2, "K10 创建调度配置: %v", err)
	}
	time.Sleep(5 * time.Second)
	runs, _ := runsOf(cfgSched.ID)
	check("K10 到点前不提前触发", len(runs) == 0, fmt.Sprintf("runs=%d due=%s", len(runs), due.Format("15:04:05")))
	runSched, err := waitTerminalRun(cfgSched.ID, due.Add(3*time.Minute))
	check("K10 daily@ 到点自动执行一次", err == nil && runSched.Status == "success" &&
		runSched.StartedAt >= due.Unix(),
		fmt.Sprintf("status=%s startedAt=%d due=%d", runSched.Status, runSched.StartedAt, due.Unix()))
	// 探针全程未对该配置调 /run（代码路径保证），触发来源=调度循环
	cfgAfter, err := configByID(cfgSched.ID)
	wantNext := due.Add(24 * time.Hour).Unix()
	check("K10 next_run_at 推进到次日同点", err == nil && cfgAfter != nil &&
		absDiff(cfgAfter.NextRunAt, wantNext) <= 600,
		func() string {
			if cfgAfter == nil {
				return "config parse failed"
			}
			return fmt.Sprintf("next=%d want≈%d", cfgAfter.NextRunAt, wantNext)
		}())

	// ---------- K11 停机补跑只补一次 ----------
	tCreate2 := time.Now()
	due2 := dueOf(tCreate2, 2)
	cfgCatch, err := newConfig("catchup", []string{src}, destDir, dailyAt(tCreate2, 2), 1)
	if err != nil {
		fatal(2, "K11 创建补跑配置: %v", err)
	}
	// 到点前 90s 重启 server，停机窗（110s+启动）跨过 due2
	time.Sleep(30 * time.Second)
	// 重启期间 token 不变（JWT secret 固定），健康与在线由探针重新确认
	cmd := exec.Command("sh", "-c", *restartCm)
	out, err := cmd.CombinedOutput()
	check("K11 server 停机重启完成", err == nil, truncate(strings.TrimSpace(string(out)), 120))
	if err != nil {
		fatal(2, "K11 重启失败: %v", err)
	}
	deadline = time.Now().Add(90 * time.Second)
	for {
		code, _ := req(http.MethodGet, "/health", nil, "")
		if code == http.StatusOK {
			break
		}
		if time.Now().After(deadline) {
			fatal(2, "K11 重启后 health 不可达")
		}
		time.Sleep(*pollStep)
	}
	deadline = time.Now().Add(120 * time.Second)
	for {
		code, raw := req(http.MethodGet, "/api/agents", nil, token)
		if code == http.StatusOK && bytes.Contains(raw, []byte(`"`+*agentID+`"`)) &&
			bytes.Contains(raw, []byte(`"online"`)) {
			break
		}
		if time.Now().After(deadline) {
			fatal(2, "K11 agent 未重新在线")
		}
		time.Sleep(*pollStep)
	}
	ev("K11 恢复在线，due2=%s（停机窗已跨过）", due2.Format("15:04:05"))
	runCatch, err := waitTerminalRun(cfgCatch.ID, time.Now().Add(*maxWait))
	runsAfter, _ := runsOf(cfgCatch.ID)
	check("K11 停机跨过后补跑恰一次", err == nil && runCatch.Status == "success" &&
		runCatch.StartedAt >= due2.Unix() && len(runsAfter) == 1,
		fmt.Sprintf("status=%s runs=%d startedAt=%d due=%d", runCatch.Status, len(runsAfter), runCatch.StartedAt, due2.Unix()))
	cfgCatchAfter, _ := configByID(cfgCatch.ID)
	wantNext2 := due2.Add(24 * time.Hour).Unix()
	check("K11 next_run_at 推进到次日同点", cfgCatchAfter != nil &&
		absDiff(cfgCatchAfter.NextRunAt, wantNext2) <= 600,
		func() string {
			if cfgCatchAfter == nil {
				return "config parse failed"
			}
			return fmt.Sprintf("next=%d want≈%d", cfgCatchAfter.NextRunAt, wantNext2)
		}())
	// 再观察一个完整扫描周期：补跑不得重放
	time.Sleep(75 * time.Second)
	runsAfter, _ = runsOf(cfgCatch.ID)
	check("K11 下一扫描窗不重放", len(runsAfter) == 1, fmt.Sprintf("runs=%d", len(runsAfter)))

	// ---------- 汇总 ----------
	raw, _ = json.Marshal(map[string]int{"passes": passes, "fails": fails})
	saveEV(evFileEvidence, raw)
	ev("== 汇总：PASS %d / FAIL %d ==", passes, fails)
	if fails > 0 {
		osExit(1)
	}
	osExit(0)
}

// ---------- 小工具 ----------

func mustList(raw []byte) []backupConfig {
	list, _ := parseConfigList(raw)
	return list
}

func absDiff(a, b int64) int64 {
	if a > b {
		return a - b
	}
	return b - a
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
