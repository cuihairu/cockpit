# TOTP 二次验证功能实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-step. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为 Cockpit 添加 TOTP 二次验证功能，支持 Google/Microsoft Authenticator 等标准认证器

**Architecture:** 后端使用 `github.com/pquerna/otp` 实现 TOTP 生成/验证，密钥 AES-256-GCM 加密存储；前端 React 实现设置向导和验证页

**Tech Stack:** Go 1.23+, GORM, React 18+, Ant Design 5+, qrcode.js

---

## 文件结构概览

### 后端新增/修改文件
```
internal/storage/
├── user.go              (修改: User 结构体)
├── totp.go              (新增: TOTP 存储操作)
└── crypto.go            (新增: 加密工具)
internal/auth/
├── totp.go              (新增: TOTP 逻辑)
├── handler.go           (修改: 登录响应)
└── middleware.go        (修改: 临时 token 验证)
internal/server/
└── totp_routes.go       (新增: TOTP 路由)
go.mod                   (修改: 新增依赖)
```

### 前端新增/修改文件
```
web/src/
├── services/
│   └── api.ts           (修改: TOTP API 方法)
├── pages/
│   ├── Login/
│   │   └── index.tsx    (修改: 处理 TOTP 流程)
│   ├── TOTPVerify/      (新增目录)
│   │   ├── index.tsx
│   │   └── index.less
│   └── SetupTOTP/       (新增目录)
│       ├── index.tsx
│       └── index.less
├── components/
│   ├── QRCodeDisplay/   (新增目录)
│   │   └── index.tsx
│   └── BackupCodesDisplay/ (新增目录)
│       └── index.tsx
└── types.ts             (修改: TOTP 相关类型)
web/package.json         (修改: 新增 qrcode 依赖)
```

---

## Task 1: 安装后端依赖

**Files:**
- Modify: `go.mod`

- [ ] **Step 1: 添加 otp 依赖到 go.mod**

```bash
cd /Users/cui/Workspaces/cockpit && go get github.com/pquerna/otp@v1.4.0
go mod tidy
```

- [ ] **Step 2: 验证依赖安装成功**

```bash
go mod verify
```

Expected: 无错误输出

- [ ] **Step 3: 提交**

```bash
git add go.mod go.sum
git commit -m "deps: add github.com/pquerna/otp v1.4.0"
```

---

## Task 2: 创建加密工具模块

**Files:**
- Create: `internal/storage/crypto.go`
- Test: `internal/storage/crypto_test.go`

- [ ] **Step 1: 编写加密工具测试**

```go
package storage

import (
    "testing"
)

func TestEncryptDecrypt(t *testing.T) {
    // 设置测试密钥
    originalKey := encryptionKey
    encryptionKey = []byte("32-byte-long-test-encryption-key!")
    defer func() { encryptionKey = originalKey }()

    plaintext := "secret-totp-key"

    encrypted, err := Encrypt(plaintext)
    if err != nil {
        t.Fatalf("Encrypt failed: %v", err)
    }

    if encrypted == plaintext {
        t.Fatal("Encrypted text should differ from plaintext")
    }

    decrypted, err := Decrypt(encrypted)
    if err != nil {
        t.Fatalf("Decrypt failed: %v", err)
    }

    if decrypted != plaintext {
        t.Errorf("Decrypted text mismatch: got %q, want %q", decrypted, plaintext)
    }
}

func TestGenerateBackupCodes(t *testing.T) {
    codes, err := GenerateBackupCodes()
    if err != nil {
        t.Fatalf("GenerateBackupCodes failed: %v", err)
    }

    if len(codes) != 10 {
        t.Errorf("Got %d codes, want 10", len(codes))
    }

    for _, code := range codes {
        if len(code) != 14 { // xxxx-xxxx-xxxx
            t.Errorf("Code format wrong: %s", code)
        }
    }
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/storage -run TestEncryptDecrypt -v
```

Expected: FAIL with "undefined: Encrypt"

- [ ] **Step 3: 实现加密工具**

```go
package storage

import (
    "crypto/aes"
    "crypto/cipher"
    "crypto/rand"
    "crypto/sha256"
    "encoding/base64"
    "encoding/json"
    "fmt"
    "io"
    "os"
    "strings"
)

var encryptionKey []byte

func init() {
    key := os.Getenv("TOTP_ENCRYPTION_KEY")
    if key == "" {
        // 开发环境默认密钥（生产环境必须设置环境变量）
        key = "change-this-totp-encryption-key-in-prod!"
    }
    hash := sha256.Sum256([]byte(key))
    encryptionKey = hash[:]
}

// Encrypt 使用 AES-256-GCM 加密明文
func Encrypt(plaintext string) (string, error) {
    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonce := make([]byte, gcm.NonceSize())
    if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
        return "", err
    }

    ciphertext := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
    return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt 解密密文
func Decrypt(ciphertext string) (string, error) {
    data, err := base64.StdEncoding.DecodeString(ciphertext)
    if err != nil {
        return "", err
    }

    block, err := aes.NewCipher(encryptionKey)
    if err != nil {
        return "", err
    }

    gcm, err := cipher.NewGCM(block)
    if err != nil {
        return "", err
    }

    nonceSize := gcm.NonceSize()
    if len(data) < nonceSize {
        return "", fmt.Errorf("ciphertext too short")
    }

    nonce, cipherData := data[:nonceSize], data[nonceSize:]
    plaintext, err := gcm.Open(nil, nonce, cipherData, nil)
    if err != nil {
        return "", err
    }

    return string(plaintext), nil
}

// GenerateBackupCodes 生成 10 个备份恢复码
func GenerateBackupCodes() ([]string, error) {
    codes := make([]string, 10)
    for i := 0; i < 10; i++ {
        // 生成 12 位随机字符
        b := make([]byte, 6)
        if _, err := io.ReadFull(rand.Reader, b); err != nil {
            return nil, err
        }
        code := fmt.Sprintf("%04x-%04x-%04x", b[0:2], b[2:4], b[4:6])
        codes[i] = strings.ToUpper(code)
    }
    return codes, nil
}

// HashBackupCodes 对备份码进行 SHA256 哈希
func HashBackupCodes(codes []string) ([]string, error) {
    hashed := make([]string, len(codes))
    for i, code := range codes {
        hash := sha256.Sum256([]byte(code))
        hashed[i] = fmt.Sprintf("%x", hash)
    }
    return hashed, nil
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/storage -run TestEncryptDecrypt -v
go test ./internal/storage -run TestGenerateBackupCodes -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/storage/crypto.go internal/storage/crypto_test.go
git commit -m "feat: add AES-256-GCM encryption for TOTP secrets and backup code generation"
```

---

## Task 3: 扩展 User 模型

**Files:**
- Modify: `internal/storage/user.go`
- Test: `internal/storage/user_test.go`

- [ ] **Step 1: 编写 User 扩展测试**

```go
package storage

import (
    "testing"
    "time"
)

func TestUserTOTPFields(t *testing.T) {
    user := &User{
        Username:     "testuser",
        Password:     "hashedpassword",
        TOTPSecret:   "encrypted_secret",
        TOTPEnabled:  true,
        BackupCodes:  `["hash1","hash2"]`,
        TOTPSetupAt:  &time.Time{},
    }

    if user.TOTPSecret != "encrypted_secret" {
        t.Errorf("TOTPSecret mismatch")
    }
    if !user.TOTPEnabled {
        t.Error("TOTPEnabled should be true")
    }
}
```

