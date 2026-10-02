package proxy

// ssh_keys_test.go 覆盖 ssh_keys.go 的全部分支：EnsureSSHKeys 的已有密钥
// 直返/目录创建失败/生成与写入各错误路径（generateEd25519Key 与
// marshalPrivateKey 注入点，见源文件注释）/defaultSSHDir 三分支/
// LoadDefaultSSHKeys 的跳过解析失败/LoadSSHKeyFromFile 三态/
// HasPrivateKeyFiles 的 .pub 配对/ListSSHKeyNames 的过滤规则。

import (
	"crypto"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

// genKeyPEM 生成一把可用作 SSH 私钥文件的 ed25519 PEM
func genKeyPEM(t *testing.T) []byte {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	block, err := ssh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(block)
}

// ============ EnsureSSHKeys ============

// TestEnsureSSHKeysExistingKey 覆盖：keyDir 为空走 defaultSSHDir（环境变量
// 注入），且目录里已有候选密钥 → 直返不生成
func TestEnsureSSHKeysExistingKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "id_rsa"), genKeyPEM(t), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("COCKPIT_SSH_KEYS", dir)

	path, err := EnsureSSHKeys("")
	if err != nil {
		t.Fatalf("EnsureSSHKeys: %v", err)
	}
	if path != filepath.Join(dir, "id_rsa") {
		t.Errorf("path = %q, want existing id_rsa", path)
	}
	// 未生成新密钥：id_ed25519 不应出现
	if _, err := os.Stat(filepath.Join(dir, "id_ed25519")); !os.IsNotExist(err) {
		t.Error("existing key path must not trigger key generation")
	}
}

// TestEnsureSSHKeysMkdirAllFail 覆盖目录创建失败：/dev/null 下建目录必败
// （对 root 同样失败——/dev/null 不是目录）
func TestEnsureSSHKeysMkdirAllFail(t *testing.T) {
	if _, err := EnsureSSHKeys("/dev/null/no-such-keydir"); err == nil ||
		!strings.Contains(err.Error(), "create ssh dir") {
		t.Errorf("EnsureSSHKeys under /dev/null = %v, want create ssh dir error", err)
	}
}

// TestEnsureSSHKeysGenerateFail 覆盖密钥生成错误分支（注入点）
func TestEnsureSSHKeysGenerateFail(t *testing.T) {
	orig := generateEd25519Key
	generateEd25519Key = func(io.Reader) (ed25519.PublicKey, ed25519.PrivateKey, error) {
		return nil, nil, errors.New("boom gen")
	}

	t.Cleanup(func() { generateEd25519Key = orig })

	if _, err := EnsureSSHKeys(t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "generate ed25519 key") {
		t.Errorf("EnsureSSHKeys with failing generator = %v, want generate error", err)
	}
}

// TestEnsureSSHKeysMarshalFail 覆盖 PEM 序列化错误分支（注入点）
func TestEnsureSSHKeysMarshalFail(t *testing.T) {
	orig := marshalPrivateKey
	marshalPrivateKey = func(crypto.PrivateKey, string) (*pem.Block, error) {
		return nil, errors.New("boom marshal")
	}
	t.Cleanup(func() { marshalPrivateKey = orig })

	if _, err := EnsureSSHKeys(t.TempDir()); err == nil ||
		!strings.Contains(err.Error(), "marshal private key") {
		t.Errorf("EnsureSSHKeys with failing marshal = %v, want marshal error", err)
	}
}

// TestEnsureSSHKeysWritePrivateFail 私钥路径预置悬空符号链接 →
// Stat 跟随链接失败（不当作已有密钥），WriteFile 打开 /proc 内目标必败
// （procfs 不可写不可建，对 root 同样失败）
func TestEnsureSSHKeysWritePrivateFail(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/proc/cockpit-no-such-entry", filepath.Join(dir, "id_ed25519")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureSSHKeys(dir); err == nil ||
		!strings.Contains(err.Error(), "write private key") {
		t.Errorf("EnsureSSHKeys with dangling symlink at key path = %v, want write private key error", err)
	}
}

// TestEnsureSSHKeysWritePublicFail 私钥写入成功、公钥路径预置悬空符号链接 →
// WriteFile(.pub) 必败
func TestEnsureSSHKeysWritePublicFail(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink("/proc/cockpit-no-such-entry", filepath.Join(dir, "id_ed25519.pub")); err != nil {
		t.Fatal(err)
	}
	if _, err := EnsureSSHKeys(dir); err == nil ||
		!strings.Contains(err.Error(), "write public key") {
		t.Errorf("EnsureSSHKeys with dangling symlink at pub path = %v, want write public key error", err)
	}
}

