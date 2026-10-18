package config

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 顶层配置结构
type Config struct {
	Server        *ServerConfig        `yaml:"server"`
	Database      *DatabaseConfig      `yaml:"database"`
	JWT           *JWTConfig           `yaml:"jwt"`
	Email         *EmailConfig         `yaml:"email"`
	Notification  *NotificationConfig  `yaml:"notification"`
	Agent         *AgentConfig         `yaml:"agent"`
	Inventory     *InventoryConfig     `yaml:"inventory,omitempty"`
	RemoteControl *RemoteControlConfig `yaml:"remote_control,omitempty"`
	DNS           *DNSConfig           `yaml:"dns,omitempty"`
	Overlay       *OverlayConfig       `yaml:"overlay,omitempty"`
	Vault         *VaultConfig         `yaml:"vault,omitempty"`
	Probe         *ProbeConfig         `yaml:"probe,omitempty"`
	Stacks        *StacksConfig        `yaml:"stacks,omitempty"`
}

// StacksConfig stacks 部署相关配置（stack-deploy-design.md P2 ②）。
type StacksConfig struct {
	// WebhookToken git 仓库 webhook 触发部署的令牌（X-Cockpit-Token 头）。
	// 空 = webhook 端点关闭（默认关，显式配置开启——数据外发/外部触达
	// 默认不接入的口径同 notification.enabled）。常量时间比对。
	WebhookToken string `yaml:"webhook_token,omitempty"`
}

// ProbeConfig 自动拨测配置（probe-enhance-design.md M1：间隔亦可经 UI 保存，
// DB 保存值优先于本文件）。
type ProbeConfig struct {
	// Interval 探测间隔（每轮完整探测的周期）。默认 5m；越界自动夹到
	// [30s, 1h]（与 internal/probe 的 Min/MaxIntervalSeconds 一致——probe
	// 包经 notification 依赖本包，本包不能反向 import 取常量）。启动时
	// 读一次；UI 保存的间隔（Setting 表）运行期仍可即时生效。
	Interval time.Duration `yaml:"interval,omitempty"`
}

// 拨测间隔默认值与边界（镜像 internal/probe 的 DefaultInterval /
// MinIntervalSeconds / MaxIntervalSeconds，见 ProbeConfig.Interval 注释）
const (
	ProbeDefaultInterval = 5 * time.Minute
	ProbeMinInterval     = 30 * time.Second
	ProbeMaxInterval     = time.Hour
)

// VaultConfig 密码箱配置（vault-design.md V4/V5/V7）：provider 选择与
// 会话口径；密钥不落 yaml，走 env（COCKPIT_VAULT_KEY，回退
// TOTP_ENCRYPTION_KEY），yaml 中不出现任何密钥字段。
type VaultConfig struct {
	// Provider 密码箱提供商：空 = local（默认本地加密实现，零外部依赖）；
	// 未知值启动即报错（V7 不降级）
	Provider string `yaml:"provider,omitempty"`
	// SessionTTL provider 级解锁会话时长；0 = 进程生命周期（自托管
	// 无人值守重启默认）。用户级重验门（vault token 10min）在 API 层不受此影响
	SessionTTL time.Duration `yaml:"session_ttl,omitempty"`
	// Providers per-provider 配置节（V7：vault.providers.<name>.*）；仅在
	// Provider 选中对应实现时生效。子树按已知字段严格解码，未知字段报错
	// （防拼错键静默失效）
	Providers *VaultProvidersConfig `yaml:"providers,omitempty"`
}

// VaultProvidersConfig per-provider 配置节（V7）。新 provider 接入时在此
// 挂子结构体与严格解码（docs/guide/vault-design.md §5）。
type VaultProvidersConfig struct {
	Persona *PersonaVaultConfig `yaml:"persona,omitempty"`
}

// PersonaVaultConfig persona Connect API 接入参数（只读实现，接口映射与
// 卡点见 docs/guide/persona-vault-mapping.md）。仅在 vault.provider: persona
// 时生效。
type PersonaVaultConfig struct {
	// Endpoint Connect API 基址（persona connect serve，如
	// http://127.0.0.1:8747），必填
	Endpoint string `yaml:"endpoint,omitempty"`
	// Token Connect token（pconn_…，视同密码）；PERSONA_CONNECT_TOKEN env
	// 优先，yaml 不落 secret
	Token string `yaml:"token,omitempty"`
	// Timeout Connect API HTTP 超时；0 = 5s
	Timeout time.Duration `yaml:"timeout,omitempty"`
	// SessionTTL persona 会话时长；0 = 回退全局 vault.session_ttl
	SessionTTL time.Duration `yaml:"session_ttl,omitempty"`
}

