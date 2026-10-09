//go:build windows

package platform

import (
	"os"

	"github.com/cuihairu/cockpit/core/platform/windows"
)

// 编译期装配：GOOS 决定装哪个实现，运行时零分支。Host 契约在此组装，
// leaf 只出原始事实（基元返回值）——leaf 命名 root 的 Paths 类型会与
// 本文件的 import 成环。
func init() { Register(host{}) }

type host struct{ windows.Host }

func (host) Paths() Paths {
	c, d, l := windows.PathDirs()
	return Paths{ConfigDir: c, DataDir: d, LogDir: l}
}

func (host) Signals() []os.Signal { return windows.GracefulSignals() }
