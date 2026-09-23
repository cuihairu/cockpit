package server

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/audit"
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

// guacamoleReadBuf guacd→WS 段读缓冲（Guacamole 指令是文本行，含 base64
// blob，单条可较大）
const guacamoleReadBuf = 256 * 1024

// guacamoleUpgrader Guacamole 隧道专用 upgrader：文本帧承载指令流，
// 位图 blob 经 base64 文本传输，缓冲给足
var guacamoleUpgrader = websocket.Upgrader{
	CheckOrigin:     isOriginAllowed,
	ReadBufferSize:  guacamoleReadBuf,
	WriteBufferSize: guacamoleReadBuf,
}

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
	Protocol string // rdp / vnc
	AgentID  string
	Host     string
	Port     int
	ClientWS *websocket.Conn
	guacd    net.Conn
	Created  time.Time

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

// guacConnectArgs 组装 connect 指令参数（name=value 形式）。
// 凭据来自票据 params（handleTicketCreate 存储），不经浏览器二次经手
// （见设计「为什么参数由服务端放进 connect 指令」）。
func guacConnectArgs(protocol, host string, port int, params map[string]string, width, height int, record bool, recordingName string) []string {
	args := []string{
		"hostname=" + host,
		"port=" + strconv.Itoa(port),
	}
	if v := params["username"]; v != "" {
		args = append(args, "username="+v)
	}
	if v := params["password"]; v != "" {
		args = append(args, "password="+v)
	}
	if v := params["domain"]; v != "" {
		args = append(args, "domain="+v)
	}
	switch protocol {
	case "rdp":
		// 办公场景验收（设计风险章节）：色深 32、忽略证书（自签/内网常见），
		// 音频留阶段二（不传 audio 即不启用）
		args = append(args,
			"security=any",
			"ignore-cert=true",
			"color-depth=32",
			"create-drive-path=true",
		)
		if width > 0 && height > 0 {
			args = append(args, "width="+strconv.Itoa(width), "height="+strconv.Itoa(height))
		}
	case "vnc":
		args = append(args, "color-depth=32")
		if v := params["password"]; v != "" {
			// VNC 的 password 即 connect 的 password 参数（上面已传），
			// 这里不重复
			_ = v
		}
	}
	// 桌面会话录制（设计「guacd session recording 白捡」）：recording-path/
	// recording-name 指向 guacd 容器卷（deployments/guacd 的
	// guacd-recordings 卷挂载点）；recording.enabled 关闭时不传
	if record {
		args = append(args,
			"recording-path="+guacamoleRecordingPath(),
			"recording-name="+recordingName,
		)
	}
	return args
}

// guacamoleRecordingPath guacd 侧录制目录（容器内路径，对应
// deployments/guacd 的 guacd-recordings 卷挂载点）。
func guacamoleRecordingPath() string { return "/var/lib/guacamole" }

// registerGuacamoleAPI 注册 Guacamole 隧道端点
func (s *Server) registerGuacamoleAPI(mux *http.ServeMux) {
	mux.HandleFunc("/api/remote/guacamole", s.handleGuacamoleWebSocket)
}

// handleGuacamoleWebSocket 浏览器 ↔ server 段：WS 文本帧即 Guacamole 协议指令。
// ticket 经 Sec-WebSocket-Protocol 传递（与 terminal/desktop 同款一次性票据）。
func (s *Server) handleGuacamoleWebSocket(w http.ResponseWriter, r *http.Request) {
	// 1. 票据：Sec-WebSocket-Protocol[0]（复用 ticket.go，ValidateTicket 即消费）
	protocols := r.Header.Values("Sec-WebSocket-Protocol")
	if len(protocols) == 0 {
		http.Error(w, `{"error":"Missing ticket"}`, http.StatusUnauthorized)
		return
	}
	ticket, ok := s.ticketMgr.ValidateTicket(protocols[0])
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
	// Guacamole 承接桌面协议（rdp/vnc）；ssh/telnet 走 TerminalModal（xterm.js）
	if protocolStr != "rdp" && protocolStr != "vnc" {
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
	guacdConn, err := net.DialTimeout("tcp", guacdAddr(), guacdDialTimeout)
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

	// 4. 升级 WS（票据作子协议回显，与 terminal/desktop 一致）
	conn, err := guacamoleUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Guacamole WebSocket upgrade failed: %v", err)
		_ = guacdConn.Close()
		return
	}

	sessionID := uuid.New().String()
	session := &GuacamoleSession{
		ID:       sessionID,
		UserID:   ticket.UserID,
		Username: ticket.Username,
		Protocol: protocolStr,
		AgentID:  agentID,
		Host:     host,
		Port:     port,
		ClientWS: conn,
		guacd:    guacdConn,
		Created:  time.Now(),
		done:     make(chan struct{}),
	}

	// 5. 握手指令发 guacd：size → connect（音频/视频不传 = 不启用，
	// 音频放阶段二，见设计风险章节）
	if width <= 0 {
		width = 1280
	}
	if height <= 0 {
		height = 800
	}
	// recording-name 用会话 ID（可追溯到审计的 Session 字段）
	connectArgs := guacConnectArgs(protocolStr, host, port, ticket.Params, width, height,
		s.recordingEnabled(), sessionID)
	handshake := guacEncode("size", strconv.Itoa(width), strconv.Itoa(height), "96") +
		guacEncode("connect", connectArgs...)
	if _, err := guacdConn.Write([]byte(handshake)); err != nil {
		log.Printf("Guacamole: handshake write failed: %v", err)
		conn.Close()
		_ = guacdConn.Close()
		return
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

// wsToGuacd 浏览器 → guacd：WS 文本帧内容直写 TCP
func (gs *GuacamoleSession) wsToGuacd() {
	defer close(gs.done)
	for {
		_, data, err := gs.ClientWS.ReadMessage()
		if err != nil {
			return
		}
		if _, err := gs.guacd.Write(data); err != nil {
			return
		}
	}
}

// guacdToWS guacd → 浏览器：按 Guacamole 指令边界（分号结尾）切分后
// 写 WS 文本帧。切分只做边界识别（分号），不解析参数内容。
func (gs *GuacamoleSession) guacdToWS() {
	reader := bufio.NewReaderSize(gs.guacd, guacamoleReadBuf)
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
		log.Printf("Guacamole session closed: %s", gs.ID)

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
