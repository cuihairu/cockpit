package rpc

import (
	"errors"
	"testing"

	"github.com/cuihairu/cockpit/core/execlog"
	"github.com/cuihairu/cockpit/internal/protocol"
)

// execFakeProvider 埋点用最小 provider：Call 返回注入的结果/错误。
type execFakeProvider struct {
	result interface{}
	err    error
}

func (p *execFakeProvider) Type() string { return "execfake" }
func (p *execFakeProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.result, nil
}

// execlogHandle 组装带留痕的 Handler 并发一次 RPC 请求，返回 Handle 错误。
func execlogHandle(t *testing.T, p *execFakeProvider, sent *[]*protocol.Message) error {
	t.Helper()
	h := NewHandler()
	h.RegisterProvider(p)
	var rec *execlog.Recorder
	if sent != nil {
		rec = execlog.New(8, func(m *protocol.Message) error {
			*sent = append(*sent, m)
			return nil
		})
		h.SetExecLogger(rec)
	}
	req := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "execfake.restart",
		"params": map[string]interface{}{"unit": "nginx.service", "password": "s3cret"},
	})
	_, err := h.Handle(req)
	if rec != nil {
		_ = rec.Flush()
	}
	return err
}

// TestHandleExeclogSuccess 成功执行留痕：provider/action/status/durationMs
// 落字段，入参过脱敏（password 键剔除）。
func TestHandleExeclogSuccess(t *testing.T) {
	var sent []*protocol.Message
	p := &execFakeProvider{result: map[string]interface{}{"ok": true}}
	if err := execlogHandle(t, p, &sent); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(sent) != 1 {
		t.Fatalf("flush sent = %d, want 1", len(sent))
	}
	got, err := protocol.DecodeExecLog(sent[0])
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(got.Entries))
	}
	e := got.Entries[0]
	if e.Provider != "execfake" || e.Action != "restart" || e.Status != protocol.ExecStatusSuccess {
		t.Fatalf("entry = %+v", e)
	}
	if e.DurationMs < 0 {
		t.Fatalf("durationMs = %d", e.DurationMs)
	}
	if e.Params["unit"] != "nginx.service" {
		t.Fatalf("params = %+v", e.Params)
	}
	if _, ok := e.Params["password"]; ok {
		t.Fatalf("sensitive param leaked: %+v", e.Params)
	}
}

// TestHandleExeclogFailure 失败执行留痕：status=failure 带 error 摘要，
// RPC 响应错误路径行为不变（err 原样返回）。
func TestHandleExeclogFailure(t *testing.T) {
	var sent []*protocol.Message
	h := NewHandler()
	h.RegisterProvider(&execFakeProvider{err: errors.New("boom")})
	rec := execlog.New(8, func(m *protocol.Message) error {
		sent = append(sent, m)
		return nil
	})
	h.SetExecLogger(rec)

	req := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "execfake.stop",
	})
	if _, err := h.Handle(req); err == nil {
		t.Fatal("handle: want error passthrough")
	}
	if err := rec.Flush(); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 {
		t.Fatalf("flush sent = %d, want 1", len(sent))
	}
	got, _ := protocol.DecodeExecLog(sent[0])
	if len(got.Entries) != 1 || got.Entries[0].Status != protocol.ExecStatusFailure ||
		got.Entries[0].Error != "boom" {
		t.Fatalf("entries = %+v", got.Entries)
	}
}

// TestHandleNilExecLogger 不挂留痕（nil）行为零变化：请求照常处理，无
// panic 无额外消息。
func TestHandleNilExecLogger(t *testing.T) {
	h := NewHandler()
	h.RegisterProvider(&execFakeProvider{result: "ok"})
	req := protocol.NewMessage(protocol.MessageTypeRPCRequest, map[string]interface{}{
		"method": "execfake.status",
	})
	resp, err := h.Handle(req)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if resp == nil {
		t.Fatal("resp = nil")
	}
}
