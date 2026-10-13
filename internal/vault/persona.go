package vault

// persona Connect API 只读实现（V7 注册表；接口映射与卡点见
// docs/guide/persona-vault-mapping.md）。凭据是 persona Connect token
// （env 注入，不落 yaml/日志），消费者无解锁/上锁/写入端点——Unlock 走
// health+items 双探握手，Write/Delete 显式 ErrUnsupported。fail-closed：
// 连不上/未解锁一律包 ErrLocked 拒绝服务，绝不降级 local。

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// personaConnectPath Connect API 基路径（persona connect-server build_router）。
const personaConnectPath = "/api/v1/connect"

// personaDefaultTimeout HTTP 超时默认值（vault-design.md §5.5）。
const personaDefaultTimeout = 5 * time.Second

// personaSecretFields type slug → data 变体名与秘密字段（PascalCase 外部
// 标签枚举，credential.rs CredentialData）。不在表内的类型无单一秘密字段
// （bank_card/certificate/identity 为多字段组合），Value 恒空。
var personaSecretFields = map[string][2]string{
	"password":         {"Password", "password"},
	"api_key":          {"ApiKey", "api_key"},
	"totp":             {"TwoFactor", "secret_key"},
	"note":             {"SecureNote", "note"},
	"server_config":    {"ServerConfig", "password"},
	"game_account":     {"GameToken", "secret_key"},
	"software_license": {"SoftwareLicense", "license_key"},
}

// PersonaProvider persona Connect API provider。
type PersonaProvider struct {
	endpoint string // Connect API 基址（http://127.0.0.1:port）
	token    string // Connect token（pconn_…，视同密码）
	client   *http.Client
	sess     *Session
	audit    AuditFunc
}

// NewPersona 构建 persona provider。cfg 键：endpoint（必填）、token（必填，
// 通常由 PERSONA_CONNECT_TOKEN 经配置默认值注入）、timeout（可选，默认 5s）。
// ttl 为已解析的会话时长（persona session_ttl 或全局回退，由装配方决定）。
// endpoint/token 缺失启动即报错（fail-fast，不半初始化）。
func NewPersona(cfg map[string]string, ttl time.Duration, audit AuditFunc) (*PersonaProvider, error) {
	endpoint := strings.TrimRight(cfg["endpoint"], "/")
	if endpoint == "" {
		return nil, errors.New("vault: persona: endpoint required")
	}
	if !strings.HasPrefix(endpoint, "http://") && !strings.HasPrefix(endpoint, "https://") {
		return nil, errors.New("vault: persona: endpoint must be an http(s) url")
	}
	token := cfg["token"]
	if token == "" {
		return nil, errors.New("vault: persona: token required (env PERSONA_CONNECT_TOKEN)")
	}
	timeout := personaDefaultTimeout
	if v := cfg["timeout"]; v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return nil, fmt.Errorf("vault: persona: timeout %q: %w", v, err)
		}
		if d <= 0 {
			return nil, errors.New("vault: persona: timeout must be positive")
		}
		timeout = d
	}
	return &PersonaProvider{
		endpoint: endpoint,
		token:    token,
		client:   &http.Client{Timeout: timeout},
		sess:     NewSession(ttl),
		audit:    audit,
	}, nil
}

// Name provider 名（V7 配置选择用）。
func (p *PersonaProvider) Name() string { return "persona" }

// Unlock 解锁握手（幂等）：persona 无消费者口令端点，passphrase 不参与
// （凭据是配置注入的 token）。双探通过 = 服务可达（health）且 token 有效、
// vault 已解锁（items），随后开本地会话门并刷新 TTL。
func (p *PersonaProvider) Unlock(passphrase string) error {
	if _, err := p.call(http.MethodGet, "/health"); err != nil {
		p.auditFail("vault.unlock", "")
		return err
	}
	if _, err := p.call(http.MethodGet, "/items"); err != nil {
		p.auditFail("vault.unlock", "")
		return err
	}
	p.sess.Unlock()
	p.auditOK("vault.unlock", "")
	return nil
}

// Lock 仅关本地会话门（锁不住 persona 本体——映射文档 §2）。
func (p *PersonaProvider) Lock() {
	p.sess.Lock()
	p.auditOK("vault.lock", "")
}

