package server

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// Guacamole 网关（见 docs/remote-desktop-guacamole-design.md D2/D3/D4）：
// 浏览器 ↔ server 段是 WebSocket（文本帧即 Guacamole 指令流），server ↔
// guacd 段是 TCP（Guacamole 原生协议）。网关只做双向字节管道——**不解析
// 指令内容**（握手指令构造除外），协议演进由 guacamole-common-js 与 guacd
// 自己对齐（见设计风险章节「chunk/base64 规则照抄官方 Tunnel」）。
//
// 复用既有远控基础设施：一次性票据（ticket.go）、出口策略
// （matchRemoteEgress）、审计（auditRemoteStart/End）。

// guacdAddrEnv guacd 地址（Go 网关反代目标）。默认同机回环，跨机部署时
// 指向 guacd 主机内网地址（见 deployments/guacd/README.md）。
const guacdAddrEnv = "GUACD_ADDR"

// guacdDefaultAddr guacd 默认地址
const guacdDefaultAddr = "127.0.0.1:4822"

// guacdDialTimeout 拨 guacd 超时
const guacdDialTimeout = 10 * time.Second

// dialGuacd 拨 guacd TCP 调用点。var 化（非内联）仅为测试注入 mock conn——
// select write 失败分支需 guacdConn.Write 立即 error，而 TCP RST 时序
// （loopback 写缓冲 + goroutine 调度）本质不确定；生产行为不变
// （恒调 net.DialTimeout）。
var dialGuacd = func(addr string, timeout time.Duration) (net.Conn, error) {
	return net.DialTimeout("tcp", addr, timeout)
}

// guacamoleReadBuf guacd→WS 段读缓冲（Guacamole 指令是文本行，含 base64
// blob，单条可较大）。upgrader 按请求构造（子协议回显因票据而异），
// 见 handleGuacamoleWebSocket 第 4 步。
const guacamoleReadBuf = 256 * 1024

// guacdAddr 返回 guacd 地址（可经 GUACD_ADDR 覆盖）。
func guacdAddr() string {
	if v := os.Getenv(guacdAddrEnv); v != "" {
		return v
	}
	return guacdDefaultAddr
}

// GuacamoleSession 一条 Guacamole 桌面会话：浏览器 WS ↔ guacd TCP 双跳管道。
// 职责：票据校验（握手时）+ 出口策略 + 审计 + 双向字节管道。
type GuacamoleSession struct {
	ID       string
	UserID   string
	Username string
	Protocol string // rdp / vnc / ssh
	AgentID  string
	Host     string
	Port     int
	// Recording 本会话是否开了 guacd 录制（connect 带 recording-*）——
	// collect 用它做零延迟跳过（未录制会话不必等 guacd finalize 轮询）
	Recording bool
	// relay 目标转发器（guac_relay.go）：guacd 拨中继、agent 拨真实目标。
	// Host/Port 保留用户视角（审计/录制元数据/出口策略语义），connect 指令
	// 里放的是中继地址
	relay    *guacRelay
	ClientWS *websocket.Conn
	guacd     net.Conn
	guacdRd   *bufio.Reader // 握手阶段建立，guacdToWS 复用（select 响应不丢）
	Created   time.Time

	writeMu sync.Mutex // gorilla WS 单写者
	done    chan struct{}
	once    sync.Once
}

// guacEncode 编码一条 Guacamole 指令：长度前缀 + 逗号分隔参数 + 分号结尾。
// 格式照抄 guacamole-common-js 的 Tunnel.sendMessage（见设计风险章节
// 「chunk/base64 规则照抄官方 Tunnel 实现，别自由发挥」）。
// 例：guacEncode("size", "1024", "768", "96") → "4.size,4.1024,3.768,2.96;"
func guacEncode(opcode string, args ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%d.%s", len(opcode), opcode)
	for _, a := range args {
		fmt.Fprintf(&b, ",%d.%s", len(a), a)
	}
	b.WriteByte(';')
	return b.String()
}

