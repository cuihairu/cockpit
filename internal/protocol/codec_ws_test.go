package protocol

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// wsPair creates a connected pair of websocket connections for codec tests.
func wsPair(t *testing.T) (client *websocket.Conn, server *websocket.Conn) {
	t.Helper()
	serverReady := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade error: %v", err)
			return
		}
		serverReady <- conn
		<-make(chan struct{}) // hold the connection open until test ends
	}))
	t.Cleanup(srv.Close)

	dialURL := "ws" + strings.TrimPrefix(srv.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(dialURL, nil)
	if err != nil {
		t.Fatalf("dial error: %v", err)
	}
	t.Cleanup(func() { conn.Close() })

	select {
	case server = <-serverReady:
	case <-time.After(5 * time.Second):
		t.Fatal("server connection was not established")
	}
	t.Cleanup(func() { server.Close() })
	return conn, server
}

func TestCodecWriteReadMessageOverWebSocket(t *testing.T) {
	client, server := wsPair(t)

	codec := NewCodec()
	sent := &Message{
		ID:   "msg-1",
		Type: MessageTypeHeartbeat,
		Payload: map[string]interface{}{
			"agentId": "agent-1",
		},
	}

	if err := codec.WriteMessage(client, sent); err != nil {
		t.Fatalf("WriteMessage() error = %v", err)
	}

	got, err := codec.ReadMessage(server)
	if err != nil {
		t.Fatalf("ReadMessage() error = %v", err)
	}
	if got.ID != "msg-1" {
		t.Errorf("ID = %q, want msg-1", got.ID)
	}
	if got.Type != MessageTypeHeartbeat {
		t.Errorf("Type = %q, want %q", got.Type, MessageTypeHeartbeat)
	}
}

func TestCodecReadMessageOnClosedConnection(t *testing.T) {
	client, server := wsPair(t)

	codec := NewCodec()
	client.Close()

	if _, err := codec.ReadMessage(server); err == nil {
		t.Error("ReadMessage() should fail after peer close")
	}
}

func TestDecodeProxyError(t *testing.T) {
	msg := &Message{
		Type: MessageTypeProxyError,
		Payload: map[string]interface{}{
			"proxyId": "px-1",
			"connId":  "conn-9",
			"error":   "upstream refused",
		},
	}

	p, err := DecodeProxyError(msg)
	if err != nil {
		t.Fatalf("DecodeProxyError() error = %v", err)
	}
	if p.ProxyID != "px-1" {
		t.Errorf("ProxyID = %q, want px-1", p.ProxyID)
	}
	if p.ConnID != "conn-9" {
		t.Errorf("ConnID = %q, want conn-9", p.ConnID)
	}
	if p.Error != "upstream refused" {
		t.Errorf("Error = %q, want upstream refused", p.Error)
	}
}

func TestDecodeDesktopDataHeader(t *testing.T) {
	msg := &Message{
		Type: MessageTypeDesktopData,
		Payload: map[string]interface{}{
			"sessionId":   "sess-1",
			"desktopType": DesktopMsgScreenUpdate,
		},
	}

	h, err := DecodeDesktopDataHeader(msg)
	if err != nil {
		t.Fatalf("DecodeDesktopDataHeader() error = %v", err)
	}
	if h.SessionID != "sess-1" {
		t.Errorf("SessionID = %q, want sess-1", h.SessionID)
	}
	if h.DesktopType != DesktopMsgScreenUpdate {
		t.Errorf("DesktopType = %q, want %q", h.DesktopType, DesktopMsgScreenUpdate)
	}
}

func TestDecodeDesktopDisconnected(t *testing.T) {
	msg := &Message{
		Type: MessageTypeDesktopClose,
		Payload: map[string]interface{}{
			"sessionId": "sess-2",
			"reason":    "user closed",
		},
	}

	d, err := DecodeDesktopDisconnected(msg)
	if err != nil {
		t.Fatalf("DecodeDesktopDisconnected() error = %v", err)
	}
	if d.SessionID != "sess-2" {
		t.Errorf("SessionID = %q, want sess-2", d.SessionID)
	}
	if d.Reason != "user closed" {
		t.Errorf("Reason = %q, want user closed", d.Reason)
	}
}

func TestDecodePayloadTypeMismatch(t *testing.T) {
	// sessionId is a string field; a number must fail unmarshalling
	msg := &Message{
		Payload: map[string]interface{}{
			"sessionId": 123,
		},
	}
	if _, err := DecodeDesktopDisconnected(msg); err == nil {
		t.Error("DecodeDesktopDisconnected() should fail on type mismatch")
	}
}

func TestDecodeProxyDataNilDataFallback(t *testing.T) {
	// data field of an unsupported type makes the fallback return the primary error
	msg := &Message{
		Payload: map[string]interface{}{
			"proxyId": "px-1",
			"connId":  "conn-1",
			"newConn": true,
			"data":    12345,
		},
	}
	if _, err := DecodeProxyData(msg); err == nil {
		t.Error("DecodeProxyData() should fail when data field cannot be decoded")
	}
}

func TestDecodeProxyDataRawBytes(t *testing.T) {
	msg := &Message{
		Payload: map[string]interface{}{
			"proxyId": "px-1",
			"connId":  "conn-1",
			"data":    []byte{0x01, 0x02, 0x03},
		},
	}
	p, err := DecodeProxyData(msg)
	if err != nil {
		t.Fatalf("DecodeProxyData() error = %v", err)
	}
	if string(p.Data) != "\x01\x02\x03" {
		t.Errorf("Data = %v, want [1 2 3]", p.Data)
	}
}
