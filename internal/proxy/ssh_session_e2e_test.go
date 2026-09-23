package proxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshTestServer 内存 SSH 服务端：接受口令认证，回显 PTY shell。
type sshTestServer struct {
	ln       net.Listener
	hostKey  ssh.Signer
	gotPty   *ptyRequest
	gotWin   []windowChangeRequest
	gotInput []string
	quit     chan struct{}
}

type ptyRequest struct {
	term string
	rows int
	cols int
}

type windowChangeRequest struct {
	rows int
	cols int
}

func startSSHTestServer(t *testing.T) *sshTestServer {
	t.Helper()
	ResetTOFUHostKeys()

	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	_ = pub

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	ts := &sshTestServer{ln: ln, hostKey: signer, quit: make(chan struct{})}
	go ts.serve()
	t.Cleanup(func() {
		close(ts.quit)
		_ = ln.Close()
	})
	return ts
}

func (ts *sshTestServer) addr() string { return ts.ln.Addr().String() }

func (ts *sshTestServer) serve() {
	config := &ssh.ServerConfig{
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() != "testuser" || string(pass) != "testpass" {
				return nil, ssh.ErrNoAuth // 显式拒绝
			}
			return nil, nil
		},
	}
	config.AddHostKey(ts.hostKey)

	for {
		conn, err := ts.ln.Accept()
		if err != nil {
			return
		}
		go ts.handleConn(conn, config)
	}
}

func (ts *sshTestServer) handleConn(nconn net.Conn, config *ssh.ServerConfig) {
	defer nconn.Close()
	sconn, chans, reqs, err := ssh.NewServerConn(nconn, config)
	if err != nil {
		return
	}
	defer sconn.Close()
	go ssh.DiscardRequests(reqs)

	for newChan := range chans {
		if newChan.ChannelType() != "session" {
			_ = newChan.Reject(ssh.UnknownChannelType, "only session")
			continue
		}
		ch, chReqs, err := newChan.Accept()
		if err != nil {
			return
		}
		go ts.handleSession(ch, chReqs)
	}
}

func (ts *sshTestServer) handleSession(ch ssh.Channel, reqs <-chan *ssh.Request) {
	defer ch.Close()
	shellStarted := false

	for req := range reqs {
		switch req.Type {
		case "pty-req":
			var payload struct {
				Term    string
				Columns uint32
				Rows    uint32
			}
			_ = ssh.Unmarshal(req.Payload, &payload)
			ts.gotPty = &ptyRequest{term: payload.Term, rows: int(payload.Rows), cols: int(payload.Columns)}
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		case "window-change":
			var payload struct {
				Columns uint32
				Rows    uint32
			}
			_ = ssh.Unmarshal(req.Payload, &payload)
			ts.gotWin = append(ts.gotWin, windowChangeRequest{rows: int(payload.Rows), cols: int(payload.Columns)})
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
		case "shell", "exec":
			if req.WantReply {
				_ = req.Reply(true, nil)
			}
			shellStarted = true
			go func() {
				buf := make([]byte, 4096)
				for {
					n, err := ch.Read(buf)
					if n > 0 {
						ts.gotInput = append(ts.gotInput, string(buf[:n]))
						// 回显，模拟 shell
						_, _ = ch.Write(buf[:n])
					}
					if err != nil {
						return
					}
				}
			}()
		default:
			if req.WantReply {
				_ = req.Reply(false, nil)
			}
		}
		if shellStarted {
			// shell 启动后继续处理 window-change 等
			continue
		}
	}
}