// Read 取条目（path = persona 条目 UUID；唯一明文出口）。
func (p *PersonaProvider) Read(path string) (*Entry, error) {
	if path == "" {
		return nil, errors.New("vault: path required")
	}
	if err := p.sess.Check(); err != nil {
		return nil, err
	}
	data, err := p.call(http.MethodGet, "/items/"+url.PathEscape(path))
	if err != nil {
		p.auditFail("vault.read", path)
		return nil, err
	}
	var item personaItem
	if err := json.Unmarshal(data, &item); err != nil {
		p.auditFail("vault.read", path)
		return nil, fmt.Errorf("vault: persona read %s: decode item: %w", path, err)
	}
	value, dataFields, err := personaExtractSecret(item.Type, item.Data)
	if err != nil {
		p.auditFail("vault.read", path)
		return nil, fmt.Errorf("vault: persona read %s: %w", path, err)
	}
	meta := map[string]string{
		"persona_title":      item.Title,
		"persona_updated_at": item.UpdatedAt,
	}
	if len(item.URLs) > 0 {
		meta["persona_urls"] = strings.Join(item.URLs, ",")
	}
	if len(dataFields) > 0 {
		meta["persona_data"] = strings.Join(dataFields, ",")
	}
	p.auditOK("vault.read", path)
	return &Entry{
		Path:     item.ID,
		Kind:     item.Type,
		Username: item.Username,
		Value:    value,
		Meta:     meta,
	}, nil
}

// Write persona 接口只读：会话门后显式 ErrUnsupported（不静默、不降级）。
func (p *PersonaProvider) Write(path string, e *Entry) error {
	if e == nil {
		return errors.New("vault: nil entry")
	}
	if path == "" {
		return errors.New("vault: path required")
	}
	if err := p.sess.Check(); err != nil {
		return err
	}
	p.auditFail("vault.write", path)
	return fmt.Errorf("vault: persona write %s: %w", path, ErrUnsupported)
}

// List 列条目元数据（明文永不出库，V2）；prefix 按 path 客户端过滤。
func (p *PersonaProvider) List(prefix string) ([]EntryMeta, error) {
	if err := p.sess.Check(); err != nil {
		return nil, err
	}
	data, err := p.call(http.MethodGet, "/items")
	if err != nil {
		p.auditFail("vault.list", prefix)
		return nil, err
	}
	var items []personaItemMeta
	if err := json.Unmarshal(data, &items); err != nil {
		p.auditFail("vault.list", prefix)
		return nil, fmt.Errorf("vault: persona list %s: decode items: %w", prefix, err)
	}
	metas := make([]EntryMeta, 0, len(items))
	for i := range items {
		if prefix != "" && !strings.HasPrefix(items[i].ID, prefix) {
			continue
		}
		_, hasSecret := personaSecretFields[items[i].Type]
		metas = append(metas, EntryMeta{
			Path:        items[i].ID,
			Kind:        items[i].Type,
			HasUsername: false, // 列表元数据无 username（映射文档 §4）
			HasValue:    hasSecret,
			UpdatedAt:   items[i].UpdatedAt,
		})
	}
	p.auditOK("vault.list", prefix)
	return metas, nil
}

// Delete persona 接口只读：同 Write，显式 ErrUnsupported。
func (p *PersonaProvider) Delete(path string) error {
	if path == "" {
		return errors.New("vault: path required")
	}
	if err := p.sess.Check(); err != nil {
		return err
	}
	p.auditFail("vault.delete", path)
	return fmt.Errorf("vault: persona delete %s: %w", path, ErrUnsupported)
}

// personaEnvelope Connect API 包络：成功 {"ok":true,"data":…}、错误
// {"ok":false,"error":{code,message}}。
type personaEnvelope struct {
	OK    bool            `json:"ok"`
	Data  json.RawMessage `json:"data"`
	Error *personaAPIError `json:"error"`
}

type personaAPIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type personaItemMeta struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Type      string   `json:"type"`
	URLs      []string `json:"urls"`
	UpdatedAt string   `json:"updated_at"`
}

// personaItem 详情 = 元数据 + username + data（外部标签变体，原样保留
// 交 personaExtractSecret 解析，避免把秘密字段展开进日志可见的结构体）。
type personaItem struct {
	personaItemMeta
	Username string          `json:"username"`
	Data     json.RawMessage `json:"data"`
}

