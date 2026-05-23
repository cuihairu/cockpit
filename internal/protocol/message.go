package protocol

import "time"

// MessageType 消息类型
type MessageType string

const (
	// Agent → Server
	MessageTypeRegister    MessageType = "register"
	MessageTypeHeartbeat   MessageType = "heartbeat"
	MessageTypeRPCResponse MessageType = "rpc_response"

	// Server → Agent
	MessageTypeRPCRequest MessageType = "rpc_request"
	MessageTypePing       MessageType = "ping"

	// 双向
	MessageTypeError MessageType = "error"
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
	AgentID      string       `json:"agentId"`
	Location     Location     `json:"location"`
	Capabilities []Capability `json:"capabilities"`
	Hostname     string       `json:"hostname,omitempty"`
	IP           string       `json:"ip,omitempty"`
}

// HeartbeatPayload 心跳消息负载
type HeartbeatPayload struct {
	AgentID string                 `json:"agentId"`
	Status  string                 `json:"status"`
	Metrics map[string]interface{} `json:"metrics,omitempty"`
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
