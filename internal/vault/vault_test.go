package vault

import (
	"bytes"
	"crypto/cipher"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// testKey 固定测试密钥（≥32 字符、非默认前缀），各测试开头 ApplyKey 保证
// 与进程环境解耦、顺序无关。
const testKey = "vault-test-key-0123456789abcdef-XYZ!"

func applyTestKey(t *testing.T) {
	t.Helper()
	if ApplyKey(testKey) {
		t.Fatal("test key flagged weak")
	}
}

func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "vault.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	})
	return db
}

func testProvider(t *testing.T) (*LocalProvider, *gorm.DB) {
	t.Helper()
	applyTestKey(t)
	db := openTestDB(t)
	p, err := NewLocal(db, 0, nil)
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	return p, db
}

func TestDeriveKey(t *testing.T) {
	// sha256("abc") 标准向量：KDF 单轮 sha256（V3 口径）
	want, _ := hex.DecodeString("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	got := DeriveKey("abc")
	if !bytes.Equal(got, want) {
		t.Fatalf("DeriveKey mismatch: %x", got)
	}
}

func TestEncryptDecryptRoundtripAndVersions(t *testing.T) {
	applyTestKey(t)
	ct, err := Encrypt("s3cret-value")
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}
	if !strings.HasPrefix(ct, keyVersionV1) {
		t.Fatalf("new ciphertext must carry version prefix, got %q", ct[:6])
	}
	plain, err := Decrypt(ct)
	if err != nil || plain != "s3cret-value" {
		t.Fatalf("roundtrip: %v %q", err, plain)
	}
	// v0 存量格式（无前缀，storage/crypto.go 历史产物）仍可解——零迁移依赖
	v0, err := encryptWith(encryptionKey, "legacy-row", false)
	if err != nil {
		t.Fatalf("encryptWith v0: %v", err)
	}
	plain, err = Decrypt(v0)
	if err != nil || plain != "legacy-row" {
		t.Fatalf("v0 compat: %v %q", err, plain)
	}
}

func TestDecryptRejectsGarbage(t *testing.T) {
	applyTestKey(t)
	// 非 base64
	if _, err := Decrypt("!!!not-base64!!!"); err == nil || !strings.Contains(err.Error(), "base64") {
		t.Fatalf("want base64 error, got %v", err)
	}
	// base64 但短于 nonce
	short := base64.StdEncoding.EncodeToString([]byte("tiny"))
	if _, err := Decrypt(short); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("want too-short error, got %v", err)
	}
	// 长度足但非密文（auth 失败）
	if _, err := Decrypt(base64.StdEncoding.EncodeToString(make([]byte, 64))); err == nil {
		t.Fatal("want auth-failed error")
	}
}

func TestApplyKeyWeakDetection(t *testing.T) {
	t.Cleanup(func() { ApplyKey(testKey) })
	if !ApplyKey("") {
		t.Error("empty secret must be weak")
	}
	if !ApplyKey("short") {
		t.Error("short secret must be weak")
	}
	if !ApplyKey("change-this-totp-encryption-key-in-prod!") {
		t.Error("default secret must be weak")
	}
	if ApplyKey(testKey) {
		t.Error("strong secret must not be weak")
	}
}

func TestValidateKeyBranches(t *testing.T) {
	t.Cleanup(func() { ApplyKey(testKey) })
	t.Setenv("COCKPIT_VAULT_KEY", "")
	t.Setenv("TOTP_ENCRYPTION_KEY", "")
	if err := ValidateKey(); err == nil || !strings.Contains(err.Error(), "not set") {
		t.Fatalf("want not-set error, got %v", err)
	}
	t.Setenv("TOTP_ENCRYPTION_KEY", "short")
	if err := ValidateKey(); err == nil || !strings.Contains(err.Error(), "32") {
		t.Fatalf("want length error, got %v", err)
	}
	t.Setenv("TOTP_ENCRYPTION_KEY", "change-this-totp-encryption-key-aaaa")
	if err := ValidateKey(); err == nil || !strings.Contains(err.Error(), "default/weak") {
		t.Fatalf("want weak error, got %v", err)
	}
	t.Setenv("TOTP_ENCRYPTION_KEY", testKey)
	if err := ValidateKey(); err != nil {
		t.Fatalf("valid key rejected: %v", err)
	}
	// COCKPIT_VAULT_KEY 优先于回退来源（V5）
	t.Setenv("COCKPIT_VAULT_KEY", testKey)
	t.Setenv("TOTP_ENCRYPTION_KEY", "")
	if err := ValidateKey(); err != nil {
		t.Fatalf("vault key source rejected: %v", err)
	}
}

