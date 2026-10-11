// Package vault 服务端凭据保管抽象（vault-design.md V1-V8）。
// Provider 六操作（解锁/读/写/列/删除），KDF 与解锁会话口径收敛在本包，
// 各实现不自带加密；明文只在内存态流转，绝不进日志/审计/错误消息。
package vault

import (
	"errors"
	"fmt"
	"sync"
)

// Entry 凭据条目。Value/Username 为明文，仅内存态流转。
type Entry struct {
	Path     string            // 唯一寻址：<域>/<名>，如 remote/<id>、nas/<name>
	Kind     string            // password / private_key / api_token / pair
	Username string            // 可选配套用户名
	Value    string            // 明文值
	Meta     map[string]string // 非敏感元数据（addr/type 等）
}

// EntryMeta 列表出参形态——明文永不出库（V2）。
type EntryMeta struct {
	Path        string    `json:"path"`
	Kind        string    `json:"kind"`
	HasUsername bool      `json:"hasUsername"`
	HasValue    bool      `json:"hasValue"`
	UpdatedAt   string    `json:"updatedAt"`
}

// Provider 密码箱提供商接口（V2）。错误只带 path 与原因类别，不带明文。
type Provider interface {
	Name() string
	// Unlock 解锁（幂等）：已解锁时重验口令并刷新会话。
	Unlock(passphrase string) error
	// Lock 立即失效会话。
	Lock()
	Read(path string) (*Entry, error)
	Write(path string, e *Entry) error
	List(prefix string) ([]EntryMeta, error)
	Delete(path string) error
}

// 错误类别（V2 红线：错误消息不含明文，调用方据类别决定降级或拒绝）。
var (
	ErrLocked          = errors.New("vault: locked")
	ErrNotFound        = errors.New("vault: entry not found")
	ErrUnknownProvider = errors.New("vault: unknown provider")
)

// AuditFunc 审计钩子（V2）：vault 包统一发审计，实现不得自行落日志。
// action ∈ vault.unlock/read/write/delete；details 只含 path 与结果，无明文。
type AuditFunc func(action, path string, success bool)

// Factory 由配置节构建 provider（V7 注册表）。
type Factory func(cfg map[string]string) (Provider, error)

var (
	regMu    sync.Mutex
	registry = map[string]Factory{}
)

// Register 注册 provider 工厂（V7）。同名覆盖（测试可重置）。
func Register(name string, f Factory) {
	regMu.Lock()
	defer regMu.Unlock()
	registry[name] = f
}

// New 按名字构建 provider；未知名字返回 ErrUnknownProvider（V7：启动即报错不降级）。
func New(name string, cfg map[string]string) (Provider, error) {
	regMu.Lock()
	f, ok := registry[name]
	regMu.Unlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrUnknownProvider, name)
	}
	return f(cfg)
}