- [ ] **Step 2: 运行测试验证需要修改**

```bash
go test ./internal/storage -run TestUserTOTPFields -v
```

Expected: 编译失败或字段不存在错误（当前 User 结构体没有这些字段）

- [ ] **Step 3: 修改 User 结构体**

在 `internal/storage/user.go` 中的 `User` 结构体添加新字段：

```go
// User 用户表
type User struct {
    ID        string    `gorm:"primaryKey" json:"id"`
    Username  string    `gorm:"uniqueIndex;not null" json:"username"`
    Password  string    `gorm:"not null" json:"-"`
    Email     string    `json:"email"`
    Role      string    `gorm:"index;default:user" json:"role"`
    // TOTP 字段
    TOTPSecret   string     `gorm:"column:totp_secret" json:"-"`
    TOTPEnabled  bool       `gorm:"column:totp_enabled;default:false" json:"totp_enabled"`
    BackupCodes  string     `gorm:"column:backup_codes;type:text" json:"-"`
    TOTPSetupAt  *time.Time `gorm:"column:totp_setup_at" json:"totp_setup_at,omitempty"`
    CreatedAt    time.Time  `json:"created_at"`
    UpdatedAt    time.Time  `json:"updated_at"`
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/storage -run TestUserTOTPFields -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/storage/user.go internal/storage/user_test.go
git commit -m "feat(storage): add TOTP fields to User model"
```

---

## Task 4: 创建 TOTP 存储操作

**Files:**
- Create: `internal/storage/totp.go`
- Test: `internal/storage/totp_test.go`

- [ ] **Step 1: 编写 TOTP 存储测试**

```go
package storage

import (
    "testing"
    "time"
)

func TestTOTPOperations(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    user := &User{
        Username:    "totpuser",
        Password:    "hash",
        TOTPSecret:  "encrypted_secret",
        TOTPEnabled: false,
    }
    if err := db.CreateUser(user); err != nil {
        t.Fatalf("CreateUser: %v", err)
    }

    // 测试启用 TOTP
    if err := db.EnableTOTP(user.ID, "encrypted_secret", `["hash1"]`); err != nil {
        t.Fatalf("EnableTOTP: %v", err)
    }

    // 验证已启用
    u, err := db.GetUserByID(user.ID)
    if err != nil {
        t.Fatalf("GetUserByID: %v", err)
    }
    if !u.TOTPEnabled {
        t.Error("TOTP should be enabled")
    }

    // 测试禁用 TOTP
    if err := db.DisableTOTP(user.ID); err != nil {
        t.Fatalf("DisableTOTP: %v", err)
    }

    u, err = db.GetUserByID(user.ID)
    if err != nil {
        t.Fatalf("GetUserByID: %v", err)
    }
    if u.TOTPEnabled {
        t.Error("TOTP should be disabled")
    }
}

func TestConsumeBackupCode(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    user := &User{
        Username:    "backupuser",
        Password:    "hash",
        TOTPEnabled: true,
        BackupCodes: `["111111111111111111111111111111111111111111111111111111111111","222222222222222222222222222222222222222222222222222222222222"]`,
    }
    if err := db.CreateUser(user); err != nil {
        t.Fatalf("CreateUser: %v", err)
    }

    // 消费第一个备份码
    valid, err := db.ConsumeBackupCode(user.ID, "111111111111111111111111111111111111111111111111111111111111")
    if err != nil {
        t.Fatalf("ConsumeBackupCode: %v", err)
    }
    if !valid {
        t.Error("Backup code should be valid")
    }

    // 验证已消费
    valid, err = db.ConsumeBackupCode(user.ID, "111111111111111111111111111111111111111111111111111111111111")
    if err != nil {
        t.Fatalf("ConsumeBackupCode: %v", err)
    }
    if valid {
        t.Error("Backup code should be consumed")
    }
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/storage -run TestTOTPOperations -v
```

Expected: FAIL with undefined methods

- [ ] **Step 3: 实现 TOTP 存储操作**

创建 `internal/storage/totp.go`:

```go
package storage

import (
    "encoding/json"
    "errors"
    "strings"
    "time"
)

var (
    ErrBackupCodeInvalid = errors.New("invalid backup code")
    ErrBackupCodeUsed    = errors.New("backup code already used")
)

// EnableTOTP 启用 TOTP 验证
func (d *DB) EnableTOTP(userID, encryptedSecret string, hashedBackupCodes []string) error {
    backupJSON, err := json.Marshal(hashedBackupCodes)
    if err != nil {
        return err
    }
    now := time.Now()
    return d.db.Model(&User{}).
        Where("id = ?", userID).
        Updates(map[string]interface{}{
            "totp_secret":   encryptedSecret,
            "totp_enabled":  true,
            "backup_codes":  string(backupJSON),
            "totp_setup_at": now,
        }).Error
}

// DisableTOTP 禁用 TOTP 验证
func (d *DB) DisableTOTP(userID string) error {
    return d.db.Model(&User{}).
        Where("id = ?", userID).
        Updates(map[string]interface{}{
            "totp_secret":   "",
            "totp_enabled":  false,
            "backup_codes":  "",
            "totp_setup_at": nil,
        }).Error
}

// UpdateTOTPSecret 更新 TOTP 密钥
func (d *DB) UpdateTOTPSecret(userID, encryptedSecret string) error {
    return d.db.Model(&User{}).
        Where("id = ?", userID).
        Update("totp_secret", encryptedSecret).Error
}

// ConsumeBackupCode 验证并消费备份码
func (d *DB) ConsumeBackupCode(userID, codeHash string) (bool, error) {
    var user User
    err := d.db.Where("id = ?", userID).First(&user).Error
    if err != nil {
        return false, err
    }

    if user.BackupCodes == "" {
        return false, ErrBackupCodeInvalid
    }

    var codes []string
    if err := json.Unmarshal([]byte(user.BackupCodes), &codes); err != nil {
        return false, err
    }

    // 查找匹配的备份码
    found := -1
    for i, code := range codes {
        if strings.EqualFold(code, codeHash) {
            found = i
            break
        }
    }

    if found == -1 {
        return false, ErrBackupCodeInvalid
    }

    // 移除已使用的备份码
    codes = append(codes[:found], codes[found+1:]...)

    // 更新数据库
    backupJSON, _ := json.Marshal(codes)
    if err := d.db.Model(&User{}).
        Where("id = ?", userID).
        Update("backup_codes", string(backupJSON)).Error; err != nil {
        return false, err
    }

    return true, nil
}

// RegenerateBackupCodes 重新生成备份码
func (d *DB) RegenerateBackupCodes(userID string, hashedBackupCodes []string) error {
    backupJSON, err := json.Marshal(hashedBackupCodes)
    if err != nil {
        return err
    }
    return d.db.Model(&User{}).
        Where("id = ?", userID).
        Update("backup_codes", string(backupJSON)).Error
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/storage -run TestTOTPOperations -v
go test ./internal/storage -run TestConsumeBackupCode -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/storage/totp.go internal/storage/totp_test.go
git commit -m "feat(storage): add TOTP storage operations"
```

---

## Task 5: 创建 TOTP 逻辑层

**Files:**
- Create: `internal/auth/totp.go`
- Test: `internal/auth/totp_test.go`

- [ ] **Step 1: 编写 TOTP 逻辑测试**