// TestEnsureSSHKeysAutoGenerate 正常自动生成：私钥 + 公钥成对落盘
func TestEnsureSSHKeysAutoGenerate(t *testing.T) {
	dir := t.TempDir()
	path, err := EnsureSSHKeys(dir)
	if err != nil {
		t.Fatalf("EnsureSSHKeys: %v", err)
	}
	if path != filepath.Join(dir, "id_ed25519") {
		t.Errorf("path = %q, want id_ed25519", path)
	}
	priv, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(priv), "PRIVATE KEY") {
		t.Errorf("private key content = %q, want PEM", priv)
	}
	pub, err := os.ReadFile(path + ".pub")
	if err != nil {
		t.Fatalf("public key not written: %v", err)
	}
	if !strings.HasPrefix(string(pub), "ssh-ed25519 ") {
		t.Errorf("public key content = %q, want authorized_keys line", pub)
	}
}

// ============ defaultSSHDir ============

// TestDefaultSSHDirBranches 三分支：COCKPIT_SSH_KEYS / SUDO_USER+HOME /
// 兜底 UserHomeDir
func TestDefaultSSHDirBranches(t *testing.T) {
	t.Setenv("COCKPIT_SSH_KEYS", "/custom/keys")
	if got := defaultSSHDir(); got != "/custom/keys" {
		t.Errorf("env dir = %q, want /custom/keys", got)
	}

	t.Setenv("COCKPIT_SSH_KEYS", "")
	t.Setenv("SUDO_USER", "alice")
	// sudo 场景 HOME 已切到目标用户（如 /root），推导取两级父目录拼
	// SUDO_USER 的 .ssh（ssh_keys.go 现行语义，与 rpc/ssh_provider.go 同款）
	t.Setenv("HOME", "/root")
	if got := defaultSSHDir(); got != "/alice/.ssh" {
		t.Errorf("sudo dir = %q, want /alice/.ssh", got)
	}

	t.Setenv("SUDO_USER", "")
	home, _ := os.UserHomeDir()
	if got := defaultSSHDir(); got != filepath.Join(home, ".ssh") {
		t.Errorf("fallback dir = %q, want %q", got, filepath.Join(home, ".ssh"))
	}
}

// ============ LoadDefaultSSHKeys ============

// TestLoadDefaultSSHKeysSkipsInvalid 覆盖：最高优先候选是垃圾内容 →
// ParsePrivateKey 失败跳过；次候选合法 → 命中
func TestLoadDefaultSSHKeysSkipsInvalid(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), []byte("garbage"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "id_rsa"), genKeyPEM(t), 0600); err != nil {
		t.Fatal(err)
	}

	methods, err := LoadDefaultSSHKeys(dir)
	if err != nil {
		t.Fatalf("LoadDefaultSSHKeys: %v", err)
	}
	if len(methods) != 1 {
		t.Fatalf("methods = %d, want 1 (invalid id_ed25519 skipped)", len(methods))
	}
}

// TestLoadDefaultSSHKeysEmptyDirFromEnv 覆盖 keyDir 为空走 defaultSSHDir，
// 目录无任何密钥 → 空列表
func TestLoadDefaultSSHKeysEmptyDirFromEnv(t *testing.T) {
	t.Setenv("COCKPIT_SSH_KEYS", t.TempDir())
	t.Setenv("SUDO_USER", "")

	methods, err := LoadDefaultSSHKeys("")
	if err != nil {
		t.Fatalf("LoadDefaultSSHKeys: %v", err)
	}
	if len(methods) != 0 {
		t.Errorf("methods = %d, want 0 for empty dir", len(methods))
	}
}

// ============ LoadSSHKeyFromFile ============

