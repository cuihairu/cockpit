// Guacamole 指令协议层（自 main.go 按职责拆分）：指令编解码、隧道会话
// （读循环/心跳/clipboard 流旁路/客户端义务）、等待原语、键盘输入。
package main

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

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
	go gs.pingLoop(pingInterval)
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
	case <-time.After(dialFirstInstrWait):
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
// 回显；探针同款义务（guacd 15s 无指令判 "User is not responding"）。
// 间隔走参数值捕获（driftScanLoop 同纪律）：goroutine 体内不读包级注入
// var——泄漏实例醒来读与测试 cleanup 的注入恢复写不构成数据竞争
func (gs *guacSession) pingLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
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
