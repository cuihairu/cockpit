package platform

import "time"

// Service 平台原生服务观测实体（P7b 挂约，scm+launchd 口径的最大公约数，
// 全字符串——平台层不做枚举翻译，映射到业务模型由消费方完成）。linux
// systemd 不经此实体：systemctl argv 通道是业务插件事实（Commander 注入
// Linux CI 可测），非平台原生面。
type Service struct {
	Name        string // 服务名（操作一律按它：SCM 服务名 / launchd label）
	DisplayName string // 显示名（→ 业务模型 Description）
	Status      string // 状态原词（SCM: Running/Stopped/Start Pending/...）
	StartType   string // 自启态原词（SCM: Automatic/Manual/Disabled/...）
}

// ServiceManager 平台原生服务管理面：枚举 + 动词。语义对齐 SCM（start/stop
// 先查询幂等、restart 等待停止完成再拉起、enable/disable 改自启态；SCM 无
// reload，动词白名单由调用方收口）。
type ServiceManager interface {
	List() ([]Service, error)
	Action(name, action string, timeout time.Duration) error
}