// DNSConfig DNS 管理配置（M1：Cloudflare；M2 扩 DNSPod/阿里云——ACME D13，
// provider 决定 ACME DNS-01 用的 API；DNS 管理与 DDNS 仍 Cloudflare 专属）
type DNSConfig struct {
	// Provider ACME DNS-01 provider：空 = cloudflare（向后兼容）/ dnspod / alidns
	Provider   string               `yaml:"provider,omitempty"`
	Cloudflare *CloudflareDNSConfig `yaml:"cloudflare,omitempty"`
	DNSPod     *DNSPodConfig        `yaml:"dnspod,omitempty"`
	AliDNS     *AliDNSConfig        `yaml:"alidns,omitempty"`
}

// CloudflareDNSConfig Cloudflare API token（建议用 env CLOUDFLARE_API_TOKEN
// 注入，secret 不落 yaml）
type CloudflareDNSConfig struct {
	APIToken string `yaml:"api_token,omitempty"`
}

// DNSPodConfig DNSPod 凭据（"ID,Token" 合并格式；env DNSPOD_LOGIN_TOKEN 优先）
type DNSPodConfig struct {
	LoginToken string `yaml:"login_token,omitempty"`
}

// AliDNSConfig 阿里云 AccessKey（env ALIYUN_ACCESS_KEY / ALIYUN_ACCESS_KEY_SECRET 优先）
type AliDNSConfig struct {
	AccessKey string `yaml:"access_key,omitempty"`
	SecretKey string `yaml:"secret_key,omitempty"`
}

// OverlayConfig 组网云管理面配置（见 overlay-design.md M2 D12；观测在 agent，
// 管理凭据是账号级的，server 直连云 API）
type OverlayConfig struct {
	ZeroTier  *ZeroTierCloudConfig  `yaml:"zerotier,omitempty"`
	Tailscale *TailscaleCloudConfig `yaml:"tailscale,omitempty"`
}

// ZeroTierCloudConfig ZeroTier Central API token（env ZEROTIER_API_TOKEN 优先）
type ZeroTierCloudConfig struct {
	APIToken string `yaml:"api_token,omitempty"`
}

// TailscaleCloudConfig Tailscale API token（env TAILSCALE_API_TOKEN 优先；
// Tailnet 空 = "-"，即 token 所属默认 tailnet）
type TailscaleCloudConfig struct {
	APIToken string `yaml:"api_token,omitempty"`
	Tailnet  string `yaml:"tailnet,omitempty"`
}

// RemoteControlConfig 远程控制（Terminal/Desktop/VNC）相关配置
type RemoteControlConfig struct {
	// AllowArbitraryTarget 为 true 时跳过目标 allow-list 校验（仅开发/调试环境启用）。
	// 默认 false：host 必须命中 AllowedTargets。
	AllowArbitraryTarget bool `yaml:"allow_arbitrary_target"`
	// AllowedTargets 显式允许的目标主机名、IP 或 CIDR 列表（端口不限）。
	AllowedTargets []string `yaml:"allowed_targets"`
	// EgressPolicies 按 Agent 约束可访问的目标和端口；为空时不启用按 Agent 限制。
	EgressPolicies []*RemoteEgressPolicy `yaml:"egress,omitempty"`
}

// RemoteEgressPolicy 定义某个 Agent 可访问的目标范围。
type RemoteEgressPolicy struct {
	AgentID        string   `yaml:"agent_id"`
	AllowedTargets []string `yaml:"allowed_targets,omitempty"`
	AllowedPorts   []int    `yaml:"allowed_ports,omitempty"`
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Host              string                   `yaml:"host"`
	Port              int                      `yaml:"port"`
	StaticDir         string                   `yaml:"static_dir"`
	AgentRegistration *AgentRegistrationConfig `yaml:"agent_registration"`
}

// AgentRegistrationConfig agent 注册准入策略（2026-10-18 审核整改：默认
// fail-closed。未知 agent 仅在携带 enrollment_secret 或显式 allow_unknown
// 时可注册；无 secret 存量 agent 重连默认拒绝，需显式 allow_secretless）。
type AgentRegistrationConfig struct {
	AllowUnknown     bool   `yaml:"allow_unknown"`
	AllowSecretless  bool   `yaml:"allow_secretless"`
	EnrollmentSecret string `yaml:"enrollment_secret"`
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret     string        `yaml:"secret"`
	Expiration time.Duration `yaml:"expiration"`
}

// EmailConfig 邮件配置
type EmailConfig struct {
	Enabled bool        `yaml:"enabled"`
	SMTP    *SMTPConfig `yaml:"smtp"`
	BaseURL string      `yaml:"base_url"`
}

// SMTPConfig SMTP 配置
type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
	FromName string `yaml:"from_name"`
}

