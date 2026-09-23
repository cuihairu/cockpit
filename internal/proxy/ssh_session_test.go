package proxy

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// ============ TOFU host key 登记 ============

func makeTestKey(t *testing.T) ssh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return sshPub
}

func TestTOFUHostKeyFirstUseRecords(t *testing.T) {
	ResetTOFUHostKeys()
	defer ResetTOFUHostKeys()

	key := makeTestKey(t)
	if err := tofuHostKeyCallback("h1:22", nil, key); err != nil {
		t.Fatalf("first use should trust: %v", err)
	}
	fp := ssh.FingerprintSHA256(key)
	tofuHostKeysMu.RLock()
	got := tofuHostKeys["h1:22"]
	tofuHostKeysMu.RUnlock()
	if got != fp {
		t.Errorf("recorded fingerprint = %q, want %q", got, fp)
	}
}

func TestTOFUHostKeySameKeyAccepted(t *testing.T) {
	ResetTOFUHostKeys()
	defer ResetTOFUHostKeys()

	key := makeTestKey(t)
	if err := tofuHostKeyCallback("h2:22", nil, key); err != nil {
		t.Fatal(err)
	}
	if err := tofuHostKeyCallback("h2:22", nil, key); err != nil {
		t.Errorf("same key should be accepted: %v", err)
	}
}

func TestTOFUHostKeyChangeRejected(t *testing.T) {
	ResetTOFUHostKeys()
	defer ResetTOFUHostKeys()

	key1 := makeTestKey(t)
	key2 := makeTestKey(t)
	if err := tofuHostKeyCallback("h3:22", nil, key1); err != nil {
		t.Fatal(err)
	}
	err := tofuHostKeyCallback("h3:22", nil, key2)
	if err == nil {
		t.Fatal("changed host key should be rejected")
	}
	if !strings.Contains(err.Error(), "has changed") {
		t.Errorf("error = %q, want mention of change", err)
	}
}

func TestTOFUHostKeysIsolatedPerHost(t *testing.T) {
	ResetTOFUHostKeys()
	defer ResetTOFUHostKeys()

	key := makeTestKey(t)
	if err := tofuHostKeyCallback("a:22", nil, key); err != nil {
		t.Fatal(err)
	}
	// 同 key 不同 host：各记各的，互不影响
	if err := tofuHostKeyCallback("b:22", nil, key); err != nil {
		t.Errorf("same key on different host should be accepted: %v", err)
	}
}

// ============ NewSSHSession 参数校验（不发起真连接） ============

func TestNewSSHSessionRejectsNoAuth(t *testing.T) {
	_, err := NewSSHSession("127.0.0.1:22", "user", "", "", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "password or private key") {
		t.Errorf("err = %v, want require password or private key", err)
	}
}

func TestNewSSHSessionRejectsNoUsername(t *testing.T) {
	_, err := NewSSHSession("127.0.0.1:22", "", "pw", "", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "username is required") {
		t.Errorf("err = %v, want require username", err)
	}
}

func TestNewSSHSessionRejectsBadPrivateKey(t *testing.T) {
	_, err := NewSSHSession("127.0.0.1:22", "user", "", "not-a-pem", 24, 80)
	if err == nil || !strings.Contains(err.Error(), "parse private key") {
		t.Errorf("err = %v, want parse private key failure", err)
	}
}

func TestNewSSHSessionDefaultsSize(t *testing.T) {
	// rows/cols <=0 取 24x80；因目标不可达会失败，但先过参数分支
	_, err := NewSSHSession("127.0.0.1:1", "user", "pw", "", 0, -5)
	if err == nil {
		t.Fatal("unreachable target should fail")
	}
	// 不应因尺寸非法而报错（应走到连接失败）
	if strings.Contains(err.Error(), "invalid terminal size") {
		t.Errorf("size should be defaulted, got %v", err)
	}
}

// ============ SSHSession.WindowChange 边界 ============

func TestSSHSessionWindowChangeClosed(t *testing.T) {
	s := &SSHSession{closed: true}
	if err := s.WindowChange(30, 100); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("err = %v, want closed", err)
	}
}

func TestSSHSessionWindowChangeInvalidSize(t *testing.T) {
	s := &SSHSession{}
	if err := s.WindowChange(0, 80); err == nil || !strings.Contains(err.Error(), "invalid terminal size") {
		t.Errorf("err = %v, want invalid size", err)
	}
	if err := s.WindowChange(24, -1); err == nil {
		t.Error("negative cols should be rejected")
	}
}

func TestSSHSessionCloseIdempotent(t *testing.T) {
	// closed 标记后重复 Close 不 panic
	s := &SSHSession{closed: true}
	if err := s.Close(); err != nil {
		t.Errorf("Close() = %v, want nil", err)
	}
	if err := s.Close(); err != nil {
		t.Errorf("second Close() = %v, want nil", err)
	}
}

// ============ sshStream 适配 ============

type fakeRWC struct {
	reads  [][]byte
	writes [][]byte
	closed int
}

func (f *fakeRWC) Read(p []byte) (int, error) {
	if len(f.reads) == 0 {
		return 0, net.ErrClosed
	}
	n := copy(p, f.reads[0])
	f.reads = f.reads[1:]
	return n, nil
}

func (f *fakeRWC) Write(p []byte) (int, error) {
	f.writes = append(f.writes, append([]byte(nil), p...))
	return len(p), nil
}

func (f *fakeRWC) Close() error {
	f.closed++
	return nil
}

func TestSSHStreamReadWriteClose(t *testing.T) {
	in := &fakeRWC{writes: nil}
	out := &fakeRWC{reads: [][]byte{[]byte("hello")}}
	closer := &fakeRWC{}
	s := &sshStream{stdin: in, stdout: out, closer: closer}

	buf := make([]byte, 8)
	n, err := s.Read(buf)
	if err != nil || string(buf[:n]) != "hello" {
		t.Errorf("Read = %q, %v", buf[:n], err)
	}

	if _, err := s.Write([]byte("ls\n")); err != nil {
		t.Errorf("Write error = %v", err)
	}
	if len(in.writes) != 1 || string(in.writes[0]) != "ls\n" {
		t.Errorf("stdin writes = %q", in.writes)
	}

	if err := s.Close(); err != nil {
		t.Errorf("Close error = %v", err)
	}
	if in.closed != 1 {
		t.Errorf("stdin closed = %d, want 1", in.closed)
	}
	if closer.closed != 1 {
		t.Errorf("closer closed = %d, want 1", closer.closed)
	}
}
