package protocol

import (
	"encoding/json"
	"io"

	"github.com/gorilla/websocket"
)

// Codec 消息编解码器
type Codec struct{}

// NewCodec 创建编解码器
func NewCodec() *Codec {
	return &Codec{}
}

// Encode 编码消息为 JSON
func (c *Codec) Encode(msg *Message) ([]byte, error) {
	return json.Marshal(msg)
}

// Decode 从 JSON 解码消息
func (c *Codec) Decode(data []byte) (*Message, error) {
	var msg Message
	err := json.Unmarshal(data, &msg)
	return &msg, err
}

// WriteMessage 写消息到 WebSocket 连接
func (c *Codec) WriteMessage(conn *websocket.Conn, msg *Message) error {
	data, err := c.Encode(msg)
	if err != nil {
		return err
	}
	return conn.WriteMessage(websocket.TextMessage, data)
}

// ReadMessage 从 WebSocket 连接读取消息
func (c *Codec) ReadMessage(conn *websocket.Conn) (*Message, error) {
	_, data, err := conn.ReadMessage()
	if err != nil {
		return nil, err
	}
	return c.Decode(data)
}

// WriteMessageV2 写消息（支持 writer 接口）
func WriteMessage(w io.Writer, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// ReadMessageV2 读消息（支持 reader 接口）
func ReadMessage(r io.Reader) (*Message, error) {
	decoder := json.NewDecoder(r)
	var msg Message
	err := decoder.Decode(&msg)
	return &msg, err
}