// NotificationConfig 通知配置
type NotificationConfig struct {
	Enabled  bool                    `yaml:"enabled"`
	Herald   *HeraldConfig           `yaml:"herald"`
	Ntfy     []*NtfyConfig           `yaml:"ntfy"`
	Webhook  []*WebhookConfig        `yaml:"webhook"`
	Telegram []*TelegramConfig       `yaml:"telegram"`
	Events   map[string]*EventConfig `yaml:"events"`
}

// HeraldConfig Herald 服务配置
type HeraldConfig struct {
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
}

// NtfyConfig ntfy 渠道配置（JSON 发布模式，POST {server}/）
type NtfyConfig struct {
	Server   string        `yaml:"server"`
	Topic    string        `yaml:"topic"`
	Token    string        `yaml:"token,omitempty"`
	Priority string        `yaml:"priority,omitempty"` // min/low/default/high/urgent，空则按 level 推导
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

// WebhookConfig 通用 webhook 渠道配置
type WebhookConfig struct {
	URL     string        `yaml:"url"`
	Secret  string        `yaml:"secret,omitempty"` // 作为 X-Cockpit-Secret 头，供接收方鉴权
	Timeout time.Duration `yaml:"timeout,omitempty"`
}

// TelegramConfig Telegram Bot 渠道配置
type TelegramConfig struct {
	BotToken string        `yaml:"bot_token"`
	ChatID   string        `yaml:"chat_id"`
	Timeout  time.Duration `yaml:"timeout,omitempty"`
}

// EventConfig 事件配置
type EventConfig struct {
	Type    string `yaml:"type"`
	Enabled bool   `yaml:"enabled"`
}

// AgentConfig Agent 配置
type AgentConfig struct {
	APIKeyHeader string `yaml:"api_key_header"`
	// ExpireMinutes Agent 过期判定阈值（分钟）：超过该时长无心跳/上报即判过期
	// ——服务端自动把 DB 里假在线行标离线，UI 默认隐藏过期行（不删库，
	// 密钥/标签/档案保留）。默认 5；负值关闭自动过期；env
	// AGENT_EXPIRE_MINUTES 优先。
	ExpireMinutes int `yaml:"expire_minutes,omitempty"`
}

// InventoryConfig Inventory 配置
type InventoryConfig struct {
	Path   string `yaml:"path,omitempty"`   // inventory 文件路径
	Watch  bool   `yaml:"watch,omitempty"`  // 是否监听文件变化
	Strict bool   `yaml:"strict,omitempty"` // 是否在启动时严格校验 inventory
}

// Load 从文件加载配置
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// 先扩展环境变量
	expandedData := expandEnvInContent(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expandedData), &cfg); err != nil {
		return nil, err
	}

	// vault.providers 子树按已知字段严格解码（V7：未知字段启动报错，防
	// 拼错键静默失效）；其余配置节维持宽松解码（向后兼容）。
	if err := strictVaultProviders([]byte(expandedData)); err != nil {
		return nil, err
	}

	applyDefaults(&cfg)
	return &cfg, nil
}

// strictVaultProviders 对 yaml 中 vault.providers 子树做 KnownFields 严格
// 解码。vault/providers 节不存在（或为 null）时跳过——env 注入的 persona
// 配置不经 yaml，无需检查。
func strictVaultProviders(data []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return err
	}
	if len(root.Content) == 0 {
		return nil
	}
	vaultNode := mappingValue(root.Content[0], "vault")
	if vaultNode == nil {
		return nil
	}
	provNode := mappingValue(vaultNode, "providers")
	if provNode == nil || provNode.Kind != yaml.MappingNode {
		return nil // 节缺失或空值（null）：无可查字段
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(provNode); err != nil {
		return err
	}
	enc.Close()
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)
	var sub VaultProvidersConfig
	if err := dec.Decode(&sub); err != nil {
		return fmt.Errorf("vault.providers: %w", err)
	}
	return nil
}

// mappingValue 在映射节点里取 key 对应的值节点；非映射或缺失返回 nil。
func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

// expandEnvInContent 扩展内容中的环境变量
func expandEnvInContent(content string) string {
	// 匹配 ${VAR_NAME} 格式
	re := regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

	return re.ReplaceAllStringFunc(content, func(match string) string {
		// 提取变量名
		varName := match[2 : len(match)-1]
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return ""
	})
}

// LoadOrDefault 加载配置或返回默认配置
func LoadOrDefault(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		cfg = &Config{}
	}
	applyDefaults(cfg)
	return cfg
}

// Normalize 返回补齐默认值后的配置。
func Normalize(cfg *Config) *Config {
	if cfg == nil {
		cfg = &Config{}
	}
	applyDefaults(cfg)
	return cfg
}

