package rdp

import (
	"encoding/base64"
	"image"
	"log/slog"
	"sync"
	"sync/atomic"

	grdp "github.com/nakagami/grdp"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// Session RDP 桌面会话，封装 grdp 客户端
type Session struct {
	ID        string
	client    *grdp.RdpClient
	sendQueue chan *protocol.Message
	width     int
	height    int
	mu        sync.Mutex
	closed    atomic.Bool
	screen    *image.RGBA
}

// NewSession 创建 RDP 会话
func NewSession(sessionID, target, domain, username, password string, width, height int) (*Session, error) {
	sendQueue := make(chan *protocol.Message, 60)

	s := &Session{
		ID:        sessionID,
		sendQueue: sendQueue,
		width:     width,
		height:    height,
		screen:    image.NewRGBA(image.Rect(0, 0, width, height)),
	}

	client := grdp.NewRdpClient(target, width, height)

	// 注册 OnBitmap 回调
	client.OnBitmap(func(bitmaps []grdp.Bitmap) {
		s.handleBitmap(bitmaps)
	})

	client.OnReady(func() {
		slog.Info("RDP session ready", "sessionID", sessionID)
		s.enqueue(&protocol.Message{
			Type: protocol.MessageTypeDesktopData,
			Payload: map[string]interface{}{
				"sessionId":   sessionID,
				"desktopType": string(protocol.DesktopMsgConnected),
				"width":       width,
				"height":      height,
			},
		})
	})

	client.OnError(func(err error) {
		slog.Error("RDP session error", "sessionID", sessionID, "error", err)
		s.enqueue(&protocol.Message{
			Type: protocol.MessageTypeDesktopData,
			Payload: map[string]interface{}{
				"sessionId":   sessionID,
				"desktopType": string(protocol.DesktopMsgError),
				"error":       err.Error(),
			},
		})
	})

	client.OnClose(func() {
		slog.Info("RDP session closed by remote", "sessionID", sessionID)
		s.enqueue(&protocol.Message{
			Type: protocol.MessageTypeDesktopData,
			Payload: map[string]interface{}{
				"sessionId":   sessionID,
				"desktopType": string(protocol.DesktopMsgDisconnected),
				"reason":      "remote closed",
			},
		})
	})

	// 剪贴板双向同步
	client.OnClipboard(
		func(text string) {
			s.enqueue(&protocol.Message{
				Type: protocol.MessageTypeDesktopData,
				Payload: map[string]interface{}{
					"sessionId":   sessionID,
					"desktopType": string(protocol.DesktopMsgClipboardData),
					"text":        text,
				},
			})
		},
		func() string {
			return s.getClipboardText()
		},
	)

	s.client = client

	// 登录
	if err := client.Login(domain, username, password); err != nil {
		return nil, err
	}

	return s, nil
}

// handleBitmap 处理位图更新回调
func (s *Session) handleBitmap(bitmaps []grdp.Bitmap) {
	if s.closed.Load() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	rects := make([]protocol.DesktopBitmapRect, 0, len(bitmaps))

	for _, bm := range bitmaps {
		// 转换为 RGBA（grdp v0.6.7 使用 RGBA() 方法）
		rgba := bm.RGBA()

		// 合成到帧缓冲
		for y := 0; y < bm.Height; y++ {
			for x := 0; x < bm.Width; x++ {
				dstX := bm.DestLeft + x
				dstY := bm.DestTop + y
				if dstX < s.width && dstY < s.height {
					srcOff := (y*bm.Width + x) * 4
					dstOff := (dstY*s.width + dstX) * 4
					copy(s.screen.Pix[dstOff:dstOff+4], rgba.Pix[srcOff:srcOff+4])
				}
			}
		}

		// base64 编码 RGBA 像素数据
		rect := protocol.DesktopBitmapRect{
			X:      bm.DestLeft,
			Y:      bm.DestTop,
			Width:  bm.Width,
			Height: bm.Height,
			Data:   base64.StdEncoding.EncodeToString(rgba.Pix),
		}
		rects = append(rects, rect)
	}

	if len(rects) == 0 {
		return
	}

	msg := &protocol.Message{
		Type: protocol.MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   s.ID,
			"desktopType": string(protocol.DesktopMsgScreenUpdate),
			"width":       s.width,
			"height":      s.height,
			"rects":       rects,
		},
	}

	s.enqueue(msg)
}

// enqueue 非阻塞入队，队列满时丢弃最旧消息
func (s *Session) enqueue(msg *protocol.Message) {
	select {
	case s.sendQueue <- msg:
	default:
		select {
		case <-s.sendQueue:
		default:
		}
		select {
		case s.sendQueue <- msg:
		default:
		}
	}
}

// HandleKeyboard 处理键盘输入
func (s *Session) HandleKeyboard(scanCode uint16, keyDown bool, extended bool) {
	if s.closed.Load() {
		return
	}

	sc := int(scanCode)
	if extended {
		sc |= 0x100
	}

	if keyDown {
		s.client.KeyDown(sc)
	} else {
		s.client.KeyUp(sc)
	}
}

// HandleMouse 处理鼠标输入
func (s *Session) HandleMouse(x, y, button, wheelDelta int, action string) {
	if s.closed.Load() {
		return
	}

	switch action {
	case "move":
		s.client.MouseMove(x, y)
	case "down":
		s.client.MouseDown(button, x, y)
	case "up":
		s.client.MouseUp(button, x, y)
	}

	if wheelDelta != 0 {
		s.client.MouseWheel(wheelDelta)
	}
}

// HandleClipboard 处理本地剪贴板数据
var clipboardText string
var clipboardMu sync.Mutex

func (s *Session) HandleClipboard(text string) {
	clipboardMu.Lock()
	clipboardText = text
	clipboardMu.Unlock()
	s.client.NotifyClipboardChanged()
}

func (s *Session) getClipboardText() string {
	clipboardMu.Lock()
	defer clipboardMu.Unlock()
	return clipboardText
}

// HandleSetResolution 通过 Reconnect 实现分辨率变更
func (s *Session) HandleSetResolution(width, height int) {
	if s.closed.Load() {
		return
	}

	if err := s.client.Reconnect(width, height); err != nil {
		slog.Error("RDP reconnect for resolution change failed", "sessionID", s.ID, "error", err)
		return
	}
	s.width = width
	s.height = height
	s.screen = image.NewRGBA(image.Rect(0, 0, width, height))
}

// Close 关闭 RDP 会话
func (s *Session) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	s.client.Close()
	close(s.sendQueue)
	slog.Info("RDP session closed", "sessionID", s.ID)
}

// SendQueue 返回发送队列
func (s *Session) SendQueue() <-chan *protocol.Message {
	return s.sendQueue
}

// IsClosed 是否已关闭
func (s *Session) IsClosed() bool {
	return s.closed.Load()
}
