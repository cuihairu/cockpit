//go:build rdp && !darwin

package rdp

import (
	"errors"
	"image"
	"testing"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
	grdp "github.com/nakagami/grdp"
)

// fakeRdpClient rdpClient 假件：记录 On* 注册的回调与调用次数，
// fire* 由测试主动触发，覆盖「登录成功」后才可达的生命周期路径。
type fakeRdpClient struct {
	loginErr     error
	reconnectErr error
	closeCalls   int

	onBitmap  func([]grdp.Bitmap)
	onReady   func()
	onError   func(error)
	onClose   func()
	onRemote  func(string)
	getLocal  func() string
	keyDowns  []int
	mouseMove int
}

func (f *fakeRdpClient) Login(domain string, user string, password string) error { return f.loginErr }
func (f *fakeRdpClient) Reconnect(width, height int) error                       { return f.reconnectErr }
func (f *fakeRdpClient) KeyDown(sc int)                                          { f.keyDowns = append(f.keyDowns, sc) }
func (f *fakeRdpClient) KeyUp(sc int)                                            {}
func (f *fakeRdpClient) MouseMove(x, y int)                                      { f.mouseMove++ }
func (f *fakeRdpClient) MouseDown(button int, x, y int)                          {}
func (f *fakeRdpClient) MouseUp(button int, x, y int)                            {}
func (f *fakeRdpClient) MouseWheel(delta float64)                                {}
func (f *fakeRdpClient) NotifyClipboardChanged()                                 {}
func (f *fakeRdpClient) Close()                                                  { f.closeCalls++ }
func (f *fakeRdpClient) OnBitmap(paint func([]grdp.Bitmap)) *grdp.RdpClient {
	f.onBitmap = paint
	return nil
}
func (f *fakeRdpClient) OnReady(fn func()) *grdp.RdpClient { f.onReady = fn; return nil }
func (f *fakeRdpClient) OnError(fn func(e error)) *grdp.RdpClient {
	f.onError = fn
	return nil
}
func (f *fakeRdpClient) OnClose(fn func()) *grdp.RdpClient { f.onClose = fn; return nil }
func (f *fakeRdpClient) OnClipboard(onRemote func(text string), getLocal func() string) *grdp.RdpClient {
	f.onRemote = onRemote
	f.getLocal = getLocal
	return nil
}

// useFakeClient 覆写 newRdpClient 工厂并注册还原。
func useFakeClient(t *testing.T, f *fakeRdpClient) {
	t.Helper()
	orig := newRdpClient
	newRdpClient = func(target string, width, height int) rdpClient { return f }
	t.Cleanup(func() { newRdpClient = orig })
}

// recvMsg 带超时从会话队列取一条消息。
func recvMsg(t *testing.T, s *Session) *protocol.Message {
	t.Helper()
	select {
	case msg := <-s.SendQueue():
		return msg
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for session message")
		return nil
	}
}

// TestNewSessionSuccessLifecycle 登录成功：会话字段就位、五个回调全部注册、
// OnReady/OnError/OnClose/OnClipboard 触发后产出对应 desktopType 消息。
func TestNewSessionSuccessLifecycle(t *testing.T) {
	fake := &fakeRdpClient{}
	useFakeClient(t, fake)

	s, err := NewSession("life-1", "10.0.0.9:3389", "", "u", "p", 640, 480)
	if err != nil {
		t.Fatalf("NewSession: %v", err)
	}
	if s.ID != "life-1" || s.width != 640 || s.height != 480 {
		t.Fatalf("session fields: %v %dx%d", s.ID, s.width, s.height)
	}
	if b := s.screen.Bounds(); b.Dx() != 640 || b.Dy() != 480 {
		t.Fatalf("screen bounds: %v", b)
	}
	// Login 成功才会走到回调注册——五个 On* 均被调用即为注册完成
	if fake.onReady == nil || fake.onError == nil || fake.onClose == nil ||
		fake.onBitmap == nil || fake.onRemote == nil || fake.getLocal == nil {
		t.Fatal("callbacks not all registered on login success")
	}

	// OnReady → connected 消息（带分辨率）
	fake.onReady()
	msg := recvMsg(t, s)
	if msg.Type != protocol.MessageTypeDesktopData ||
		msg.Payload["desktopType"] != string(protocol.DesktopMsgConnected) ||
		msg.Payload["width"] != 640 || msg.Payload["height"] != 480 {
		t.Fatalf("ready msg: %+v", msg.Payload)
	}

	// OnError → error 消息（带错误文案）
	fake.onError(errors.New("tls boom"))
	msg = recvMsg(t, s)
	if msg.Payload["desktopType"] != string(protocol.DesktopMsgError) || msg.Payload["error"] != "tls boom" {
		t.Fatalf("error msg: %+v", msg.Payload)
	}

	// OnClose → disconnected 消息（remote closed）
	fake.onClose()
	msg = recvMsg(t, s)
	if msg.Payload["desktopType"] != string(protocol.DesktopMsgDisconnected) {
		t.Fatalf("close msg: %+v", msg.Payload)
	}

	// OnClipboard 推送 → clipboard 消息；getLocal 回调读到 HandleClipboard 写入值
	fake.onRemote("from-remote")
	msg = recvMsg(t, s)
	if msg.Payload["desktopType"] != string(protocol.DesktopMsgClipboardData) || msg.Payload["text"] != "from-remote" {
		t.Fatalf("clipboard msg: %+v", msg.Payload)
	}
	s.HandleClipboard("local-draft")
	if got := fake.getLocal(); got != "local-draft" {
		t.Fatalf("getLocal = %q", got)
	}

	// OnBitmap 闭包 → handleBitmap（空位图集走 rects 空早退，不产出消息）
	fake.onBitmap(nil)
	select {
	case msg := <-s.SendQueue():
		t.Fatalf("empty bitmap set should not enqueue, got %+v", msg.Payload)
	case <-time.After(50 * time.Millisecond):
	}

	s.Close()
	if fake.closeCalls != 1 || !s.IsClosed() {
		t.Fatalf("close: calls=%d closed=%v", fake.closeCalls, s.IsClosed())
	}
}