// guacConnectArgs 组装 connect 指令参数：与 guacd args 指令声明的参数名
// 逐位对齐的值序列（未设置的参数留空占位，首元素回显协议版本位）。
// Guacamole 协议的 connect 是位置参数——稀疏 name=value 形态会被真 guacd
// 以「Client did not return the expected number of arguments」拒绝
// （真机验收 guacamole/guacd:1.5.5 发现，2026-09-30）。
// 凭据来自票据 params（handleTicketCreate 存储），不经浏览器二次经手
// （见设计「为什么参数由服务端放进 connect 指令」）。
func guacConnectArgs(argNames []string, protocol, host string, port int, params map[string]string, width, height int, record bool, recordingName string) []string {
	values := map[string]string{
		"hostname": host,
		"port":     strconv.Itoa(port),
	}
	if v := params["username"]; v != "" {
		values["username"] = v
	}
	if v := params["password"]; v != "" {
		values["password"] = v
	}
	if v := params["domain"]; v != "" {
		values["domain"] = v
	}
	switch protocol {
	case "rdp":
		// 办公场景验收（设计风险章节）：色深 32、忽略证书（自签/内网常见）。
		// 音频：不传 disable-audio（guacd 默认启用），音频流由服务端主动推
		// audio 指令 + blob 流，自动经本网关字节管道透传（见 todo.md M4 D1）
		values["security"] = "any"
		values["ignore-cert"] = "true"
		values["color-depth"] = "32"
		values["create-drive-path"] = "true"
		if width > 0 && height > 0 {
			values["width"] = strconv.Itoa(width)
			values["height"] = strconv.Itoa(height)
		}
	case "vnc":
		// VNC 的 password 即同名参数，上面已统一映射
		values["color-depth"] = "32"
	case "ssh":
		// guacd ssh 插件（libssh2，docs/remote-access-integration-design.md D2）：
		// username/password 上面已统一传；私钥走 private-key 参数。
		// 必须传 PEM 原文，不能 base64——settings.h 注释写 "encoded as
		// base64" 是误导，实现链 guac_user_parse_args_string →
		// guac_common_ssh_key_alloc 是 strlen+memcpy 零解码，base64 文本
		// 直接进 libssh2 PEM_read 必失败（真机 A/B 实测 2026-09-30：
		// base64 → "Unsupported private key file format"，原文 → 两格式
		// 认证全过、终端输出正常）。
		// width/height 不进 connect——字符终端，尺寸经隧道层 size 指令由
		// guacd 按字体度量换算列/行；domain 是 RDP 专属概念不传。
		if v := params["private_key"]; v != "" {
			values["private-key"] = v
		}
	}
	// 桌面会话录制（设计「guacd session recording 白捡」）：recording-path/
	// recording-name 指向 guacd 容器卷（deployments/guacd 的
	// guacd-recordings 卷挂载点）；recording.enabled 关闭时不设置
	if record {
		values["recording-path"] = guacamoleRecordingPath()
		values["recording-name"] = recordingName
		values["create-recording-path"] = "true"
	}
	out := make([]string, 0, len(argNames))
	for _, name := range argNames {
		if strings.HasPrefix(name, "VERSION_") {
			// args 列表首位的协议版本标记（1.5.0 握手协商），原样回显
			out = append(out, name)
			continue
		}
		out = append(out, values[name]) // 未配置参数留空占位
	}
	return out
}

// guacParseArgNames 解析 guacd 对 select 的响应（args 指令）：返回去 opcode
// 后的参数名列表；畸形输入返回 nil（调用方按握手失败处理）。
func guacParseArgNames(line string) []string {
	s := strings.TrimSuffix(line, ";")
	parts := strings.Split(s, ",")
	if len(parts) < 2 {
		return nil
	}
	names := make([]string, 0, len(parts)-1)
	for _, p := range parts[1:] {
		dot := strings.IndexByte(p, '.')
		if dot < 0 {
			return nil
		}
		names = append(names, p[dot+1:])
	}
	return names
}

// guacdRecordingPathEnv guacd 录制目录（M3 D3）：同一路径同时作 guacd 的
// recording-path 参数（guacd 内路径）与 server 的收集源路径——同机部署即
// 同目录；Docker 部署把同一卷挂到两侧同路径（deployments/guacd 的
// guacd-recordings 卷挂载点）。两侧视角合一避免双配置漂移。
const guacdRecordingPathEnv = "GUACD_RECORDING_PATH"