// call 发一次带 token 的 Connect API 请求并解包络。传输失败/超时/非 200/
// ok=false 统一交 personaHTTPError 分类。
func (p *PersonaProvider) call(method, path string) (json.RawMessage, error) {
	req, err := http.NewRequest(method, p.endpoint+personaConnectPath+path, nil)
	if err != nil {
		return nil, fmt.Errorf("vault: persona %s %s: build request: %w", method, path, ErrLocked)
	}
	if p.token != "" {
		req.Header.Set("Authorization", "Bearer "+p.token)
	}
	// 不设 Origin：persona 对带 Origin 的请求回 403（浏览器防越），非浏览器
	// 客户端按 EXTERNAL_INTEGRATION.md 口径不带该头。
	resp, err := p.client.Do(req)
	if err != nil {
		// 传输失败/超时 fail-closed：按锁定类别拒绝（不降级 local，V7）。
		return nil, fmt.Errorf("vault: persona %s %s unreachable: %v: %w", method, path, err, ErrLocked)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<22))
	if err != nil {
		return nil, fmt.Errorf("vault: persona %s %s: read response: %v: %w", method, path, err, ErrLocked)
	}
	var env personaEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("vault: persona %s %s: status %d: invalid envelope: %w", method, path, resp.StatusCode, ErrLocked)
	}
	if resp.StatusCode != http.StatusOK || !env.OK {
		return nil, personaHTTPError(resp.StatusCode, &env)
	}
	return env.Data, nil
}

// personaHTTPError 按 (状态码, 错误码) 分类：404→ErrNotFound；401→显式
// token 错误（处置是换 token，不等解锁，故不包 ErrLocked）；429→显式限流；
// 其余（503 vault_locked/421/403/5xx）一律包 ErrLocked fail-closed。
// 消息只带状态码与错误码，绝不含 token/明文（红线）。
func personaHTTPError(status int, env *personaEnvelope) error {
	code := ""
	if env != nil && env.Error != nil {
		code = env.Error.Code
	}
	switch {
	case status == http.StatusNotFound || code == "not_found":
		return fmt.Errorf("vault: persona: %w (not_found)", ErrNotFound)
	case status == http.StatusUnauthorized || code == "unauthorized":
		return fmt.Errorf("vault: persona: token rejected (unauthorized): check PERSONA_CONNECT_TOKEN")
	case status == http.StatusTooManyRequests || code == "rate_limited":
		return fmt.Errorf("vault: persona: rate limited (120 req/min per token)")
	case status == http.StatusServiceUnavailable || code == "vault_locked":
		return fmt.Errorf("vault: persona: %w (vault_locked, owner must unlock)", ErrLocked)
	default:
		return fmt.Errorf("vault: persona: %w (status %d code %q)", ErrLocked, status, code)
	}
}

// personaExtractSecret 从 data 变体取秘密字段值。返回 (值, 字段名清单, 错误)：
// 有秘密映射的类型取字段值；无映射类型（bank_card 等）值空、字段名清单非空
// （供 Meta.persona_data，字段名非敏感）；data 缺失两空。变体与 type 不符
// 显式报错（数据形状异常不静默当空）。
func personaExtractSecret(kind string, data json.RawMessage) (string, []string, error) {
	if len(data) == 0 || string(data) == "null" {
		return "", nil, nil
	}
	var variants map[string]json.RawMessage
	if err := json.Unmarshal(data, &variants); err != nil {
		return "", nil, fmt.Errorf("decode data: %w", err)
	}
	if len(variants) != 1 {
		return "", nil, errors.New("data must have exactly one variant")
	}
	var variant string
	var payload json.RawMessage
	for k, v := range variants {
		variant, payload = k, v
	}
	if ref, ok := personaSecretFields[kind]; ok {
		if variant != ref[0] {
			return "", nil, fmt.Errorf("data variant %q does not match type %q", variant, kind)
		}
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &fields); err != nil {
			return "", nil, fmt.Errorf("decode %s data: %w", variant, err)
		}
		raw, ok := fields[ref[1]]
		if !ok || string(raw) == "null" {
			return "", nil, nil // 字段可空（如 server_config.password 为 Option）
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", nil, fmt.Errorf("decode %s.%s: %w", variant, ref[1], err)
		}
		return value, nil, nil
	}
	// 无秘密映射类型：列字段名清单（不含值，非敏感）。
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return "", nil, fmt.Errorf("decode %s data: %w", variant, err)
	}
	names := make([]string, 0, len(fields))
	for name := range fields {
		names = append(names, name)
	}
	sort.Strings(names)
	return "", names, nil
}

func (p *PersonaProvider) auditOK(action, path string) {
	if p.audit != nil {
		p.audit(action, path, true)
	}
}

func (p *PersonaProvider) auditFail(action, path string) {
	if p.audit != nil {
		p.audit(action, path, false)
	}
}
