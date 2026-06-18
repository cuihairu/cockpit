package protocol

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// DecodePayload 将 Message.Payload（map[string]interface{}）解码到泛型结构 T。
//
// 内部实现：先 marshal payload 为 JSON，再 unmarshal 到 T。
// 适用于 payload 已经是反序列化的 map，需要按字段类型化提取的场景。
//
// 注意：调用方需保证 T 是带 json tag 的结构体指针或可寻址类型。
func DecodePayload[T any](msg *Message) (T, error) {
	var zero T
	if msg == nil {
		return zero, fmt.Errorf("nil message")
	}
	if msg.Payload == nil {
		return zero, fmt.Errorf("nil payload")
	}

	raw, err := json.Marshal(msg.Payload)
	if err != nil {
		return zero, fmt.Errorf("marshal payload: %w", err)
	}

	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, fmt.Errorf("unmarshal payload: %w", err)
	}
	return result, nil
}

// DecodeRegister 解析注册消息
func DecodeRegister(msg *Message) (RegisterPayload, error) {
	return DecodePayload[RegisterPayload](msg)
}

// DecodeHeartbeat 解析心跳消息
func DecodeHeartbeat(msg *Message) (HeartbeatPayload, error) {
	return DecodePayload[HeartbeatPayload](msg)
}

// DecodeRPCRequest 解析 RPC 请求
func DecodeRPCRequest(msg *Message) (RPCRequestPayload, error) {
	return DecodePayload[RPCRequestPayload](msg)
}

// DecodeRPCResponse 解析 RPC 响应
func DecodeRPCResponse(msg *Message) (RPCResponsePayload, error) {
	return DecodePayload[RPCResponsePayload](msg)
}

// DecodeProxyNew 解析新建代理连接消息
func DecodeProxyNew(msg *Message) (ProxyNewPayload, error) {
	return DecodePayload[ProxyNewPayload](msg)
}

// DecodeProxyData 解析代理数据消息
//
// Data 字段在 wire 上有三种兼容格式：
//   - base64 字符串（最常见，json.Unmarshal 可直接还原为 []byte）
//   - JSON 数字数组（旧客户端兼容）
//   - 原始 []byte（codec 内部调用）
//
// 主路径直接走标准 JSON 解码；当 Data 不是字符串（数字数组）时，
// 提取其它字段并手动转换 Data。
func DecodeProxyData(msg *Message) (ProxyDataPayload, error) {
	primary, err := DecodePayload[ProxyDataPayload](msg)
	if err == nil {
		return primary, nil
	}

	// Fallback：Data 不是字符串，无法用 []byte 解码
	// 手动提取字段 + 兼容三种 Data 格式
	var p ProxyDataPayload
	if v, ok := msg.Payload["proxyId"].(string); ok {
		p.ProxyID = v
	}
	if v, ok := msg.Payload["connId"].(string); ok {
		p.ConnID = v
	}
	if v, ok := msg.Payload["newConn"].(bool); ok {
		p.NewConn = v
	}
	p.Data = decodeDataField(msg.Payload["data"])
	if p.Data == nil {
		return primary, fmt.Errorf("decode proxy data: %w", err)
	}
	return p, nil
}

// decodeDataField 兼容解码 Data 字段的三种 wire 格式
func decodeDataField(v interface{}) []byte {
	switch d := v.(type) {
	case string:
		decoded, err := base64.StdEncoding.DecodeString(d)
		if err != nil {
			return []byte(d)
		}
		return decoded
	case []byte:
		return d
	case []interface{}:
		buf := make([]byte, len(d))
		for i, b := range d {
			if f, ok := b.(float64); ok {
				buf[i] = byte(f)
			}
		}
		return buf
	}
	return nil
}

// DecodeProxyClose 解析关闭代理连接消息
func DecodeProxyClose(msg *Message) (ProxyClosePayload, error) {
	return DecodePayload[ProxyClosePayload](msg)
}

// DecodeProxyError 解析代理错误消息
func DecodeProxyError(msg *Message) (ProxyErrorPayload, error) {
	return DecodePayload[ProxyErrorPayload](msg)
}

// DecodeDesktopDataHeader 解析桌面数据消息的路由头（sessionId + desktopType）
func DecodeDesktopDataHeader(msg *Message) (DesktopDataHeaderPayload, error) {
	return DecodePayload[DesktopDataHeaderPayload](msg)
}

// DecodeDesktopDisconnected 解析桌面断开事件
func DecodeDesktopDisconnected(msg *Message) (DesktopDisconnectedPayload, error) {
	return DecodePayload[DesktopDisconnectedPayload](msg)
}