// guacamoleRecordingPath guacd 侧录制目录（可经 GUACD_RECORDING_PATH 覆盖）。
func guacamoleRecordingPath() string {
	if v := os.Getenv(guacdRecordingPathEnv); v != "" {
		return v
	}
	return "/var/lib/guacamole"
}

// collectGuacRecording 会话结束时把 guacd 卷里的 <sid>.guac 收到
// recordingsDir（M3 D2）并回填元数据。跨分区用 copy+remove（非 rename）；
// 源文件缺失静默跳过（guacd 未写或已被外部清理），失败只记日志不阻塞
// 会话出口路径（与 .cast 的「写失败停录不影响转发」同纪律）。
func (s *Server) collectGuacRecording(gs *GuacamoleSession) {
	// 未开录制（connect 指令未带 recording-*）零延迟跳过
	if !gs.Recording {
		return
	}
	src := filepath.Join(guacamoleRecordingPath(), gs.ID+".guac")
	// guacd 在 TCP 关闭后才 finalize 录制文件（client 析构时 flush）：
	// 先等文件出现，再等 size 稳定（两次一致），防拷到截断文件
	var size int64
	stable := false
	for i := 0; i < 10; i++ {
		info, err := os.Stat(src)
		if err == nil && info.Size() == size && size > 0 {
			stable = true
			break
		}
		if err == nil {
			size = info.Size()
		}
		time.Sleep(300 * time.Millisecond)
	}
	if !stable {
		log.Printf("Guacamole: recording %s not finalized by guacd, skip collect", gs.ID)
		return
	}
	info, err := os.Stat(src)
	if err != nil {
		return
	}
	dst := filepath.Join(s.recordingsDir(), gs.ID+".guac")
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		log.Printf("Guacamole: create recordings dir failed: %v", err)
		return
	}
	if err := copyFile(src, dst, 0600); err != nil {
		log.Printf("Guacamole: collect %s failed: %v", gs.ID, err)
		return
	}
	_ = os.Remove(src) // 收完即清 guacd 卷，防卷膨胀
	duration := time.Since(gs.Created).Milliseconds()
	if err := s.db.FinishTerminalRecording(gs.ID, duration, info.Size()); err != nil {
		log.Printf("Guacamole: finish recording meta %s failed: %v", gs.ID, err)
	}
	// 异地归档（不拖会话出口路径，与 .cast M2 D16 同款）
	go s.pushRecordingRemote(gs.ID)
}

// copyFile 跨分区复制（guacd 卷 → recordingsDir 可能不同挂载点，
// os.Rename 跨设备会失败）。
func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// registerGuacamoleAPI 注册 Guacamole 隧道端点
func (s *Server) registerGuacamoleAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/guacamole", s.handleGuacamoleWebSocket)
}

