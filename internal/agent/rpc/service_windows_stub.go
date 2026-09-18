//go:build !windows

package rpc

import "fmt"

// Windows SCM 服务管理 stub（service-design.md D9.5）：非 Windows 平台
// 编译占位。正常路径下探测门控（agent.go 仅 GOOS=windows 上报 windows-scm
// capability）根本不会注册本 provider，Call 是防御层显式报错。
type WindowsServiceProvider struct{}

func NewWindowsServiceProvider() *WindowsServiceProvider { return &WindowsServiceProvider{} }

func (p *WindowsServiceProvider) Type() string { return "service" }

func (p *WindowsServiceProvider) Call(action string, params map[string]interface{}) (interface{}, error) {
	return nil, fmt.Errorf("windows-scm service backend requires a windows agent build")
}