func applyDefaults(cfg *Config) {
	if cfg.Server == nil {
		cfg.Server = &ServerConfig{}
	}
	if cfg.Server.AgentRegistration == nil {
		cfg.Server.AgentRegistration = &AgentRegistrationConfig{}
	}
	if cfg.Server.Host == "" {
		cfg.Server.Host = "127.0.0.1" // 默认仅监听本地，更安全
	}
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 9000
	}

	if cfg.Database == nil {
		cfg.Database = &DatabaseConfig{}
	}
	if cfg.Database.Path == "" {
		cfg.Database.Path = "./data/cockpit.db"
	}

	if cfg.JWT == nil {
		cfg.JWT = &JWTConfig{}
	}
	if cfg.JWT.Secret == "" {
		cfg.JWT.Secret = "change-me"
	}
	if cfg.JWT.Expiration == 0 {
		cfg.JWT.Expiration = 24 * time.Hour
	}

	if cfg.Email == nil {
		cfg.Email = &EmailConfig{}
	}
	if cfg.Notification == nil {
		cfg.Notification = &NotificationConfig{}
	}

	if cfg.Probe == nil {
		cfg.Probe = &ProbeConfig{}
	}
	if cfg.Stacks == nil {
		cfg.Stacks = &StacksConfig{}
	}
	if cfg.Probe.Interval <= 0 {
		cfg.Probe.Interval = ProbeDefaultInterval
	}
	if cfg.Probe.Interval < ProbeMinInterval {
		cfg.Probe.Interval = ProbeMinInterval
	}
	if cfg.Probe.Interval > ProbeMaxInterval {
		cfg.Probe.Interval = ProbeMaxInterval
	}

	if cfg.Agent == nil {
		cfg.Agent = &AgentConfig{}
	}
	if cfg.Agent.APIKeyHeader == "" {
		cfg.Agent.APIKeyHeader = "X-API-Key"
	}
	if cfg.Agent.ExpireMinutes == 0 {
		cfg.Agent.ExpireMinutes = 5
	}

	if cfg.Inventory == nil {
		cfg.Inventory = &InventoryConfig{}
	}
	if cfg.RemoteControl == nil {
		cfg.RemoteControl = &RemoteControlConfig{}
	}

	if cfg.DNS == nil {
		cfg.DNS = &DNSConfig{}
	}
	if cfg.DNS.Cloudflare == nil {
		cfg.DNS.Cloudflare = &CloudflareDNSConfig{}
	}
	if cfg.DNS.DNSPod == nil {
		cfg.DNS.DNSPod = &DNSPodConfig{}
	}
	if cfg.DNS.AliDNS == nil {
		cfg.DNS.AliDNS = &AliDNSConfig{}
	}
	// env 优先于 yaml（secret 不落文件的惯例）
	if v := os.Getenv("CLOUDFLARE_API_TOKEN"); v != "" {
		cfg.DNS.Cloudflare.APIToken = v
	}
	if v := os.Getenv("DNSPOD_LOGIN_TOKEN"); v != "" {
		cfg.DNS.DNSPod.LoginToken = v
	}
	if v := os.Getenv("ALIYUN_ACCESS_KEY"); v != "" {
		cfg.DNS.AliDNS.AccessKey = v
	}
	if v := os.Getenv("ALIYUN_ACCESS_KEY_SECRET"); v != "" {
		cfg.DNS.AliDNS.SecretKey = v
	}

	if cfg.Overlay == nil {
		cfg.Overlay = &OverlayConfig{}
	}
	if cfg.Overlay.ZeroTier == nil {
		cfg.Overlay.ZeroTier = &ZeroTierCloudConfig{}
	}
	if cfg.Overlay.Tailscale == nil {
		cfg.Overlay.Tailscale = &TailscaleCloudConfig{}
	}
	if v := os.Getenv("ZEROTIER_API_TOKEN"); v != "" {
		cfg.Overlay.ZeroTier.APIToken = v
	}
	if v := os.Getenv("TAILSCALE_API_TOKEN"); v != "" {
		cfg.Overlay.Tailscale.APIToken = v
	}

	if cfg.Vault == nil {
		cfg.Vault = &VaultConfig{}
	}
	if cfg.Vault.Provider == "" {
		cfg.Vault.Provider = "local"
	}
	// persona token：env 优先于 yaml（secret 不落文件的惯例）；Providers 节
	// 惰性创建，local-only 部署的 yaml 不出现空节
	if v := os.Getenv("PERSONA_CONNECT_TOKEN"); v != "" {
		if cfg.Vault.Providers == nil {
			cfg.Vault.Providers = &VaultProvidersConfig{}
		}
		if cfg.Vault.Providers.Persona == nil {
			cfg.Vault.Providers.Persona = &PersonaVaultConfig{}
		}
		cfg.Vault.Providers.Persona.Token = v
	}
}