// handleGuacamoleWebSocket 浏览器 ↔ server 段：WS 文本帧即 Guacamole 协议指令。
//
// 票据传递走双通道（与 terminal/desktop 的 Sec-WebSocket-Protocol 同款一次性
// 票据，复用 ticket.go）：
//   - URL query ?ticket=（首选）：guacamole-common-js 的 WebSocketTunnel 把
//     client.connect(data) 的数据拼进 URL query 且**硬编码 subprotocol
//     "guacamole"**（new WebSocket(url + "?" + data, "guacamole")），无法自定义
//     subprotocol 传票据——这是上游实现约束，不是自由发挥；
//   - Sec-WebSocket-Protocol[0]（兼容位）：自定义 Tunnel 或不走 common-js 时
//     可沿用 terminal/desktop 的同款形状。
//
// 票据仍是一次性 5 分钟票据，泄漏窗口不变；WS 路径不进通用审计
// （AuditMiddleware 跳过），由专门的远控审计覆盖。
func (s *Server) handleGuacamoleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 1. 票据：query 优先，子协议兜底（复用 ticket.go，ValidateTicket 即消费）
	ticketID := r.URL.Query().Get("ticket")
	if ticketID == "" {
		if protocols := r.Header.Values("Sec-WebSocket-Protocol"); len(protocols) > 0 {
			ticketID = protocols[0]
		}
	}
	if ticketID == "" {
		http.Error(w, `{"error":"Missing ticket"}`, http.StatusUnauthorized)
		return
	}
	ticket, ok := s.ticketMgr.ValidateTicket(ticketID)
	if !ok {
		http.Error(w, `{"error":"Invalid or expired ticket"}`, http.StatusUnauthorized)
		return
	}

	agentID := ticket.Params["agent_id"]
	host := ticket.Params["host"]
	portStr := ticket.Params["port"]
	protocolStr := ticket.Params["protocol"]
	width, _ := strconv.Atoi(ticket.Params["width"])
	height, _ := strconv.Atoi(ticket.Params["height"])

	if agentID == "" || host == "" || portStr == "" {
		http.Error(w, `{"error":"Missing connection parameters"}`, http.StatusBadRequest)
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		http.Error(w, `{"error":"Invalid port"}`, http.StatusBadRequest)
		return
	}
	// Guacamole 承接 rdp/vnc/ssh 三协议（docs/remote-access-integration-design.md
	// D1）；telnet 走 TerminalModal（xterm.js）
	if protocolStr != "rdp" && protocolStr != "vnc" && protocolStr != "ssh" {
		http.Error(w, `{"error":"Unsupported protocol for Guacamole tunnel"}`, http.StatusBadRequest)
		return
	}

	// 2. 出口策略（复用 matchRemoteEgress：allow-list / CIDR / per-agent）
	egressMatch, reason := s.matchRemoteEgress(agentID, host, port)
	if egressMatch == nil {
		s.auditRemoteFailure(ticket.UserID, ticket.Username, r.RemoteAddr, r.UserAgent(),
			&audit.RemoteSessionDetails{
				Protocol: protocolStr,
				AgentID:  agentID,
				Host:     host,
				Port:     port,
				Reason:   reason,
			})
		rejectTargetDenied(w, reason)
		return
	}

	// 3. 拨 guacd TCP（不暴露 guacd 端口给浏览器——网关反代，见设计 D2）
	sessionID := uuid.New().String()
	guacdConn, err := dialGuacd(guacdAddr(), guacdDialTimeout)
	if err != nil {
		log.Printf("Guacamole: dial guacd %s failed: %v", guacdAddr(), err)
		s.auditRemoteFailure(ticket.UserID, ticket.Username, r.RemoteAddr, r.UserAgent(),
			&audit.RemoteSessionDetails{
				Protocol: protocolStr,
				AgentID:  agentID,
				Host:     host,
				Port:     port,
				Egress:   egressMatch.summary(),
				Reason:   "guacd unreachable",
			})
		http.Error(w, `{"error":"Guacamole daemon unavailable"}`, http.StatusBadGateway)
		return
	}

	// 3.5 目标转发器：guacd 拨回环中继，字节流经 agent WS 通道到 agent、
	// 由 agent 拨真实目标（架构：agent 是目标内网的唯一落点，guacd 不直拨
	// ——设计修正 2026-10-02）。中继起不来按 agent 不可达处理。
	relay, relayAddr, err := startGuacRelay(agentID, guacRelayPrefix+sessionID,
		net.JoinHostPort(host, portStr), s.SendToAgent)
	if err != nil {
		log.Printf("Guacamole: start target relay failed: %v", err)
		s.auditRemoteFailure(ticket.UserID, ticket.Username, r.RemoteAddr, r.UserAgent(),
			&audit.RemoteSessionDetails{
				Protocol: protocolStr,
				AgentID:  agentID,
				Host:     host,
				Port:     port,
				Egress:   egressMatch.summary(),
				Reason:   "target relay unavailable",
			})
		_ = guacdConn.Close()
		http.Error(w, `{"error":"Agent relay unavailable"}`, http.StatusBadGateway)
		return
	}

	// 4. 升级 WS。子协议必须回显其一：浏览器按 RFC 6455，服务端未选中任何
	// 所 offering 的子协议时直接判握手失败（1006 断开）。common-js 的
	// WebSocketTunnel 硬编码 offering "guacamole"；自定义 Tunnel 用票据作
	// 子协议传票据时回显票据本身（真机验收发现缺省不回显，2026-09-30）。
	up := websocket.Upgrader{
		CheckOrigin:     isOriginAllowed,
		ReadBufferSize:  guacamoleReadBuf,
		WriteBufferSize: guacamoleReadBuf,
		Subprotocols:    []string{"guacamole"},
	}
	if ticketID != "guacamole" {
		up.Subprotocols = append(up.Subprotocols, ticketID)
	}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Guacamole WebSocket upgrade failed: %v", err)
		_ = guacdConn.Close()
		relay.Close()
		return
	}

	session := &GuacamoleSession{
		ID:       sessionID,
		UserID:   ticket.UserID,
		Username: ticket.Username,
		Protocol: protocolStr,
		AgentID:  agentID,
		Host:     host,
		Port:     port,
		relay:    relay,
		ClientWS: conn,
		guacd:    guacdConn,
		Created:  time.Now(),
		done:     make(chan struct{}),
	}

	// 5a. tunnel UUID 先发浏览器（INTERNAL_DATA 单元素指令，即
	// Guacamole.Tunnel.INTERNAL_DATA_OPCODE 空 opcode）：common-js 首条指令
	// 若为内部指令单元素则 setUUID，否则仅置 OPEN（uuid 留 null）。
	// 这是 tunnel 层（浏览器↔网关），不进 guacd。
	session.writeWS([]byte(guacEncode("", sessionID)))

	// 5b. 握手（Guacamole 协议，设计 line 91 的 select/size/connect 序列）：
	// select → guacd 回参数列表 → size + 媒体能力声明 + connect。
	// audio/video 空声明 = 不启用（音频放阶段二，见设计风险章节）；image 必须声明：
	// 官方架构里 guacd 握手由服务端（Java webapp）代客户端完成，always 声明
	// image/png——若不声明，guacd 侧 user->info.image_mimetypes 为 NULL，渲染
	// 走 guac_user_supports_webp 遍历该数组时空指针解引用，guacd 子进程直接
	// segfault（首帧能渲染、一旦有输入触发新字形 img 流即断，VNC/RDP 同理）。
	// guacamole/guacd 1.5.5 与 1.6.0 均复现（真机验收定位，2026-09-30）。
	hsReader := bufio.NewReaderSize(guacdConn, guacamoleReadBuf)
	if _, err := guacdConn.Write([]byte(guacEncode("select", protocolStr))); err != nil {
		log.Printf("Guacamole: select write failed: %v", err)
		conn.Close()
		_ = guacdConn.Close()
		relay.Close()
		return
	}
	// 读 guacd 的 select 响应：args 参数名列表。connect 必须与这份列表逐位
	// 对齐（含 VERSION 版本位）——解析出名字再按位组装值
	// （真机验收发现稀疏形态被真 guacd 拒绝，2026-09-30）
	argsLine, err := hsReader.ReadString(';')
	if err != nil {
		log.Printf("Guacamole: read select response failed: %v", err)
		conn.Close()
		_ = guacdConn.Close()
		relay.Close()
		return
	}
	argNames := guacParseArgNames(argsLine)
	if len(argNames) == 0 {
		log.Printf("Guacamole: guacd args response malformed: %q", argsLine)
		conn.Close()
		_ = guacdConn.Close()
		relay.Close()
		return
	}
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 800
	}
	// recording-name 用会话 ID（可追溯到审计的 Session 字段）+ .guac 后缀：
	// guacd 把 recording-name 原样作文件名（不补后缀），不带后缀则
	// collectGuacRecording 的 <sid>.guac 永远 Stat 不到（真机验收发现
	// 2026-09-30：rec/ 里全是无后缀孤儿文件、录制页恒空）
	recording := s.recordingEnabled()
	session.Recording = recording
	// SSH 密钥自动获取：票据既没传 private_key 也没传 password 时，向 agent
	// 要默认密钥兜底（门控细则见 applySSHDefaultKey）
	if protocolStr == "ssh" {
		s.applySSHDefaultKey(ticket.Params, agentID, host)
	}

	// connect 的 hostname/port 用中继地址（guacd 拨回环，真实目标由 agent 拨）；
	// username/password/private-key 等凭据参数语义不变
	relayHost, relayPortStr, err := net.SplitHostPort(relayAddr)
	if err != nil {
		log.Printf("Guacamole: relay addr malformed %q: %v", relayAddr, err)
		conn.Close()
		_ = guacdConn.Close()
		relay.Close()
		return
	}
	relayPort, _ := strconv.Atoi(relayPortStr)
	connectArgs := guacConnectArgs(argNames, protocolStr, relayHost, relayPort, ticket.Params, width, height,
		recording, sessionID+".guac")
	handshake := guacEncode("size", strconv.Itoa(width), strconv.Itoa(height), "96") +
		guacEncode("audio") + // 空 = 不启用音频（阶段二）
		guacEncode("video") + // 空 = 不启用视频
		guacEncode("image", "image/png") + // 必须声明，见上方 5b 注释（guacd segfault 根因）
		guacEncode("connect", connectArgs...)
	if _, err := guacdConn.Write([]byte(handshake)); err != nil {
		log.Printf("Guacamole: handshake write failed: %v", err)
		conn.Close()
		_ = guacdConn.Close()
		relay.Close()
		return
	}
	session.guacdRd = hsReader

	// 5c. 录制元数据登记（M3 D2）：会话开始即登记（Format=guac），保持与
	// .cast「进行中也在列」语义一致；文件由 guacd 落盘、会话结束时收集
	if s.recordingEnabled() {
		if err := s.db.CreateTerminalRecording(&storage.TerminalRecording{
			SessionID: sessionID,
			Username:  ticket.Username,
			AgentID:   agentID,
			Host:      host,
			Port:      port,
			Protocol:  protocolStr,
			Format:    "guac",
			StartedAt: time.Now(),
		}); err != nil {
			log.Printf("Guacamole: create recording meta %s failed: %v", sessionID, err)
		}
	}

	// 6. 审计开始（复用 auditRemoteStart；不含口令——connect 参数的
	// password 不进审计详情）
	s.auditRemoteStart(audit.ActionRemoteStart, ticket.UserID, ticket.Username,
		r.RemoteAddr, r.UserAgent(),
		&audit.RemoteSessionDetails{
			Protocol: protocolStr,
			AgentID:  agentID,
			Host:     host,
			Port:     port,
			Session:  sessionID,
			Egress:   egressMatch.summary(),
		})

	log.Printf("Guacamole session created: %s -> %s:%d (%s)", sessionID, host, port, protocolStr)

	// 7. 双向字节管道：不解析指令内容（设计风险章节——协议私有，
	// Go 网关只做字节管道，编解码由 common-js 与 guacd 完成）
	go session.guacdToWS()
	session.wsToGuacd()

	// 8. 任一侧断开 → 关闭 + 审计结束
	s.closeGuacamoleSession(session)
}