// TestSessionSetResolutionSuccess Reconnect 成功：更新宽高并重建帧缓冲
// （此前只有失败分支可达——需要真实握手，工厂注入后可测）。
func TestSessionSetResolutionSuccess(t *testing.T) {
	fake := &fakeRdpClient{}
	s := &Session{
		ID:        "res-ok",
		client:    fake,
		sendQueue: make(chan *protocol.Message, 4),
		width:     100,
		height:    100,
		screen:    image.NewRGBA(image.Rect(0, 0, 100, 100)),
	}
	s.HandleSetResolution(200, 300)
	if s.width != 200 || s.height != 300 {
		t.Fatalf("resolution not updated: %dx%d", s.width, s.height)
	}
	if b := s.screen.Bounds(); b.Dx() != 200 || b.Dy() != 300 {
		t.Fatalf("screen not rebuilt: %v", b)
	}

	// 已关闭会话：直接忽略，不动宽高
	s.Close()
	s.HandleSetResolution(1, 1)
	if s.width != 200 || s.height != 300 {
		t.Fatalf("closed session should ignore resolution change: %dx%d", s.width, s.height)
	}
}

// TestHandleDesktopNewSuccess 新建会话成功：登记进 Handler、发送协程启动；
// 会话关闭后队列闭合 → 协程退出并反注册（收敛不泄漏）。
func TestHandleDesktopNewSuccess(t *testing.T) {
	fake := &fakeRdpClient{}
	useFakeClient(t, fake)

	h := NewHandler()
	h.SetSendFunc(func(msg *protocol.Message) error { return nil })

	h.HandleDesktopNew(&protocol.Message{Payload: map[string]interface{}{
		"sessionId": "s-new",
		"target":    "10.0.0.9:3389",
		"width":     float64(640),
		"height":    float64(480),
	}})

	h.mu.RLock()
	s, ok := h.sessions["s-new"]
	h.mu.RUnlock()
	if !ok {
		t.Fatal("session not registered after successful New")
	}
	if s.width != 640 || s.height != 480 {
		t.Fatalf("resolution not applied: %dx%d", s.width, s.height)
	}

	// 关闭 → sessionSendLoop 消费完队列退出 → 反注册
	s.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		h.mu.RLock()
		_, still := h.sessions["s-new"]
		h.mu.RUnlock()
		if !still {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("session not unregistered after close")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestNewRdpClientRealDialer 默认工厂走原生 dialer：对无监听的 127.0.0.1:1
// 拨号即时 connection refused（无网络等待），覆盖工厂闭包内的 DialTimeout
// 路径与 Login 拨号失败透传。
func TestNewRdpClientRealDialer(t *testing.T) {
	orig := newRdpClient
	t.Cleanup(func() { newRdpClient = orig }) // 防御：本测不改工厂，还原是兜底
	c := newRdpClient("127.0.0.1:1", 100, 100)
	if err := c.Login("", "u", "p"); err == nil {
		t.Fatal("dial to closed port should fail")
	}
}

// 编译期保证假件与真件满足同一接口面
var _ rdpClient = (*fakeRdpClient)(nil)
