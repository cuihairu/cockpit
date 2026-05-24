package protocol

import (
	"bytes"
	"testing"
)

func TestNewCodec(t *testing.T) {
	codec := NewCodec()
	if codec == nil {
		t.Fatal("NewCodec() should not return nil")
	}
}

func TestCodecEncode(t *testing.T) {
	codec := NewCodec()
	msg := &Message{
		ID:        "msg-123",
		Type:      MessageTypeRegister,
		Timestamp: 1234567890,
		Payload: map[string]interface{}{
			"key": "value",
		},
	}

	data, err := codec.Encode(msg)
	if err != nil {
		t.Errorf("Encode() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Encode() should return non-empty data")
	}

	// Should be valid JSON
	if !bytes.HasPrefix(data, []byte("{")) {
		t.Error("Encode() should return JSON object")
	}
}

func TestCodecDecode(t *testing.T) {
	jsonData := []byte(`{"id":"msg-456","type":"register","timestamp":1234567890,"payload":{"test":true}}`)

	codec := NewCodec()
	msg, err := codec.Decode(jsonData)

	if err != nil {
		t.Errorf("Decode() error = %v", err)
	}

	if msg == nil {
		t.Fatal("Decode() should not return nil")
	}

	if msg.ID != "msg-456" {
		t.Errorf("ID = %v, want msg-456", msg.ID)
	}

	if msg.Type != MessageTypeRegister {
		t.Errorf("Type = %v, want register", msg.Type)
	}
}

func TestCodecEncodeDecode(t *testing.T) {
	codec := NewCodec()
	original := &Message{
		ID:        "test-id",
		Type:      MessageTypeHeartbeat,
		Timestamp: 9876543210,
		Payload: map[string]interface{}{
			"status": "online",
			"metrics": map[string]interface{}{
				"cpu": 50.0,
			},
		},
	}

	// Encode
	data, err := codec.Encode(original)
	if err != nil {
		t.Errorf("Encode() error = %v", err)
	}

	// Decode
	decoded, err := codec.Decode(data)
	if err != nil {
		t.Errorf("Decode() error = %v", err)
	}

	// Verify
	if decoded.ID != original.ID {
		t.Errorf("ID = %v, want %v", decoded.ID, original.ID)
	}

	if decoded.Type != original.Type {
		t.Errorf("Type = %v, want %v", decoded.Type, original.Type)
	}

	if decoded.Timestamp != original.Timestamp {
		t.Errorf("Timestamp = %v, want %v", decoded.Timestamp, original.Timestamp)
	}
}

func TestCodecDecodeInvalidJSON(t *testing.T) {
	codec := NewCodec()
	invalidData := []byte("not valid json")

	_, err := codec.Decode(invalidData)
	if err == nil {
		t.Error("Decode() should return error for invalid JSON")
	}
}

func TestCodecDecodeEmptyData(t *testing.T) {
	codec := NewCodec()
	_, err := codec.Decode([]byte{})

	if err == nil {
		t.Error("Decode() should return error for empty data")
	}
}

func TestWriteMessageV2(t *testing.T) {
	msg := &Message{
		ID:        "write-test",
		Type:      MessageTypePing,
		Timestamp: 1111111111,
		Payload:   map[string]interface{}{"ping": "pong"},
	}

	var buf bytes.Buffer
	err := WriteMessage(&buf, msg)

	if err != nil {
		t.Errorf("WriteMessage() error = %v", err)
	}

	if buf.Len() == 0 {
		t.Error("WriteMessage() should write data")
	}

	// Verify it's valid JSON
	result := buf.String()
	if result[0] != '{' {
		t.Error("WriteMessage() should write JSON object")
	}
}

func TestReadMessageV2(t *testing.T) {
	jsonData := `{"id":"read-test","type":"heartbeat","timestamp":2222222222,"payload":{}}`
	reader := bytes.NewReader([]byte(jsonData))

	msg, err := ReadMessage(reader)
	if err != nil {
		t.Errorf("ReadMessage() error = %v", err)
	}

	if msg == nil {
		t.Fatal("ReadMessage() should not return nil")
	}

	if msg.ID != "read-test" {
		t.Errorf("ID = %v, want read-test", msg.ID)
	}

	if msg.Type != MessageTypeHeartbeat {
		t.Errorf("Type = %v, want heartbeat", msg.Type)
	}
}

func TestReadMessageV2Invalid(t *testing.T) {
	reader := bytes.NewReader([]byte("invalid json"))

	_, err := ReadMessage(reader)
	if err == nil {
		t.Error("ReadMessage() should return error for invalid JSON")
	}
}