```go
package auth

import (
    "testing"
    "time"

    "github.com/pquerna/otp/totp"
)

func TestGenerateTOTPSecret(t *testing.T) {
    secret, err := GenerateTOTPSecret("testuser", "Cockpit")
    if err != nil {
        t.Fatalf("GenerateTOTPSecret: %v", err)
    }

    if secret == "" {
        t.Error("Secret should not be empty")
    }

    // 验证是有效的 Base32
    if len(secret) < 16 {
        t.Error("Secret too short")
    }
}

func TestValidateTOTP(t *testing.T) {
    key, err := totp.Generate(totp.GenerateOpts{
        Issuer:      "Cockpit",
        AccountName: "test@example.com",
    })
    if err != nil {
        t.Fatal(err)
    }

    // 生成有效代码
    code, err := totp.GenerateCode(key.Secret(), time.Now())
    if err != nil {
        t.Fatal(err)
    }

    // 验证应该成功
    if !ValidateTOTP(key.Secret(), code) {
        t.Error("Valid code should pass")
    }

    // 无效代码应该失败
    if ValidateTOTP(key.Secret(), "000000") {
        t.Error("Invalid code should fail")
    }
}

func TestGenerateQRCode(t *testing.T) {
    secret := "JBSWY3DPEHPK3PXP"
    url, err := GenerateTOTPURL(secret, "testuser", "Cockpit")
    if err != nil {
        t.Fatalf("GenerateTOTPURL: %v", err)
    }

    if !strings.Contains(url, "otpauth://totp") {
        t.Error("URL should be otpauth format")
    }
}
```

- [ ] **Step 2: 运行测试验证失败**

```bash
go test ./internal/auth -run TestGenerateTOTPSecret -v
```

Expected: FAIL with undefined functions

- [ ] **Step 3: 实现 TOTP 逻辑**

创建 `internal/auth/totp.go`:

```go
package auth

import (
    "crypto/rand"
    "encoding/base32"
    "fmt"
    "strings"
    "time"

    "github.com/pquerna/otp"
    "github.com/pquerna/otp/totp"
)

// GenerateTOTPSecret 生成 TOTP 密钥
func GenerateTOTPSecret(username, issuer string) (string, error) {
    key, err := totp.Generate(totp.GenerateOpts{
        Issuer:      issuer,
        AccountName: username,
        SecretSize:  20,
    })
    if err != nil {
        return "", err
    }
    return key.Secret(), nil
}

// GenerateTOTPURL 生成 TOTP URL（用于 QR 码）
func GenerateTOTPURL(secret, username, issuer string) (string, error) {
    key, err := totp.Generate(totp.GenerateOpts{
        Issuer:      issuer,
        AccountName: username,
        Secret:      secret,
    })
    if err != nil {
        return "", err
    }
    return key.URL(), nil
}

// ValidateTOTP 验证 TOTP 代码
func ValidateTOTP(secret, code string) bool {
    // 允许 ±1 个时间步长的容错
    valid, _ := totp.ValidateCustom(
        code,
        secret,
        time.Now(),
        totp.ValidateOpts{
            Period:    30,
            Skew:      1,
            Digits:    otp.DigitsSix,
            Algorithm: otp.AlgorithmSHA1,
        },
    )
    return valid
}

// GenerateQRCodeData 生成 QR 码数据
// 注意：实际 QR 码图片生成在前端进行，这里只返回 URL
func GenerateQRCodeData(secret, username, issuer string) (string, error) {
    url, err := GenerateTOTPURL(secret, username, issuer)
    if err != nil {
        return "", err
    }
    return url, nil
}

// FormatBackupCode 格式化备份码显示
func FormatBackupCode(code string) string {
    // xxxx-xxxx-xxxx 格式
    if len(code) == 12 {
        return strings.ToUpper(fmt.Sprintf("%s-%s-%s", code[0:4], code[4:8], code[8:12]))
    }
    return strings.ToUpper(code)
}

// GenerateRandomString 生成随机字符串（用于测试）
func GenerateRandomString(length int) (string, error) {
    bytes := make([]byte, length)
    if _, err := rand.Read(bytes); err != nil {
        return "", err
    }
    return base32.StdEncoding.EncodeToString(bytes)[:length], nil
}
```

- [ ] **Step 4: 运行测试验证通过**

```bash
go test ./internal/auth -run TestGenerateTOTPSecret -v
go test ./internal/auth -run TestValidateTOTP -v
go test ./internal/auth -run TestGenerateQRCode -v
```

Expected: PASS

- [ ] **Step 5: 提交**

```bash
git add internal/auth/totp.go internal/auth/totp_test.go
git commit -m "feat(auth): add TOTP generation and validation logic"
```

---

## Task 6: 修改登录响应结构

**Files:**
- Modify: `internal/auth/handler.go`

- [ ] **Step 1: 修改 LoginResponse 结构**

在 `internal/auth/handler.go` 中修改 `LoginResponse`:

```go
// LoginResponse 登录响应
type LoginResponse struct {
    Token       string `json:"token,omitempty"`
    ExpiresAt   int64  `json:"expires_at,omitempty"`
    UserID      string `json:"user_id,omitempty"`
    Username    string `json:"username,omitempty"`
    Role        string `json:"role,omitempty"`
    RequiresTOTP bool  `json:"requires_totp"`        // 新增
    TmpToken    string `json:"tmp_token,omitempty"`  // 新增
}
```

- [ ] **Step 2: 修改 HandleLogin 函数**

在 `internal/auth/handler.go` 中修改 `HandleLogin` 函数的密码验证后部分：

```go
// HandleLogin 处理登录
func HandleLogin(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }

    var req LoginRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
        return
    }

    // 验证用户名密码
    user, err := DB.VerifyPassword(req.Username, req.Password)
    if err != nil {
        http.Error(w, `{"error":"Invalid username or password"}`, http.StatusUnauthorized)
        return
    }

    // 检查是否启用 TOTP
    if user.TOTPEnabled {
        // 生成临时 token
        tmpToken := generateTmpToken(user.ID)
        response := LoginResponse{
            UserID:      user.ID,
            Username:    user.Username,
            RequiresTOTP: true,
            TmpToken:    tmpToken,
        }
        w.Header().Set("Content-Type", "application/json")
        json.NewEncoder(w).Encode(response)
        return
    }

    // 生成正式 token
    token, err := GenerateToken(user.ID, user.Username, user.Role)
    if err != nil {
        http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
        return
    }

    // 返回 token
    response := LoginResponse{
        Token:     token,
        ExpiresAt: time.Now().Add(24 * time.Hour).Unix(),
        UserID:    user.ID,
        Username:  user.Username,
        Role:      user.Role,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// generateTmpToken 生成临时会话 token
func generateTmpToken(userID string) string {
    return fmt.Sprintf("tmp:%s:%d", userID, time.Now().Unix())
}
```

- [ ] **Step 3: 添加 time 导入**

确保文件顶部有必要的导入：

```go
import (
    "encoding/json"
    "fmt"
    "net/http"
    "time"

    "github.com/cuihairu/cockpit/internal/storage"
)
```

- [ ] **Step 4: 运行测试确保编译通过**

```bash
go build ./internal/auth/...
```

Expected: 无编译错误

- [ ] **Step 5: 提交**

```bash
git add internal/auth/handler.go
git commit -m "feat(auth): add TOTP requirement to login response"
```

---

## Task 7: 创建 TOTP API 处理器

**Files:**
- Create: `internal/server/totp_handlers.go`