func TestSessionTTLAndRelock(t *testing.T) {
	s := NewSession(time.Minute)
	base := time.Unix(1700000000, 0)
	now := base
	s.nowFn = func() time.Time { return now }
	if err := s.Check(); !errors.Is(err, ErrLocked) {
		t.Fatalf("fresh session must be locked, got %v", err)
	}
	s.Unlock()
	if err := s.Check(); err != nil {
		t.Fatalf("unlocked session must pass: %v", err)
	}
	now = base.Add(2 * time.Minute)
	if err := s.Check(); !errors.Is(err, ErrLocked) {
		t.Fatalf("expired session must relock, got %v", err)
	}
	if err := s.Check(); !errors.Is(err, ErrLocked) {
		t.Fatalf("relock must persist, got %v", err)
	}
	// 到期后凭解锁刷新时限
	s.Unlock()
	now = base.Add(2*time.Minute + time.Second)
	if err := s.Check(); err != nil {
		t.Fatalf("renew after expiry: %v", err)
	}
	// 进程生命周期会话（ttl=0）不过期
	s0 := NewSession(0)
	s0.nowFn = func() time.Time { return base.Add(100 * time.Hour) }
	s0.Unlock()
	if err := s0.Check(); err != nil {
		t.Fatalf("ttl=0 must not expire: %v", err)
	}
	s0.Lock()
	if err := s0.Check(); !errors.Is(err, ErrLocked) {
		t.Fatalf("explicit lock must hold: %v", err)
	}
}

func TestRegistryNewAndUnknown(t *testing.T) {
	Register("test-factory", func(cfg map[string]string) (Provider, error) {
		if cfg["k"] != "v" {
			t.Error("factory must receive config")
		}
		return &LocalProvider{}, nil
	})
	p, err := New("test-factory", map[string]string{"k": "v"})
	if err != nil || p == nil {
		t.Fatalf("New: %v", err)
	}
	_, err = New("no-such-provider", nil)
	if !errors.Is(err, ErrUnknownProvider) {
		t.Fatalf("want ErrUnknownProvider, got %v", err)
	}
}

func TestLocalWriteReadRoundtrip(t *testing.T) {
	p, db := testProvider(t)
	e := &Entry{
		Path:     "remote/cred-1",
		Kind:     "pair",
		Username: "admin-user",
		Value:    "sup3r-secret-password",
		Meta:     map[string]string{"addr": "192.168.1.10:5001", "type": "dsm"},
	}
	if err := p.Write(e.Path, e); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// 落库形态：明文不出现，新写入带版本前缀
	var row VaultEntry
	if err := db.First(&row, "path = ?", e.Path).Error; err != nil {
		t.Fatalf("fetch row: %v", err)
	}
	if strings.Contains(row.UsernameEnc, "admin-user") || strings.Contains(row.ValueEnc, "sup3r") {
		t.Fatal("plaintext leaked at rest")
	}
	if !strings.HasPrefix(row.ValueEnc, keyVersionV1) {
		t.Fatal("value must be v1-prefixed")
	}
	got, err := p.Read(e.Path)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Path != e.Path || got.Kind != e.Kind || got.Username != e.Username || got.Value != e.Value {
		t.Fatalf("roundtrip mismatch: %+v", got)
	}
	if got.Meta["addr"] != "192.168.1.10:5001" || got.Meta["type"] != "dsm" {
		t.Fatalf("meta mismatch: %v", got.Meta)
	}
}

