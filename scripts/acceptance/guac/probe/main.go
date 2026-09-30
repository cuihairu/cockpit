// 远控三协议（Guacamole SSH/RDP/VNC）协议级验收探针。
//
// 以「浏览器客户端」身份直连本地 cockpit 实例：REST 登录/建票据 → WS Guacamole
// 隧道（?ticket=）→ 原生 Guacamole 指令（key/size/clipboard）。终端输出在
// guacd 侧是图形渲染（libguac-terminal），文本证据走 sshd 容器文件系统侧信道
// （docker exec cat），图形渲染证据用指令流统计（img/copy/rect/sync 计数）。
//
// 证据落 .acceptance/guac/evidence/probe.log（PASS/FAIL 逐条）。
// 任何 FAIL → exit 1。
package main

import (
	"bytes"
	"encoding/base64"
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

	"github.com/gorilla/websocket"
)

var (
	apiBase   = flag.String("api", "http://127.0.0.1:19990", "cockpit API base")
	adminUser = flag.String("user", "admin", "管理员用户名")
	adminPass = flag.String("pass", "e2e-strong-pass-1", "管理员口令")
	agentID   = flag.String("agent", "guac-acc-agent", "验收 agent id")

	sshHost = flag.String("ssh-host", "127.0.0.1", "容器 sshd 主机")
	sshPort = flag.Int("ssh-port", 2222, "容器 sshd 端口（口令+双格式私钥目标）")
	sshUser = flag.String("ssh-user", "accept", "容器 sshd 用户")
	sshPass = flag.String("ssh-pass", "Accept-Guac-2026", "容器 sshd 口令")

	keysDir = flag.String("keys", "", "验收密钥目录（默认 .acceptance/guac/keys）")
	cuiKey  = flag.String("host-key", "", "本机 sshd 私钥（默认 ~/.ssh/cui，只读）")
	cidUser = flag.String("host-user", "", "本机 sshd 用户（默认 $USER）")

	vncPass = flag.String("vnc-pass", "Accept-Guac-2026", "VNC 目标口令")

	evDir     = flag.String("ev", "", "证据目录（默认 .acceptance/guac/evidence）")
	only      = flag.String("s", "", "只跑名字含该子串的场景（调试用）")
	container = flag.String("container", "cockpit-acc-sshd", "sshd 目标容器名")
)

var (
	token  string
	evFile *os.File
	evMu   sync.Mutex
	fails  int
	passes int
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

// sceneWanted -s 过滤下该场景是否要跑（空 = 全跑）
func sceneWanted(name string) bool {
	return *only == "" || strings.Contains(name, *only)
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
	os.Exit(2)
}

// ---------- REST ----------

func api(method, path string, body interface{}) (int, []byte) {
	var rd io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, *apiBase+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, []byte(err.Error())
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, b
}

func login() {
	for i := 0; i < 20; i++ {
		st, body := api("POST", "/api/auth/login", map[string]string{"username": *adminUser, "password": *adminPass})
		if st == 200 {
			var r struct {
				Token string `json:"token"`
			}
			if json.Unmarshal(body, &r) == nil && r.Token != "" {
				token = r.Token
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	fatal("登录失败")
}

// createTicket POST /api/remote/tickets，返回票据与 HTTP 状态码
func createTicket(p map[string]interface{}) (string, int, string) {
	st, body := api("POST", "/api/remote/tickets", p)
	if st != 200 {
		return "", st, string(body)
	}
	var r struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.Ticket == "" {
		return "", st, "ticket 缺失: " + string(body)
	}
	return r.Ticket, st, ""
}

// ---------- Guacamole 指令 ----------

func guacEncode(opcode string, args ...string) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "%d.%s", len(opcode), opcode)
	for _, a := range args {
		fmt.Fprintf(&sb, ",%d.%s", len(a), a)
	}
	sb.WriteString(";")
	return sb.String()
}

type instr struct {
	Opcode string
	Args   []string
}

// guacSession 一条 Guacamole 隧道（浏览器视角）
type guacSession struct {
	ws        *websocket.Conn
	sessionID string // 网关首条内部指令里的会话 UUID
	instrCh   chan instr
	errCh     chan error
	opCounts  map[string]int
	mu        sync.Mutex
	closed    bool

	// clipboard 流旁路：服务端 clipboard 流的 blob 指令高频（渲染 img 流
	// 同为 blob），instrCh 的 <200 缓冲条件会把它挤掉——流数据在 readLoop
	// 就地拼装，收完经 clipCh 投递
	clipMu  sync.Mutex
	clipIdx string
	clipB64 strings.Builder
	clipCh  chan string

	pingStop chan struct{}
}

// wsBase http(s):// → ws(s)://
func wsBase() string {
	return strings.Replace(*apiBase, "http://", "ws://", 1)
}

func dialGuac(ticket string) (*guacSession, error) {
	ws, _, err := websocket.DefaultDialer.Dial(wsBase()+"/api/remote/guacamole?ticket="+ticket, nil)
	if err != nil {
		return nil, err
	}
	gs := &guacSession{
		ws:       ws,
		instrCh:  make(chan instr, 256),
		errCh:    make(chan error, 4),
		opCounts: map[string]int{},
		clipCh:   make(chan string, 4),
		pingStop: make(chan struct{}),
	}
	go gs.readLoop()
	go gs.pingLoop()
	// 首条：网关内部 UUID 指令（单元素、空 opcode）
	select {
	case in := <-gs.instrCh:
		if in.Opcode == "" && len(in.Args) == 1 {
			gs.sessionID = in.Args[0]
		} else {
			return gs, fmt.Errorf("首条指令非内部 UUID: %+v", in)
		}
	case err := <-gs.errCh:
		return gs, err
	case <-time.After(10 * time.Second):
		return gs, fmt.Errorf("等待网关 UUID 指令超时")
	}
	return gs, nil
}

func (gs *guacSession) readLoop() {
	var buf []byte
	// 阻塞读、不设 deadline：gorilla 在 read 出错（含超时）后进入 failed
	// 态，再 read 直接 panic「repeated read on failed websocket
	// connection」——ping 心跳由 pingLoop 独立 goroutine 承担
	for {
		_, data, err := gs.ws.ReadMessage()
		if err != nil {
			gs.errCh <- err
			return
		}
		buf = append(buf, data...)
		for {
			i := bytes.IndexByte(buf, ';')
			if i < 0 {
				break
			}
			raw := buf[:i+1]
			buf = buf[i+1:]
			if in, ok := parseInstr(raw); ok {
				gs.mu.Lock()
				gs.opCounts[in.Opcode]++
				gs.mu.Unlock()
				gs.clipboardStream(in)
				gs.clientObligations(in)
				if in.Opcode == "clipboard" || in.Opcode == "ready" || in.Opcode == "error" ||
					in.Opcode == "disconnect" || len(gs.instrCh) < 200 {
					gs.instrCh <- in
				}
			}
		}
	}
}

// pingLoop 心跳：common-js 的 WebSocketTunnel 每 ~1s 发内部 ping、网关原样
// 回显；探针同款义务（guacd 15s 无指令判 "User is not responding"）
func (gs *guacSession) pingLoop() {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-gs.pingStop:
			return
		case <-ticker.C:
			gs.send("", "ping", strconv.FormatInt(time.Now().UnixMilli(), 10))
		}
	}
}

