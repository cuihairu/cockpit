package proxy

import (
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/crypto/ssh"
)

// sshStream 把 ssh.Session 的 stdin/stdout 适配为 io.ReadWriteCloser，
// 使 SSH 会话与裸 TCP 连接共用 AgentTargetConn 的转发管道。
// stderr 合并进 stdout（PTY 分配后 shell 本就混流）。
type sshStream struct {
	stdin  io.WriteCloser
	stdout io.Reader
	closer io.Closer
}

func (s *sshStream) Read(p []byte) (int, error)  { return s.stdout.Read(p) }
func (s *sshStream) Write(p []byte) (int, error) { return s.stdin.Write(p) }

func (s *sshStream) Close() error {
	// stdin 关闭让远端 shell 感知 EOF；closer 关闭整个 SSH 会话/连接
	_ = s.stdin.Close()
	return s.closer.Close()
}

// SSHSession 单条 SSH 终端会话：Dial → 认证 → PTY → Shell。
type SSHSession struct {
	client *ssh.Client
	sess   *ssh.Session
	stream *sshStream
	rows   int
	cols   int
	mu     sync.Mutex
	closed bool
}

// tofuHostKeys TOFU（trust on first use）host key 登记表：target -> SHA256 指纹。
// 首连记录指纹，后续连接指纹不一致即拒绝并提示（防中间人）。agent 进程内存态，
// 重启后重新登记；持久化 known_hosts 留后续。
var (
	tofuHostKeysMu sync.RWMutex
	tofuHostKeys   = map[string]string{}
)

// tofuHostKeyCallback 首次连接记录 host key 指纹，变更时拒绝。
func tofuHostKeyCallback(hostname string, _ net.Addr, key ssh.PublicKey) error {
	fp := ssh.FingerprintSHA256(key)
	tofuHostKeysMu.Lock()
	defer tofuHostKeysMu.Unlock()
	if known, ok := tofuHostKeys[hostname]; ok {
		if known != fp {
			return fmt.Errorf("SSH host key for %s has changed (was %s, now %s); possible man-in-the-middle",
				hostname, known, fp)
		}
		return nil
	}
	tofuHostKeys[hostname] = fp
	return nil
}

// ResetTOFUHostKeys 清空 TOFU 登记表（测试用）。
func ResetTOFUHostKeys() {
	tofuHostKeysMu.Lock()
	tofuHostKeys = map[string]string{}
	tofuHostKeysMu.Unlock()
}

// stdinPipeFn StdinPipe 调用点。var 化（非内联）仅为测试注入失败分支——
// x/crypto/ssh 的 StdinPipe 失败条件（s.started / Stdin 已设）在
// NewSSHSession 调用序列下不可能成立，防御性错误处理无法经黑盒触发；
// 生产行为不变（恒调 s.StdinPipe()）。
var stdinPipeFn = func(s *ssh.Session) (io.WriteCloser, error) {
	return s.StdinPipe()
}

// NewSSHSession 建立 SSH 终端会话。
// target 形如 "host:22"；认证：PrivateKey（PEM）优先，其次 Password；
// rows/cols 为 PTY 初始尺寸（<=0 取 24x80）。
func NewSSHSession(target, username, password, privateKey string, rows, cols int) (*SSHSession, error) {
	var auths []ssh.AuthMethod
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return nil, fmt.Errorf("parse private key: %w", err)
		}
		auths = append(auths, ssh.PublicKeys(signer))
	}
	if password != "" {
		auths = append(auths, ssh.Password(password))
	}
	if len(auths) == 0 {
		return nil, fmt.Errorf("SSH requires password or private key")
	}
	if username == "" {
		return nil, fmt.Errorf("SSH username is required")
	}

	if rows <= 0 {
		rows = 24
	}
	if cols <= 0 {
		cols = 80
	}

	config := &ssh.ClientConfig{
		User:            username,
		Auth:            auths,
		HostKeyCallback: tofuHostKeyCallback,
		Timeout:         10 * time.Second,
	}

	client, err := ssh.Dial("tcp", target, config)
	if err != nil {
		return nil, fmt.Errorf("SSH connect to %s failed: %w", target, err)
	}

	sess, err := client.NewSession()
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("SSH new session failed: %w", err)
	}

	// PTY 分配：远端 shell 才会出提示符/行编辑/全屏程序（vim、top）
	modes := ssh.TerminalModes{
		ssh.ECHO:          1,
		ssh.TTY_OP_ISPEED: 14400,
		ssh.TTY_OP_OSPEED: 14400,
	}
	if err := sess.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("SSH request PTY failed: %w", err)
	}

	stdin, err := stdinPipeFn(sess)
	if err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("SSH stdin pipe failed: %w", err)
	}

	// stdout/stderr 合并到同一个 pipe：PTY 模式下远端本就混流，
	// io.Pipe 的 Write 内部有锁，多 goroutine 写安全。
	outR, outW := io.Pipe()
	sess.Stdout = outW
	sess.Stderr = outW

	if err := sess.Shell(); err != nil {
		_ = sess.Close()
		_ = client.Close()
		return nil, fmt.Errorf("SSH start shell failed: %w", err)
	}

	// Wait 阻塞到会话结束；结束后关 outW 让 readFromTarget 读到 EOF
	go func() {
		_ = sess.Wait()
		_ = outW.Close()
	}()

	return &SSHSession{
		client: client,
		sess:   sess,
		stream: &sshStream{stdin: stdin, stdout: outR, closer: client},
		rows:   rows,
		cols:   cols,
	}, nil
}

// Stream 适配为 io.ReadWriteCloser 的双向流（接 proxy 转发管道）。
func (s *SSHSession) Stream() io.ReadWriteCloser { return s.stream }

// WindowChange 调整 PTY 尺寸（浏览器窗口变化时触发）。
func (s *SSHSession) WindowChange(rows, cols int) error {
	if rows <= 0 || cols <= 0 {
		return fmt.Errorf("invalid terminal size %dx%d", cols, rows)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.sess == nil {
		return fmt.Errorf("SSH session closed")
	}
	s.rows, s.cols = rows, cols
	return s.sess.WindowChange(rows, cols)
}

// Close 关闭会话与底层连接（幂等；nil 字段安全）。
func (s *SSHSession) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	sess, client := s.sess, s.client
	s.mu.Unlock()

	var err error
	if sess != nil {
		err = sess.Close()
	}
	if client != nil {
		if cerr := client.Close(); err == nil {
			err = cerr
		}
	}
	return err
}