// TestNewSSHSessionEndToEnd 内存 SSH 服务端全链路：认证 → PTY → Shell → 输入回显 → WindowChange → Close
func TestNewSSHSessionEndToEnd(t *testing.T) {
	ts := startSSHTestServer(t)

	sess, err := NewSSHSession(ts.addr(), "testuser", "testpass", "", 30, 100)
	if err != nil {
		t.Fatalf("NewSSHSession: %v", err)
	}
	defer sess.Close()

	// PTY 尺寸已上报
	deadline := time.Now().Add(3 * time.Second)
	for ts.gotPty == nil && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if ts.gotPty == nil {
		t.Fatal("server did not receive pty-req")
	}
	if ts.gotPty.rows != 30 || ts.gotPty.cols != 100 {
		t.Errorf("pty size = %dx%d, want 100x30", ts.gotPty.cols, ts.gotPty.rows)
	}
	if ts.gotPty.term != "xterm-256color" {
		t.Errorf("term = %q, want xterm-256color", ts.gotPty.term)
	}

	// 输入经 Stream 写入 → 服务端回显 → Stream 读回
	stream := sess.Stream()
	if _, err := stream.Write([]byte("echo hi\n")); err != nil {
		t.Fatalf("write: %v", err)
	}

	buf := make([]byte, 64)
	// io.Pipe 无 deadline，直接读（服务端回显后可达）
	done := make(chan string, 1)
	go func() {
		n, err := stream.Read(buf)
		if err != nil {
			done <- ""
			return
		}
		done <- string(buf[:n])
	}()
	var got string
	select {
	case got = <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout reading echo")
	}
	if !strings.Contains(got, "echo hi") {
		t.Errorf("echo = %q, want contain %q", got, "echo hi")
	}

	// WindowChange
	if err := sess.WindowChange(50, 200); err != nil {
		t.Fatalf("WindowChange: %v", err)
	}
	deadline = time.Now().Add(3 * time.Second)
	for len(ts.gotWin) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(ts.gotWin) == 0 {
		t.Fatal("server did not receive window-change")
	}
	if ts.gotWin[0].rows != 50 || ts.gotWin[0].cols != 200 {
		t.Errorf("window change = %dx%d, want 200x50", ts.gotWin[0].cols, ts.gotWin[0].rows)
	}

	// Close 幂等
	if err := sess.Close(); err != nil {
		t.Errorf("Close: %v", err)
	}
	if err := sess.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

// TestNewSSHSessionBadPassword 认证失败路径
func TestNewSSHSessionBadPassword(t *testing.T) {
	ts := startSSHTestServer(t)
	_, err := NewSSHSession(ts.addr(), "testuser", "wrong", "", 24, 80)
	if err == nil {
		t.Fatal("bad password should fail")
	}
	if !strings.Contains(err.Error(), "SSH connect") {
		t.Errorf("err = %v, want SSH connect failure", err)
	}
}

// TestNewSSHSessionHostKeyChangeRejected TOFU 变更拒绝
func TestNewSSHSessionHostKeyChangeRejected(t *testing.T) {
	ts := startSSHTestServer(t)

	// 首连登记
	sess, err := NewSSHSession(ts.addr(), "testuser", "testpass", "", 24, 80)
	if err != nil {
		t.Fatalf("first connect: %v", err)
	}
	_ = sess.Close()

	// 换 host key 重启服务 → 指纹变更拒绝
	_ = ts.ln.Close()
	pub2, priv2, err := ed25519.GenerateKey(rand.Reader)
	_ = pub2
	if err != nil {
		t.Fatal(err)
	}
	signer2, err := ssh.NewSignerFromKey(priv2)
	if err != nil {
		t.Fatal(err)
	}
	ln2, err := net.Listen("tcp", ts.addr())
	if err != nil {
		// 端口被占则跳过（已 Close，理论可复用）
		t.Skipf("cannot rebind %s: %v", ts.addr(), err)
	}
	defer ln2.Close()
	go func() {
		config := &ssh.ServerConfig{
			PasswordCallback: func(ssh.ConnMetadata, []byte) (*ssh.Permissions, error) { return nil, nil },
		}
		config.AddHostKey(signer2)
		for {
			c, err := ln2.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _, _, _ = ssh.NewServerConn(conn, config)
			}(c)
		}
	}()

	_, err = NewSSHSession(ts.addr(), "testuser", "testpass", "", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "has changed") {
		t.Errorf("err = %v, want host key change rejection", err)
	}
}
