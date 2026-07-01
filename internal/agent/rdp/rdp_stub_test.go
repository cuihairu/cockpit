//go:build !rdp || darwin

package rdp

import (
	"strings"
	"testing"

	"github.com/cuihairu/cockpit/internal/protocol"
)

func TestStubHandleDesktopNewReportsUnsupportedBuild(t *testing.T) {
	h := NewHandler()

	var captured *protocol.Message
	h.SetSendFunc(func(msg *protocol.Message) error {
		captured = msg
		return nil
	})

	h.HandleDesktopNew(protocol.NewMessage(protocol.MessageTypeDesktopNew, map[string]interface{}{
		"sessionId": "session-1",
		"target":    "127.0.0.1:3389",
	}))

	if captured == nil {
		t.Fatal("expected unsupported-build error message")
	}
	if captured.Type != protocol.MessageTypeDesktopData {
		t.Fatalf("Type = %v, want %v", captured.Type, protocol.MessageTypeDesktopData)
	}
	if captured.Payload["sessionId"] != "session-1" {
		t.Fatalf("sessionId = %v, want session-1", captured.Payload["sessionId"])
	}
	if captured.Payload["desktopType"] != string(protocol.DesktopMsgError) {
		t.Fatalf("desktopType = %v, want %v", captured.Payload["desktopType"], protocol.DesktopMsgError)
	}
	errMsg, _ := captured.Payload["error"].(string)
	if !strings.Contains(errMsg, "-tags rdp") {
		t.Fatalf("error = %q, want build tag guidance", errMsg)
	}
}
