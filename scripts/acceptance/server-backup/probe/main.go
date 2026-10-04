// 面板数据库备份（server 侧）真机验收探针（server-backup-design.md，
// acceptance-checklist「面板数据库备份」节）。以面板用户身份走真实链路
// （REST + 目录产物 + sqlite3 重开抽查），双实例并行：
//
//	a  :19995  retention_days=1（按天清理实证）
//	b  :19996  retention_days=0（0=永久 形态）
//
//	B0  双实例登录；config GET 基线（新鲜 DB → 缺省 24h/7d）
//	B1  config PUT：a=1h/1d，b=1h/0d；回读一致
//	B2  植入「上轮遗留」老文件（合法文件名 + ModTime 2026-07-01），
//	    列表 API 可见（严格文件名校验放行、进 latestServerBackupAt 解析）
//	B3  定时触发等待：轮询列表直至两实例各出现非植入产物——探针全程未调
//	    POST /run，新产物只能来自 serverBackupLoop 的 1h tick（最新文件名
//	    为植入老件 → since≈96d ≥ 1h，首个 tick 必跑）
//	B4  retention：a 的老文件被清（ModTime < cutoff=now-1d）；b 的老文件
//	    仍在（0=永久，cleanup 首行 return）——同一次 tick 双形态
//	B5  sqlite3 重开定时产物：integrity_check=ok；users/audit_logs/settings
//	    核心表在；行数抽查 ≥1；体积对照线上 db（紧凑完整副本）
//	B6  手动 POST /run（a）→ 产物落盘；download 与盘上文件 sha256 一致；
//	    sqlite3 重开亦完整
//	B7  手动 POST /run（b）→ 列表恰三件（老+定时+手动），0=永久 在手动
//	    路径下同样成立
//	B8  校验面：interval/retention 越界 400；未知名 download/delete 404
//	B9  审计：POST /run（create）、config PUT（update）、download（export）
//	    各落 server_backup 审计条目
//
// retention 两种形态都只能由 loop tick 之后的 cleanupServerBackups 观测
// （手动 /run 只跑 runServerBackup 不清 retention），单实例串行要等两次
// tick（2h+）；双实例并行则一次等待同证两行。
//
// 证据落 .acceptance/server-backup/evidence/（probe.log + config-*.json +
// list-*.json + run-*.json + sqlite-*.txt + sha256-*.txt + audit.json）；
// FAIL → exit 1。
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var (
	apiA      = flag.String("api-a", "http://127.0.0.1:19995", "实例 a（retention=1）基址")
	apiB      = flag.String("api-b", "http://127.0.0.1:19996", "实例 b（retention=0）基址")
	dirA      = flag.String("dir-a", ".acceptance/server-backup/instance-a/data/server-backups", "实例 a 备份目录")
	dirB      = flag.String("dir-b", ".acceptance/server-backup/instance-b/data/server-backups", "实例 b 备份目录")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/server-backup/evidence）")
	maxWait   = flag.Duration("max-wait", 80*time.Minute, "定时产物最长等待")
	pollStep  = flag.Duration("poll-step", 30*time.Second, "定时产物轮询步长")
)

var (
	evFile = (*os.File)(nil)
	evMu   sync.Mutex
	passes int
	fails  int
	httpC  = &http.Client{Timeout: 30 * time.Second}
)

// fakeOldName 植入老件的文件名：严格模式形态（能过服务端校验进列表），
// 时间戳 2026-07-01 → latestServerBackupAt 解析出 since≈96d，首个 tick
// 必触发；ModTime 同步到该日 → retention cutoff（now-1d）之外
const fakeOldName = "cockpit-20260701-000000.db"

// 行为中性注入点（先例：jobs/probe pgrepCmd）——fatal 退出与真机 sqlite3
// 命令面在单测注入桩覆盖分支，默认值即原行为
var (
	osExit = os.Exit
	// sqlite3Cmd 需真 sqlite3 二进制（checklist 要求产物重开抽查）
	sqlite3Cmd = func(db, sql string) *exec.Cmd {
		return exec.Command("sqlite3", db, sql)
	}
)

