package proxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// 覆盖率缺口补测（只补测试零业务改动）：openSSHProxy 成功/失败、
// NewSSHSession 的合法私钥路径与 NewSession/RequestPty/Shell 错误分支。

// genTestPEMPrivateKey 生成合法 ed25519 PEM 私钥（覆盖 ParsePrivateKey 成功
// 后的 auths append 分支——现有测试只走 password / 坏 key 路径）。
func genTestPEMPrivateKey(t *testing.T) string {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	// x/crypto/ssh 提供 MarshalPrivateKey（OpenSSH 格式）
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return string(pem.EncodeToMemory(block))
}

func TestNewSSHSessionWithValidPrivateKey(t *testing.T) {
	ResetTOFUHostKeys()
	pemKey := genTestPEMPrivateKey(t)
	// 目标不可达：过 ParsePrivateKey 成功路径（auths append）后 Dial 失败
	_, err := NewSSHSession("127.0.0.1:1", "user", "", pemKey, 24, 80)
	if err == nil {
		t.Fatal("unreachable target should fail")
	}
	if !strings.Contains(err.Error(), "SSH connect") {
		t.Errorf("err = %v, want SSH connect failure (private key parsed ok)", err)
	}
}

// rejectSSHServer 可配置拒绝行为的内存 SSH 服务端：
// rejectSession=true → 拒绝 session channel（NewSession 失败）
// rejectPty=true    → pty-req 回失败（RequestPty 失败）
// rejectShell=true  → shell-req 回失败（Shell 失败）
type rejectSSHServer struct {
	ln      net.Listener
	hostKey ssh.Signer
}

func startRejectSSHServer(t *testing.T, rejectSession, rejectPty, rejectShell bool) *rejectSSHServer {
	t.Helper()
	ResetTOFUHostKeys()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ts := &rejectSSHServer{ln: ln, hostKey: signer}
	go ts.serve(rejectSession, rejectPty, rejectShell)
	t.Cleanup(func() { _ = ln.Close() })
	return ts
}

func (ts *rejectSSHServer) addr() string { return ts.ln.Addr().String() }

func (ts *rejectSSHServer) serve(rejectSession, rejectPty, rejectShell bool) {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	config.AddHostKey(ts.hostKey)
	for {
		conn, err := ts.ln.Accept()
		if err != nil {
			return
		}
		go func(nconn net.Conn) {
			defer nconn.Close()
			sconn, chans, reqs, err := ssh.NewServerConn(nconn, config)
			if err != nil {
				return
			}
			defer sconn.Close()
			go ssh.DiscardRequests(reqs)
			for newChan := range chans {
				if rejectSession {
					_ = newChan.Reject(ssh.Prohibited, "session rejected")
					continue
				}
				if newChan.ChannelType() != "session" {
					_ = newChan.Reject(ssh.UnknownChannelType, "only session")
					continue
				}
				ch, chReqs, err := newChan.Accept()
				if err != nil {
					return
				}
				go func(ch ssh.Channel, reqs <-chan *ssh.Request) {
					defer ch.Close()
					for req := range reqs {
						switch req.Type {
						case "pty-req":
							if req.WantReply {
								_ = req.Reply(!rejectPty, nil)
							}
						case "shell", "exec":
							if req.WantReply {
								_ = req.Reply(!rejectShell, nil)
							}
						default:
							if req.WantReply {
								_ = req.Reply(false, nil)
							}
						}
					}
				}(ch, chReqs)
			}
		}(conn)
	}
}

func TestNewSSHSessionNewSessionRejected(t *testing.T) {
	ts := startRejectSSHServer(t, true, false, false)
	_, err := NewSSHSession(ts.addr(), "u", "p", "", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "new session") {
		t.Errorf("err = %v, want new session failure", err)
	}
}

func TestNewSSHSessionRequestPtyRejected(t *testing.T) {
	ts := startRejectSSHServer(t, false, true, false)
	_, err := NewSSHSession(ts.addr(), "u", "p", "", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "request PTY") {
		t.Errorf("err = %v, want request PTY failure", err)
	}
}

func TestNewSSHSessionShellRejected(t *testing.T) {
	ts := startRejectSSHServer(t, false, false, true)
	_, err := NewSSHSession(ts.addr(), "u", "p", "", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "start shell") {
		t.Errorf("err = %v, want start shell failure", err)
	}
}

func TestHandleProxyNewSSHSuccess(t *testing.T) {
	ts := startRejectSSHServer(t, false, false, false)
	h := NewHandler()
	msgs := make(chan *protocol.Message, 8)
	h.SetSendFunc(collectSendFunc(msgs))
	h.Start(nil)
	defer h.Stop()

	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId":  "px-ssh",
		"connId":   "conn-ssh",
		"target":   ts.addr(),
		"username": "u",
		"password": "p",
		"terminal": true,
		"protocol": string(protocol.RemoteProtocolSSH),
	})
	if err := h.HandleProxyNew(msg); err != nil {
		t.Fatalf("HandleProxyNew ssh: %v", err)
	}
	// 会话已登记
	h.mu.Lock()
	_, ok := h.conns["conn-ssh"]
	h.mu.Unlock()
	if !ok {
		t.Fatal("ssh proxy conn not registered")
	}
}

func TestHandleProxyNewSSHSessionFail(t *testing.T) {
	h := NewHandler()
	msgs := make(chan *protocol.Message, 8)
	h.SetSendFunc(collectSendFunc(msgs))
	h.Start(nil)
	defer h.Stop()

	// 不可达目标 → NewSSHSession 失败 → SendError
	msg := protocol.NewMessage(protocol.MessageTypeProxyNew, map[string]interface{}{
		"proxyId":  "px-ssh-fail",
		"connId":   "conn-ssh-fail",
		"target":   "127.0.0.1:1",
		"username": "u",
		"password": "p",
		"terminal": true,
		"protocol": string(protocol.RemoteProtocolSSH),
	})
	if err := h.HandleProxyNew(msg); err == nil {
		t.Fatal("HandleProxyNew ssh unreachable should fail")
	}
	errMsg := waitForMessage(t, msgs, protocol.MessageTypeProxyError)
	if errMsg.Payload["connId"] != "conn-ssh-fail" {
		t.Errorf("proxy error connId = %v", errMsg.Payload["connId"])
	}
}


// TestNewSSHSessionStdinPipeFail 覆盖 StdinPipe 失败分支（防御性错误处理）。
// x/crypto/ssh 的 StdinPipe 失败条件是 s.started（仅 Shell→start() 设）或
// s.Stdin 已设，NewSSHSession 调用序列 NewSession→RequestPty→StdinPipe→Shell
// 下二者皆不可能——经 stdinPipeFn 注入点触发（与 rdpClientAvailable var 化
// 同性质：行为中性的测试注入点，生产恒调 s.StdinPipe()）。
func TestNewSSHSessionStdinPipeFail(t *testing.T) {
	old := stdinPipeFn
	stdinPipeFn = func(s *ssh.Session) (io.WriteCloser, error) {
		return nil, errors.New("injected stdin pipe failure")
	}
	defer func() { stdinPipeFn = old }()

	// NewSession/RequestPty 成功后走到 StdinPipe
	ts := startRejectSSHServer(t, false, false, false)
	_, err := NewSSHSession(ts.addr(), "u", "p", "", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "stdin pipe") {
		t.Errorf("err = %v, want stdin pipe failure", err)
	}
}