func TestLoadSSHKeyFromFileBranches(t *testing.T) {
	// 读失败：文件不存在
	if _, err := LoadSSHKeyFromFile(filepath.Join(t.TempDir(), "missing")); err == nil ||
		!strings.Contains(err.Error(), "read key") {
		t.Errorf("LoadSSHKeyFromFile(missing) = %v, want read key error", err)
	}

	// 解析失败：内容不是私钥
	bad := filepath.Join(t.TempDir(), "bad")
	if err := os.WriteFile(bad, []byte("not a key"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSSHKeyFromFile(bad); err == nil ||
		!strings.Contains(err.Error(), "parse key") {
		t.Errorf("LoadSSHKeyFromFile(bad) = %v, want parse key error", err)
	}

	// 成功
	good := filepath.Join(t.TempDir(), "good")
	if err := os.WriteFile(good, genKeyPEM(t), 0600); err != nil {
		t.Fatal(err)
	}
	if m, err := LoadSSHKeyFromFile(good); err != nil || m == nil {
		t.Errorf("LoadSSHKeyFromFile(good) = %v, %v, want method", m, err)
	}
}

// ============ HasPrivateKeyFiles ============

func TestHasPrivateKeyFilesBranches(t *testing.T) {
	// keyDir 为空走 defaultSSHDir（环境变量指向空目录）→ false
	t.Setenv("COCKPIT_SSH_KEYS", t.TempDir())
	t.Setenv("SUDO_USER", "")
	if HasPrivateKeyFiles("") {
		t.Error("empty dir should report no keys")
	}

	dir := t.TempDir()
	// 私钥在但 .pub 缺失 → 不算密钥对
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), genKeyPEM(t), 0600); err != nil {
		t.Fatal(err)
	}
	if HasPrivateKeyFiles(dir) {
		t.Error("private key without .pub must not count")
	}
	// 补上 .pub → true
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519.pub"), []byte("k"), 0644); err != nil {
		t.Fatal(err)
	}
	if !HasPrivateKeyFiles(dir) {
		t.Error("key pair should be detected")
	}
}

// ============ ListSSHKeyNames ============

func TestListSSHKeyNamesFiltering(t *testing.T) {
	// 目录不存在 → nil
	if got := ListSSHKeyNames(filepath.Join(t.TempDir(), "missing")); got != nil {
		t.Errorf("ListSSHKeyNames(missing) = %v, want nil", got)
	}

	// keyDir 为空走 defaultSSHDir（环境变量指向下面构造的目录）
	dir := t.TempDir()
	keyPEM := genKeyPEM(t)
	for _, name := range []string{"id_rsa", "id_dsa"} {
		if err := os.WriteFile(filepath.Join(dir, name), keyPEM, 0600); err != nil {
			t.Fatal(err)
		}
	}
	// 干扰项：.pub 文件、隐藏文件、同名子目录、无关文件
	os.WriteFile(filepath.Join(dir, "id_ecdsa.pub"), []byte("k"), 0644)
	os.WriteFile(filepath.Join(dir, ".id_ed25519"), keyPEM, 0600)
	os.Mkdir(filepath.Join(dir, "id_ecdsa"), 0700)
	os.WriteFile(filepath.Join(dir, "readme"), []byte("x"), 0644)
	t.Setenv("COCKPIT_SSH_KEYS", dir)

	got := ListSSHKeyNames("")
	if len(got) != 2 {
		t.Fatalf("ListSSHKeyNames = %v, want exactly [id_rsa id_dsa]", got)
	}
	seen := map[string]bool{}
	for _, name := range got {
		seen[name] = true
	}
	if !seen["id_rsa"] || !seen["id_dsa"] {
		t.Errorf("ListSSHKeyNames = %v, want id_rsa and id_dsa", got)
	}
}

// ============ ssh_session.go / handler.go 的孤立缺口 ============

// TestSSHSessionFallsBackToDefaultKeys 覆盖 NewSSHSession 的「显式凭据不足
// 时加载默认密钥」分支：keyDir 有合法密钥 → auths 非空（不再是
// "requires password or private key"），随后对拒绝端口的拨号失败返回
func TestSSHSessionFallsBackToDefaultKeys(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "id_ed25519"), genKeyPEM(t), 0600); err != nil {
		t.Fatal(err)
	}

	// 127.0.0.1:1 连接必拒：密钥加载分支已过，走到拨号即返回
	_, err := NewSSHSession("127.0.0.1:1", "covuser", "", "", dir, 24, 80)
	if err == nil {
		t.Fatal("NewSSHSession to refused port should fail")
	}
	if strings.Contains(err.Error(), "requires password or private key") {
		t.Errorf("default keys not loaded: %v", err)
	}
}

// TestProxyHandlerSetKeyDir 覆盖 SetKeyDir 赋值
func TestProxyHandlerSetKeyDir(t *testing.T) {
	h := NewHandler()
	h.SetKeyDir("/custom/key/dir")
	if h.keyDir != "/custom/key/dir" {
		t.Errorf("keyDir = %q, want /custom/key/dir", h.keyDir)
	}
}