// clipboardStream 服务端 clipboard 流就地拼装（clipboard,<idx>,<mime>; →
// blob,<idx>,<b64>;* → end,<idx>;），收完解码投 clipCh
func (gs *guacSession) clipboardStream(in instr) {
	gs.clipMu.Lock()
	defer gs.clipMu.Unlock()
	switch in.Opcode {
	case "clipboard":
		ev("[clipdbg] clipboard args=%v", in.Args)
		if len(in.Args) > 0 {
			gs.clipIdx = in.Args[0]
			gs.clipB64.Reset()
		}
	case "blob":
		if len(in.Args) >= 2 && in.Args[0] == gs.clipIdx {
			gs.clipB64.WriteString(in.Args[1])
		}
	case "end":
		if len(in.Args) > 0 && in.Args[0] == gs.clipIdx {
			dec, err := base64.StdEncoding.DecodeString(gs.clipB64.String())
			if err != nil {
				dec = []byte("B64_ERR:" + gs.clipB64.String())
			}
			select {
			case gs.clipCh <- string(dec):
			default:
			}
		}
	}
}

// clientObligations 履行官方客户端义务（不做会导致 guacd 流控停滞/断连）：
//   - blob/img/argv 等流式指令 → ack.<idx>,OK,0;（guacd 流控等 ack，VNC/RDP
//     大量 img 流不 ack 会 "User is not responding" 断流）
//   - sync → 回同时间戳 sync（帧级流控，common-js display.flush 后同款）
func (gs *guacSession) clientObligations(in instr) {
	switch in.Opcode {
	case "blob", "img", "argv", "audio", "file", "pipe", "end":
		if len(in.Args) > 0 {
			gs.send("ack", in.Args[0], "OK", "0")
		}
	case "sync":
		if len(in.Args) > 0 {
			gs.send("sync", in.Args[0])
		}
	}
}

// parseInstr 解析一条分号结尾的 Guacamole 指令
func parseInstr(raw []byte) (instr, bool) {
	s := strings.TrimSuffix(string(raw), ";")
	if s == "" {
		return instr{}, false
	}
	parts := strings.Split(s, ",")
	out := instr{}
	for _, p := range parts {
		dot := strings.IndexByte(p, '.')
		if dot < 0 {
			return instr{}, false
		}
		out.Args = append(out.Args, p[dot+1:])
	}
	if len(out.Args) == 0 {
		return instr{}, false
	}
	out.Opcode = out.Args[0]
	out.Args = out.Args[1:]
	return out, true
}

func (gs *guacSession) send(opcode string, args ...string) {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	_ = gs.ws.WriteMessage(websocket.TextMessage, []byte(guacEncode(opcode, args...)))
}

// waitReady 等 guacd 的 ready 指令（连接成功的权威信号）
func (gs *guacSession) waitReady(timeout time.Duration) error {
	deadline := time.After(timeout)
	for {
		select {
		case in := <-gs.instrCh:
			if in.Opcode == "ready" {
				return nil
			}
			if in.Opcode == "error" {
				return fmt.Errorf("guacd error: %v", in.Args)
			}
			if in.Opcode == "disconnect" {
				return fmt.Errorf("guacd disconnect: %v", in.Args)
			}
		case err := <-gs.errCh:
			return fmt.Errorf("ws 断开: %v", err)
		case <-deadline:
			return fmt.Errorf("等待 ready 超时")
		}
	}
}

// waitClipboardData 等待 readLoop 旁路拼装完的一条 clipboard 流载荷
func (gs *guacSession) waitClipboardData(timeout time.Duration) (string, bool) {
	select {
	case got := <-gs.clipCh:
		return got, true
	case <-gs.errCh:
		return "", false
	case <-time.After(timeout):
		return "", false
	}
}