// ---------- 证据 ----------

func ev(format string, args ...interface{}) {
	evMu.Lock()
	defer evMu.Unlock()
	line := fmt.Sprintf(format, args...)
	fmt.Println(line)
	if evFile != nil {
		fmt.Fprintf(evFile, "%s\n", line)
	}
}

func check(name string, ok bool, detail string) {
	status := "PASS"
	if !ok {
		status = "FAIL"
		fails++
	} else {
		passes++
	}
	ev("[%s] %s — %s", status, name, detail)
}

func fatal(format string, args ...interface{}) {
	ev("[FATAL] "+format, args...)
	osExit(2)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// saveEV 场景原始响应落证据目录
func saveEV(name string, raw []byte) {
	if *evDir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(*evDir, name), raw, 0o644)
}

// ---------- cockpit REST ----------

// req 显式带 token（双实例各持一份，不设包级 token）
func req(method, url string, body interface{}, tok string) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	httpReq, _ := http.NewRequest(method, url, rd)
	httpReq.Header.Set("Content-Type", "application/json")
	if tok != "" {
		httpReq.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := httpC.Do(httpReq)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// loginAs 登录取 token
func loginAs(base, user, pass string) (string, error) {
	code, raw := req(http.MethodPost, base+"/api/auth/login",
		map[string]string{"username": user, "password": pass}, "")
	if code != 200 {
		return "", fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var r struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Token == "" {
		return "", fmt.Errorf("响应无 token: %s", truncate(string(raw), 200))
	}
	return r.Token, nil
}

// ---------- config 响应解析 ----------

// parseConfigResp config GET/PUT 响应 → 当前生效 interval/retention
func parseConfigResp(raw []byte) (int, int, error) {
	var r struct {
		IntervalHours int    `json:"interval_hours"`
		RetentionDays int    `json:"retention_days"`
		RemoteDest    string `json:"remote_dest"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return 0, 0, fmt.Errorf("bad config resp: %s", truncate(string(raw), 200))
	}
	return r.IntervalHours, r.RetentionDays, nil
}

// ---------- 备份列表解析 ----------

type backupEntry struct {
	Name    string    `json:"name"`
	Size    int64     `json:"size"`
	ModTime time.Time `json:"modTime"`
}

// parseBackups 列表响应 {data:[...]} → 条目切片（文件名倒序 = 最新在前）
func parseBackups(raw []byte) ([]backupEntry, error) {
	var r struct {
		Data []backupEntry `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("bad backups list: %s", truncate(string(raw), 200))
	}
	return r.Data, nil
}

// findBackup 按文件名查条目
func findBackup(list []backupEntry, name string) *backupEntry {
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

// listHas 列表中是否存在指定文件名
func listHas(list []backupEntry, name string) bool {
	return findBackup(list, name) != nil
}

// backupNames 文件名列表（证据行用）
func backupNames(list []backupEntry) []string {
	names := make([]string, 0, len(list))
	for _, b := range list {
		names = append(names, b.Name)
	}
	return names
}

// firstOtherName 第一个非 exclude 的文件名（定时/手动产物识别：植入老件
// 文件名固定，本轮新产物文件名必然不同）
func firstOtherName(list []backupEntry, exclude string) string {
	for _, b := range list {
		if b.Name != exclude {
			return b.Name
		}
	}
	return ""
}

// ---------- 老文件植入 / 完整性对照 ----------

// plantFakeOld 植入一份「上轮遗留」备份：合法文件名 + 久远 ModTime。
// 内容为任意字节——列表/清理/定时判断只看文件名与 ModTime，不读内容。
func plantFakeOld(dir, name string, mod time.Time) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("acceptance leftover\n"), 0o600); err != nil {
		return err
	}
	return os.Chtimes(path, mod, mod)
}

// fileSHA256 下载对照用（download 与盘上文件逐字节一致）
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------- sqlite3 产物抽查 ----------

// sqliteQuery sqlite3 CLI 查询（输出 TrimSpace；错误含 stderr 摘要）
func sqliteQuery(db, sql string) (string, error) {
	out, err := sqlite3Cmd(db, sql).CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("sqlite3 %s: %v: %s",
			filepath.Base(db), err, truncate(string(out), 200))
	}
	return strings.TrimSpace(string(out)), nil
}