- [ ] **Step 1: 创建 TOTP API 处理器**

创建 `internal/server/totp_handlers.go`:

```go
package server

import (
    "encoding/json"
    "errors"
    "fmt"
    "net/http"
    "strconv"
    "time"

    "github.com/cuihairu/cockpit/internal/auth"
    "github.com/cuihairu/cockpit/internal/storage"
)

// TOTPGenerateRequest 生成密钥请求
type TOTPGenerateRequest struct {
    Username string `json:"username"`
}

// TOTPGenerateResponse 生成密钥响应
type TOTPGenerateResponse struct {
    Secret string `json:"secret"`
    QRCode string `json:"qr_code_url"`
}

// TOTPEnableRequest 启用 TOTP 请求
type TOTPEnableRequest struct {
    TmpToken string   `json:"tmp_token"`
    Code     string   `json:"code"`
    Secret   string   `json:"secret"`
}

// TOTPVerifyRequest 验证 TOTP 请求
type TOTPVerifyRequest struct {
    TmpToken    string `json:"tmp_token"`
    Code        string `json:"code"`
    BackupCode  string `json:"backup_code,omitempty"`
}

// TOTPDisableRequest 禁用 TOTP 请求
type TOTPDisableRequest struct {
    Code string `json:"code"`
}

// handleTOTPGenerate 生成 TOTP 密钥和 QR 码
func (s *Server) handleTOTPGenerate(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }

    // 从 JWT 获取用户信息
    userID := r.Context().Value("user_id").(string)
    if userID == "" {
        http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
        return
    }

    user, err := s.db.GetUserByID(userID)
    if err != nil {
        http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
        return
    }

    // 生成新密钥
    secret, err := auth.GenerateTOTPSecret(user.Username, "Cockpit")
    if err != nil {
        http.Error(w, `{"error":"Failed to generate secret"}`, http.StatusInternalServerError)
        return
    }

    // 生成 QR 码 URL
    qrURL, err := auth.GenerateTOTPURL(secret, user.Username, "Cockpit")
    if err != nil {
        http.Error(w, `{"error":"Failed to generate QR code"}`, http.StatusInternalServerError)
        return
    }

    // 加密密钥后临时存储（5分钟有效期）
    encryptedSecret, err := storage.Encrypt(secret)
    if err != nil {
        http.Error(w, `{"error":"Encryption failed"}`, http.StatusInternalServerError)
        return
    }

    // 存储在临时缓存中
    tmpKey := fmt.Sprintf("totp_setup:%s:%d", userID, time.Now().Unix())
    // 注意：这里应该使用缓存服务，简化实现直接返回
    // 实际生产环境需要 Redis 等缓存

    response := TOTPGenerateResponse{
        Secret: secret, // 开发环境直接返回，生产环境应从缓存获取
        QRCode: qrURL,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// handleTOTPEnable 启用 TOTP
func (s *Server) handleTOTPEnable(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }

    userID := r.Context().Value("user_id").(string)
    if userID == "" {
        http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
        return
    }

    var req TOTPEnableRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
        return
    }

    // 验证 TOTP 代码
    if !auth.ValidateTOTP(req.Secret, req.Code) {
        http.Error(w, `{"error":"Invalid TOTP code"}`, http.StatusBadRequest)
        return
    }

    // 加密密钥
    encryptedSecret, err := storage.Encrypt(req.Secret)
    if err != nil {
        http.Error(w, `{"error":"Encryption failed"}`, http.StatusInternalServerError)
        return
    }

    // 生成备份码
    backupCodes, err := storage.GenerateBackupCodes()
    if err != nil {
        http.Error(w, `{"error":"Failed to generate backup codes"}`, http.StatusInternalServerError)
        return
    }

    // 哈希备份码
    hashedCodes, err := storage.HashBackupCodes(backupCodes)
    if err != nil {
        http.Error(w, `{"error":"Failed to hash backup codes"}`, http.StatusInternalServerError)
        return
    }

    // 启用 TOTP
    if err := s.db.EnableTOTP(userID, encryptedSecret, hashedCodes); err != nil {
        http.Error(w, `{"error":"Failed to enable TOTP"}`, http.StatusInternalServerError)
        return
    }

    response := map[string]interface{}{
        "success":      true,
        "backup_codes": backupCodes, // 仅在设置时返回一次
        "message":      "TOTP enabled successfully. Please save your backup codes securely.",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// handleTOTPVerify 验证 TOTP 代码
func (s *Server) handleTOTPVerify(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }

    var req TOTPVerifyRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
        return
    }

    // 验证临时 token
    userID, err := validateTmpToken(req.TmpToken)
    if err != nil {
        http.Error(w, `{"error":"Invalid or expired temporary token"}`, http.StatusUnauthorized)
        return
    }

    user, err := s.db.GetUserByID(userID)
    if err != nil {
        http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
        return
    }

    var valid bool

    // 优先验证 TOTP 代码
    if req.Code != "" {
        // 解密密钥
        secret, err := storage.Decrypt(user.TOTPSecret)
        if err != nil {
            http.Error(w, `{"error":"Decryption failed"}`, http.StatusInternalServerError)
            return
        }

        valid = auth.ValidateTOTP(secret, req.Code)
    } else if req.BackupCode != "" {
        // 验证备份码
        hashedCode := storage.HashSingleBackupCode(req.BackupCode)
        valid, err = s.db.ConsumeBackupCode(userID, hashedCode)
        if err != nil && !errors.Is(err, storage.ErrBackupCodeUsed) {
            http.Error(w, `{"error":"Backup code validation failed"}`, http.StatusInternalServerError)
            return
        }
    }

    if !valid {
        http.Error(w, `{"error":"Invalid code"}`, http.StatusBadRequest)
        return
    }

    // 生成正式 token
    token, err := auth.GenerateToken(user.ID, user.Username, user.Role)
    if err != nil {
        http.Error(w, `{"error":"Failed to generate token"}`, http.StatusInternalServerError)
        return
    }

    response := map[string]interface{}{
        "token":     token,
        "expires_at": time.Now().Add(24 * time.Hour).Unix(),
        "user_id":   user.ID,
        "username":  user.Username,
        "role":      user.Role,
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// handleTOTPDisable 禁用 TOTP
func (s *Server) handleTOTPDisable(w http.ResponseWriter, r *http.Request) {
    if r.Method != http.MethodPost {
        http.Error(w, `{"error":"Method not allowed"}`, http.StatusMethodNotAllowed)
        return
    }

    userID := r.Context().Value("user_id").(string)
    if userID == "" {
        http.Error(w, `{"error":"Unauthorized"}`, http.StatusUnauthorized)
        return
    }

    var req TOTPDisableRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, `{"error":"Invalid request body"}`, http.StatusBadRequest)
        return
    }

    user, err := s.db.GetUserByID(userID)
    if err != nil {
        http.Error(w, `{"error":"User not found"}`, http.StatusNotFound)
        return
    }

    // 验证 TOTP 代码
    secret, err := storage.Decrypt(user.TOTPSecret)
    if err != nil {
        http.Error(w, `{"error":"Decryption failed"}`, http.StatusInternalServerError)
        return
    }

    if !auth.ValidateTOTP(secret, req.Code) {
        http.Error(w, `{"error":"Invalid TOTP code"}`, http.StatusBadRequest)
        return
    }

    // 禁用 TOTP
    if err := s.db.DisableTOTP(userID); err != nil {
        http.Error(w, `{"error":"Failed to disable TOTP"}`, http.StatusInternalServerError)
        return
    }

    response := map[string]interface{}{
        "success": true,
        "message": "TOTP disabled successfully",
    }

    w.Header().Set("Content-Type", "application/json")
    json.NewEncoder(w).Encode(response)
}

// validateTmpToken 验证临时 token
func validateTmpToken(tmpToken string) (string, error) {
    // 格式: tmp:userID:timestamp
    parts := splitString(tmpToken, ":")
    if len(parts) != 3 || parts[0] != "tmp" {
        return "", errors.New("invalid token format")
    }

    userID := parts[1]
    timestamp, err := strconv.ParseInt(parts[2], 10, 64)
    if err != nil {
        return "", errors.New("invalid timestamp")
    }

    // 检查过期（5分钟）
    if time.Now().Unix()-timestamp > 300 {
        return "", errors.New("token expired")
    }

    return userID, nil
}

func splitString(s, sep string) []string {
    if s == "" {
        return []string{}
    }
    parts := []string{}
    start := 0
    for i := 0; i < len(s); i++ {
        if i+len(sep) <= len(s) && s[i:i+len(sep)] == sep {
            parts = append(parts, s[start:i])
            start = i + len(sep)
            i += len(sep) - 1
        }
    }
    parts = append(parts, s[start:])
    return parts
}
```

