//go:build !linux && !darwin && !windows

package platform

import "github.com/cuihairu/cockpit/core/platform/other"

// 编译期装配：GOOS 决定装哪个实现，运行时零分支
func init() { Register(other.Host{}) }
