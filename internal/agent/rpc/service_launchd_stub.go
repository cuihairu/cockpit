//go:build !darwin

package rpc

import "fmt"

// launchd 服务管理 stub（service-design.md D10.5）：非 darwin 平台编译占位。
// 正常路径下探测门控（agent.go 仅 GOOS=darwin 上报 launchd capability）
// 根本不会注册本 provider，Call 是防御层显式报错。
type LaunchdServiceProvider struct{}

func NewLaunchdServiceProvider(run Commander) *LaunchdServiceProvider {
	return &LaunchdServiceProvider{}
}

func (p *LaunchdServiceProvider) Type() string { return "service" }

func (p *LaunchdServiceProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	return nil, fmt.Errorf("launchd service backend requires a darwin agent build")
}