// sshDefaultKeyLookup agent 默认 SSH 密钥查询。包级变量仅为测试可注入
// （同 wsRegistryLookup 惯例），生产路径固定打 ssh.getDefaultKey RPC。
var sshDefaultKeyLookup = func(s *Server, agentID string) (pem, username string, err error) {
	resp, err := s.CallAgent(agentID, "ssh.getDefaultKey", nil)
	if err != nil {
		return "", "", err
	}
	key, ok := resp.Payload["data"].(map[string]interface{})
	if !ok {
		return "", "", nil
	}
	pem, _ = key["privateKey"].(string)
	username, _ = key["username"].(string)
	return pem, username, nil
}

// applySSHDefaultKey SSH 密钥自动获取的门控：票据**既没传 private_key 也
// 没传 password** 时才向 agent 要默认密钥兜底。只在 private_key 上判空
// 会把口令认证也劫持成公钥认证——guacd/libssh2 双参数并存时优先公钥，
// agent 密钥又未必在目标机 authorized_keys 里，显式口令的会话直接起不来
// （2026-10-02 验收探针 S1/S5-S9 全挂的回归根因）。显式凭据（口令或密钥）
// 一律原样透传。
func (s *Server) applySSHDefaultKey(params map[string]string, agentID, host string) {
	if params["private_key"] != "" || params["password"] != "" {
		return
	}
	pem, username, err := sshDefaultKeyLookup(s, agentID)
	if err != nil {
		log.Printf("Guacamole: failed to get agent SSH key: %v", err)
		return
	}
	if pem != "" {
		params["private_key"] = pem
		log.Printf("Guacamole: using agent default SSH key for %s", host)
	}
	if username != "" && params["username"] == "" {
		params["username"] = username
	}
}