func TestWriteReadMessageV2RoundTrip(t *testing.T) {
	original := &Message{
		ID:        "roundtrip",
		Type:      MessageTypeRPCRequest,
		Timestamp: 3333333333,
		Payload: map[string]interface{}{
			"method": "test.method",
			"params": map[string]interface{}{
				"arg1": "value1",
			},
		},
	}

	var buf bytes.Buffer

	// Write
	err := WriteMessage(&buf, original)
	if err != nil {
		t.Errorf("WriteMessage() error = %v", err)
	}

	// Read
	read, err := ReadMessage(&buf)
	if err != nil {
		t.Errorf("ReadMessage() error = %v", err)
	}

	// Verify
	if read.ID != original.ID {
		t.Errorf("ID = %v, want %v", read.ID, original.ID)
	}

	if read.Type != original.Type {
		t.Errorf("Type = %v, want %v", read.Type, original.Type)
	}
}

func TestCodecEncodeNilMessage(t *testing.T) {
	codec := NewCodec()

	// json.Marshal(nil) returns "null", not an error
	data, err := codec.Encode(nil)
	if err != nil {
		t.Errorf("Encode() error = %v", err)
	}

	if string(data) != "null" {
		t.Errorf("Encode() should return 'null' for nil message, got %s", string(data))
	}
}

func TestCodecEncodeEmptyMessage(t *testing.T) {
	codec := NewCodec()
	msg := &Message{}

	data, err := codec.Encode(msg)
	if err != nil {
		t.Errorf("Encode() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Encode() should return data for empty message")
	}
}

func TestCodecDecodeEmptyMessage(t *testing.T) {
	codec := NewCodec()
	emptyJSON := []byte("{}")

	msg, err := codec.Decode(emptyJSON)
	if err != nil {
		t.Errorf("Decode() error = %v", err)
	}

	if msg == nil {
		t.Fatal("Decode() should not return nil for empty JSON")
	}

	if msg.Type != "" {
		t.Errorf("Type should be empty for empty JSON, got %v", msg.Type)
	}
}

func TestWriteMessageNil(t *testing.T) {
	var buf bytes.Buffer
	// json.Marshal(nil) returns "null", not an error
	err := WriteMessage(&buf, nil)

	if err != nil {
		t.Errorf("WriteMessage() error = %v", err)
	}

	if buf.String() != "null" {
		t.Errorf("WriteMessage() should write 'null' for nil message, got %s", buf.String())
	}
}

func TestReadMessageEmptyReader(t *testing.T) {
	reader := bytes.NewReader([]byte{})
	_, err := ReadMessage(reader)

	if err == nil {
		t.Error("ReadMessage() should return error for empty reader")
	}
}

func TestMessagePayloadEncoding(t *testing.T) {
	codec := NewCodec()

	payloads := []map[string]interface{}{
		{"string": "value"},
		{"number": 123},
		{"float": 45.67},
		{"bool": true},
		{"null": nil},
		{"array": []string{"a", "b"}},
		{"nested": map[string]interface{}{"key": "val"}},
	}

	for i, p := range payloads {
		t.Run("payload test", func(t *testing.T) {
			msg := &Message{
				ID:        "test",
				Type:      MessageTypeRegister,
				Timestamp: 1234567890,
				Payload:   p,
			}

			data, err := codec.Encode(msg)
			if err != nil {
				t.Errorf("payload %d: Encode() error = %v", i, err)
			}

			decoded, err := codec.Decode(data)
			if err != nil {
				t.Errorf("payload %d: Decode() error = %v", i, err)
			}

			if len(decoded.Payload) != len(p) {
				t.Errorf("payload %d: Payload length = %d, want %d", i, len(decoded.Payload), len(p))
			}
		})
	}
}

func TestCodecSpecialCharacters(t *testing.T) {
	codec := NewCodec()

	msg := &Message{
		ID:        "test-special",
		Type:      MessageTypeRegister,
		Timestamp: 1234567890,
		Payload: map[string]interface{}{
			"unicode": "Hello 世界",
			"emoji":   "🚀🎉",
			"quotes":  `text with "quotes"`,
		},
	}

	data, err := codec.Encode(msg)
	if err != nil {
		t.Errorf("Encode() error = %v", err)
	}

	decoded, err := codec.Decode(data)
	if err != nil {
		t.Errorf("Decode() error = %v", err)
	}

	if decoded.Payload["unicode"] != "Hello 世界" {
		t.Error("Unicode not preserved")
	}

	if decoded.Payload["emoji"] != "🚀🎉" {
		t.Error("Emoji not preserved")
	}

	if decoded.Payload["quotes"] != `text with "quotes"` {
		t.Error("Quotes not preserved")
	}
}
