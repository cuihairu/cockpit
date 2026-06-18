package protocol

import (
	"reflect"
	"testing"
)

func TestDecodePayload_NilMessage(t *testing.T) {
	_, err := DecodePayload[RegisterPayload](nil)
	if err == nil {
		t.Fatal("expected error for nil message")
	}
}

func TestDecodePayload_NilPayload(t *testing.T) {
	msg := &Message{Type: MessageTypeRegister, Payload: nil}
	_, err := DecodePayload[RegisterPayload](msg)
	if err == nil {
		t.Fatal("expected error for nil payload")
	}
}

func TestDecodeRegister(t *testing.T) {
	msg := &Message{
		Type: MessageTypeRegister,
		Payload: map[string]interface{}{
			"agentId":  "agent-1",
			"secret":   "s3cret",
			"hostname": "host-1",
			"location": map[string]interface{}{
				"region": "cn-bj",
				"zone":   "cn-bj-1",
			},
			"capabilities": []interface{}{
				map[string]interface{}{"type": "docker-api", "endpoint": "unix:///var/run/docker.sock"},
			},
		},
	}

	got, err := DecodeRegister(msg)
	if err != nil {
		t.Fatalf("DecodeRegister failed: %v", err)
	}
	if got.AgentID != "agent-1" || got.Secret != "s3cret" || got.Hostname != "host-1" {
		t.Errorf("unexpected basic fields: %+v", got)
	}
	if got.Location.Region != "cn-bj" || got.Location.Zone != "cn-bj-1" {
		t.Errorf("unexpected location: %+v", got.Location)
	}
	if len(got.Capabilities) != 1 || got.Capabilities[0].Type != "docker-api" {
		t.Errorf("unexpected capabilities: %+v", got.Capabilities)
	}
}

func TestDecodeHeartbeat(t *testing.T) {
	msg := &Message{
		Type: MessageTypeHeartbeat,
		Payload: map[string]interface{}{
			"agentId": "agent-1",
			"status":  "online",
			"systemInfo": map[string]interface{}{
				"cpuUsage":   55.5,
				"cpuCores":   4,
				"memTotal":   8589934592,
				"hostname":   "host-1",
				"osName":     "linux",
				"load1":      0.5,
			},
		},
	}

	got, err := DecodeHeartbeat(msg)
	if err != nil {
		t.Fatalf("DecodeHeartbeat failed: %v", err)
	}
	if got.AgentID != "agent-1" || got.Status != "online" {
		t.Errorf("unexpected basic fields: %+v", got)
	}
	if got.SystemInfo == nil {
		t.Fatal("expected systemInfo")
	}
	if got.SystemInfo.CPUUsage != 55.5 || got.SystemInfo.CPUCores != 4 {
		t.Errorf("unexpected systemInfo: %+v", got.SystemInfo)
	}
	if got.SystemInfo.MemTotal != 8589934592 {
		t.Errorf("unexpected memTotal: %d", got.SystemInfo.MemTotal)
	}
}

func TestDecodeRPCRequest(t *testing.T) {
	msg := &Message{
		Type: MessageTypeRPCRequest,
		Payload: map[string]interface{}{
			"method": "pve.vms.list",
			"params": map[string]interface{}{"node": "pve1"},
		},
	}

	got, err := DecodeRPCRequest(msg)
	if err != nil {
		t.Fatalf("DecodeRPCRequest failed: %v", err)
	}
	if got.Method != "pve.vms.list" {
		t.Errorf("unexpected method: %s", got.Method)
	}
	if got.Params["node"] != "pve1" {
		t.Errorf("unexpected params: %+v", got.Params)
	}
}

func TestDecodeRPCResponse(t *testing.T) {
	msg := &Message{
		Type: MessageTypeRPCResponse,
		Payload: map[string]interface{}{
			"status": "success",
			"data":   map[string]interface{}{"foo": "bar"},
		},
	}

	got, err := DecodeRPCResponse(msg)
	if err != nil {
		t.Fatalf("DecodeRPCResponse failed: %v", err)
	}
	if got.Status != "success" {
		t.Errorf("unexpected status: %s", got.Status)
	}
}

func TestDecodeProxyNew(t *testing.T) {
	msg := &Message{
		Type: MessageTypeProxyNew,
		Payload: map[string]interface{}{
			"proxyId":   "p1",
			"proxyType": "tcp",
			"target":    "192.168.31.1:80",
		},
	}

	got, err := DecodeProxyNew(msg)
	if err != nil {
		t.Fatalf("DecodeProxyNew failed: %v", err)
	}
	if got.ProxyID != "p1" || got.ProxyType != "tcp" || got.Target != "192.168.31.1:80" {
		t.Errorf("unexpected payload: %+v", got)
	}
}

func TestDecodeProxyData_Base64String(t *testing.T) {
	// base64 of "hello"
	const b64 = "aGVsbG8="
	msg := &Message{
		Type: MessageTypeProxyData,
		Payload: map[string]interface{}{
			"proxyId": "p1",
			"connId":  "c1",
			"data":    b64,
		},
	}

	got, err := DecodeProxyData(msg)
	if err != nil {
		t.Fatalf("DecodeProxyData failed: %v", err)
	}
	if got.ProxyID != "p1" || got.ConnID != "c1" {
		t.Errorf("unexpected ids: %+v", got)
	}
	if string(got.Data) != "hello" {
		t.Errorf("unexpected data: %q", got.Data)
	}
}

func TestDecodeProxyData_NumberArray(t *testing.T) {
	// 数字数组（旧客户端兼容）
	msg := &Message{
		Type: MessageTypeProxyData,
		Payload: map[string]interface{}{
			"proxyId": "p1",
			"connId":  "c1",
			"data":    []interface{}{float64(104), float64(105)},
		},
	}

	got, err := DecodeProxyData(msg)
	if err != nil {
		t.Fatalf("DecodeProxyData failed: %v", err)
	}
	if !reflect.DeepEqual(got.Data, []byte{104, 105}) {
		t.Errorf("unexpected data: %v", got.Data)
	}
}

func TestDecodeProxyClose(t *testing.T) {
	msg := &Message{
		Type: MessageTypeProxyClose,
		Payload: map[string]interface{}{
			"proxyId": "p1",
			"connId":  "c1",
			"reason":  "client closed",
		},
	}

	got, err := DecodeProxyClose(msg)
	if err != nil {
		t.Fatalf("DecodeProxyClose failed: %v", err)
	}
	if got.ProxyID != "p1" || got.ConnID != "c1" || got.Reason != "client closed" {
		t.Errorf("unexpected payload: %+v", got)
	}
}

func TestDecodeDataField(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want []byte
	}{
		{"base64", "aGVsbG8=", []byte("hello")},
		{"raw bytes", []byte("hi"), []byte("hi")},
		{"number array", []interface{}{float64(65), float64(66)}, []byte("AB")},
		{"invalid base64 falls back to raw", "not!base64?", []byte("not!base64?")},
		{"nil", nil, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := decodeDataField(c.in)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("decodeDataField(%v) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}