func TestLocalWriteUpdateBranch(t *testing.T) {
	p, _ := testProvider(t)
	if err := p.Write("nas/a", &Entry{Kind: "pair", Value: "first"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := p.Write("nas/a", &Entry{Kind: "pair", Username: "u", Value: "second"}); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := p.Read("nas/a")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Value != "second" || got.Username != "u" {
		t.Fatalf("update not applied: %+v", got)
	}
}

func TestLocalEmptyFieldsSemantics(t *testing.T) {
	p, db := testProvider(t)
	if err := p.Write("remote/no-user", &Entry{Kind: "password", Value: "only-value"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var row VaultEntry
	if err := db.First(&row, "path = ?", "remote/no-user").Error; err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if row.UsernameEnc != "" {
		t.Fatal("empty username must not be encrypted")
	}
	got, err := p.Read("remote/no-user")
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if got.Username != "" || got.Value != "only-value" {
		t.Fatalf("empty-field semantics broken: %+v", got)
	}
}

func TestLocalWriteValidations(t *testing.T) {
	p, _ := testProvider(t)
	if err := p.Write("", &Entry{Kind: "pair"}); err == nil || !strings.Contains(err.Error(), "path required") {
		t.Fatalf("want path-required error, got %v", err)
	}
	if err := p.Write("x", nil); err == nil || !strings.Contains(err.Error(), "nil entry") {
		t.Fatalf("want nil-entry error, got %v", err)
	}
}

func TestLocalLockedGate(t *testing.T) {
	p, _ := testProvider(t)
	p.Lock()
	if _, err := p.Read("remote/x"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Read: want ErrLocked, got %v", err)
	}
	if err := p.Write("remote/x", &Entry{Kind: "pair"}); !errors.Is(err, ErrLocked) {
		t.Fatalf("Write: want ErrLocked, got %v", err)
	}
	if _, err := p.List("remote/"); !errors.Is(err, ErrLocked) {
		t.Fatalf("List: want ErrLocked, got %v", err)
	}
	if err := p.Delete("remote/x"); !errors.Is(err, ErrLocked) {
		t.Fatalf("Delete: want ErrLocked, got %v", err)
	}
}

func TestLocalUnlockReverify(t *testing.T) {
	p, _ := testProvider(t)
	// 已解锁：同源口令幂等刷新；异源口令拒绝（防误切密钥）
	if err := p.Unlock(testKey); err != nil {
		t.Fatalf("same-source re-verify: %v", err)
	}
	if err := p.Unlock("another-key-0123456789abcdef-XYZ!"); err == nil ||
		!strings.Contains(err.Error(), "mismatch") {
		t.Fatalf("want mismatch error, got %v", err)
	}
	// 空口令拒绝
	if err := p.Unlock(""); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("want empty-passphrase error, got %v", err)
	}
	// 上锁后凭口令重开门（重派生）
	p.Lock()
	if err := p.Unlock(testKey); err != nil {
		t.Fatalf("unlock after lock: %v", err)
	}
	if _, err := p.Read("anything"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unlocked again but ops failing: %v", err)
	}
}

func TestLocalListMetadataOnly(t *testing.T) {
	p, _ := testProvider(t)
	entries := []*Entry{
		{Path: "nas/alpha", Kind: "pair", Username: "user-one", Value: "val-one"},
		{Path: "nas/beta", Kind: "api_token", Value: "val-two"},
		{Path: "remote/gamma", Kind: "password", Value: "val-three"},
	}
	for _, e := range entries {
		if err := p.Write(e.Path, e); err != nil {
			t.Fatalf("Write %s: %v", e.Path, err)
		}
	}
	metas, err := p.List("nas/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(metas) != 2 {
		t.Fatalf("prefix filter: %d metas", len(metas))
	}
	if metas[0].Path != "nas/alpha" || metas[1].Path != "nas/beta" {
		t.Fatalf("order/path wrong: %+v", metas)
	}
	if !metas[0].HasUsername || !metas[0].HasValue || metas[1].HasUsername || !metas[1].HasValue {
		t.Fatalf("flags wrong: %+v", metas)
	}
	// 全量列（空前缀）
	all, err := p.List("")
	if err != nil || len(all) != 3 {
		t.Fatalf("full list: %v %d", err, len(all))
	}
	// 元数据 JSON 可序列化且明文永不出库
	b, err := json.Marshal(all)
	if err != nil {
		t.Fatalf("metas must be JSON-marshalable: %v", err)
	}
	if strings.Contains(string(b), "val-") || strings.Contains(string(b), "user-one") {
		t.Fatal("metadata leaked plaintext")
	}
}

func TestLocalAuditTrail(t *testing.T) {
	applyTestKey(t)
	db := openTestDB(t)
	var audits []string
	p, err := NewLocal(db, 0, func(action, path string, ok bool) {
		mark := "ok"
		if !ok {
			mark = "fail"
		}
		audits = append(audits, action+":"+path+":"+mark)
	})
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	const secretValue = "audit-plain-XYZ"
	if err := p.Write("remote/a", &Entry{Kind: "pair", Username: "audit-user-9q", Value: secretValue}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := p.Read("remote/a"); err != nil {
		t.Fatalf("Read: %v", err)
	}
	if _, err := p.List("remote/"); err != nil {
		t.Fatalf("List: %v", err)
	}
	if _, err := p.Read("remote/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := p.Delete("remote/a"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	p.Lock()
	p.Unlock(testKey)
	want := []string{
		"vault.write:remote/a:ok",
		"vault.read:remote/a:ok",
		"vault.list:remote/:ok",
		"vault.read:remote/missing:fail",
		"vault.delete:remote/a:ok",
		"vault.lock::ok",
		"vault.unlock::ok",
	}
	if strings.Join(audits, "|") != strings.Join(want, "|") {
		t.Fatalf("audit trail mismatch:\n got %v\nwant %v", audits, want)
	}
	for _, a := range audits {
		if strings.Contains(a, secretValue) || strings.Contains(a, "audit-user-9q") {
			t.Fatal("audit must not contain plaintext")
		}
	}
}

func TestLocalReadNotFoundAndDelete(t *testing.T) {
	p, _ := testProvider(t)
	if _, err := p.Read("remote/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if err := p.Delete("remote/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete missing: want ErrNotFound, got %v", err)
	}
	if err := p.Write("remote/gone", &Entry{Kind: "pair", Value: "x"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := p.Delete("remote/gone"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := p.Read("remote/gone"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("read after delete: %v", err)
	}
}

func TestLocalCorruptCiphertext(t *testing.T) {
	p, db := testProvider(t)
	if err := p.Write("remote/c", &Entry{Kind: "pair", Username: "cu", Value: "cv"}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// 非 base64
	db.Model(&VaultEntry{}).Where("path = ?", "remote/c").
		Updates(map[string]any{"value_enc": "!!!not-b64!!!"})
	if _, err := p.Read("remote/c"); err == nil || !strings.Contains(err.Error(), "base64") {
		t.Fatalf("want base64 error, got %v", err)
	}
	// 短密文
	db.Model(&VaultEntry{}).Where("path = ?", "remote/c").
		Updates(map[string]any{"value_enc": base64.StdEncoding.EncodeToString([]byte("hi"))})
	if _, err := p.Read("remote/c"); err == nil || !strings.Contains(err.Error(), "too short") {
		t.Fatalf("want too-short error, got %v", err)
	}
	// username 密文损坏（独立分支）
	db.Model(&VaultEntry{}).Where("path = ?", "remote/c").
		Updates(map[string]any{"value_enc": "", "username_enc": "!!!not-b64!!!"})
	if _, err := p.Read("remote/c"); err == nil || !strings.Contains(err.Error(), "username") {
		t.Fatalf("want username decrypt error, got %v", err)
	}
	// meta 损坏
	db.Model(&VaultEntry{}).Where("path = ?", "remote/c").
		Updates(map[string]any{"username_enc": "", "meta": "not-json"})
	if _, err := p.Read("remote/c"); err == nil || !strings.Contains(err.Error(), "meta") {
		t.Fatalf("want meta error, got %v", err)
	}
}

func TestLocalClosedDBFailureBranches(t *testing.T) {
	applyTestKey(t)
	db := openTestDB(t)
	var audits []string
	p, err := NewLocal(db, 0, func(action, path string, ok bool) {
		audits = append(audits, action+":"+path)
	})
	if err != nil {
		t.Fatalf("NewLocal: %v", err)
	}
	sqlDB, _ := db.DB()
	sqlDB.Close()
	if _, err := NewLocal(db, 0, nil); err == nil || !strings.Contains(err.Error(), "migrate") {
		t.Fatalf("closed-db NewLocal must fail at migrate, got %v", err)
	}
	audits = nil
	if err := p.Write("remote/x", &Entry{Kind: "pair", Value: "closed-plain"}); err == nil {
		t.Fatal("closed-db Write must fail")
	}
	if _, err := p.Read("remote/x"); err == nil {
		t.Fatal("closed-db Read must fail")
	}
	if _, err := p.List(""); err == nil {
		t.Fatal("closed-db List must fail")
	}
	if err := p.Delete("remote/x"); err == nil {
		t.Fatal("closed-db Delete must fail")
	}
	if len(audits) != 4 {
		t.Fatalf("audit hook must fire on each failure, got %v", audits)
	}
	for _, a := range audits {
		if strings.Contains(a, "closed-plain") {
			t.Fatal("audit must not contain plaintext")
		}
	}
}

func TestLocalEncryptFailureInjection(t *testing.T) {
	p, _ := testProvider(t)
	orig := newCipher
	newCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("injected") }
	t.Cleanup(func() { newCipher = orig })
	// username 加密失败（先于 value）
	if err := p.Write("remote/x", &Entry{Kind: "pair", Username: "u", Value: "v"}); err == nil ||
		!strings.Contains(err.Error(), "encrypt username") {
		t.Fatalf("want username encrypt failure, got %v", err)
	}
	// value 加密失败（username 为空走另一分支）
	if err := p.Write("remote/x", &Entry{Kind: "pair", Value: "v"}); err == nil ||
		!strings.Contains(err.Error(), "encrypt value") {
		t.Fatalf("want value encrypt failure, got %v", err)
	}
}

func TestCryptoPrimitiveFailureInjection(t *testing.T) {
	applyTestKey(t)
	// newGCM 失败：加密与解密同源
	origGCM, origRand, origCipher := newGCM, cryptoRand, newCipher
	newGCM = func(cipher.Block) (cipher.AEAD, error) { return nil, errors.New("injected gcm") }
	if _, err := Encrypt("x"); err == nil {
		t.Fatal("encrypt must fail on gcm error")
	}
	if _, err := Decrypt("v1:" + base64.StdEncoding.EncodeToString(make([]byte, 64))); err == nil {
		t.Fatal("decrypt must fail on gcm error")
	}
	newGCM = origGCM
	// 随机源失败：nonce 读取
	ioErr := errors.New("injected rand")
	cryptoRand = failingReader{err: ioErr}
	if _, err := Encrypt("x"); err == nil {
		t.Fatal("encrypt must fail on rand error")
	}
	cryptoRand = origRand
	// 块密码失败：解密路径（base64 已过，newCipher 出错）
	newCipher = func([]byte) (cipher.Block, error) { return nil, errors.New("injected cipher") }
	if _, err := Decrypt("v1:" + base64.StdEncoding.EncodeToString(make([]byte, 64))); err == nil {
		t.Fatal("decrypt must fail on cipher error")
	}
	newCipher = origCipher
}

type failingReader struct{ err error }

func (f failingReader) Read([]byte) (int, error) { return 0, f.err }

func TestIsUsingDefaultKey(t *testing.T) {
	// init 依环境决定弱密钥位；此处只验证读取口径不 panic 且布尔
	applyTestKey(t)
	ApplyKey("short")
	if !IsUsingDefaultKey() {
		t.Fatal("short key must mark default/weak")
	}
	ApplyKey(testKey)
	if IsUsingDefaultKey() {
		t.Fatal("strong key must not mark default/weak")
	}
}

func TestLocalDBWriteDeleteFailureInjection(t *testing.T) {
	p, db := testProvider(t)
	if err := p.Write("remote/x", &Entry{Kind: "pair", Value: "v"}); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	// 注入 Update/Create/Delete 失败（覆盖 count 成功但写入失败的分支）
	reg := func(name string, fn func(*gorm.DB)) {
		t.Helper()
		if err := db.Callback().Create().Before("gorm:create").Register(name+"_c", fn); err != nil {
			t.Fatalf("register create callback: %v", err)
		}
		if err := db.Callback().Update().Before("gorm:update").Register(name+"_u", fn); err != nil {
			t.Fatalf("register update callback: %v", err)
		}
		if err := db.Callback().Delete().Before("gorm:delete").Register(name+"_d", fn); err != nil {
			t.Fatalf("register delete callback: %v", err)
		}
	}
	inject := func(tx *gorm.DB) { _ = tx.AddError(errors.New("injected db failure")) }
	reg("fail_once", inject)
	// 新建路径 Create 失败
	if err := p.Write("remote/new", &Entry{Kind: "pair", Value: "v"}); err == nil ||
		!strings.Contains(err.Error(), "vault: write") {
		t.Fatalf("want write failure, got %v", err)
	}
	// 更新路径 Update 失败
	if err := p.Write("remote/x", &Entry{Kind: "pair", Value: "v2"}); err == nil ||
		!strings.Contains(err.Error(), "vault: write") {
		t.Fatalf("want update failure, got %v", err)
	}
	// 删除路径失败
	if err := p.Delete("remote/x"); err == nil || !strings.Contains(err.Error(), "vault: delete") {
		t.Fatalf("want delete failure, got %v", err)
	}
}

func TestNewLocalNilDB(t *testing.T) {
	if _, err := NewLocal(nil, 0, nil); err == nil || !strings.Contains(err.Error(), "nil db") {
		t.Fatalf("want nil-db error, got %v", err)
	}
}

func TestProviderName(t *testing.T) {
	p, _ := testProvider(t)
	if p.Name() != "local" {
		t.Fatalf("name = %q", p.Name())
	}
}
