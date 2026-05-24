package protocol

// RemoteProtocol 远程协议类型
type RemoteProtocol string

const (
	RemoteProtocolSSH  RemoteProtocol = "ssh"  // SSH
	RemoteProtocolRDP  RemoteProtocol = "rdp"  // RDP (Remote Desktop Protocol)
	RemoteProtocolVNC  RemoteProtocol = "vnc"  // VNC
	RemoteProtocolTelnet RemoteProtocol = "telnet" // Telnet
	RemoteProtocolFTP  RemoteProtocol = "ftp"  // FTP
)

// RemoteConnectionInfo 远程连接信息
type RemoteConnectionInfo struct {
	Protocol RemoteProtocol `json:"protocol"`     // 协议类型
	Host     string         `json:"host"`         // 目标主机
	Port     int            `json:"port"`         // 端口
	Username string         `json:"username"`     // 用户名（可选）
	Password string         `json:"password"`     // 密码（可选，通常不存储）
	AuthType string         `json:"authType"`     // 认证类型: password, key
	Name     string         `json:"name"`         // 连接名称
}

// RemoteServiceInfo 远程服务信息（能力检测返回）
type RemoteServiceInfo struct {
	Protocol RemoteProtocol `json:"protocol"`
	Host     string         `json:"host"`      // 监听地址 (0.0.0.0, 127.0.0.1)
	Port     int            `json:"port"`      // 监听端口
	Name     string         `json:"name"`      // 服务名称
	Running  bool           `json:"running"`   // 是否运行中
}

// RemoteProxyStartPayload 启动远程代理负载
type RemoteProxyStartPayload struct {
	ConnectionID string                `json:"connectionId"` // 连接ID
	Protocol     RemoteProtocol        `json:"protocol"`     // 协议类型
	Target       string                `json:"target"`        // 目标地址 host:port
	Timeout      int                   `json:"timeout"`      // 超时时间（秒）
}

// RemoteProxyDataPayload 远程代理数据负载
type RemoteProxyDataPayload struct {
	ConnectionID string `json:"connectionId"` // 连接ID
	Data         []byte `json:"data"`         // 数据
}

// RemoteProxyClosePayload 关闭远程代理负载
type RemoteProxyClosePayload struct {
	ConnectionID string `json:"connectionId"` // 连接ID
	Reason       string `json:"reason"`       // 关闭原因
}