- [ ] **Step 2: 在 crypto.go 中添加备份码哈希函数**

在 `internal/storage/crypto.go` 中添加：

```go
// HashSingleBackupCode 单个备份码哈希（用于验证时对比）
func HashSingleBackupCode(code string) string {
    hash := sha256.Sum256([]byte(code))
    return fmt.Sprintf("%x", hash)
}
```

- [ ] **Step 3: 运行编译检查**

```bash
go build ./internal/server/...
```

Expected: 可能有编译错误（`s.db` 未定义），下一步修复

- [ ] **Step 4: 提交**

```bash
git add internal/server/totp_handlers.go internal/storage/crypto.go
git commit -m "feat(server): add TOTP API handlers"
```

---

## Task 8: 注册 TOTP 路由

**Files:**
- Modify: `internal/server/server.go`

- [ ] **Step 1: 查看现有路由注册**

```bash
rg "HandleLogin|/auth/login" internal/server/
```

Expected: 找到路由注册位置

- [ ] **Step 2: 添加 TOTP 路由**

在 `Start()` 函数中的 API 路由注册区域添加（与登录刷新路由并列）：

```go
// TOTP 路由（不需要认证的）
mux.HandleFunc("/api/auth/totp/verify", s.handleTOTPVerify)

// 需要认证的 TOTP 路由
// 在 auth.Middleware 包裹的区域添加
```

同时在需要认证的路由处理中添加 TOTP 路由判断：

```go
// 在 auth.Middleware 的 serveAPI 中添加 TOTP 路由
if strings.HasPrefix(r.URL.Path, "/api/auth/totp/") {
    // TOTP 路由需要特殊处理
    switch r.URL.Path {
    case "/api/auth/totp/generate":
        auth.Middleware(s.handleTOTPGenerate)(w, r)
    case "/api/auth/totp/enable":
        auth.Middleware(s.handleTOTPEnable)(w, r)
    case "/api/auth/totp/disable":
        auth.Middleware(s.handleTOTPDisable)(w, r)
    default:
        auth.Middleware(s.serveAPI)(w, r)
    }
    return
}
```

- [ ] **Step 3: 运行编译检查**

```bash
go build ./...
```

Expected: PASS

- [ ] **Step 4: 提交**

```bash
git add internal/server/server.go
git commit -m "feat(server): register TOTP API routes"
```

---

## Task 9: 安装前端依赖

**Files:**
- Modify: `web/package.json`

- [ ] **Step 1: 安装 qrcode 库**

```bash
cd /Users/cui/Workspaces/cockpit/web
npm install qrcode @types/qrcode
```

- [ ] **Step 2: 验证安装**

```bash
grep -A2 "qrcode" package.json
```

Expected: 看到 qrcode 和 @types/qrcode

- [ ] **Step 3: 提交**

```bash
git add web/package.json web/package-lock.json
git commit -m "deps(web): add qrcode library for TOTP setup"
```

---

## Task 10: 扩展前端类型定义

**Files:**
- Modify: `web/src/types.ts` 或创建 `web/src/types/auth.ts`

- [ ] **Step 1: 添加 TOTP 相关类型**

```typescript
// TOTP 相关类型
export interface TOTPSetupResponse {
  secret: string
  qr_code_url: string
}

export interface TOTPEnableRequest {
  tmp_token?: string
  code: string
  secret: string
}

export interface TOTPEnableResponse {
  success: boolean
  backup_codes: string[]
  message: string
}

export interface TOTPVerifyRequest {
  tmp_token: string
  code?: string
  backup_code?: string
}

export interface LoginResponse {
  token?: string
  expires_at?: number
  user_id?: string
  username?: string
  role?: string
  requires_totp?: boolean
  tmp_token?: string
}
```

- [ ] **Step 2: 提交**

```bash
git add web/src/types/
git commit -m "feat(types): add TOTP related TypeScript types"
```

---

## Task 11: 扩展前端 API 客户端

**Files:**
- Modify: `web/src/services/api.ts`

- [ ] **Step 1: 添加 TOTP API 方法**

在 `ApiService` 类中添加：

```typescript
// ========== TOTP 二次验证 ==========

async login(username: string, password: string): Promise<LoginResponse> {
  const data = await this.client.post<any, LoginResponse>('/auth/login', { username, password })
  return data
}

async generateTOTP(): Promise<TOTPSetupResponse> {
  return this.client.post<any, TOTPSetupResponse>('/auth/totp/generate', {})
}

async enableTOTP(req: TOTPEnableRequest): Promise<TOTPEnableResponse> {
  return this.client.post<any, TOTPEnableResponse>('/auth/totp/enable', req)
}

async verifyTOTP(req: TOTPVerifyRequest): Promise<{ token: string; expires_at: number; user_id: string; username: string; role: string }> {
  return this.client.post<any, any>('/auth/totp/verify', req)
}

async disableTOTP(code: string): Promise<{ success: boolean; message: string }> {
  return this.client.post<any, any>('/auth/totp/disable', { code })
}
```

- [ ] **Step 2: 导入类型**

在文件顶部添加类型导入：

```typescript
import type {
  // ... 现有导入
  TOTPSetupResponse,
  TOTPEnableRequest,
  TOTPEnableResponse,
  TOTPVerifyRequest,
  LoginResponse,
} from '@/types'
```

- [ ] **Step 3: 提交**

```bash
git add web/src/services/api.ts
git commit -m "feat(api): add TOTP API methods"
```

---

## Task 12: 创建 QR 码显示组件

**Files:**
- Create: `web/src/components/QRCodeDisplay/index.tsx`
- Create: `web/src/components/QRCodeDisplay/index.less`

- [ ] **Step 1: 创建 QR 码组件**

