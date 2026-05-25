package protocol

import "time"

// MessageType 消息类型
type MessageType string

const (
	// Agent → Server
	MessageTypeRegister    MessageType = "register"
	MessageTypeHeartbeat   MessageType = "heartbeat"
	MessageTypeRPCResponse MessageType = "rpc_response"
	MessageTypeProxyClose  MessageType = "proxy_close"     // 关闭代理连接
	MessageTypeProxyError  MessageType = "proxy_error"     // 代理错误

	// Server → Agent
	MessageTypeRPCRequest MessageType = "rpc_request"
	MessageTypePing       MessageType = "ping"
	MessageTypeProxyNew   MessageType = "proxy_new"       // 新建代理连接

	// 双向
	MessageTypeError    MessageType = "error"
	MessageTypeProxyData MessageType = "proxy_data"      // 代理数据转发

	// 桌面会话 (远程桌面)
	MessageTypeDesktopNew   MessageType = "desktop_new"   // Server -> Agent: 开始桌面会话
	MessageTypeDesktopData  MessageType = "desktop_data"  // 双向: 桌面数据传输
	MessageTypeDesktopClose MessageType = "desktop_close" // 双向: 关闭桌面会话
)

// Message WebSocket 消息
type Message struct {
	ID        string                 `json:"id"`
	Type      MessageType           `json:"type"`
	Timestamp int64                  `json:"timestamp"`
	Payload   map[string]interface{} `json:"payload"`
}

// NewMessage 创建新消息
func NewMessage(typ MessageType, payload map[string]interface{}) *Message {
	return &Message{
		ID:        GenerateID(),
		Type:      typ,
		Timestamp: time.Now().Unix(),
		Payload:   payload,
	}
}

// Location 位置信息
type Location struct {
	Region string `json:"region"`
	Zone   string `json:"zone"`
}

// Capability 能力声明
type Capability struct {
	Type     string                 `json:"type"`
	Endpoint string                 `json:"endpoint,omitempty"`
	Version  string                 `json:"version,omitempty"`
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// RegisterPayload 注册消息负载
type RegisterPayload struct {
	AgentID      string                 `json:"agentId"`
	Location     Location               `json:"location"`
	Capabilities []Capability           `json:"capabilities"`
	Hostname     string                 `json:"hostname,omitempty"`
	IP           string                 `json:"ip,omitempty"`
	// 虚拟化信息
	Virtualization *VirtualizationInfo  `json:"virtualization,omitempty"`
	// 标签（支持键值对、数组、字符串等）
	Labels         map[string]interface{} `json:"labels,omitempty"`
}

// VirtualizationInfo 虚拟化信息
type VirtualizationInfo struct {
	Type     string `json:"type"`     // kvm, vmware, qemu, xen, docker, none
	Role     string `json:"role"`     // guest (虚拟机), host (物理机)
	Platform string `json:"platform,omitempty"` // 具体平台信息
}

// HeartbeatPayload 心跳消息负载
type HeartbeatPayload struct {
	AgentID string                 `json:"agentId"`
	Status  string                 `json:"status"`
	Metrics map[string]interface{} `json:"metrics,omitempty"`
	SystemInfo *SystemInfoPayload  `json:"systemInfo,omitempty"` // 系统资源信息
}

// SystemInfoPayload 系统信息负载
type SystemInfoPayload struct {
	// CPU 信息
	CPUUsage      float64 `json:"cpuUsage"`       // CPU 使用率 (0-100)
	CPUCores      int     `json:"cpuCores"`       // CPU 核心数
	CPUFreqMHz    float64 `json:"cpuFreqMhz"`     // CPU 频率

	// 内存信息
	MemTotal      uint64  `json:"memTotal"`       // 总内存 (bytes)
	MemUsed       uint64  `json:"memUsed"`        // 已用内存 (bytes)
	MemAvailable  uint64  `json:"memAvailable"`   // 可用内存 (bytes)
	MemUsagePercent float64 `json:"memUsagePercent"` // 内存使用率

	// 磁盘信息
	DiskTotal     uint64  `json:"diskTotal"`      // 总磁盘空间 (bytes)
	DiskUsed      uint64  `json:"diskUsed"`       // 已用磁盘空间 (bytes)
	DiskFree      uint64  `json:"diskFree"`       // 可用磁盘空间 (bytes)
	DiskUsagePercent float64 `json:"diskUsagePercent"` // 磁盘使用率

	// 网络信息
	NetBytesSent  uint64  `json:"netBytesSent"`   // 发送字节数
	NetBytesRecv  uint64  `json:"netBytesRecv"`   // 接收字节数

	// 系统信息
	OSName        string  `json:"osName"`         // 操作系统名称
	OSVersion     string  `json:"osVersion"`      // 操作系统版本
	Arch          string  `json:"arch"`           // 架构 (amd64, arm64等)
	Uptime        uint64  `json:"uptime"`         // 系统运行时间 (seconds)
	Hostname      string  `json:"hostname"`       // 主机名

	// 负载信息 (Unix-like)
	Load1         float64 `json:"load1"`          // 1分钟负载
	Load5         float64 `json:"load5"`          // 5分钟负载
	Load15        float64 `json:"load15"`         // 15分钟负载
}

// RPCRequestPayload RPC 请求负载
type RPCRequestPayload struct {
	Method string                 `json:"method"`
	Params map[string]interface{} `json:"params"`
}

// RPCResponsePayload RPC 响应负载
type RPCResponsePayload struct {
	Status string      `json:"status"` // success / error
	Data   interface{} `json:"data,omitempty"`
	Error  string      `json:"error,omitempty"`
}

// RegisterResponse 注册响应负载
type RegisterResponse struct {
	Status            string `json:"status"`
	ServerTime        int64  `json:"serverTime"`
	HeartbeatInterval int    `json:"heartbeatInterval"`
}

// ========== 代理相关消息类型 ==========

// ProxyNewPayload 新建代理连接负载
type ProxyNewPayload struct {
	ProxyID   string `json:"proxyId"`             // 代理ID
	ProxyType string `json:"proxyType"`           // tcp / udp
	Target    string `json:"target"`              // 目标地址，如 192.168.31.1:80
}

// ProxyDataPayload 代理数据转发负载
type ProxyDataPayload struct {
	ProxyID string `json:"proxyId"`               // 代理ID
	ConnID  string `json:"connId"`                // 连接ID
	Data    []byte `json:"data"`                 // 数据

	// Server -> Agent 时表示新建连接请求
	NewConn bool   `json:"newConn,omitempty"`    // 是否为新连接
}

// ProxyClosePayload 关闭代理连接负载
type ProxyClosePayload struct {
	ProxyID string `json:"proxyId"`               // 代理ID
	ConnID  string `json:"connId"`                // 连接ID
	Reason  string `json:"reason,omitempty"`     // 关闭原因
}

// ProxyErrorPayload 代理错误负载
type ProxyErrorPayload struct {
	ProxyID string `json:"proxyId"`               // 代理ID
	ConnID  string `json:"connId,omitempty"`     // 连接ID
	Error   string `json:"error"`                // 错误信息
}
