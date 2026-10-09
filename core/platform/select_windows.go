//go:build windows

package platform

import "github.com/cuihairu/cockpit/core/platform/windows"

// 编译期装配：GOOS 决定装哪个实现，运行时零分支
func init() { Register(windows.Host{}) }