```typescript
import { useEffect, useRef } from 'react'
import QRCode from 'qrcode'
import './index.less'

interface QRCodeDisplayProps {
  value: string
  size?: number
  title?: string
}

const QRCodeDisplay: React.FC<QRCodeDisplayProps> = ({ value, size = 200, title }) => {
  const canvasRef = useRef<HTMLCanvasElement>(null)

  useEffect(() => {
    const canvas = canvasRef.current
    if (!canvas) return

    QRCode.toCanvas(canvas, value, {
      width: size,
      margin: 2,
      color: {
        dark: '#000000',
        light: '#ffffff',
      },
    })
  }, [value, size])

  return (
    <div className="qrcode-display">
      {title && <div className="qrcode-title">{title}</div>}
      <canvas ref={canvasRef} style={{ width: size, height: size }} />
      <div className="qrcode-hint">请使用认证器应用扫描二维码</div>
    </div>
  )
}

export default QRCodeDisplay
```

- [ ] **Step 2: 创建样式文件**

```less
.qrcode-display {
  display: flex;
  flex-direction: column;
  align-items: center;
  padding: 24px;
  background: #fff;
  border-radius: 8px;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.1);

  .qrcode-title {
    font-size: 16px;
    font-weight: 500;
    margin-bottom: 16px;
    color: #333;
  }

  canvas {
    border: 1px solid #e8e8e8;
    border-radius: 4px;
  }

  .qrcode-hint {
    margin-top: 12px;
    font-size: 14px;
    color: #666;
  }
}
```

- [ ] **Step 3: 提交**

```bash
git add web/src/components/QRCodeDisplay/
git commit -m "feat(components): add QR code display component"
```

---

## Task 13: 创建备份码显示组件

**Files:**
- Create: `web/src/components/BackupCodesDisplay/index.tsx`
- Create: `web/src/components/BackupCodesDisplay/index.less`

- [ ] **Step 1: 创建备份码组件**

```typescript
import { useState } from 'react'
import { Button, message, Modal } from 'antd'
import { CopyOutlined, DownloadOutlined } from '@ant-design/icons'
import './index.less'

interface BackupCodesDisplayProps {
  codes: string[]
  onComplete: () => void
}

const BackupCodesDisplay: React.FC<BackupCodesDisplayProps> = ({ codes, onComplete }) => {
  const [copied, setCopied] = useState(false)
  const [confirmed, setConfirmed] = useState(false)

  const handleCopy = () => {
    navigator.clipboard.writeText(codes.join('\n'))
    setCopied(true)
    message.success('已复制到剪贴板')
    setTimeout(() => setCopied(false), 2000)
  }

  const handleDownload = () => {
    const blob = new Blob([codes.join('\n')], { type: 'text/plain' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `cockpit-backup-codes-${Date.now()}.txt`
    a.click()
    URL.revokeObjectURL(url)
    message.success('备份码已下载')
  }

  const handleConfirm = () => {
    Modal.confirm({
      title: '确认已保存备份码？',
      content: '请确保您已安全保存这些备份码。一旦离开此页面，您将无法再次查看它们。',
      okText: '我已保存',
      cancelText: '尚未保存',
      onOk: () => {
        setConfirmed(true)
        onComplete()
      },
    })
  }

  return (
    <div className="backup-codes-display">
      <h3>备份恢复码</h3>
      <p className="warning-text">
        请将这些代码保存在安全的地方。如果您的认证器设备丢失或损坏，您可以使用这些代码恢复账户访问。
      </p>
      <div className="codes-grid">
        {codes.map((code, index) => (
          <div key={index} className="code-item">
            {code}
          </div>
        ))}
      </div>
      <div className="action-buttons">
        <Button icon={<CopyOutlined />} onClick={handleCopy}>
          {copied ? '已复制' : '复制全部'}
        </Button>
        <Button icon={<DownloadOutlined />} onClick={handleDownload}>
          下载保存
        </Button>
      </div>
      <div className="confirm-section">
        <Button type="primary" size="large" onClick={handleConfirm}>
          我已安全保存备份码
        </Button>
      </div>
    </div>
  )
}

export default BackupCodesDisplay
```

- [ ] **Step 2: 创建样式文件**

```less
.backup-codes-display {
  padding: 24px;
  background: #fff;
  border-radius: 8px;

  h3 {
    font-size: 18px;
    margin-bottom: 16px;
    color: #333;
  }

  .warning-text {
    color: #faad14;
    margin-bottom: 24px;
    line-height: 1.6;
  }

  .codes-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: 12px;
    margin-bottom: 24px;

    @media (max-width: 480px) {
      grid-template-columns: 1fr;
    }
  }

  .code-item {
    padding: 12px;
    background: #f5f5f5;
    border: 1px solid #e8e8e8;
    border-radius: 4px;
    font-family: 'Monaco', 'Consolas', monospace;
    font-size: 14px;
    text-align: center;
    letter-spacing: 1px;
  }

  .action-buttons {
    display: flex;
    gap: 12px;
    margin-bottom: 24px;
  }

  .confirm-section {
    border-top: 1px solid #e8e8e8;
    padding-top: 24px;
    text-align: center;
  }
}
```

- [ ] **Step 3: 提交**

```bash
git add web/src/components/BackupCodesDisplay/
git commit -m "feat(components): add backup codes display component"
```

---

## Task 14: 创建 TOTP 验证页面

**Files:**
- Create: `web/src/pages/TOTPVerify/index.tsx`
- Create: `web/src/pages/TOTPVerify/index.less`

- [ ] **Step 1: 创建验证页面**

```typescript
import { useState } from 'react'
import { Button, Input, Form, App, Alert, Space } from 'antd'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { api } from '@/services/api'
import './index.less'

const TOTPVerify = () => {
  const [loading, setLoading] = useState(false)
  const [useBackupCode, setUseBackupCode] = useState(false)
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const { message } = App.useApp()
  const [form] = Form.useForm()

  const tmpToken = searchParams.get('token')

  if (!tmpToken) {
    navigate('/login')
    return null
  }

  const onFinish = async (values: { code: string }) => {
    setLoading(true)
    try {
      const req = useBackupCode
        ? { tmp_token: tmpToken, backup_code: values.code }
        : { tmp_token: tmpToken, code: values.code }

      const response = await api.verifyTOTP(req)

      // 保存 token
      localStorage.setItem('token', response.token)
      localStorage.setItem('username', response.username)

      message.success('登录成功')
      navigate('/')
    } catch (error: any) {
      message.error(error.response?.data?.error || '验证失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="totp-verify-container">
      <div className="totp-verify-box">
        <h2>双因素验证</h2>
        <Alert
          message="请输入您的认证器应用中显示的 6 位验证码"
          type="info"
          showIcon
          style={{ marginBottom: 24 }}
        />
        <Form
          form={form}
          name="totp-verify"
          onFinish={onFinish}
          autoComplete="off"
          size="large"
        >
          <Form.Item
            name="code"
            rules={[
              { required: true, message: useBackupCode ? '请输入备份码' : '请输入验证码' },
              {
                pattern: useBackupCode ? /^[A-Z0-9-]{14}$/ : /^\d{6}$/,
                message: useBackupCode ? '备份码格式不正确' : '验证码必须是 6 位数字',
              },
            ]}
          >
            <Input
              placeholder={useBackupCode ? 'xxxx-xxxx-xxxx' : '000000'}
              maxLength={useBackupCode ? 14 : 6}
              style={{ textAlign: 'center', letterSpacing: '4px' }}
              autoFocus
            />
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading}>
              验证
            </Button>
          </Form.Item>
        </Form>
        <div className="totp-verify-footer">
          <Button
            type="link"
            onClick={() => setUseBackupCode(!useBackupCode)}
            style={{ padding: 0 }}
          >
            {useBackupCode ? '使用验证器' : '丢失设备？使用备份码'}
          </Button>
        </div>
      </div>
    </div>
  )
}

export default TOTPVerify
```

