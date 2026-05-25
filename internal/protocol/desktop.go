package protocol

// DesktopMessageType 桌面消息子类型，通过 Message.Payload["desktopType"] 传递
type DesktopMessageType string

const (
	// Server -> Agent: 连接管理
	DesktopMsgConnect    DesktopMessageType = "connect"
	DesktopMsgDisconnect DesktopMessageType = "disconnect"

	// Agent -> Server -> Browser: 会话事件
	DesktopMsgConnected    DesktopMessageType = "connected"
	DesktopMsgDisconnected DesktopMessageType = "disconnected"
	DesktopMsgError        DesktopMessageType = "error"

	// Agent -> Server -> Browser: 屏幕更新
	DesktopMsgScreenUpdate DesktopMessageType = "screen_update"

	// Browser -> Server -> Agent: 输入事件
	DesktopMsgKeyboard DesktopMessageType = "keyboard"
	DesktopMsgMouse    DesktopMessageType = "mouse"

	// Browser -> Agent: 设置
	DesktopMsgSetResolution DesktopMessageType = "set_resolution"
	DesktopMsgSetQuality    DesktopMessageType = "set_quality"

	// 双向: 剪贴板
	DesktopMsgClipboardRequest DesktopMessageType = "clipboard_request"
	DesktopMsgClipboardData    DesktopMessageType = "clipboard_data"

	// 文件传输 (Phase 4)
	DesktopMsgFileUpload   DesktopMessageType = "file_upload"
	DesktopMsgFileDownload DesktopMessageType = "file_download"
	DesktopMsgFileData     DesktopMessageType = "file_data"

	// 会话录制 (Phase 4)
	DesktopMsgRecordingStart DesktopMessageType = "recording_start"
	DesktopMsgRecordingStop  DesktopMessageType = "recording_stop"

	// 多用户共享 (Phase 4)
	DesktopMsgShareGrant  DesktopMessageType = "share_grant"
	DesktopMsgShareRevoke DesktopMessageType = "share_revoke"
)

// DesktopConnectPayload RDP 连接参数
type DesktopConnectPayload struct {
	SessionID  string `json:"sessionId"`
	Target     string `json:"target"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	Domain     string `json:"domain"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	ColorDepth int    `json:"colorDepth"`
}

// DesktopScreenUpdatePayload 位图更新
type DesktopScreenUpdatePayload struct {
	SessionID string              `json:"sessionId"`
	Width     int                 `json:"width"`
	Height    int                 `json:"height"`
	Rects     []DesktopBitmapRect `json:"rects"`
}

// DesktopBitmapRect 脏矩形
type DesktopBitmapRect struct {
	X      int    `json:"x"`
	Y      int    `json:"y"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
	Data   string `json:"data"` // base64 编码 RGBA 像素数据
}

// DesktopKeyboardPayload 键盘输入
type DesktopKeyboardPayload struct {
	SessionID string `json:"sessionId"`
	ScanCode  uint16 `json:"scanCode"`
	KeyDown   bool   `json:"keyDown"`
	Extended  bool   `json:"extended"`
}

// DesktopMousePayload 鼠标输入
type DesktopMousePayload struct {
	SessionID   string `json:"sessionId"`
	X           int    `json:"x"`
	Y           int    `json:"y"`
	Buttons     int    `json:"buttons"`    // 位标志: 1=左, 2=右, 4=中
	WheelDelta  int    `json:"wheelDelta"` // 滚轮量
	Action      string `json:"action"`     // move, down, up
}

// DesktopClipboardPayload 剪贴板数据
type DesktopClipboardPayload struct {
	SessionID string `json:"sessionId"`
	Text      string `json:"text"`
}

// DesktopResolutionPayload 分辨率变更
type DesktopResolutionPayload struct {
	SessionID string `json:"sessionId"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// DesktopConnectedPayload 连接成功事件
type DesktopConnectedPayload struct {
	SessionID string `json:"sessionId"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
}

// DesktopErrorPayload 错误事件
type DesktopErrorPayload struct {
	SessionID string `json:"sessionId"`
	Error     string `json:"error"`
}

// DesktopDisconnectedPayload 断开事件
type DesktopDisconnectedPayload struct {
	SessionID string `json:"sessionId"`
	Reason    string `json:"reason"`
}