// waitForOpcode 等待指定 opcode（消费其它指令）
func (gs *guacSession) waitForOpcode(op string, timeout time.Duration) (instr, bool) {
	deadline := time.After(timeout)
	for {
		select {
		case in := <-gs.instrCh:
			if in.Opcode == op {
				return in, true
			}
		case <-gs.errCh:
			return instr{}, false
		case <-deadline:
			return instr{}, false
		}
	}
}

// keysym 常量（X11）
const (
	keyEnter  = 0xFF0D
	keyEsc    = 0xFF1B
	keyCtrlL  = 0xFFE3
	keyShiftL = 0xFFE1
)

// typeText 逐字符 press/release
func (gs *guacSession) typeText(s string) {
	for _, r := range s {
		gs.send("key", strconv.Itoa(int(r)), "1")
		gs.send("key", strconv.Itoa(int(r)), "0")
		time.Sleep(8 * time.Millisecond)
	}
}

func (gs *guacSession) typeLine(s string) {
	gs.typeText(s)
	gs.send("key", strconv.Itoa(keyEnter), "1")
	gs.send("key", strconv.Itoa(keyEnter), "0")
}

func (gs *guacSession) count(op string) int {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	return gs.opCounts[op]
}

func (gs *guacSession) totalOps() int {
	gs.mu.Lock()
	defer gs.mu.Unlock()
	n := 0
	for _, c := range gs.opCounts {
		n += c
	}
	return n
}

func (gs *guacSession) close() {
	if gs == nil || gs.closed {
		return
	}
	gs.closed = true
	close(gs.pingStop)
	_ = gs.ws.Close()
}

// ---------- 侧信道：容器/本机命令 ----------

// dockerExec 容器侧信道读文件/执行命令；失败返回 ""（错误另行记证据，
// 不混进断言文本——错误串里若含标记词会造成假阳性）
func dockerExec(args ...string) string {
	cmd := exec.Command("docker", append([]string{"exec", *container}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		ev("[sidechannel] docker exec 失败: %v: %s", err, strings.TrimSpace(string(out)))
		return ""
	}
	return strings.TrimSpace(string(out))
}

// hostReadFile 本机侧信道（S4 本机 sshd 场景）；失败同上返回 ""
func hostReadFile(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		ev("[sidechannel] host read 失败: %v", err)
		return ""
	}
	return strings.TrimSpace(string(b))
}

func fileReadCmd(p string) string { return "cat " + p }

// ---------- 场景 ----------

// openSSHSession 建票据 + WS + ready；返回会话
func openSSHSession(name string, extra map[string]interface{}) *guacSession {
	p := map[string]interface{}{
		"agent_id": *agentID, "host": *sshHost, "port": *sshPort,
		"protocol": "ssh", "username": *sshUser, "password": *sshPass,
		// password 恒带：私钥场景 guacd 侧 key 优先（ssh.c if/else），
		// 口令场景 S5–S10 必需（此前缺此字段致 6 场景认证失败）
	}
	for k, v := range extra {
		p[k] = v
	}
	ticket, st, body := createTicket(p)
	if st != 200 {
		check(name, false, fmt.Sprintf("建票据失败 HTTP %d: %s", st, body))
		return nil
	}
	gs, err := dialGuac(ticket)
	if err != nil {
		check(name, false, fmt.Sprintf("WS 建连失败: %v", err))
		return nil
	}
	if err := gs.waitReady(15 * time.Second); err != nil {
		gs.close()
		check(name, false, fmt.Sprintf("ready 未到: %v", err))
		return nil
	}
	return gs
}

// authScenario 通用认证场景：登录 → 输出标记文件 → 侧信道核对
func authScenario(name, marker string, extra map[string]interface{}, readFile func() string) {
	if !sceneWanted(name) {
		return
	}
	gs := openSSHSession(name, extra)
	if gs == nil {
		return
	}
	defer gs.close()
	time.Sleep(1200 * time.Millisecond) // 等 shell 提示符就绪
	gs.typeLine("echo " + marker + " > " + markerPath(marker))
	time.Sleep(1500 * time.Millisecond)
	got := readFile()
	check(name, strings.Contains(got, marker), fmt.Sprintf("远端回读=%q session=%s", got, gs.sessionID))
}

var markerDir = "/tmp"

func markerPath(m string) string { return markerDir + "/" + m + ".txt" }