// ---------- 审计解析 ----------

type auditEntry struct {
	Username   string `json:"username"`
	Action     string `json:"action"`
	Resource   string `json:"resource"`
	ResourceID string `json:"resource_id"`
	Details    string `json:"details"`
}

// parseAuditEntries 审计列表响应 {data:[...], pagination:...} → 条目切片
func parseAuditEntries(raw []byte) ([]auditEntry, error) {
	var r struct {
		Data []auditEntry `json:"data"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("bad audit list: %v", err)
	}
	return r.Data, nil
}

// countAudit 满足 action+resource_id 的审计条目（B9 断言恰一条/≥1）
func countAudit(entries []auditEntry, action, resourceID string) []auditEntry {
	var out []auditEntry
	for _, e := range entries {
		if e.Action == action && e.ResourceID == resourceID {
			out = append(out, e)
		}
	}
	return out
}

// ---------- 场景编排 ----------

func main() {
	flag.Parse()
	if *evDir == "" {
		*evDir = ".acceptance/server-backup/evidence"
	}
	if err := os.MkdirAll(*evDir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, "evidence dir:", err)
		osExit(2)
	}
	f, err := os.OpenFile(filepath.Join(*evDir, "probe.log"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fmt.Fprintln(os.Stderr, "probe.log:", err)
		osExit(2)
	}
	evFile = f
	defer f.Close()

	start := time.Now()
	ev("== 面板数据库备份真机验收 %s ==", start.Format(time.RFC3339))

	tokA := mustLogin(*apiA)
	tokB := mustLogin(*apiB)

	// fetchListEV 拉当前备份列表（raw 可选落证据）
	fetchListEV := func(base, tok, evName string) ([]backupEntry, error) {
		code, raw := req(http.MethodGet, base+"/api/server-backups", nil, tok)
		if code != 200 {
			return nil, fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
		}
		if evName != "" {
			saveEV(evName, raw)
		}
		return parseBackups(raw)
	}

	// ---------- B0 config 基线 ----------
	type instRef struct {
		tag, base, tok string
	}
	instances := []instRef{{"a", *apiA, tokA}, {"b", *apiB, tokB}}
	for _, inst := range instances {
		code, raw := req(http.MethodGet, inst.base+"/api/server-backups/config", nil, inst.tok)
		if code != 200 {
			fatal("B0 config GET (%s): HTTP %d %s", inst.tag, code, truncate(string(raw), 200))
		}
		interval, retention, err := parseConfigResp(raw)
		if err != nil {
			fatal("B0 config 解析 (%s): %v", inst.tag, err)
		}
		saveEV("config-baseline-"+inst.tag+".json", raw)
		check(fmt.Sprintf("B0 config 基线（%s）", inst.tag), interval == 24 && retention == 7,
			fmt.Sprintf("interval=%d retention=%d（新鲜 DB 缺省 24h/7d）", interval, retention))
	}

	// ---------- B1 config PUT ----------
	putCfg := func(inst instRef, interval, retention int) (int, int) {
		code, raw := req(http.MethodPut, inst.base+"/api/server-backups/config",
			map[string]int{"interval_hours": interval, "retention_days": retention}, inst.tok)
		if code != 200 {
			fatal("B1 config PUT (%s): HTTP %d %s", inst.tag, code, truncate(string(raw), 200))
		}
		gi, gr, err := parseConfigResp(raw)
		if err != nil {
			fatal("B1 config 回读 (%s): %v", inst.tag, err)
		}
		saveEV("config-put-"+inst.tag+".json", raw)
		return gi, gr
	}
	ivA, rtA := putCfg(instances[0], 1, 1)
	ivB, rtB := putCfg(instances[1], 1, 0)
	check("B1 config 生效（a: 1h/1d）", ivA == 1 && rtA == 1,
		fmt.Sprintf("回读 interval=%d retention=%d", ivA, rtA))
	check("B1 config 生效（b: 1h/0d）", ivB == 1 && rtB == 0,
		fmt.Sprintf("回读 interval=%d retention=%d", ivB, rtB))

	// ---------- B2 植入老文件 ----------
	oldMod := time.Date(2026, 7, 1, 0, 0, 0, 0, time.Local)
	for _, inst := range instances {
		dir := *dirA
		if inst.tag == "b" {
			dir = *dirB
		}
		if err := plantFakeOld(dir, fakeOldName, oldMod); err != nil {
			fatal("B2 植入 (%s): %v", inst.tag, err)
		}
		list, err := fetchListEV(inst.base, inst.tok, fmt.Sprintf("list-b2-%s.json", inst.tag))
		if err != nil {
			fatal("B2 列表 (%s): %v", inst.tag, err)
		}
		check(fmt.Sprintf("B2 老文件可见（%s）", inst.tag),
			listHas(list, fakeOldName) && len(list) == 1,
			"names="+strings.Join(backupNames(list), ","))
	}

	// ---------- B3 定时触发等待 ----------
	// 探针全程未调 POST /run——此间出现的新产物只能来自 serverBackupLoop
	// tick（生产节奏 1h；最新文件名为植入老件 → since≈96d ≥ 1h，首 tick
	// 必跑）。这一等待是 checklist「定时触发 VACUUM INTO」的证据本体。
	ev("B3 等待定时产物（每 %s 轮询，上限 %s；tick 预计 T+60min）…", *pollStep, *maxWait)
	deadline := start.Add(*maxWait)
	var gotA, gotB bool
	var listA, listB []backupEntry
	hasNew := func(l []backupEntry) bool { return firstOtherName(l, fakeOldName) != "" }
	for !gotA || !gotB {
		if time.Now().After(deadline) {
			fatal("B3 超时（%s）：a 有定时产物=%v, b=%v", *maxWait, gotA, gotB)
		}
		time.Sleep(*pollStep)
		if !gotA {
			if l, err := fetchListEV(*apiA, tokA, ""); err == nil && hasNew(l) {
				gotA, listA = true, l
			}
		}
		if !gotB {
			if l, err := fetchListEV(*apiB, tokB, ""); err == nil && hasNew(l) {
				gotB, listB = true, l
			}
		}
	}
	sA := firstOtherName(listA, fakeOldName)
	sB := firstOtherName(listB, fakeOldName)
	waitSec := time.Since(start).Seconds()
	ev("B3 定时产物出现（等待 %.0fs）：a=%s b=%s", waitSec, sA, sB)
	check("B3 定时触发产物（a，非手动）", sA != "",
		fmt.Sprintf("等待 %.0fs 后出现 %s", waitSec, sA))
	check("B3 定时触发产物（b，非手动）", sB != "",
		fmt.Sprintf("等待 %.0fs 后出现 %s", waitSec, sB))

	// ---------- B4 retention 两形态 ----------
	// tick 循环体：runServerBackup → cleanupServerBackups，毫秒级窗口内
	// 轮询可能看到「新件已落、老件未删」中间态——给 10s 收敛窗再断言
	settleA := time.Now().Add(10 * time.Second)
	var listAAfter []backupEntry
	for {
		l, err := fetchListEV(*apiA, tokA, "")
		if err != nil {
			fatal("B4 列表 (a): %v", err)
		}
		listAAfter = l
		if !listHas(l, fakeOldName) || time.Now().After(settleA) {
			break
		}
		time.Sleep(2 * time.Second)
	}
	listAAfter, err = fetchListEV(*apiA, tokA, "list-b4-a.json")
	if err != nil {
		fatal("B4 列表快照 (a): %v", err)
	}
	_, statA := os.Stat(filepath.Join(*dirA, fakeOldName))
	check("B4 retention=1 按天清理（a 老文件被清）",
		!listHas(listAAfter, fakeOldName) && os.IsNotExist(statA),
		fmt.Sprintf("列表=%s; 老文件 stat=%v", strings.Join(backupNames(listAAfter), ","), statA))

	listBAfter, err := fetchListEV(*apiB, tokB, "list-b4-b.json")
	if err != nil {
		fatal("B4 列表 (b): %v", err)
	}
	_, statB := os.Stat(filepath.Join(*dirB, fakeOldName))
	check("B4 retention=0 永久（b 老文件仍在）",
		listHas(listBAfter, fakeOldName) && statB == nil,
		fmt.Sprintf("列表=%s", strings.Join(backupNames(listBAfter), ",")))

	// ---------- B5 定时产物 sqlite3 抽查 ----------
	if sA == "" {
		fatal("B5 实例 a 无定时产物可查")
	}
	dbA := filepath.Join(*dirA, sA)
	integ, err := sqliteQuery(dbA, "PRAGMA integrity_check;")
	check("B5 定时产物 integrity_check=ok", err == nil && integ == "ok",
		fmt.Sprintf("%s: %s", sA, truncate(integ, 40)))
	tables, err := sqliteQuery(dbA,
		"SELECT group_concat(name) FROM sqlite_master WHERE type='table' AND name IN ('users','audit_logs','settings');")
	check("B5 定时产物核心表在（users/audit_logs/settings）",
		err == nil && strings.Contains(tables, "users") &&
			strings.Contains(tables, "audit_logs") && strings.Contains(tables, "settings"),
		truncate(tables, 80))
	userN, errUser := sqliteQuery(dbA, "SELECT COUNT(*) FROM users;")
	userCount, convUser := strconv.Atoi(userN)
	check("B5 定时产物 users 行数 ≥1", errUser == nil && convUser == nil && userCount >= 1,
		"count="+userN)
	auditN, errAudit := sqliteQuery(dbA, "SELECT COUNT(*) FROM audit_logs;")
	auditCount, convAudit := strconv.Atoi(auditN)
	check("B5 定时产物 audit_logs 行数 ≥1", errAudit == nil && convAudit == nil && auditCount >= 1,
		"count="+auditN)
	var sizeLine string
	if info, err := os.Stat(dbA); err == nil {
		sizeLine = fmt.Sprintf("备份=%dB", info.Size())
	}
	if live, err := os.Stat(filepath.Join(filepath.Dir(*dirA), "cockpit.db")); err == nil {
		sizeLine = "线上 db=" + fmt.Sprint(live.Size()) + "B, " + sizeLine
	}
	ev("B5 体积对照（紧凑完整副本，不作硬断言）: %s", sizeLine)
	saveEV("sqlite-"+sA+".txt", []byte(fmt.Sprintf(
		"integrity_check: %s\ntables: %s\nusers: %s\naudit_logs: %s\n%s\n",
		integ, tables, userN, auditN, sizeLine)))

	// ---------- B6 手动备份 + 下载对照（实例 a） ----------
	code, raw := req(http.MethodPost, *apiA+"/api/server-backups/run", nil, tokA)
	if code != 200 {
		fatal("B6 run (a): HTTP %d %s", code, truncate(string(raw), 200))
	}
	saveEV("run-a.json", raw)
	var runResp struct {
		Name string `json:"name"`
	}
	_ = json.Unmarshal(raw, &runResp)
	mA := runResp.Name
	_, statM := os.Stat(filepath.Join(*dirA, mA))
	check("B6 手动产物落盘", statM == nil, mA)

	code, raw = req(http.MethodGet, *apiA+"/api/server-backups/"+mA+"/download", nil, tokA)
	check("B6 下载 HTTP 200", code == 200, fmt.Sprintf("HTTP %d, %d bytes", code, len(raw)))
	sumAPI := ""
	if code == 200 {
		h := sha256.Sum256(raw)
		sumAPI = hex.EncodeToString(h[:])
	}
	sumDisk, errDisk := fileSHA256(filepath.Join(*dirA, mA))
	check("B6 下载与盘上文件 sha256 一致",
		errDisk == nil && sumAPI != "" && sumAPI == sumDisk, sumDisk)
	saveEV("sha256-a.txt", []byte("api="+sumAPI+"\ndisk="+sumDisk+"\n"))

	integM, errIntegM := sqliteQuery(filepath.Join(*dirA, mA), "PRAGMA integrity_check;")
	check("B6 手动产物 integrity_check=ok", errIntegM == nil && integM == "ok",
		fmt.Sprintf("%s: %s", mA, truncate(integM, 40)))

	// ---------- B7 手动备份（实例 b，0=永久 共存形态） ----------
	code, raw = req(http.MethodPost, *apiB+"/api/server-backups/run", nil, tokB)
	if code != 200 {
		fatal("B7 run (b): HTTP %d %s", code, truncate(string(raw), 200))
	}
	saveEV("run-b.json", raw)
	_ = json.Unmarshal(raw, &runResp)
	listB3, err := fetchListEV(*apiB, tokB, "list-b7-b.json")
	if err != nil {
		fatal("B7 列表 (b): %v", err)
	}
	check("B7 0=永久 三件共存（老+定时+手动）",
		len(listB3) == 3 && listHas(listB3, fakeOldName) && listHas(listB3, runResp.Name),
		"names="+strings.Join(backupNames(listB3), ","))

	// ---------- B8 校验面 ----------
	code, _ = req(http.MethodPut, *apiA+"/api/server-backups/config",
		map[string]int{"interval_hours": 200}, tokA)
	check("B8 interval 越界 400", code == 400, fmt.Sprintf("HTTP %d", code))
	code, _ = req(http.MethodPut, *apiA+"/api/server-backups/config",
		map[string]int{"retention_days": -1}, tokA)
	check("B8 retention 负值 400", code == 400, fmt.Sprintf("HTTP %d", code))
	code, _ = req(http.MethodGet, *apiA+"/api/server-backups/cockpit-20990101-000000.db/download", nil, tokA)
	check("B8 未知名 download 404", code == 404, fmt.Sprintf("HTTP %d", code))
	code, _ = req(http.MethodDelete, *apiA+"/api/server-backups/cockpit-20990101-000000.db", nil, tokA)
	check("B8 未知名 delete 404", code == 404, fmt.Sprintf("HTTP %d", code))

	// ---------- B9 审计 ----------
	code, raw = req(http.MethodGet, *apiA+"/api/admin/audit/logs?resource=server_backup&page_size=50", nil, tokA)
	if code != 200 {
		fatal("B9 审计查询: HTTP %d", code)
	}
	saveEV("audit.json", raw)
	entries, err := parseAuditEntries(raw)
	if err != nil {
		fatal("B9 审计解析: %v", err)
	}
	creates := countAudit(entries, "create", mA)
	check("B9 手动备份 create 审计恰一条",
		len(creates) == 1 && creates[0].Username == *adminUser,
		fmt.Sprintf("resource_id=%s 命中 %d 条", mA, len(creates)))
	exports := countAudit(entries, "export", mA)
	check("B9 download export 审计 ≥1", len(exports) >= 1,
		fmt.Sprintf("命中 %d 条", len(exports)))
	updates := countAudit(entries, "update", "config")
	check("B9 config PUT update 审计 ≥1", len(updates) >= 1,
		fmt.Sprintf("命中 %d 条", len(updates)))

	ev("== 汇总：PASS %d / FAIL %d / 用时 %.0fs ==", passes, fails, time.Since(start).Seconds())
	if f := evFile; f != nil {
		_ = f.Sync()
	}
	if fails > 0 {
		osExit(1)
	}
}

// mustLogin 登录失败即终止
func mustLogin(base string) string {
	tk, err := loginAs(base, *adminUser, *adminPass)
	if err != nil {
		fatal("登录失败（%s）: %v", base, err)
	}
	return tk
}
