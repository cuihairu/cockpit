package rpc

// 覆盖率补充测试：router.go 与 system_provider.go（不改动既有测试文件）。

import (
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestCovRegisteredTypes(t *testing.T) {
	h := NewHandler()
	if got := h.RegisteredTypes(); len(got) != 0 {
		t.Fatalf("empty handler types = %v, want empty", got)
	}
	h.RegisterProvider(NewSystemProvider())
	h.RegisterProvider(&FileProvider{})
	got := h.RegisteredTypes()
	if len(got) != 2 || got[0] != "file" || got[1] != "system" {
		t.Fatalf("types = %v, want [file system]", got)
	}
}

func TestCovHandleDecodeErrors(t *testing.T) {
	h := NewHandler()
	h.RegisterProvider(NewSystemProvider())

	// nil 消息
	if _, err := h.Handle(nil); err == nil {
		t.Error("nil message should fail")
	}
	// nil payload
	msg := &protocol.Message{Type: protocol.MessageTypeRPCRequest}
	if _, err := h.Handle(msg); err == nil {
		t.Error("nil payload should fail")
	}
	// method 字段类型不合法 → unmarshal 失败
	msg2 := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": []interface{}{1},
	})
	if _, err := h.Handle(msg2); err == nil {
		t.Error("non-string method should fail decode")
	}
	// 空 method
	msg3 := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "",
	})
	if _, err := h.Handle(msg3); err == nil {
		t.Error("empty method should fail")
	}
	// 未注册的 provider
	msg4 := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "ghost.action",
	})
	if _, err := h.Handle(msg4); err == nil || err.Error() != "unknown provider: ghost" {
		t.Errorf("unknown provider err = %v", err)
	}
	// 点分隔但空段（splitMethod 忽略空段 → 单段 → system provider）
	msg5 := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "..version",
	})
	resp, err := h.Handle(msg5)
	if err != nil {
		t.Fatalf("empty segments should collapse to system provider: %v", err)
	}
	if resp.Payload["status"] != "success" {
		t.Errorf("payload = %v", resp.Payload)
	}
}

func TestCovSystemProviderVersionAndUnknown(t *testing.T) {
	p := NewSystemProvider()
	if p.Type() != "system" {
		t.Fatalf("type = %q", p.Type())
	}
	res, err := p.Call("version", nil)
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	m := res.(map[string]interface{})
	if m["version"] != "0.1.0" || m["build"] != "dev" {
		t.Errorf("version = %v", m)
	}
	if _, err := p.Call("bogus", nil); err == nil || err.Error() != "unknown action: bogus" {
		t.Errorf("unknown action err = %v", err)
	}
	// info / status 直调
	if _, err := p.Info(nil); err != nil {
		t.Errorf("info: %v", err)
	}
	if _, err := p.Status(nil); err != nil {
		t.Errorf("status: %v", err)
	}
}
