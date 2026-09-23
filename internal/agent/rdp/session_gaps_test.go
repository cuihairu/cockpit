//go:build rdp && !darwin

package rdp

import (
	"image"
	"net"
	"testing"
	"time"

	grdp "github.com/nakagami/grdp"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// 覆盖率缺口补测（只补测试零业务改动）：HandleKeyboard/HandleMouse/
// HandleClipboard/Close 的输入分发分支。grdp 零值 &RdpClient{} 安全——
// 输入方法首行 `if !g.eventReady.Load() { return }`（零值 false 立即返回，
// 不碰 g.pdu），Close() 判 nil 内部字段。故无需真实 RDP 连接即可覆盖
// 分发分支（KeyDown/KeyUp/Move/Down/Up/Wheel 体）。

func newInputTestSession() *Session {
	return &Session{
		ID:        "input-test",
		client:    &grdp.RdpClient{},
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}
}

func TestSessionHandleKeyboardBothBranches(t *testing.T) {
	s := newInputTestSession()
	// keyDown 分支（KeyDown 体）
	s.HandleKeyboard(0x1e, true, false)
	// keyUp 分支 + extended 位（KeyUp 体）
	s.HandleKeyboard(0x1e, false, true)
}

func TestSessionHandleMouseAllActions(t *testing.T) {
	s := newInputTestSession()
	// move / down / up 三分支
	s.HandleMouse(1, 2, 0, 0, "move")
	s.HandleMouse(1, 2, 1, 0, "down")
	s.HandleMouse(1, 2, 1, 0, "up")
	// 未知 action 不分发（switch default）
	s.HandleMouse(1, 2, 0, 0, "hover")
}

func TestSessionHandleMouseWheel(t *testing.T) {
	s := newInputTestSession()
	// wheelDelta != 0 → MouseWheel 体
	s.HandleMouse(1, 2, 0, 1, "move")
	s.HandleMouse(1, 2, 0, -1, "move")
}

func TestSessionHandleClipboardWithClient(t *testing.T) {
	s := newInputTestSession()
	// HandleClipboard 体（NotifyClipboardChanged 对零值 client no-op）
	s.HandleClipboard("copied text")
	if got := s.getClipboardText(); got != "copied text" {
		t.Errorf("clipboard = %q, want copied text", got)
	}
}

func TestSessionCloseWithClient(t *testing.T) {
	s := newInputTestSession()
	// Close 的 `if s.client != nil { s.client.Close() }` 体
	s.Close()
	if !s.IsClosed() {
		t.Error("session should be closed")
	}
	// 幂等：二次 Close 不再走 client.Close()
	s.Close()
}

func TestSessionInputHandlersOnClosedSession(t *testing.T) {
	s := newInputTestSession()
	s.Close()
	// closed 提前返回分支（不碰 client）
	s.HandleKeyboard(0x1e, true, false)
	s.HandleMouse(1, 2, 0, 0, "move")
	s.HandleClipboard("ignored")
	s.HandleSetResolution(200, 200)
}

func TestHandleDesktopCloseDecodeFail(t *testing.T) {
	h := NewHandler()
	// sessionId 非字符串 → DecodeDesktopDisconnected 失败 → 提前 return
	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopClose,
		Payload: map[string]interface{}{
			"sessionId": 123,
		},
	}
	h.HandleDesktopClose(msg) // 只验证不 panic、decode 失败分支被走到
}

func TestSessionHandleSetResolutionReconnectFail(t *testing.T) {
	// grdp.NewRdpClient 注入真实 dialer（零值 client 的 dialer 为 nil 会 panic）；
	// 目标不可达 → Reconnect 内部 doLogin 三次重试后返回 error → 走 err != nil 分支
	client := grdp.NewRdpClient("127.0.0.1:1", 100, 100, func(addr string) (net.Conn, error) {
		return net.DialTimeout("tcp", addr, 50*time.Millisecond)
	})
	s := &Session{
		ID:        "res-test",
		client:    client,
		sendQueue: make(chan *protocol.Message, 10),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}
	// Reconnect 失败 → slog.Error + return（不改 width/height/screen）
	s.HandleSetResolution(200, 300)
	if s.width != 100 || s.height != 100 {
		t.Errorf("size = %dx%d, want unchanged 100x100 after failed reconnect", s.width, s.height)
	}
}
