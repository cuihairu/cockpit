// Traefik 后端热加载真机验收探针（proxy-design.md M2 真机项）。
//
// 以面板用户身份走真实 REST 链路（server → agent traefik provider →
// 宿主动态目录 → Traefik 容器 file provider watch），验证：
//
//	T1  capability（traefik-proxy + dynamicDir）与 status（reloadMode=hot）
//	T2  新增站点：渲染落盘 → Traefik 路由注册 → 端到端命中上游（双域名）
//	T3  修改站点：upstream 切换 → 热加载生效（无 reload 命令）
//	T4  site.get meta 回读（首行 # cockpit:meta JSON 与渲染全文）
//	T5  https 站点：301 永久跳转 + 证书文件引用真被加载（自有 CA 握手验证）
//	T6  extra 拒绝（D14 注入面）：报错且不落盘
//	T7  坏文件热更新真实语义（实测修正 D13「局部隔离」口径）：任一坏文件
//	    冻结整目录热更新（存量站点 last-good 照常服务、新变更落盘但拒载），
//	    坏文件 rm / 面板重下发覆盖即解冻，冻结期积压变更一并生效
//	T8  drift 四态（ok/drifted/missing/no_baseline + error）与 record「以当前为准」
//	T9  删除站点：文件摘除 → 路由摘除 → 404
//	T10 审计留痕（proxy_apply/proxy_delete/drift_record）
//
// 证据落 .acceptance/traefik/evidence/probe.log；任何 FAIL → exit 1。
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	apiBase    = flag.String("api", "http://127.0.0.1:19991", "cockpit server 基址")
	traefikAPI = flag.String("traefik", "http://127.0.0.1:18181", "Traefik admin API 基址（api.insecure）")
	webEP      = flag.String("web", "http://127.0.0.1:18180", "Traefik web entrypoint 基址")
	tlsAddr    = flag.String("tls-addr", "127.0.0.1:18453", "websecure entrypoint 宿主侧地址")
	dynDir     = flag.String("dyn", "", "宿主动态目录（默认 .acceptance/traefik/dynamic）")
	certPem    = flag.String("cert", "", "自签证书 PEM（默认 .acceptance/traefik/certs/accept.pem）")
	agentID    = flag.String("agent", "traefik-acc-agent", "验收 agent ID")
	adminUser  = flag.String("user", "admin", "管理员用户名")
	adminPass  = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	evDir      = flag.String("ev", "", "证据目录（默认 .acceptance/traefik/evidence）")
)

var (
	evFile  *os.File
	evMu    sync.Mutex
	passes  int
	fails   int
	token   string
	httpC   = &http.Client{Timeout: 15 * time.Second}
	tlsC    *http.Client    // ServerName=s2.accept.test + 自有 CA + 拨号固定到 tls-addr
	noRedir = &http.Client{ // 不跟随重定向（308 断言用）
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
)

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

// 行为中性注入点（先例：guac/probe osExit、logs/probe systemdRunCmd、
// services/probe shExec）——fatal 退出在单测注入桩覆盖分支，默认值即原行为
var osExit = os.Exit

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

// ---------- cockpit REST ----------

func reqJSON(method, url string, body interface{}) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, url, rd)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpC.Do(req)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

// rest 调 cockpit API，200 时解 JSON；非 200 返回错误（带响应体摘要）
func rest(method, path string, body interface{}) (map[string]interface{}, error) {
	code, raw := reqJSON(method, *apiBase+path, body)
	var m map[string]interface{}
	_ = json.Unmarshal(raw, &m)
	if code != http.StatusOK {
		if msg, _ := m["error"].(string); msg != "" {
			return m, fmt.Errorf("HTTP %d: %s", code, msg)
		}
		return m, fmt.Errorf("HTTP %d: %s", code, truncate(string(raw), 200))
	}
	return m, nil
}