// wsToGuacd 浏览器 → guacd：WS 文本帧内容直写 TCP。
// 隧道内部控制指令（opcode 空 = Guacamole.Tunnel.INTERNAL_DATA_OPCODE）在此
// 终结：common-js 的 WebSocketTunnel 会周期发 ping（sendPing），期望服务端
// "respond with an identical ping"；不回显则 receiveTimeout（15s）判上游超时
// 关隧道、unstableThreshold（1.5s）判连接不稳。该层指令 guacd 不认识，绝不转发。
func (gs *GuacamoleSession) wsToGuacd() {
	defer close(gs.done)
	for {
		_, data, err := gs.ClientWS.ReadMessage()
		if err != nil {
			return
		}
		if guacIsInternal(data) {
			// 原样回显（identical ping）；uuid 等其他内部指令客户端自行忽略
			gs.writeWS(data)
			continue
		}
		if _, err := gs.guacd.Write(data); err != nil {
			return
		}
	}
}

// guacIsInternal 判定指令是否为隧道内部控制指令（opcode 长度 0）。
// 只读首段长度前缀，不解析参数内容——协议私有，网关不自由发挥（设计风险
// 章节「照抄官方 Tunnel」）。
func guacIsInternal(frame []byte) bool {
	dot := indexByte(frame, '.')
	if dot <= 0 {
		return false
	}
	n := 0
	for i := 0; i < dot; i++ {
		c := frame[i]
		if c < '0' || c > '9' {
			return false
		}
		n = n*10 + int(c-'0')
	}
	return n == 0
}

