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
	guacdRd  *bufio.Reader // 握手阶段建立，guacdToWS 复用（select 响应不丢）
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
		// 办公场景验收（设计风险章节）：色深 32、忽略证书（自签/内网常见）。
		// 音频：不传 disable-audio（guacd 默认启用），音频流由服务端主动推
		// audio 指令 + blob 流，自动经本网关字节管道透传（见 todo.md M4 D1）
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
	src := filepath.Join(guacamoleRecordingPath(), gs.ID+".guac")
	info, err := os.Stat(src)
	if err != nil {
		// 未开录制（connect 指令未带 recording-*）或 guacd 未落盘
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

	// 5a. tunnel UUID 先发浏览器（INTERNAL_DATA 单元素指令，即
	// Guacamole.Tunnel.INTERNAL_DATA_OPCODE 空 opcode）：common-js 首条指令
	// 若为内部指令单元素则 setUUID，否则仅置 OPEN（uuid 留 null）。
	// 这是 tunnel 层（浏览器↔网关），不进 guacd。
	session.writeWS([]byte(guacEncode("", sessionID)))

	// 5b. 握手（Guacamole 协议，设计 line 91 的 select/size/connect 序列）：
	// select → guacd 回参数列表 → size + connect。音频/视频不传 = 不启用
	//（音频放阶段二，见设计风险章节）。
	hsReader := bufio.NewReaderSize(guacdConn, guacamoleReadBuf)
	if _, err := guacdConn.Write([]byte(guacEncode("select", protocolStr))); err != nil {
		log.Printf("Guacamole: select write failed: %v", err)
		conn.Close()
		_ = guacdConn.Close()
		return
	}
	// 读 guacd 的 select 响应（参数名列表指令）——网关不解析内容，但必须
	// 消费掉（共享 reader 才能继续读后续 img/sync 流）
	if _, err := hsReader.ReadString(';'); err != nil {
		log.Printf("Guacamole: read select response failed: %v", err)
		conn.Close()
		_ = guacdConn.Close()
		return
	}
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