- [ ] **Step 2: 创建样式文件**

```less
.totp-verify-container {
  display: flex;
  justify-content: center;
  align-items: center;
  min-height: 100vh;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);

  .totp-verify-box {
    width: 400px;
    padding: 40px;
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.1);

    h2 {
      text-align: center;
      margin-bottom: 24px;
      color: #333;
    }

    .totp-verify-footer {
      text-align: center;
      margin-top: 16px;
    }
  }
}
```

- [ ] **Step 3: 提交**

```bash
git add web/src/pages/TOTPVerify/
git commit -m "feat(pages): add TOTP verification page"
```

---

## Task 15: 创建 TOTP 设置页面

**Files:**
- Create: `web/src/pages/SetupTOTP/index.tsx`
- Create: `web/src/pages/SetupTOTP/index.less`

- [ ] **Step 1: 创建设置页面**

```typescript
import { useState } from 'react'
import { Button, Steps, Input, Form, App, Alert, Space } from 'antd'
import { useNavigate } from 'react-router-dom'
import { api } from '@/services/api'
import QRCodeDisplay from '@/components/QRCodeDisplay'
import BackupCodesDisplay from '@/components/BackupCodesDisplay'
import './index.less'

const { Step } = Steps

const SetupTOTP = () => {
  const [current, setCurrent] = useState(0)
  const [loading, setLoading] = useState(false)
  const [secret, setSecret] = useState('')
  const [qrURL, setQRURL] = useState('')
  const [backupCodes, setBackupCodes] = useState<string[]>([])
  const navigate = useNavigate()
  const { message } = App.useApp()
  const [form] = Form.useForm()

  // 步骤 1: 生成密钥
  const handleGenerate = async () => {
    setLoading(true)
    try {
      const response = await api.generateTOTP()
      setSecret(response.secret)
      setQRURL(response.qr_code_url)
      setCurrent(1)
    } catch (error: any) {
      message.error(error.response?.data?.error || '生成失败')
    } finally {
      setLoading(false)
    }
  }

  // 步骤 2: 验证并启用
  const handleEnable = async (values: { code: string }) => {
    setLoading(true)
    try {
      const response = await api.enableTOTP({
        code: values.code,
        secret,
      })
      setBackupCodes(response.backup_codes)
      setCurrent(2)
      message.success('TOTP 已启用')
    } catch (error: any) {
      message.error(error.response?.data?.error || '验证失败')
    } finally {
      setLoading(false)
    }
  }

  // 步骤 3: 完成
  const handleComplete = () => {
    message.success('设置完成！')
    navigate('/')
  }

  const steps = [
    {
      title: '开始设置',
      content: (
        <div className="setup-step">
          <Alert
            message="启用双因素验证可以提高账户安全性"
            description="设置后，每次登录时除了输入密码，还需要输入认证器应用中生成的验证码。"
            type="info"
            showIcon
            style={{ marginBottom: 24 }}
          />
          <div className="supported-auth">
            <p>支持的认证器应用：</p>
            <ul>
              <li>Google Authenticator</li>
              <li>Microsoft Authenticator</li>
              <li>Authy</li>
              <li>1Password</li>
              <li>Bitwarden</li>
            </ul>
          </div>
        </div>
      ),
    },
    {
      title: '扫描二维码',
      content: (
        <div className="setup-step">
          <Alert
            message="请使用认证器应用扫描下方二维码"
            type="info"
            showIcon
            style={{ marginBottom: 24 }}
          />
          <QRCodeDisplay value={qrURL} size={220} />
          <Form
            form={form}
            onFinish={handleEnable}
            style={{ marginTop: 24 }}
            layout="vertical"
          >
            <Form.Item
              name="code"
              label="输入验证码"
              rules={[
                { required: true, message: '请输入验证码' },
                { pattern: /^\d{6}$/, message: '验证码必须是 6 位数字' },
              ]}
            >
              <Input
                placeholder="000000"
                maxLength={6}
                style={{ textAlign: 'center', letterSpacing: '4px' }}
              />
            </Form.Item>
            <Space>
              <Button onClick={() => setCurrent(0)}>上一步</Button>
              <Button type="primary" htmlType="submit" loading={loading}>
                验证并启用
              </Button>
            </Space>
          </Form>
        </div>
      ),
    },
    {
      title: '保存备份码',
      content: (
        <div className="setup-step">
          <BackupCodesDisplay codes={backupCodes} onComplete={handleComplete} />
        </div>
      ),
    },
  ]

  return (
    <div className="setup-totp-container">
      <div className="setup-totp-box">
        <h2>设置双因素验证</h2>
        <Steps current={current} style={{ marginBottom: 32 }}>
          {steps.map((step, index) => (
            <Step key={index} title={step.title} />
          ))}
        </Steps>
        <div className="steps-content">{steps[current].content}</div>
        {current === 0 && (
          <div className="steps-action">
            <Button type="primary" size="large" onClick={handleGenerate} loading={loading}>
              开始设置
            </Button>
            <Button size="large" onClick={() => navigate('/')}>
              稍后设置
            </Button>
          </div>
        )}
      </div>
    </div>
  )
}

export default SetupTOTP
```

- [ ] **Step 2: 创建样式文件**

```less
.setup-totp-container {
  display: flex;
  justify-content: center;
  min-height: 100vh;
  padding: 40px 20px;
  background: #f5f5f5;

  .setup-totp-box {
    width: 100%;
    max-width: 600px;
    padding: 40px;
    background: #fff;
    border-radius: 12px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.1);

    h2 {
      text-align: center;
      margin-bottom: 32px;
      color: #333;
    }

    .steps-content {
      min-height: 300px;
    }

    .setup-step {
      padding: 16px 0;

      .supported-auth {
        background: #f9f9f9;
        padding: 16px;
        border-radius: 8px;

        ul {
          margin-top: 12px;
          padding-left: 24px;

          li {
            padding: 4px 0;
            color: #555;
          }
        }
      }
    }

    .steps-action {
      margin-top: 32px;
      text-align: center;
      display: flex;
      justify-content: center;
      gap: 12px;
    }
  }
}
```

- [ ] **Step 3: 提交**

```bash
git add web/src/pages/SetupTOTP/
git commit -m "feat(pages): add TOTP setup wizard page"
```

---

## Task 16: 修改登录页面处理 TOTP 流程

**Files:**
- Modify: `web/src/pages/Login/index.tsx`

- [ ] **Step 1: 修改登录处理逻辑**

修改 `LoginContent` 组件的 `onFinish` 函数：

