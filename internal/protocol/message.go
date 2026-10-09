package protocol

import "time"

// MessageType 消息类型
type MessageType string

const (
	// Agent → Server
	MessageTypeRegister    MessageType = "register"
	MessageTypeHeartbeat   MessageType = "heartbeat"
	MessageTypeRPCResponse MessageType = "rpc_response"
	MessageTypeProxyClose  MessageType = "proxy_close"  // 关闭代理连接
	MessageTypeProxyError  MessageType = "proxy_error"  // 代理错误
	MessageTypeProbeReport MessageType = "probe_report" // 探针 agent 观测上报（服务检测 agent B5）

	// Server → Agent
	MessageTypeRPCRequest MessageType = "rpc_request"
	MessageTypePing       MessageType = "ping"
	MessageTypeProxyNew   MessageType = "proxy_new" // 新建代理连接

	// 双向
	MessageTypeError     MessageType = "error"
	MessageTypeProxyData MessageType = "proxy_data" // 代理数据转发
)

// Message WebSocket 消息
type Message struct {
	ID        string                 `json:"id"`
	Type      MessageType            `json:"type"`
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

// RemoteServicePayload 远控服务面上报项：agent 探测到的本机开放服务
// （SSH/RDP/VNC/telnet…）。服务端据此持久化「这台机器真开了什么」，
// 客户端只展示真正可用的协议入口——不再无条件铺 SSH/RDP/VNC 三个按钮。
type RemoteServicePayload struct {
	Protocol    string   `json:"protocol"`
	Host        string   `json:"host"`
	Port        int      `json:"port"`
	Name        string   `json:"name,omitempty"`
	Running     bool     `json:"running"`
	AuthMethods []string `json:"authMethods,omitempty"`
	// DetectedAt 本次探测时刻（Unix 秒）。服务面随心跳刷新，
	// 时间戳让客户端能判断「多久前探到的」。
	DetectedAt int64 `json:"detectedAt,omitempty"`
}

// RegisterPayload 注册消息负载
type RegisterPayload struct {
	AgentID      string       `json:"agentId"`
	Secret       string       `json:"secret,omitempty"` // Agent 认证密钥
	Location     Location     `json:"location"`
	Capabilities []Capability `json:"capabilities"`
	Hostname     string       `json:"hostname,omitempty"`
	IP           string       `json:"ip,omitempty"`
	LocalIPs     []string     `json:"localIps,omitempty"`
	// 虚拟化信息
	Virtualization *VirtualizationInfo `json:"virtualization,omitempty"`
	// 标签（支持键值对、数组、字符串等）
	Labels map[string]interface{} `json:"labels,omitempty"`
	// 元数据：自定义 key-value 存储
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Version Agent 二进制版本（cmd/cockpit-agent 的 version 注入值）
	Version string `json:"version,omitempty"`
	// StartedAt agent 进程启动时刻（Unix 秒）。与系统 uptime 不同：
	// 服务端据此算「本次运行时长」，agent 重启即刷新
	StartedAt int64 `json:"startedAt,omitempty"`
	// Services 本机开放的服务面（探测结果，注册时先给一份基线）
	Services []RemoteServicePayload `json:"services,omitempty"`
}

// VirtualizationInfo 虚拟化信息
type VirtualizationInfo struct {
	Type     string `json:"type"`               // kvm, vmware, qemu, xen, docker, none
	Role     string `json:"role"`               // guest (虚拟机), host (物理机)
	Platform string `json:"platform,omitempty"` // 具体平台信息
}

// HeartbeatPayload 心跳消息负载
type HeartbeatPayload struct {
	AgentID    string                 `json:"agentId"`
	Status     string                 `json:"status"`
	Metrics    map[string]interface{} `json:"metrics,omitempty"`
	SystemInfo *SystemInfoPayload     `json:"systemInfo,omitempty"` // 系统资源信息

	// StartedAt agent 进程启动时刻（Unix 秒），随每次心跳回带：
	// 服务端刷新启动时间与运行时长，重启后自然归零
	StartedAt int64 `json:"startedAt,omitempty"`
	// Services 服务面刷新（默认约 5 分钟一次重探测）：
	// 新开/关掉 SSH/RDP/VNC 最多一个周期内同步到服务端
	Services []RemoteServicePayload `json:"services,omitempty"`
}

// SystemInfoPayload 系统信息负载
type SystemInfoPayload struct {
	// CPU 信息
	CPUUsage   float64 `json:"cpuUsage"`   // CPU 使用率 (0-100)
	CPUCores   int     `json:"cpuCores"`   // CPU 核心数
	CPUFreqMHz float64 `json:"cpuFreqMhz"` // CPU 频率

	// 内存信息
	MemTotal        uint64  `json:"memTotal"`        // 总内存 (bytes)
	MemUsed         uint64  `json:"memUsed"`         // 已用内存 (bytes)
	MemAvailable    uint64  `json:"memAvailable"`    // 可用内存 (bytes)
	MemUsagePercent float64 `json:"memUsagePercent"` // 内存使用率

	// 磁盘信息
	DiskTotal        uint64  `json:"diskTotal"`        // 总磁盘空间 (bytes)
	DiskUsed         uint64  `json:"diskUsed"`         // 已用磁盘空间 (bytes)
	DiskFree         uint64  `json:"diskFree"`         // 可用磁盘空间 (bytes)
	DiskUsagePercent float64 `json:"diskUsagePercent"` // 磁盘使用率

	// 网络信息
	NetBytesSent uint64 `json:"netBytesSent"` // 发送字节数
	NetBytesRecv uint64 `json:"netBytesRecv"` // 接收字节数

	// 系统信息
	OSName    string `json:"osName"`    // 操作系统名称
	OSVersion string `json:"osVersion"` // 操作系统版本
	Arch      string `json:"arch"`      // 架构 (amd64, arm64等)
	Uptime    uint64 `json:"uptime"`    // 系统运行时间 (seconds)
	Hostname  string `json:"hostname"`  // 主机名

	// 负载信息 (Unix-like)
	Load1  float64 `json:"load1"`  // 1分钟负载
	Load5  float64 `json:"load5"`  // 5分钟负载
	Load15 float64 `json:"load15"` // 15分钟负载
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
	ProxyID   string `json:"proxyId"`            // 代理ID
	ProxyType string `json:"proxyType"`          // 仅 tcp（udp 预留未实现，API 拒收，见 D-2026-10-08-2）
	Target    string `json:"target"`             // 目标地址，如 192.168.31.1:80
	ConnID    string `json:"connId,omitempty"`   // 连接ID（运行时附加）
	Terminal  bool   `json:"terminal,omitempty"` // 终端会话标记
	Protocol  string `json:"protocol,omitempty"` // 远程协议（ssh/telnet/rdp/vnc…）

	// SSH 认证凭据（protocol=ssh 时生效；telnet 等裸 TCP 协议忽略）。
	// 口令与私钥仅经加密 WS 通道下发到 agent，不落盘、不进日志/审计。
	Username   string `json:"username,omitempty"`   // 登录用户名
	Password   string `json:"password,omitempty"`   // 口令认证（与 PrivateKey 二选一）
	PrivateKey string `json:"privateKey,omitempty"` // PEM 格式私钥（优先于 Password）
}

// ProxyDataPayload 代理数据转发负载
type ProxyDataPayload struct {
	ProxyID string `json:"proxyId"` // 代理ID
	ConnID  string `json:"connId"`  // 连接ID
	Data    []byte `json:"data"`    // 数据

	// Server -> Agent 时表示新建连接请求
	NewConn bool `json:"newConn,omitempty"` // 是否为新连接

	// 运行时附加：标记 terminal/vnc 等特殊通道，避免依赖 proxyId 前缀
	Terminal bool `json:"terminal,omitempty"` // 终端会话标记

	// 终端窗口尺寸变更（terminal 会话专用；Data 为空）。
	// server 已按浏览器 {type:"resize",rows,cols} 转发，agent 调 PTY WindowChange。
	Resize bool `json:"resize,omitempty"` // 是否为窗口尺寸变更
	Rows   int  `json:"rows,omitempty"`   // 行数
	Cols   int  `json:"cols,omitempty"`   // 列数
}

// ProxyClosePayload 关闭代理连接负载
type ProxyClosePayload struct {
	ProxyID  string `json:"proxyId"`            // 代理ID
	ConnID   string `json:"connId"`             // 连接ID
	Reason   string `json:"reason,omitempty"`   // 关闭原因
	Terminal bool   `json:"terminal,omitempty"` // 终端会话标记
}

// ProxyErrorPayload 代理错误负载
type ProxyErrorPayload struct {
	ProxyID string `json:"proxyId"`          // 代理ID
	ConnID  string `json:"connId,omitempty"` // 连接ID
	Error   string `json:"error"`            // 错误信息
}
