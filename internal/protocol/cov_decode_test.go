package protocol

import (
	"strings"
	"testing"
)

// covBadPayloadMessage 构造一个 Payload 无法被 JSON 序列化的消息
func covBadPayloadMessage(t *testing.T) *Message {
	t.Helper()
	return &Message{
		Type:    MessageTypeHeartbeat,
		Payload: map[string]interface{}{"bad": func() {}},
	}
}

func TestCovCodecWriteMessageEncodeError(t *testing.T) {
	c := NewCodec()
	msg := covBadPayloadMessage(t)

	// Encode 失败时应直接返回错误，不需要真实连接
	err := c.WriteMessage(nil, msg)
	if err == nil {
		t.Fatal("WriteMessage() should return error when payload cannot be encoded")
	}
}

func TestCovCodecEncodeError(t *testing.T) {
	c := NewCodec()
	msg := covBadPayloadMessage(t)

	_, err := c.Encode(msg)
	if err == nil {
		t.Fatal("Encode() should return error for unserializable payload")
	}
}

func TestCovWriteMessageV2MarshalError(t *testing.T) {
	msg := covBadPayloadMessage(t)

	var sb strings.Builder
	if err := WriteMessage(&sb, msg); err == nil {
		t.Fatal("WriteMessage() should return error when payload cannot be marshaled")
	}
}

func TestCovDecodePayloadUnmarshalError(t *testing.T) {
	msg := &Message{
		Type:    MessageTypeHeartbeat,
		Payload: map[string]interface{}{"nested": "value"},
	}

	// 顶层 payload 为 object，无法解码为 int
	if _, err := DecodePayload[int](msg); err == nil {
		t.Fatal("DecodePayload[int]() should fail for object payload")
	}
}

func TestCovDecodeProxyDataNumberArrayFallback(t *testing.T) {
	// 旧客户端 wire 格式：data 为 JSON 数字数组
	msg := &Message{
		Type: MessageTypeProxyData,
		Payload: map[string]interface{}{
			"proxyId": "px-1",
			"connId":  "cn-1",
			"newConn": true,
			"data":    []interface{}{float64(104), float64(105)},
		},
	}

	p, err := DecodeProxyData(msg)
	if err != nil {
		t.Fatalf("DecodeProxyData() fallback error = %v", err)
	}
	if p.ProxyID != "px-1" {
		t.Errorf("ProxyID = %q, want px-1", p.ProxyID)
	}
	if p.ConnID != "cn-1" {
		t.Errorf("ConnID = %q, want cn-1", p.ConnID)
	}
	if !p.NewConn {
		t.Error("NewConn should be true")
	}
	if string(p.Data) != "hi" {
		t.Errorf("Data = %q, want %q", string(p.Data), "hi")
	}
}

func TestCovDecodeProxyDataInvalidBase64Fallback(t *testing.T) {
	// data 为非法 base64 字符串：主路径 json.Unmarshal([]byte) 报 base64 错误，
	// fallback 中 decodeDataField 将原文按字节返回
	msg := &Message{
		Type: MessageTypeProxyData,
		Payload: map[string]interface{}{
			"proxyId": "px-2",
			"connId":  "cn-2",
			"data":    "###not-base64###",
		},
	}

	p, err := DecodeProxyData(msg)
	if err != nil {
		t.Fatalf("DecodeProxyData() fallback error = %v", err)
	}
	if p.ProxyID != "px-2" || p.ConnID != "cn-2" {
		t.Errorf("ProxyID/ConnID = %q/%q, want px-2/cn-2", p.ProxyID, p.ConnID)
	}
	if string(p.Data) != "###not-base64###" {
		t.Errorf("Data = %q, want raw passthrough %q", string(p.Data), "###not-base64###")
	}
}

func TestCovDecodePayloadMarshalError(t *testing.T) {
	msg := &Message{
		Type:    MessageTypeHeartbeat,
		Payload: map[string]interface{}{"bad": func() {}},
	}

	if _, err := DecodePayload[int](msg); err == nil {
		t.Fatal("DecodePayload() should fail when payload cannot be marshaled")
	}
}

func TestCovDecodeProxyDataNilDataError(t *testing.T) {
	// data 为 object：主路径解码失败，fallback 中 decodeDataField 也返回 nil
	msg := &Message{
		Type: MessageTypeProxyData,
		Payload: map[string]interface{}{
			"proxyId": "px-1",
			"connId":  "cn-1",
			"data":    map[string]interface{}{"unexpected": true},
		},
	}

	p, err := DecodeProxyData(msg)
	if err == nil {
		t.Fatal("DecodeProxyData() should return error when data field is undecodable")
	}
	if p.Data != nil {
		t.Errorf("Data = %v, want nil", p.Data)
	}
}