```typescript
import { useState } from 'react'
import { Form, Input, Button, App, Checkbox, Modal } from 'antd'
import { UserOutlined, LockOutlined } from '@ant-design/icons'
import { useNavigate, Navigate } from 'react-router-dom'
import { useUser } from '@/contexts/UserContext'
import { api } from '@/services/api'
import logo from '@/assets/logo.svg'
import './index.less'

const LoginContent = () => {
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()
  const { login } = useUser()
  const { message } = App.useApp()
  const [form] = Form.useForm()
  const [showTOTPModal, setShowTOTPModal] = useState(false)

  const onFinish = async (values: { username: string; password: string; remember?: boolean }) => {
    setLoading(true)
    try {
      const response = await api.login(values.username, values.password)

      if (response.requires_totp) {
        // 需要二次验证，跳转到验证页面
        navigate(`/totp-verify?token=${encodeURIComponent(response.tmp_token || '')}`)
      } else {
        // 正常登录
        localStorage.setItem('token', response.token!)
        localStorage.setItem('username', response.username!)
        message.success('登录成功')
        navigate('/')
      }
    } catch (error: any) {
      message.error(error.response?.data?.error || '用户名或密码错误')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="login-container">
      <div className="login-background">
        <div className="login-bg-circle circle-1"></div>
        <div className="login-bg-circle circle-2"></div>
        <div className="login-bg-circle circle-3"></div>
      </div>
      <div className="login-box">
        <div className="login-logo-section">
          <img src={logo} alt="Cockpit" className="login-logo" />
          <h1 className="login-title">Cockpit</h1>
          <p className="login-subtitle">个人混合基础设施控制台</p>
        </div>
        <Form
          form={form}
          name="login"
          onFinish={onFinish}
          autoComplete="off"
          size="large"
          initialValues={{
            username: 'admin',
            password: 'admin',
            remember: true
          }}
          className="login-form"
        >
          <Form.Item
            name="username"
            rules={[{ required: true, message: '请输入用户名' }]}
          >
            <Input
              prefix={<UserOutlined />}
              placeholder="用户名"
              variant="borderless"
            />
          </Form.Item>
          <Form.Item
            name="password"
            rules={[{ required: true, message: '请输入密码' }]}
          >
            <Input.Password
              prefix={<LockOutlined />}
              placeholder="密码"
              variant="borderless"
            />
          </Form.Item>
          <Form.Item>
            <Form.Item name="remember" valuePropName="checked" noStyle>
              <Checkbox>记住账号</Checkbox>
            </Form.Item>
          </Form.Item>
          <Form.Item>
            <Button type="primary" htmlType="submit" block loading={loading} className="login-button">
              登 录
            </Button>
          </Form.Item>
        </Form>
        <div className="login-footer">
          <p>默认账号: admin / admin</p>
        </div>
      </div>
    </div>
  )
}

const Login = () => {
  return (
    <App>
      <LoginContent />
    </App>
  )
}

export default Login
```

- [ ] **Step 2: 提交**

```bash
git add web/src/pages/Login/index.tsx
git commit -m "feat(pages): update login to handle TOTP flow"
```

---

## Task 17: 添加路由配置

**Files:**
- Modify: `web/src/App.tsx`

- [ ] **Step 1: 查看现有路由**

```bash
rg "Route|path=" web/src/App.tsx
```

Expected: 找到路由定义位置

- [ ] **Step 2: 添加 TOTP 相关路由**

在路由配置中添加：

```typescript
import TOTPVerify from '@/pages/TOTPVerify'
import SetupTOTP from '@/pages/SetupTOTP'

// 在路由中添加
<Route path="/totp-verify" element={<TOTPVerify />} />
<Route path="/setup-totp" element={<SetupTOTP />} />
```

- [ ] **Step 3: 提交**

```bash
git add web/src/App.tsx
git commit -m "feat(router): add TOTP verify and setup routes"
```

---

## Task 18: 数据库迁移

**Files:**
- Modify: 数据库迁移脚本

- [ ] **Step 1: 创建迁移**

在数据库初始化位置添加字段迁移：

```go
// AutoMigrate 运行自动迁移
func (d *DB) AutoMigrate() error {
    return d.db.AutoMigrate(
        &User{},
        // ... 其他模型
    )
}
```

GORM 会自动添加新字段。

- [ ] **Step 2: 验证迁移**

```bash
# 启动应用检查迁移是否成功
./cockpit
```

Expected: 无迁移错误

- [ ] **Step 3: 提交**

```bash
git commit -m "feat(db): add TOTP fields migration (via GORM AutoMigrate)"
```

---

## Task 19: 集成测试

**Files:**
- Test: `internal/auth/totp_integration_test.go`

- [ ] **Step 1: 编写集成测试**

```go
package auth

import (
    "testing"
    "time"

    "github.com/cuihairu/cockpit/internal/storage"
)

func TestTOTPLoginFlow(t *testing.T) {
    db := setupTestDB(t)
    defer db.Close()

    // 1. 创建用户
    user := &storage.User{
        Username: "totptest",
        Password: "hashed",
    }
    if err := db.CreateUser(user); err != nil {
        t.Fatal(err)
    }

    // 2. 生成 TOTP 密钥
    secret, err := GenerateTOTPSecret(user.Username, "Cockpit")
    if err != nil {
        t.Fatal(err)
    }

    // 3. 启用 TOTP
    backupCodes, _ := storage.GenerateBackupCodes()
    hashedCodes, _ := storage.HashBackupCodes(backupCodes)
    encryptedSecret, _ := storage.Encrypt(secret)

    if err := db.EnableTOTP(user.ID, encryptedSecret, hashedCodes); err != nil {
        t.Fatal(err)
    }

    // 4. 验证用户已启用 TOTP
    u, _ := db.GetUserByID(user.ID)
    if !u.TOTPEnabled {
        t.Error("TOTP should be enabled")
    }

    // 5. 生成有效 TOTP 代码
    code, err := totp.GenerateCode(secret, time.Now())
    if err != nil {
        t.Fatal(err)
    }

    // 6. 验证代码
    if !ValidateTOTP(secret, code) {
        t.Error("Valid TOTP code should pass")
    }

    // 7. 测试备份码
    hash := storage.HashSingleBackupCode(backupCodes[0])
    valid, _ := db.ConsumeBackupCode(user.ID, hash)
    if !valid {
        t.Error("Backup code should be valid")
    }
}

// 注意：此测试需要 setupTestDB 辅助函数，通常放在测试文件中
// 实际实现时需要根据项目现有的测试基础设施调整
```

- [ ] **Step 2: 运行集成测试**

```bash
go test ./internal/auth -run TestTOTPLoginFlow -v
```

Expected: PASS

- [ ] **Step 3: 提交**

```bash
git add internal/auth/totp_integration_test.go
git commit -m "test(auth): add TOTP integration tests"
```

---

## Task 20: 环境变量配置文档

**Files:**
- Modify: `docs/deployment.md` 或 `README.md`

- [ ] **Step 1: 添加环境变量文档**

在文档中添加：

```markdown
## 环境变量

### TOTP 二次验证

| 变量名 | 必需 | 默认值 | 说明 |
|--------|------|--------|------|
| `TOTP_ENCRYPTION_KEY` | 推荐 | (开发默认值) | TOTP 密钥加密密钥（AES-256），生产环境必须设置 |

**生成加密密钥：**
```bash
openssl rand -base64 32
```
```

- [ ] **Step 2: 提交**

```bash
git add docs/
git commit -m "docs: add TOTP environment variables documentation"
```

---

## 验收测试

- [ ] **手动测试清单**

1. 登录页面正常显示
2. 启用 TOTP 后登录流程跳转到验证页
3. TOTP 验证页可以成功验证
4. 备份码可以正常使用
5. 设置向导流程完整可用
6. QR 码可以正常扫描
7. 各认证器应用兼容性测试

```bash
# 前端启动测试
cd web && npm run dev

# 后端启动测试
./cockpit
```

---

## 完成检查清单

- [ ] 所有单元测试通过
- [ ] 集成测试通过
- [ ] 前端 TypeScript 编译无错误
- [ ] 后端 Go 编译无错误
- [ ] 手动测试清单完成
- [ ] 代码已提交
- [ ] 文档已更新