func login() {
	code, raw := reqJSON(http.MethodPost, *apiBase+"/api/auth/login",
		map[string]string{"username": *adminUser, "password": *adminPass})
	if code != 200 {
		fatal("登录失败 HTTP %d: %s", code, truncate(string(raw), 200))
	}
	var r struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(raw, &r) != nil || r.Token == "" {
		fatal("登录响应无 token: %s", truncate(string(raw), 200))
	}
	token = r.Token
}

// agentCapabilities 取目标 agent 的 capabilities（/api/agents 返回裸数组，
// 兼容将来改成 {agents:[..]} 包装）
func agentCapabilities() ([]interface{}, error) {
	_, raw := reqJSON(http.MethodGet, *apiBase+"/api/agents", nil)
	type agentEntry struct {
		ID           string        `json:"id"`
		Capabilities []interface{} `json:"capabilities"`
	}
	var list []agentEntry
	if json.Unmarshal(raw, &list) != nil {
		var wrapper struct {
			Agents []agentEntry `json:"agents"`
		}
		if err := json.Unmarshal(raw, &wrapper); err != nil {
			return nil, fmt.Errorf("bad /api/agents: %v", err)
		}
		list = wrapper.Agents
	}
	for _, a := range list {
		if a.ID == *agentID {
			return a.Capabilities, nil
		}
	}
	return nil, fmt.Errorf("agent %s 不在列表", *agentID)
}

// driftStatus 取 kind=traefik 指定 name 的状态（""=无条目）
func driftStatus(name string) (string, error) {
	m, err := rest(http.MethodPost, "/api/agents/"+*agentID+"/drift/check", nil)
	if err != nil {
		return "", err
	}
	items, _ := m["items"].([]interface{})
	for _, it := range items {
		e, _ := it.(map[string]interface{})
		if e["kind"] == "traefik" && e["name"] == name {
			s, _ := e["status"].(string)
			return s, nil
		}
	}
	return "", nil
}

// ---------- Traefik admin API ----------

