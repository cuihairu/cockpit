package config

import (
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
	Host      string `yaml:"host"`
	Port      int    `yaml:"port"`
	StaticDir string `yaml:"static_dir"`
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
	Enabled bool                    `yaml:"enabled"`
	Herald  *HeraldConfig           `yaml:"herald"`
	Events  map[string]*EventConfig `yaml:"events"`
}

// HeraldConfig Herald 服务配置
type HeraldConfig struct {
	BaseURL string        `yaml:"base_url"`
	Timeout time.Duration `yaml:"timeout"`
}

// EventConfig 事件配置
type EventConfig struct {
	Type    string `yaml:"type"`
	Enabled bool   `yaml:"enabled"`
}

// AgentConfig Agent 配置
type AgentConfig struct {
	APIKeyHeader string `yaml:"api_key_header"`
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

	applyDefaults(&cfg)
	return &cfg, nil
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

	if cfg.Agent == nil {
		cfg.Agent = &AgentConfig{}
	}
	if cfg.Agent.APIKeyHeader == "" {
		cfg.Agent.APIKeyHeader = "X-API-Key"
	}

	if cfg.Inventory == nil {
		cfg.Inventory = &InventoryConfig{}
	}
	if cfg.RemoteControl == nil {
		cfg.RemoteControl = &RemoteControlConfig{}
	}
}
