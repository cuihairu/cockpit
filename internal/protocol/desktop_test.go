package protocol

import (
	"encoding/json"
	"testing"
)

func TestDesktopMessageTypes(t *testing.T) {
	types := []DesktopMessageType{
		DesktopMsgConnect,
		DesktopMsgConnected,
		DesktopMsgDisconnected,
		DesktopMsgError,
		DesktopMsgScreenUpdate,
		DesktopMsgKeyboard,
		DesktopMsgMouse,
		DesktopMsgSetResolution,
		DesktopMsgSetQuality,
		DesktopMsgClipboardRequest,
		DesktopMsgClipboardData,
		DesktopMsgFileUpload,
		DesktopMsgFileDownload,
		DesktopMsgFileData,
		DesktopMsgRecordingStart,
		DesktopMsgRecordingStop,
		DesktopMsgShareGrant,
		DesktopMsgShareRevoke,
	}

	for _, dt := range types {
		if string(dt) == "" {
			t.Errorf("DesktopMessageType should not be empty: %v", dt)
		}
	}
}

func TestDesktopConnectPayloadSerialization(t *testing.T) {
	payload := DesktopConnectPayload{
		SessionID:  "test-session",
		Target:     "192.168.1.100:3389",
		Username:   "admin",
		Password:   "secret",
		Domain:     "WORKGROUP",
		Width:      1920,
		Height:     1080,
		ColorDepth: 32,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DesktopConnectPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.SessionID != payload.SessionID {
		t.Errorf("SessionID = %v, want %v", decoded.SessionID, payload.SessionID)
	}
	if decoded.Target != payload.Target {
		t.Errorf("Target = %v, want %v", decoded.Target, payload.Target)
	}
	if decoded.Width != payload.Width {
		t.Errorf("Width = %v, want %v", decoded.Width, payload.Width)
	}
}

func TestDesktopScreenUpdatePayloadSerialization(t *testing.T) {
	payload := DesktopScreenUpdatePayload{
		SessionID: "test-session",
		Width:     1920,
		Height:    1080,
		Rects: []DesktopBitmapRect{
			{X: 0, Y: 0, Width: 64, Height: 64, Data: "base64data"},
			{X: 100, Y: 200, Width: 32, Height: 32, Data: "moredata"},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DesktopScreenUpdatePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if len(decoded.Rects) != 2 {
		t.Fatalf("Rects length = %d, want 2", len(decoded.Rects))
	}
	if decoded.Rects[0].X != 0 || decoded.Rects[0].Width != 64 {
		t.Errorf("First rect mismatch: %+v", decoded.Rects[0])
	}
}

func TestDesktopKeyboardPayloadSerialization(t *testing.T) {
	payload := DesktopKeyboardPayload{
		SessionID: "test-session",
		ScanCode:  0x1C,
		KeyDown:   true,
		Extended:  false,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DesktopKeyboardPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.ScanCode != 0x1C {
		t.Errorf("ScanCode = 0x%X, want 0x1C", decoded.ScanCode)
	}
	if !decoded.KeyDown {
		t.Error("KeyDown should be true")
	}
}

func TestDesktopMousePayloadSerialization(t *testing.T) {
	payload := DesktopMousePayload{
		SessionID:  "test-session",
		X:          500,
		Y:          300,
		Buttons:    1, // left
		WheelDelta: 0,
		Action:     "down",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DesktopMousePayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.X != 500 || decoded.Y != 300 {
		t.Errorf("Position = (%d,%d), want (500,300)", decoded.X, decoded.Y)
	}
	if decoded.Action != "down" {
		t.Errorf("Action = %v, want down", decoded.Action)
	}
}

func TestDesktopMessageInEnvelope(t *testing.T) {
	msg := NewMessage(MessageTypeDesktopData, map[string]interface{}{
		"sessionId":   "test",
		"desktopType": string(DesktopMsgKeyboard),
		"scanCode":    28,
		"keyDown":     true,
	})

	if msg.Type != MessageTypeDesktopData {
		t.Errorf("Type = %v, want %v", msg.Type, MessageTypeDesktopData)
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded Message
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Type != MessageTypeDesktopData {
		t.Errorf("Decoded type = %v, want %v", decoded.Type, MessageTypeDesktopData)
	}
}

func TestDesktopErrorPayload(t *testing.T) {
	payload := DesktopErrorPayload{
		SessionID: "test",
		Error:     "connection refused",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DesktopErrorPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Error != "connection refused" {
		t.Errorf("Error = %v, want 'connection refused'", decoded.Error)
	}
}

func TestDesktopResolutionPayload(t *testing.T) {
	payload := DesktopResolutionPayload{
		SessionID: "test",
		Width:     2560,
		Height:    1440,
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DesktopResolutionPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Width != 2560 || decoded.Height != 1440 {
		t.Errorf("Resolution = %dx%d, want 2560x1440", decoded.Width, decoded.Height)
	}
}

func TestDesktopClipboardPayload(t *testing.T) {
	payload := DesktopClipboardPayload{
		SessionID: "test",
		Text:      "clipboard content with 中文",
	}

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	var decoded DesktopClipboardPayload
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	if decoded.Text != payload.Text {
		t.Errorf("Text = %v, want %v", decoded.Text, payload.Text)
	}
}
