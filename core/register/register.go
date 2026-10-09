// Package register 注册上线（agent-core ① 通用能力层）：ID 派生、注册
// 报文组装、首包直写、等响应、放行回调。平台事实（machine-id/IP/虚拟化）
// 与传输由装配方注入（这些探测在插件宿主侧，P4 平台层收编前由 Agent 供
// 给），core 零业务、零平台分支。依赖 internal/protocol 为白名单例外。
package register

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/cuihairu/cockpit/internal/protocol"
)

// Config 注册静态配置（Agent Config 的注册面投影）。
type Config struct {
	AgentID  string
	Bias     int
	Secret   string
	Region   string
	Zone     string
	Labels   map[string]interface{}
	Metadata map[string]interface{}
	Version  string
}

// Facts 平台事实注入口：machine-id 派生 / IP / 虚拟化检测留在宿主侧，
// 这里只消费结果。
type Facts struct {
	MachineID      func() string
	PublicIP       func() string
	LocalIPs       func() []string
	Virtualization func() interface{}
}

// Registrar 注册上线。
type Registrar struct {
	cfg   Config
	facts Facts

	// write 注册首包直写（走统一上行 WriteNow：注册完成前放行门不放行）
	write func(*protocol.Message) error
	// readResp 等注册响应（装配方锁内快照 conn 后读——Stop/reconnect 可
	// 并发把 conn 置 nil，裸读会把 nil 传进 ReadMessage 导致 panic）
	readResp func() (*protocol.Message, error)
	// onAccepted 注册被接受后的放行回调
	onAccepted func()
}

// New 装配。
func New(cfg Config, facts Facts,
	write func(*protocol.Message) error,
	readResp func() (*protocol.Message, error),
	onAccepted func()) *Registrar {
	return &Registrar{cfg: cfg, facts: facts, write: write, readResp: readResp, onAccepted: onAccepted}
}

// DeriveID ID 派生：显式 ID 优先（bias 追加后缀）；否则 machine-id 前 8
// 字符 + hostname（重启不变）；无 machine-id 回退 hostname+随机（每次重
// 启变化）；bias 一律追加后缀。
func DeriveID(cfg Config, machineID string, hostname string) string {
	if cfg.AgentID != "" {
		if cfg.Bias > 0 {
			return fmt.Sprintf("%s-%d", cfg.AgentID, cfg.Bias)
		}
		return cfg.AgentID
	}
	id := ""
	if machineID != "" {
		// 取 machine-id 前 8 字符，可读且足够区分
		if len(machineID) > 8 {
			machineID = machineID[:8]
		}
		id = fmt.Sprintf("agent-%s-%s", hostname, machineID)
	} else {
		id = protocol.GenerateIDWithPrefix("agent-" + hostname)
	}
	if cfg.Bias > 0 {
		id = fmt.Sprintf("%s-%d", id, cfg.Bias)
	}
	return id
}

// DetectLocation 位置：cfg 指定优先，缺省回退环境变量
// COCKPIT_REGION/COCKPIT_ZONE，最终 unknown/unknown。
func DetectLocation(cfg Config) protocol.Location {
	loc := protocol.Location{
		Region: cfg.Region,
		Zone:   cfg.Zone,
	}
	if loc.Region == "" {
		if region := os.Getenv("COCKPIT_REGION"); region != "" {
			loc.Region = region
		} else {
			loc.Region = "unknown"
		}
	}
	if loc.Zone == "" {
		if zone := os.Getenv("COCKPIT_ZONE"); zone != "" {
			loc.Zone = zone
		} else {
			loc.Zone = "unknown"
		}
	}
	return loc
}

// Register 组装注册报文并直写首包，等待响应；被接受后回调 onAccepted。
// 返回派生的 ID 与位置（装配方落自己的字段供心跳复用）。
func (r *Registrar) Register(startedAt time.Time, caps []protocol.Capability,
	services []protocol.RemoteServicePayload) (string, protocol.Location, error) {
	hostname, _ := os.Hostname()
	mid := ""
	if r.facts.MachineID != nil {
		mid = r.facts.MachineID()
	}
	id := DeriveID(r.cfg, mid, hostname)
	loc := DetectLocation(r.cfg)

	// 平台事实注入口可省（装配面宽容：最小装配只需 write/readResp）
	var ip string
	if r.facts.PublicIP != nil {
		ip = r.facts.PublicIP()
	}
	var ips []string
	if r.facts.LocalIPs != nil {
		ips = r.facts.LocalIPs()
	}
	var virt interface{}
	if r.facts.Virtualization != nil {
		virt = r.facts.Virtualization()
	}

	payload := map[string]any{
		"agentId":        id,
		"secret":         r.cfg.Secret,
		"location":       loc,
		"capabilities":   caps,
		"hostname":       hostname,
		"ip":             ip,
		"localIps":       ips,
		"virtualization": virt,
		"labels":         r.cfg.Labels,
		"metadata":       r.cfg.Metadata,
		// 版本 / 进程启动时刻 / 服务面：主机列表的元信息列与协议入口
		// 数据源（此前只在 capabilities 里带一份启动时快照，运行期开服务
		// 不会反映）
		"version":   r.cfg.Version,
		"startedAt": startedAt.Unix(),
		"services":  services,
	}

	// 派生结果随错误一并返回：原 agent.register 语义是 ID/位置在首包
	// 直写前即落定（写失败后调用方仍可读到），保持一致
	if err := r.write(protocol.NewMessage(protocol.MessageTypeRegister, payload)); err != nil {
		return id, loc, err
	}

	log.Printf("Registered as agent: %s at %s/%s", id, loc.Region, loc.Zone)

	resp, err := r.readResp()
	if err != nil {
		return id, loc, err
	}
	if resp.Type != protocol.MessageTypeRegister {
		return id, loc, fmt.Errorf("expected register response, got: %s", resp.Type)
	}

	if r.onAccepted != nil {
		r.onAccepted()
	}
	log.Printf("Registration accepted")
	return id, loc, nil
}
