package detector

// remote_auth_test.go 覆盖 probeSSHAuthMethods 的两条难达分支：
// 连接被拒（无认证格式可解析 → 错误返回）与免认证服务器（Dial 直接成功）。

import (
	"crypto/ed25519"
	"crypto/rand"
	"net"
	"testing"

	"golang.org/x/crypto/ssh"
)

// TestProbeSSHAuthMethodsUnreachable 覆盖错误返回分支：目标端口关闭，
// 错误消息里没有 "attempted methods [...]" 可解析
func TestProbeSSHAuthMethodsUnreachable(t *testing.T) {
	// 监听后立即关闭，拿一个必然连接拒绝的本地端口
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()

	methods, err := probeSSHAuthMethods("127.0.0.1", port)
	if err == nil {
		t.Fatalf("probeSSHAuthMethods() = %v, want error for unreachable host", methods)
	}
	if len(methods) != 0 {
		t.Errorf("methods = %v, want nil on error", methods)
	}
}

// TestProbeSSHAuthMethodsNoAuthServer 覆盖成功分支：NoClientAuth 服务器
// 接受无凭据连接，Dial 成功 → 返回默认方法列表
func TestProbeSSHAuthMethodsNoAuthServer(t *testing.T) {
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
	defer ln.Close()
	go func() {
		config := &ssh.ServerConfig{NoClientAuth: true}
		config.AddHostKey(signer)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				_, _, _, _ = ssh.NewServerConn(conn, config)
			}(c)
		}
	}()

	port := ln.Addr().(*net.TCPAddr).Port
	methods, err := probeSSHAuthMethods("127.0.0.1", port)
	if err != nil {
		t.Fatalf("probeSSHAuthMethods() error = %v, want nil for no-auth server", err)
	}
	if len(methods) == 0 {
		t.Error("methods is empty, want non-empty default list")
	}
}