func traefikAPIGet(path string, out interface{}) error {
	resp, err := httpC.Get(*traefikAPI + path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return json.Unmarshal(b, out)
}

// waitRouter 轮询 /api/http/routers 直到 name（@file 后缀）出现且 rule 匹配子串
func waitRouter(name, ruleContains string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var routers []map[string]interface{}
		if err := traefikAPIGet("/api/http/routers", &routers); err == nil {
			for _, r := range routers {
				n, _ := r["name"].(string)
				rule, _ := r["rule"].(string)
				if strings.HasPrefix(n, name+"@") && strings.Contains(rule, ruleContains) {
					return true
				}
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

// waitRouterGone 轮询直到路由摘除
func waitRouterGone(name string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var routers []map[string]interface{}
		if err := traefikAPIGet("/api/http/routers", &routers); err == nil {
			found := false
			for _, r := range routers {
				n, _ := r["name"].(string)
				if strings.HasPrefix(n, name+"@") {
					found = true
				}
			}
			if !found {
				return true
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

// serviceServerURL 取 service 的 loadBalancer 首个 server URL
func serviceServerURL(name string) string {
	var services []map[string]interface{}
	if err := traefikAPIGet("/api/http/services", &services); err != nil {
		return ""
	}
	for _, s := range services {
		n, _ := s["name"].(string)
		if !strings.HasPrefix(n, name+"@") {
			continue
		}
		lb, _ := s["loadBalancer"].(map[string]interface{})
		servers, _ := lb["servers"].([]interface{})
		if len(servers) > 0 {
			srv, _ := servers[0].(map[string]interface{})
			u, _ := srv["url"].(string)
			return u
		}
	}
	return ""
}

func waitServiceURL(name, contains string, timeout time.Duration) string {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if u := serviceServerURL(name); strings.Contains(u, contains) {
			return u
		}
		time.Sleep(150 * time.Millisecond)
	}
	return serviceServerURL(name)
}

// ---------- 经 Traefik 的端到端请求 ----------

// viaTraefik 打 web entrypoint（Host 头定路由），返回 (状态码, body)
func viaTraefik(host string) (int, string) {
	req, _ := http.NewRequest(http.MethodGet, *webEP+"/", nil)
	req.Host = host
	resp, err := noRedir.Do(req)
	if err != nil {
		return -1, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return resp.StatusCode, string(b)
}

// viaTraefikTLS 打 websecure：URL 用域名、拨号固定到宿主侧端口、
// ServerName+RootCAs 用验收证书——握手成功即证明 Traefik 加载了
// 动态配置里引用的那份证书文件
func viaTraefikTLS(hostname, path string) (int, string, error) {
	if tlsC == nil {
		return 0, "", fmt.Errorf("tls client 未初始化")
	}
	resp, err := tlsC.Get("https://" + hostname + path)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<16))
	return resp.StatusCode, string(b), nil
}

// pollViaTraefik 轮询直到状态码匹配（热加载传播窗口）
func pollViaTraefik(host string, wantCode int, wantBody string, timeout time.Duration) (int, string) {
	deadline := time.Now().Add(timeout)
	code, body := 0, ""
	for time.Now().Before(deadline) {
		code, body = viaTraefik(host)
		if code == wantCode && (wantBody == "" || strings.Contains(body, wantBody)) {
			return code, body
		}
		time.Sleep(200 * time.Millisecond)
	}
	return code, body
}

// ---------- 站点操作 ----------

func applySite(name string, body map[string]interface{}) (map[string]interface{}, error) {
	body["name"] = name
	return rest(http.MethodPut, "/api/agents/"+*agentID+"/proxy/sites/"+name, body)
}

func deleteSite(name string) error {
	_, err := rest(http.MethodDelete, "/api/agents/"+*agentID+"/proxy/sites/"+name, nil)
	return err
}

func sitesList() (map[string]map[string]interface{}, error) {
	m, err := rest(http.MethodGet, "/api/agents/"+*agentID+"/proxy/sites", nil)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]interface{}{}
	if arr, ok := m["sites"].([]interface{}); ok {
		for _, s := range arr {
			e, _ := s.(map[string]interface{})
			n, _ := e["name"].(string)
			out[n] = e
		}
	}
	return out, nil
}

// ---------- 场景 ----------

func main() {
	flag.Parse()
	if _, err := os.Stat("go.mod"); err != nil {
		fatal("请在仓库根目录运行（找不到 go.mod）")
	}
	repo, _ := os.Getwd()
	if *evDir == "" {
		*evDir = filepath.Join(repo, ".acceptance/traefik/evidence")
	}
	if *dynDir == "" {
		*dynDir = filepath.Join(repo, ".acceptance/traefik/dynamic")
	}
	if *certPem == "" {
		*certPem = filepath.Join(repo, ".acceptance/traefik/certs/accept.pem")
	}
	_ = os.MkdirAll(*evDir, 0o755)
	f, err := os.Create(filepath.Join(*evDir, "probe.log"))
	if err != nil {
		fatal("证据文件创建失败: %v", err)
	}
	evFile = f
	defer evFile.Close()

	// TLS 客户端：自有 CA + 拨号固定
	pem, err := os.ReadFile(*certPem)
	if err != nil {
		fatal("读证书 %s 失败: %v", *certPem, err)
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pem) {
		fatal("证书 %s 不是合法 PEM", *certPem)
	}
	tlsC = &http.Client{
		Timeout: 15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
		Transport: &http.Transport{
			// URL 里的域名只用于 Host/SNI，实际拨号固定到宿主侧 websecure 端口
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, "tcp", *tlsAddr)
			},
			TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "s2.accept.test"},
		},
	}

	ev("=== Traefik 后端热加载真机验收 %s ===", time.Now().Format(time.RFC3339))
	ev("api=%s traefik=%s web=%s tls=%s dyn=%s", *apiBase, *traefikAPI, *webEP, *tlsAddr, *dynDir)

	login()
	ev("登录成功（%s）", *adminUser)

	siteFile := func(name string) string {
		return filepath.Join(*dynDir, "cockpit-site-"+name+".yml")
	}
	upstreamA := "host.docker.internal:19091"
	upstreamB := "host.docker.internal:19092"

	// ---- T1 capability 与 status ----
	func() {
		name := "T1 traefik-proxy capability 探测与 status 概览"
		caps, err := agentCapabilities()
		if err != nil {
			check(name, false, err.Error())
			return
		}
		var dynMeta string
		hit := false
		for _, c := range caps {
			e, _ := c.(map[string]interface{})
			if e["type"] == "traefik-proxy" {
				hit = true
				md, _ := e["metadata"].(map[string]interface{})
				dynMeta, _ = md["dynamicDir"].(string)
			}
		}
		if !hit {
			check(name, false, "capabilities 无 traefik-proxy（目录探测失败？）")
			return
		}
		st, err := rest(http.MethodGet, "/api/agents/"+*agentID+"/proxy/status", nil)
		if err != nil {
			check(name, false, "status: "+err.Error())
			return
		}
		backend, _ := st["backend"].(string)
		mode, _ := st["reloadMode"].(string)
		confDir, _ := st["confDir"].(string)
		version, _ := st["version"].(string)
		count := int(st["siteCount"].(float64))
		installed := st["installed"] == true
		// version 恒空是「容器化宿主无 traefik 二进制」的既定口径（D16）
		ok := hit && dynMeta == *dynDir && backend == "traefik" && mode == "hot" &&
			confDir == *dynDir && installed && version == "" && count == 0
		check(name, ok, fmt.Sprintf("dynamicDir=%q backend=%q reloadMode=%q confDir=%q version=%q(容器化宿主恒空) siteCount=%d",
			dynMeta, backend, mode, confDir, version, count))
	}()

	// ---- T2 新增站点：落盘 → 路由注册 → 端到端命中 ----
	func() {
		name := "T2 新增站点热加载生效（渲染落盘 → Traefik 路由 → 端到端）"
		resp, err := applySite("s1", map[string]interface{}{
			"serverNames": []string{"s1.accept.test", "alt1.accept.test"},
			"upstream":    upstreamA,
			"scheme":      "http",
		})
		if err != nil {
			check(name, false, "apply: "+err.Error())
			return
		}
		file, _ := resp["file"].(string)
		if file != "cockpit-site-s1.yml" {
			check(name, false, fmt.Sprintf("apply 响应 file=%q", file))
			return
		}
		// 落盘内容：首行 meta JSON roundtrip + 渲染体
		b, err := os.ReadFile(siteFile("s1"))
		if err != nil {
			check(name, false, "宿主动态目录无片段文件: "+err.Error())
			return
		}
		lines := strings.SplitN(string(b), "\n", 2)
		if !strings.HasPrefix(lines[0], "# cockpit:meta ") {
			check(name, false, "首行缺 meta 注释: "+truncate(lines[0], 80))
			return
		}
		var meta map[string]interface{}
		if json.Unmarshal([]byte(strings.TrimPrefix(lines[0], "# cockpit:meta ")), &meta) != nil ||
			meta["name"] != "s1" || meta["upstream"] != upstreamA || meta["scheme"] != "http" {
			check(name, false, "meta JSON 回读不符: "+truncate(lines[0], 160))
			return
		}
		// Traefik 路由注册（热加载直接证据）+ 端到端（双域名 rule）
		if !waitRouter("cockpit-s1", "Host(`s1.accept.test`)", 10*time.Second) {
			check(name, false, "Traefik /api/http/routers 未出现 cockpit-s1（热加载未生效）")
			return
		}
		code, body := pollViaTraefik("s1.accept.test", 200, "BACKEND-A", 10*time.Second)
		if code != 200 {
			check(name, false, fmt.Sprintf("Host s1.accept.test → %d %q", code, truncate(body, 120)))
			return
		}
		codeAlt, bodyAlt := pollViaTraefik("alt1.accept.test", 200, "BACKEND-A", 10*time.Second)
		ev("      rule=Host(`s1.accept.test`) || Host(`alt1.accept.test`)，路由 API 已注册；s1=%d alt1=%d", code, codeAlt)
		check(name, codeAlt == 200 && strings.Contains(bodyAlt, "BACKEND-A"),
			fmt.Sprintf("端到端命中上游 A（s1/alt1 双域名各 %d，body=%q）", code, truncate(body, 40)))
	}()

	// ---- T3 修改站点：upstream 切换热加载 ----
	func() {
		name := "T3 修改站点热加载生效（upstream A→B，无 reload 命令）"
		if _, err := applySite("s1", map[string]interface{}{
			"serverNames": []string{"s1.accept.test", "alt1.accept.test"},
			"upstream":    upstreamB,
			"scheme":      "http",
		}); err != nil {
			check(name, false, "apply: "+err.Error())
			return
		}
		url := waitServiceURL("cockpit-s1", upstreamB, 10*time.Second)
		code, body := pollViaTraefik("s1.accept.test", 200, "BACKEND-B", 10*time.Second)
		check(name, strings.Contains(url, upstreamB) && code == 200 && strings.Contains(body, "BACKEND-B"),
			fmt.Sprintf("service url=%q → 端到端 body=%q（上游已切到 B）", url, truncate(body, 40)))
	}()

	// ---- T4 site.get meta 回读与渲染全文 ----
	func() {
		name := "T4 site.get 回读（meta 解析 + 渲染全文）"
		m, err := rest(http.MethodGet, "/api/agents/"+*agentID+"/proxy/sites/s1", nil)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		site, _ := m["site"].(map[string]interface{})
		content, _ := m["content"].(string)
		up, _ := site["upstream"].(string)
		names, _ := site["serverNames"].([]interface{})
		ok := up == upstreamB && len(names) == 2 &&
			strings.Contains(content, "cockpit-s1") && strings.HasPrefix(content, "# cockpit:meta ")
		check(name, ok, fmt.Sprintf("site.upstream=%q serverNames=%d 内容含渲染 router/service=%v",
			up, len(names), strings.Contains(content, "cockpit-s1")))
	}()

	// ---- T5 https：301 跳转 + 证书文件引用加载 ----
	func() {
		name := "T5 https 站点（301 永久跳转 + 证书文件引用被 Traefik 加载）"
		if _, err := applySite("s2", map[string]interface{}{
			"serverNames": []string{"s2.accept.test"},
			"upstream":    upstreamA,
			"scheme":      "https",
			"tlsCert":     "/etc/traefik/certs/accept.pem",
			"tlsKey":      "/etc/traefik/certs/accept.key",
		}); err != nil {
			check(name, false, "apply: "+err.Error())
			return
		}
		if !waitRouter("cockpit-s2-websecure", "Host(`s2.accept.test`)", 10*time.Second) {
			check(name, false, "websecure 路由未注册")
			return
		}
		// http → 301 permanent（Traefik redirectScheme permanent=true 实测口径，
		// 非 308——设计文档原表述已一并修正）
		req, _ := http.NewRequest(http.MethodGet, *webEP+"/", nil)
		req.Host = "s2.accept.test"
		resp, err := noRedir.Do(req)
		var code int
		var loc string
		if err == nil {
			code = resp.StatusCode
			loc = resp.Header.Get("Location")
			resp.Body.Close()
		}
		// https：自有 CA 握手（证明引用的证书文件真被加载）+ 端到端
		hcode, hbody, herr := viaTraefikTLS("s2.accept.test", "/")
		tlsOK := herr == nil && hcode == 200 && strings.Contains(hbody, "BACKEND-A")
		detail := fmt.Sprintf("http=%d Location=%q；https=%d body=%q err=%v", code, loc, hcode, truncate(hbody, 40), herr)
		check(name, code == 301 && strings.HasPrefix(loc, "https://s2.accept.test") && tlsOK,
			detail+"（CA 握手成功 = 动态配置引用的证书文件已被加载）")
	}()

	// ---- T6 extra 拒绝 ----
	func() {
		name := "T6 extra 拒绝（traefik 无 nginx 直通口，D14 注入面）"
		_, err := applySite("s3", map[string]interface{}{
			"serverNames": []string{"s3.accept.test"},
			"upstream":    upstreamA,
			"scheme":      "http",
			"extra":       "http:\n  routers:\n    inject: {}",
		})
		_, statErr := os.Stat(siteFile("s3"))
		list, _ := sitesList()
		_, listed := list["s3"]
		check(name, err != nil && strings.Contains(err.Error(), "extra directives are not supported") &&
			os.IsNotExist(statErr) && !listed,
			fmt.Sprintf("err=%v 未落盘=%v 未入列=%v", err != nil, os.IsNotExist(statErr), !listed))
	}()

	// ---- T7 坏文件热更新真实语义（修正 D13「局部隔离」口径）----
	func() {
		name := "T7 坏文件语义（冻结整目录热更新、存量 last-good 照常、修复即解冻）"
		broken := filepath.Join(*dynDir, "user-broken.yml")
		// a) 坏用户文件在场：面板更新照常落盘，但 Traefik 拒载整目录
		//    （watcher callback 整体失败）——s1 仍服务末次有效配置（upstream=B）
		if err := os.WriteFile(broken, []byte("http:\n  routers: {{{{\n"), 0o644); err != nil {
			check(name, false, "写坏文件失败: "+err.Error())
			return
		}
		time.Sleep(2 * time.Second) // 给 watch 一轮，让坏文件先触发冻结
		if _, err := applySite("s1", map[string]interface{}{
			"serverNames": []string{"s1.accept.test", "alt1.accept.test"},
			"upstream":    upstreamA,
			"scheme":      "http",
		}); err != nil {
			check(name, false, "坏文件在场时 apply: "+err.Error())
			return
		}
		diskA := false
		for i := 0; i < 20; i++ {
			if b, err := os.ReadFile(siteFile("s1")); err == nil && strings.Contains(string(b), upstreamA) {
				diskA = true
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		time.Sleep(3 * time.Second) // 冻结观察窗：正常热加载 <1s 应已生效
		frozenCode, frozenBody := viaTraefik("s1.accept.test")
		frozen := diskA && frozenCode == 200 && strings.Contains(frozenBody, "BACKEND-B")
		ev("      冻结证据：落盘 upstream=A 而 s1 实际仍服务 %q（%d）——整目录热更新被拒载", truncate(frozenBody, 20), frozenCode)

		// b) rm 坏文件 → 下一轮 watch 事件自动解冻，冻结期积压的更新生效
		_ = os.Remove(broken)
		code, body := pollViaTraefik("s1.accept.test", 200, "BACKEND-A", 10*time.Second)
		healed := code == 200 && strings.Contains(body, "BACKEND-A")

		// c) cockpit 片段被手改坏：同样冻结——新站点 s5 拒载（404），
		//    但存量站点照常服务（s1 last-good、s2 https），日志点名坏文件
		if err := os.WriteFile(siteFile("s1"), []byte("::: not yaml at all ::: [\n"), 0o644); err != nil {
			check(name, false, "手改坏 s1 失败: "+err.Error())
			return
		}
		time.Sleep(2 * time.Second)
		if _, err := applySite("s5", map[string]interface{}{
			"serverNames": []string{"s5.accept.test"},
			"upstream":    upstreamA,
			"scheme":      "http",
		}); err != nil {
			check(name, false, "坏片段在场时下发新站点 s5: "+err.Error())
			return
		}
		time.Sleep(3 * time.Second) // 冻结期观察窗：s5 不应注册
		s5code, _ := viaTraefik("s5.accept.test")
		s1code, s1body := viaTraefik("s1.accept.test")
		s2code, _, _ := viaTraefikTLS("s2.accept.test", "/")
		lastGood := s1code == 200 && strings.Contains(s1body, "BACKEND-A") // b) 解冻后 s1=A 即 last-good
		logOut, _ := exec.Command("docker", "logs", "cockpit-acc-traefik").CombinedOutput()
		logHit := strings.Contains(string(logOut), "user-broken") || strings.Contains(string(logOut), "cockpit-site-s1")
		ev("      s5(新)=%d（冻结拒载）s1(存量)=%d %q s2(https 存量)=%d 日志点名坏文件=%v",
			s5code, s1code, truncate(s1body, 20), s2code, logHit)

		// d) 面板重下发 s1（有效内容覆盖坏片段）→ 解冻，冻结期积压的 s5 一并生效
		if _, err := applySite("s1", map[string]interface{}{
			"serverNames": []string{"s1.accept.test", "alt1.accept.test"},
			"upstream":    upstreamB,
			"scheme":      "http",
		}); err != nil {
			check(name, false, "重下发恢复 s1: "+err.Error())
			return
		}
		rCode, rBody := pollViaTraefik("s1.accept.test", 200, "BACKEND-B", 10*time.Second)
		s5code2, s5body2 := pollViaTraefik("s5.accept.test", 200, "BACKEND-A", 10*time.Second)
		ev("      重下发后：s1=%d %q；积压的 s5=%d %q（一并生效）", rCode, truncate(rBody, 20), s5code2, truncate(s5body2, 20))

		ok := frozen && healed && s5code == 404 && lastGood && s2code == 200 && logHit &&
			rCode == 200 && strings.Contains(rBody, "BACKEND-B") && s5code2 == 200 && strings.Contains(s5body2, "BACKEND-A")
		check(name, ok, fmt.Sprintf(
			"冻结=%v 解冻(rm坏文件)=%v 冻结期新站点404=%v 存量last-good=%v https存量=%v 日志点名=%v 重下发治愈=%v 积压生效=%v",
			frozen, healed, s5code == 404, lastGood, s2code == 200, logHit,
			rCode == 200 && strings.Contains(rBody, "BACKEND-B"), s5code2 == 200))
		// 清掉 s5（后续 drift/删除场景不留杂音）
		if err := deleteSite("s5"); err != nil {
			ev("[WARN] 清理 s5: %v", err)
		}
		time.Sleep(500 * time.Millisecond)
	}()

	// ---- T8 drift 四态 + record ----
	func() {
		name := "T8 drift 四态（drifted/ok/missing/no_baseline/error）与 record"
		// 前置：T7 已把 s1 恢复为有效内容且基线同步。手改 s1 追加一行注释
		// ——合法 YAML（路由不受影响、不触发冻结），原始字节与基线不符 → drifted
		if b, err := os.ReadFile(siteFile("s1")); err == nil {
			_ = os.WriteFile(siteFile("s1"), append(b, []byte("# hand-edit drift\n")...), 0o644)
		}
		time.Sleep(1 * time.Second)
		st1, err := driftStatus("s1")
		if err != nil {
			check(name, false, "check: "+err.Error())
			return
		}
		st2, _ := driftStatus("s2")
		if st1 != "drifted" || st2 != "ok" {
			check(name, false, fmt.Sprintf("初始态不符：s1=%q（want drifted，T7b 手改）s2=%q（want ok）", st1, st2))
			return
		}
		// record「以当前为准」：手改坏的内容成为新基线
		if _, err := rest(http.MethodPost, "/api/agents/"+*agentID+"/drift/record",
			map[string]string{"kind": "traefik", "name": "s1"}); err != nil {
			check(name, false, "record s1: "+err.Error())
			return
		}
		st1b, _ := driftStatus("s1")
		// 面板重下发恢复 s1（真实恢复路径：apply 覆盖坏内容 + 重建基线）
		if _, err := applySite("s1", map[string]interface{}{
			"serverNames": []string{"s1.accept.test", "alt1.accept.test"},
			"upstream":    upstreamB,
			"scheme":      "http",
		}); err != nil {
			check(name, false, "恢复 s1: "+err.Error())
			return
		}
		st1c, _ := driftStatus("s1")
		code, body := pollViaTraefik("s1.accept.test", 200, "BACKEND-B", 10*time.Second)
		// missing：基线在、文件被手删
		_ = os.Remove(siteFile("s2"))
		stMiss, _ := driftStatus("s2")
		// no_baseline：绕过面板手放一个片段（内容随意——drift 只看原始字节 hash）
		s1Bytes, _ := os.ReadFile(siteFile("s1"))
		_ = os.WriteFile(siteFile("s4"), s1Bytes, 0o644)
		stNB, _ := driftStatus("s4")
		// error：读失败（非 root 下 chmod 000）
		_ = os.Chmod(siteFile("s4"), 0o000)
		stErr, _ := driftStatus("s4")
		_ = os.Chmod(siteFile("s4"), 0o644)
		_ = os.Remove(siteFile("s4"))
		ev("      s1: drifted→record=%q→apply恢复=%q（路由 %d %q）；s2 删除=%q；s4 手放=%q chmod000=%q",
			st1b, st1c, code, truncate(body, 30), stMiss, stNB, stErr)
		ok := st1 == "drifted" && st1b == "ok" && st1c == "ok" && code == 200 &&
			stMiss == "missing" && stNB == "no_baseline" && stErr == "error"
		check(name, ok, fmt.Sprintf("四态+record 全链：drifted=%q ok(record)=%q ok(apply)=%q missing=%q no_baseline=%q error=%q",
			st1, st1b, st1c, stMiss, stNB, stErr))
	}()

	// ---- T9 删除站点 ----
	func() {
		name := "T9 删除站点热加载生效（文件/路由/列表三面摘除）"
		if err := deleteSite("s1"); err != nil {
			check(name, false, "delete: "+err.Error())
			return
		}
		_, statErr := os.Stat(siteFile("s1"))
		gone := waitRouterGone("cockpit-s1", 10*time.Second)
		code, _ := pollViaTraefik("s1.accept.test", 404, "", 10*time.Second)
		list, _ := sitesList()
		_, s1Listed := list["s1"]
		check(name, os.IsNotExist(statErr) && gone && code == 404 && !s1Listed,
			fmt.Sprintf("文件已删=%v 路由已摘=%v 端到端=%d 列表不含=%v", os.IsNotExist(statErr), gone, code, !s1Listed))
	}()

	// ---- T10 审计留痕 ----
	func() {
		name := "T10 审计留痕（proxy_apply/proxy_delete/drift_record）"
		m, err := rest(http.MethodGet, "/api/admin/audit/logs?page_size=100", nil)
		if err != nil {
			check(name, false, err.Error())
			return
		}
		have := map[string]bool{"apply_s1": false, "apply_s2": false, "delete_s1": false, "record_s1": false}
		if arr, ok := m["data"].([]interface{}); ok { // 审计列表响应键是 data（非 logs）
			for _, e := range arr {
				l, _ := e.(map[string]interface{})
				action, _ := l["action"].(string)
				rid, _ := l["resource_id"].(string)
				switch {
				case action == "proxy_apply" && rid == "s1":
					have["apply_s1"] = true
				case action == "proxy_apply" && rid == "s2":
					have["apply_s2"] = true
				case action == "proxy_delete" && rid == "s1":
					have["delete_s1"] = true
				case action == "drift_record" && rid == "traefik/s1":
					have["record_s1"] = true
				}
			}
		}
		all := true
		for _, v := range have {
			all = all && v
		}
		check(name, all, fmt.Sprintf("apply_s1=%v apply_s2=%v delete_s1=%v drift_record(traefik/s1)=%v",
			have["apply_s1"], have["apply_s2"], have["delete_s1"], have["record_s1"]))
	}()

	// Traefik 运行日志整卷入证据（坏文件报错等原始记录）
	if out, err := exec.Command("docker", "logs", "cockpit-acc-traefik").CombinedOutput(); err == nil {
		_ = os.WriteFile(filepath.Join(*evDir, "traefik.log"), out, 0o644)
	}

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}