func main() {
	flag.Parse()

	repo, _ := os.Getwd()
	if _, err := os.Stat("go.mod"); err != nil {
		fatal("请在仓库根目录运行（找不到 go.mod，cwd=%s）", repo)
	}
	if *keysDir == "" {
		*keysDir = filepath.Join(repo, ".acceptance/guac/keys")
	}
	if *cuiKey == "" {
		home, _ := os.UserHomeDir()
		*cuiKey = filepath.Join(home, ".ssh/cui")
	}
	if *cidUser == "" {
		*cidUser = os.Getenv("USER")
	}
	if *evDir == "" {
		*evDir = filepath.Join(repo, ".acceptance/guac/evidence")
	}
	_ = os.MkdirAll(*evDir, 0o755)
	f, err := os.Create(filepath.Join(*evDir, "probe.log"))
	if err != nil {
		fatal("证据文件创建失败: %v", err)
	}
	evFile = f
	defer evFile.Close()

	ev("=== 远控三协议协议级验收探针 %s ===", time.Now().Format(time.RFC3339))
	login()
	ev("登录成功 user=%s", *adminUser)

	sshKey := func(name string) string {
		b, err := os.ReadFile(filepath.Join(*keysDir, name))
		if err != nil {
			fatal("读私钥失败 %s: %v", name, err)
		}
		return string(b)
	}

	// ---- S1 口令认证（容器 sshd）----
	authScenario("S1 SSH 口令认证（accept/Accept-Guac-2026 → 127.0.0.1:2222）", "AUTH_PW_OK",
		map[string]interface{}{"password": *sshPass},
		func() string { return dockerExec("sh", "-c", fileReadCmd(markerPath("AUTH_PW_OK"))) })

	// ---- S2 ed25519（OpenSSH 新格式）私钥认证 ----
	authScenario("S2 SSH 私钥认证 ed25519 OpenSSH 格式", "AUTH_ED_OK",
		map[string]interface{}{"private_key": sshKey("ed25519")},
		func() string { return dockerExec("sh", "-c", fileReadCmd(markerPath("AUTH_ED_OK"))) })

	// ---- S3 RSA PEM 私钥认证（对照 guacd/libssh2 兼容面）----
	authScenario("S3 SSH 私钥认证 RSA PEM 格式", "AUTH_RSA_OK",
		map[string]interface{}{"private_key": sshKey("rsa_pem")},
		func() string { return dockerExec("sh", "-c", fileReadCmd(markerPath("AUTH_RSA_OK"))) })

	// ---- S4 本机真 sshd（:22，cui 私钥，只读）----
	cuiKeyBody, err := os.ReadFile(*cuiKey)
	if err == nil {
		func() {
			name := "S4 本机 sshd 私钥认证（127.0.0.1:22 + " + filepath.Base(*cuiKey) + "）"
			gs := openSSHSession(name, map[string]interface{}{
				"host": "127.0.0.1", "port": 22, // 显式指向本机 sshd（默认值是容器 2222）
				"private_key": string(cuiKeyBody), "username": *cidUser,
			})
			if gs == nil {
				return
			}
			defer gs.close()
			// 本机登录 shell（zsh + rc 文件）首登远慢于容器 sh：等 3s、
			// 先发空行促提示符就绪再打命令（S1-S3 的 1.2s 不够）
			time.Sleep(3 * time.Second)
			gs.typeLine("")
			time.Sleep(500 * time.Millisecond)
			gs.typeLine("echo AUTH_CUI_OK > " + markerPath("AUTH_CUI_OK"))
			got := ""
			for i := 0; i < 4; i++ { // 慢 shell 兜底：2s×4 轮询
				time.Sleep(2 * time.Second)
				got = hostReadFile(markerPath("AUTH_CUI_OK"))
				if strings.Contains(got, "AUTH_CUI_OK") {
					break
				}
			}
			check(name, strings.Contains(got, "AUTH_CUI_OK"),
				fmt.Sprintf("本机回读=%q session=%s", got, gs.sessionID))
		}()
	} else {
		ev("[SKIP] S4 本机 sshd 私钥认证 — %s 不可读: %v", *cuiKey, err)
	}

	// ---- S5 尺寸同步：size 指令 → PTY 行/列随之变化 ----
	func() {
		name := "S5 窗口 size 变更 → stty 行/列同步"
		if !sceneWanted(name) {
			return
		}
		gs := openSSHSession(name, nil)
		if gs == nil {
			return
		}
		defer gs.close()
		time.Sleep(1200 * time.Millisecond)
		readSize := func(tag string) (rows, cols int) {
			gs.typeLine("stty size > " + markerPath(tag))
			time.Sleep(1200 * time.Millisecond)
			out := dockerExec("sh", "-c", fileReadCmd(markerPath(tag)))
			fs := strings.Fields(out)
			if len(fs) == 2 {
				rows, _ = strconv.Atoi(fs[0])
				cols, _ = strconv.Atoi(fs[1])
			}
			return
		}
		r1, c1 := readSize("SZ_A") // 初值：网关握手 size 1280x800
		// 放大窗口（像素）→ 行列应变大
		gs.send("size", "1920", "1080", "96")
		time.Sleep(1200 * time.Millisecond)
		r2, c2 := readSize("SZ_B")
		// 复原 → 应回到初值（确定性）
		gs.send("size", "1280", "800", "96")
		time.Sleep(1200 * time.Millisecond)
		r3, c3 := readSize("SZ_C")
		ok := r2 > r1 && c2 > c1 && r3 == r1 && c3 == c1 && r1 > 0 && c1 > 0
		check(name, ok, fmt.Sprintf("1280x800=(%d行,%d列) → 1920x1080=(%d行,%d列) → 复原=(%d行,%d列) session=%s",
			r1, c1, r2, c2, r3, c3, gs.sessionID))
	}()

	// ---- S6 vim 全屏程序 ----
	func() {
		name := "S6 vim 全屏编辑（输入穿透 + 落盘）"
		if !sceneWanted(name) {
			return
		}
		gs := openSSHSession(name, nil)
		if gs == nil {
			return
		}
		defer gs.close()
		time.Sleep(1200 * time.Millisecond)
		dockerExec("rm", "-f", markerPath("VIMTXT"))
		// 容器镜像只带 vim.tiny（命令名是 vi），vim 命令不存在
		gs.typeLine("vi -u NONE " + markerPath("VIMTXT"))
		time.Sleep(2500 * time.Millisecond)
		gs.typeText("i") // 进入插入模式
		time.Sleep(300 * time.Millisecond)
		gs.typeText("ACCEPT-VIM-FULLSCREEN-2026")
		time.Sleep(300 * time.Millisecond)
		gs.send("key", strconv.Itoa(keyEsc), "1")
		gs.send("key", strconv.Itoa(keyEsc), "0")
		time.Sleep(400 * time.Millisecond)
		gs.typeText(":wq")
		gs.send("key", strconv.Itoa(keyEnter), "1")
		gs.send("key", strconv.Itoa(keyEnter), "0")
		time.Sleep(2000 * time.Millisecond)
		got := dockerExec("sh", "-c", fileReadCmd(markerPath("VIMTXT")))
		check(name, strings.Contains(got, "ACCEPT-VIM-FULLSCREEN-2026"),
			fmt.Sprintf("vim 落盘内容=%q session=%s", got, gs.sessionID))
	}()

	// ---- S7 top 全屏重绘（指令流统计对照）----
	func() {
		name := "S7 top 全屏周期重绘（img/copy/sync 指令流量）"
		if !sceneWanted(name) {
			return
		}
		gs := openSSHSession(name, nil)
		if gs == nil {
			return
		}
		defer gs.close()
		time.Sleep(1500 * time.Millisecond)
		baseTotal := gs.totalOps()
		time.Sleep(2500 * time.Millisecond)
		idle := gs.totalOps() - baseTotal

		gs.typeLine("top -d 1")
		topStart := gs.totalOps()
		time.Sleep(4500 * time.Millisecond)
		topFrames := gs.totalOps() - topStart
		gs.typeText("q")

		check(name, topFrames > idle*3 && topFrames > 100,
			fmt.Sprintf("空闲 %d 条/2.5s vs top %d 条/4.5s（img=%d copy=%d rect=%d sync=%d cursor=%d）session=%s",
				idle, topFrames, gs.count("img"), gs.count("copy"), gs.count("rect"), gs.count("sync"), gs.count("cursor"), gs.sessionID))
	}()

	// ---- S8 剪贴板：浏览器 → 远端 ----
	func() {
		name := "S8 剪贴板 浏览器→远端（clipboard 指令 + Ctrl+Shift+V）"
		if !sceneWanted(name) {
			return
		}
		gs := openSSHSession(name, nil)
		if gs == nil {
			return
		}
		defer gs.close()
		time.Sleep(1200 * time.Millisecond)
		gs.typeLine("read CL; echo GOT:$CL > " + markerPath("CLIP_IN"))
		time.Sleep(1500 * time.Millisecond)
		paste := func(round string) bool {
			// 客户端→服务端 clipboard 指令是 clipboard,<idx>,<mimetype>;
			// （__guac_handle_clipboard 对 argv[0] 做 atoi 取流号——首参
			// 不是 mimetype！）数据走 blob,<idx>,<b64>;、end,<idx>; 收口
			// 载荷补结尾换行：远端 read 按行阻塞，官方复制命令行文本
			// 本就含 \n，无换行则 read 永不返回
			gs.send("clipboard", "7", "text/plain")
			gs.send("blob", "7", base64.StdEncoding.EncodeToString([]byte("PROBE-PASTE-2026\n")))
			gs.send("end", "7")
			time.Sleep(500 * time.Millisecond)
			dockerExec("rm", "-f", markerPath("CLIP_IN"))
			switch round {
			case "right-click":
				// terminal.c:1715 右/中键弹起即粘贴（官方路径）
				gs.send("mouse", "200", "100", "4")
				time.Sleep(80 * time.Millisecond)
				gs.send("mouse", "200", "100", "0")
			case "ctrl-V":
				// terminal.c:1522 粘贴键是大写 V(0x56)+Ctrl（非小写 v+Ctrl+Shift）
				for _, ks := range []struct {
					sym  int
					down bool
				}{{keyCtrlL, true}, {0x56, true}, {0x56, false}, {keyCtrlL, false}} {
					gs.send("key", strconv.Itoa(ks.sym), map[bool]string{true: "1", false: "0"}[ks.down])
					time.Sleep(80 * time.Millisecond)
				}
			}
			for i := 0; i < 4; i++ {
				time.Sleep(1 * time.Second)
				if strings.Contains(dockerExec("sh", "-c", fileReadCmd(markerPath("CLIP_IN"))), "PROBE-PASTE-2026") {
					return true
				}
			}
			return false
		}
		ok := paste("right-click") || paste("ctrl-V")
		check(name, ok,
			fmt.Sprintf("右键/大写V+Ctrl 两轮粘贴，远端 read 到标记=%v session=%s", ok, gs.sessionID))
	}()

	// ---- S9 剪贴板：远端 → 浏览器（鼠标拖选自动复制 → clipboard 指令）----
	func() {
		name := "S9 剪贴板 远端→浏览器（拖选复制 → clipboard 指令）"
		if !sceneWanted(name) {
			return
		}
		gs := openSSHSession(name, nil)
		if gs == nil {
			return
		}
		defer gs.close()
		time.Sleep(1500 * time.Millisecond)
		gs.typeLine("echo CLIP_FROM_REMOTE_2026")
		time.Sleep(1200 * time.Millisecond) // 等输出渲染进屏幕
		// guacamole-server 1.5.5 无 OSC52（源码零命中）；官方「远端→浏览器」
		// 路径 = 终端拖选，弹起时整段复制并下发 clipboard 指令（select.c）
		// 跨多行大范围拖选（提示符行+命令行+输出行全包）：单行 y 定位
		// 若落在空行，append_row 取 0 字符 → 复制流空 blob（实测定位）
		gs.send("mouse", "8", "5", "1") // 左上角按下
		time.Sleep(120 * time.Millisecond)
		gs.send("mouse", "300", "120", "1") // 中途拖动（选区逐格扩展）
		time.Sleep(120 * time.Millisecond)
		gs.send("mouse", "600", "350", "1") // 拖到右下角（按住左键）
		time.Sleep(120 * time.Millisecond)
		gs.send("mouse", "600", "350", "0") // 弹起 → 复制
		got, ok := gs.waitClipboardData(6 * time.Second)
		if !ok {
			check(name, false, fmt.Sprintf("6s 内未收到完整 clipboard 流（clipboard=%d blob=%d end=%d）",
				gs.count("clipboard"), gs.count("blob"), gs.count("end")))
			return
		}
		check(name, strings.Contains(got, "CLIP_FROM_REMOTE_2026"),
			fmt.Sprintf("clipboard 流载荷=%q（选中行文本；clipboard=%d blob=%d end=%d）session=%s",
				got, gs.count("clipboard"), gs.count("blob"), gs.count("end"), gs.sessionID))
	}()

	// ---- S10 录制收集 + 回放取流 ----
	var recSessionID string
	func() {
		name := "S10 .guac 录制落盘收集 + /recordings 列表 + cast 回放"
		if !sceneWanted(name) {
			return
		}
		gs := openSSHSession(name, nil)
		if gs == nil {
			return
		}
		recSessionID = gs.sessionID
		defer gs.close()
		time.Sleep(1200 * time.Millisecond)
		for i := 0; i < 3; i++ {
			gs.typeLine("echo GUAC_REC_PROBE_" + strconv.Itoa(i))
			time.Sleep(400 * time.Millisecond)
		}
		time.Sleep(1500 * time.Millisecond)
		gs.close() // 触发 closeGuacamoleSession → collectGuacRecording

		// 等 DB 元数据收口（duration/size 回填）
		var entry map[string]interface{}
		for i := 0; i < 20; i++ {
			st, body := api("GET", "/api/recordings", nil)
			if st == 200 {
				var r struct {
					Data []map[string]interface{} `json:"data"`
				}
				if json.Unmarshal(body, &r) == nil {
					for _, e := range r.Data {
						if fmt.Sprint(e["sessionId"]) == recSessionID {
							entry = e
							break
						}
					}
				}
			}
			if entry != nil {
				break
			}
			time.Sleep(500 * time.Millisecond)
		}
		if entry == nil {
			check(name, false, "录制元数据未出现在 /api/recordings（session="+recSessionID+"）")
			return
		}
		dur, _ := entry["durationMs"].(float64)
		if dur == 0 { // 收口未落，再等一轮
			time.Sleep(2 * time.Second)
			if st, body := api("GET", "/api/recordings", nil); st == 200 {
				var r struct {
					Data []map[string]interface{} `json:"data"`
				}
				if json.Unmarshal(body, &r) == nil {
					for _, e := range r.Data {
						if fmt.Sprint(e["sessionId"]) == recSessionID {
							entry = e
						}
					}
				}
			}
			dur, _ = entry["durationMs"].(float64)
		}
		// cast 取流
		st, body := api("GET", "/api/recordings/"+recSessionID+"/cast", nil)
		castOK := st == 200 && len(body) > 0 && dur > 0
		_ = os.WriteFile(filepath.Join(*evDir, recSessionID+".guac"), body, 0o644)
		check(name, castOK,
			fmt.Sprintf("meta=%s cast HTTP=%d bytes=%d durationMs=%.0f（证据已存 evidence/%s.guac）", marshalShort(entry), st, len(body), dur, recSessionID))
	}()

	// ---- S11 审计链路 ----
	func() {
		name := "S11 远控审计（remote_start/remote_end 落账、凭据不泄漏）"
		if !sceneWanted(name) {
			return
		}
		if recSessionID == "" {
			check(name, false, "S10 未取得会话 ID，审计断言跳过前提不成立")
			return
		}
		time.Sleep(1200 * time.Millisecond) // 等 remote_end 写入
		st, body := api("GET", "/api/admin/audit/logs?action=remote_start&page_size=100", nil)
		if st != 200 {
			check(name, false, fmt.Sprintf("audit logs HTTP %d", st))
			return
		}
		startHas := strings.Contains(string(body), recSessionID)
		st2, body2 := api("GET", "/api/admin/audit/logs?action=remote_end&page_size=100", nil)
		endHas := st2 == 200 && strings.Contains(string(body2), recSessionID)
		// 凭据泄漏扫描（两种审计响应体）
		leak := strings.Contains(string(body), *sshPass) || strings.Contains(string(body2), *sshPass) ||
			strings.Contains(string(body), "PRIVATE KEY") || strings.Contains(string(body2), "PRIVATE KEY")
		st3, _ := api("GET", "/api/admin/audit/logs?action=remote_start&status=failure&page_size=100", nil)
		_ = st3
		check(name, startHas && endHas && !leak,
			fmt.Sprintf("remote_start 含会话=%v remote_end 含会话=%v 口令/私钥泄漏=%v session=%s", startHas, endHas, leak, recSessionID))
		ev("      审计样本（remote_start 首条含 details）: %s", auditSample())
	}()

	// ---- S12 出口策略拒绝 ----
	func() {
		name := "S12 出口策略：allow-list 外目标 403 + failure 审计"
		if !sceneWanted(name) {
			return
		}
		_, st, body := createTicket(map[string]interface{}{
			"agent_id": *agentID, "host": "203.0.113.10", "port": 22,
			"protocol": "ssh", "username": "x", "password": "y",
		})
		denied := st == 403
		time.Sleep(800 * time.Millisecond)
		st2, body2 := api("GET", "/api/admin/audit/logs?action=remote_start&status=failure&page_size=20", nil)
		audited := st2 == 200 && strings.Contains(string(body2), "203.0.113.10")
		check(name, denied && audited, fmt.Sprintf("HTTP=%d body=%q failure审计含目标=%v", st, truncate(body, 120), audited))
	}()

	// ---- S13 VNC（容器 Xvnc + xterm）----
	func() {
		name := "S13 VNC 链路（Xvnc:5900 口令认证 + 图形指令流）"
		if !sceneWanted(name) {
			return
		}
		ticket, st, body := createTicket(map[string]interface{}{
			"agent_id": *agentID, "host": "127.0.0.1", "port": 5900,
			"protocol": "vnc", "password": *vncPass, "width": 1280, "height": 800,
		})
		if st != 200 {
			check(name, false, fmt.Sprintf("建票据 HTTP %d: %s", st, body))
			return
		}
		gs, err := dialGuac(ticket)
		if err != nil {
			check(name, false, fmt.Sprintf("WS: %v", err))
			return
		}
		defer gs.close()
		if err := gs.waitReady(15 * time.Second); err != nil {
			check(name, false, fmt.Sprintf("ready: %v", err))
			return
		}
		time.Sleep(2500 * time.Millisecond) // 等首帧（静止桌面仅首屏 img）
		draw := gs.count("img") + gs.count("copy") + gs.count("rect")
		// 静止桌面无 dirty rect（img=2 首帧而已），双路制造屏幕活动：
		// 1) 键盘：先点击 xterm 中心取焦（Xvnc 无 WM，焦点不保证）再打字
		gs.send("mouse", "350", "300", "1")
		time.Sleep(120 * time.Millisecond)
		gs.send("mouse", "350", "300", "0")
		time.Sleep(300 * time.Millisecond)
		gs.typeText("vnc-probe-2026-activity")
		gs.send("key", strconv.Itoa(keyEnter), "1")
		gs.send("key", strconv.Itoa(keyEnter), "0")
		// 2) 侧信道：直接向 xterm 里 bash 的 tty 注入输出（纯服务端屏幕
		// 变化，不依赖焦点，验证 VNC 增量帧链路本身）
		dockerExec("sh", "-c", "for d in /proc/[0-9]*; do [ \"$(cat $d/cmdline 2>/dev/null | tr -d '\\000')\" = bash ] && echo VNC_SIDECAR_ACTIVITY >> $d/fd/1; done")
		time.Sleep(2500 * time.Millisecond)
		draw2 := gs.count("img") + gs.count("copy") + gs.count("rect")
		check(name, draw2 > 10 && draw2 > draw,
			fmt.Sprintf("打字前 img+copy+rect=%d 打字后=%d（img=%d copy=%d rect=%d sync=%d）session=%s",
				draw, draw2, gs.count("img"), gs.count("copy"), gs.count("rect"), gs.count("sync"), gs.sessionID))
	}()

	// ---- S14 RDP（容器 xrdp，尽力而为）----
	func() {
		name := "S14 RDP 链路（xrdp:3389）"
		if !sceneWanted(name) {
			return
		}
		ticket, st, body := createTicket(map[string]interface{}{
			"agent_id": *agentID, "host": "127.0.0.1", "port": 3389,
			"protocol": "rdp", "username": *sshUser, "password": *sshPass,
			"width": 1280, "height": 800,
		})
		if st != 200 {
			check(name, false, fmt.Sprintf("建票据 HTTP %d: %s", st, body))
			return
		}
		gs, err := dialGuac(ticket)
		if err != nil {
			check(name, false, fmt.Sprintf("WS: %v", err))
			return
		}
		defer gs.close()
		if err := gs.waitReady(20 * time.Second); err != nil {
			check(name, false, fmt.Sprintf("ready 未到（xrdp 容器尽力而为）: %v", err))
			return
		}
		time.Sleep(2500 * time.Millisecond)
		draw := gs.count("img") + gs.count("copy") + gs.count("rect")
		// 同 S13：登录屏/静止桌面帧少，打字制造活动看增量流
		gs.typeText("rdp-probe-2026")
		gs.send("key", strconv.Itoa(keyEnter), "1")
		gs.send("key", strconv.Itoa(keyEnter), "0")
		time.Sleep(2500 * time.Millisecond)
		draw2 := gs.count("img") + gs.count("copy") + gs.count("rect")
		check(name, draw2 > 5 && draw2 > draw,
			fmt.Sprintf("打字前 img+copy+rect=%d 打字后=%d（img=%d copy=%d rect=%d sync=%d）session=%s",
				draw, draw2, gs.count("img"), gs.count("copy"), gs.count("rect"), gs.count("sync"), gs.sessionID))
	}()

	// ---- S15 内置终端兜底（不经 guacd：agent 直连 SSH）----
	func() {
		name := "S15 内置终端（经 Agent）兜底链路（guacd 无关）"
		if !sceneWanted(name) {
			return
		}
		ticket, st, body := createTicket(map[string]interface{}{
			"agent_id": *agentID, "host": *sshHost, "port": *sshPort,
			"protocol": "ssh", "username": *sshUser, "password": *sshPass,
		})
		if st != 200 {
			check(name, false, fmt.Sprintf("建票据 HTTP %d: %s", st, body))
			return
		}
		d := &websocket.Dialer{Subprotocols: []string{ticket}, HandshakeTimeout: 15 * time.Second}
		ws2, _, err := d.Dial(wsBase()+"/api/remote/terminal", nil)
		if err != nil {
			check(name, false, fmt.Sprintf("terminal WS（subprotocol 票据）: %v", err))
			return
		}
		defer ws2.Close()
		// connect 帧
		ws2.SetReadDeadline(time.Now().Add(15 * time.Second))
		_, msg, err := ws2.ReadMessage()
		if err != nil {
			check(name, false, fmt.Sprintf("首帧: %v", err))
			return
		}
		if !strings.Contains(string(msg), `"connect"`) {
			check(name, false, "首帧非 connect: "+string(msg))
			return
		}
		dockerExec("rm", "-f", markerPath("FB"))
		_ = ws2.WriteJSON(map[string]string{"type": "input", "data": "echo FALLBACK_OK > " + markerPath("FB") + "\n"})
		gotData := false
		deadline := time.Now().Add(8 * time.Second)
		for time.Now().Before(deadline) {
			ws2.SetReadDeadline(time.Now().Add(2 * time.Second))
			_, msg, err := ws2.ReadMessage()
			if err != nil {
				break
			}
			if strings.Contains(string(msg), `"data"`) {
				gotData = true
			}
		}
		time.Sleep(1000 * time.Millisecond)
		out := dockerExec("sh", "-c", fileReadCmd(markerPath("FB")))
		check(name, strings.Contains(out, "FALLBACK_OK") && gotData,
			fmt.Sprintf("终端回显=%v 远端落盘=%q", gotData, out))
	}()

	// ---- S16 guacd 停机兜底（guac 入口失败 + 内置终端照常）----
	func() {
		name := "S16 guacd 停机兜底（guac WS 失败、内置终端仍可用）"
		// 停 guacd（验收自建容器，结束前恢复）
		if out, err := exec.Command("docker", "stop", "cockpit-acc-guacd").CombinedOutput(); err != nil {
			check(name, false, fmt.Sprintf("docker stop guacd 失败: %v %s", err, out))
			return
		}
		defer func() {
			if out, err := exec.Command("docker", "start", "cockpit-acc-guacd").CombinedOutput(); err != nil {
				ev("[FATAL] guacd 容器恢复失败（后续 CDP 阶段需手动 start）: %v %s", err, out)
			} else {
				ev("      guacd 容器已恢复（docker start）")
			}
		}()

		// 1) guac 入口：票据能建（票据与 guacd 无关），WS 隧道必须失败
		//    （网关 dial 127.0.0.1:4822 被拒）
		time.Sleep(500 * time.Millisecond) // 等 listen socket 释放
		ticket, st, body := createTicket(map[string]interface{}{
			"agent_id": *agentID, "host": *sshHost, "port": *sshPort,
			"protocol": "ssh", "username": *sshUser, "password": *sshPass,
		})
		guacFail := false
		guacDetail := ""
		if st == 200 && ticket != "" {
			gs2, err := dialGuac(ticket)
			if err != nil {
				guacFail, guacDetail = true, fmt.Sprintf("WS 拒连: %v", err)
			} else {
				// 网关 select write 失败 → WS 关闭（非 ready 即兜底生效）
				if err := gs2.waitReady(5 * time.Second); err != nil {
					guacFail, guacDetail = true, fmt.Sprintf("无 ready: %v", err)
				}
				gs2.close()
			}
		} else {
			guacFail, guacDetail = true, fmt.Sprintf("HTTP %d %s（票据侧）", st, truncate(body, 80))
		}

		// 2) 内置终端（经 Agent 直连 SSH）：guacd 停机不影响
		termOK := false
		termDetail := ""
		if st, _, _ := createTicket(map[string]interface{}{
			"agent_id": *agentID, "host": *sshHost, "port": *sshPort,
			"protocol": "ssh", "username": *sshUser, "password": *sshPass,
		}); true {
			// 复用 S15 的建连方式：terminal WS 用独立票据
			t2, st2, b2 := createTicket(map[string]interface{}{
				"agent_id": *agentID, "host": *sshHost, "port": *sshPort,
				"protocol": "ssh", "username": *sshUser, "password": *sshPass,
			})
			_ = st
			if st2 != 200 {
				termDetail = fmt.Sprintf("terminal 票据 HTTP %d: %s", st2, truncate(b2, 80))
			} else {
				d := &websocket.Dialer{Subprotocols: []string{t2}, HandshakeTimeout: 15 * time.Second}
				ws2, _, err := d.Dial(wsBase()+"/api/remote/terminal", nil)
				if err != nil {
					termDetail = fmt.Sprintf("terminal WS: %v", err)
				} else {
					ws2.SetReadDeadline(time.Now().Add(15 * time.Second))
					_, msg, err := ws2.ReadMessage()
					if err == nil && strings.Contains(string(msg), "\"connect\"") {
						termOK = true
						termDetail = "connect 帧到达"
					} else if err != nil {
						termDetail = fmt.Sprintf("首帧: %v", err)
					} else {
						termDetail = "首帧非 connect: " + truncate(string(msg), 80)
					}
					ws2.Close()
				}
			}
		}
		check(name, guacFail && termOK,
			fmt.Sprintf("guac 入口失败=%v（%s）；内置终端可用=%v（%s）", guacFail, guacDetail, termOK, termDetail))
	}()

	ev("=== 汇总：PASS=%d FAIL=%d（%s） ===", passes, fails, time.Now().Format(time.RFC3339))
	if fails > 0 {
		os.Exit(1)
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func marshalShort(v interface{}) string {
	b, _ := json.Marshal(v)
	return truncate(string(b), 300)
}

func auditSample() string {
	_, b := api("GET", "/api/admin/audit/logs?action=remote_start&page_size=1", nil)
	return truncate(string(b), 400)
}