// guacdToWS guacd → 浏览器：按 Guacamole 指令边界（分号结尾）切分后
// 写 WS 文本帧。切分只做边界识别（分号），不解析参数内容。
func (gs *GuacamoleSession) guacdToWS() {
	// guacdRd 在握手阶段建立（select 响应消费用），总为非 nil
	reader := gs.guacdRd
	var pending []byte
	buf := make([]byte, 32*1024)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
			// 指令以 ';' 结尾；按边界切分逐条下发（保证 common-js 的
			// Tunnel 每帧一条完整指令）
			for {
				idx := indexByte(pending, ';')
				if idx < 0 {
					break
				}
				frame := pending[:idx+1]
				pending = pending[idx+1:]
				gs.writeWS(frame)
			}
		}
		if err != nil {
			if err != io.EOF {
				log.Printf("Guacamole: read from guacd failed: %v", err)
			}
			return
		}
	}
}

// writeWS 线程安全写 WS 文本帧
func (gs *GuacamoleSession) writeWS(payload []byte) {
	gs.writeMu.Lock()
	defer gs.writeMu.Unlock()
	_ = gs.ClientWS.WriteMessage(websocket.TextMessage, payload)
}

// indexByte 在字节切片里找首个 b 的位置（避免 strings 转换开销）
func indexByte(b []byte, c byte) int {
	for i, x := range b {
		if x == c {
			return i
		}
	}
	return -1
}

// closeGuacamoleSession 幂等关闭：WS + guacd TCP + 审计结束
func (s *Server) closeGuacamoleSession(gs *GuacamoleSession) {
	gs.once.Do(func() {
		gs.ClientWS.Close()
		_ = gs.guacd.Close()
		if gs.relay != nil {
			gs.relay.Close() // 拆中继 + 通知 agent 拆目标连接
		}
		log.Printf("Guacamole session closed: %s", gs.ID)

		// M3 D2：收集 guacd 卷里的 .guac → recordingsDir（先于审计，
		// 文件与元数据同刻回填；失败只记日志不阻塞审计出口）
		s.collectGuacRecording(gs)

		// 审计结束（含 duration；不含口令）
		s.auditRemoteEnd(gs.UserID, gs.Username, "", "",
			&audit.RemoteSessionDetails{
				Protocol: gs.Protocol,
				AgentID:  gs.AgentID,
				Host:     gs.Host,
				Port:     gs.Port,
				Session:  gs.ID,
				Duration: time.Since(gs.Created).String(),
			})
	})
}
