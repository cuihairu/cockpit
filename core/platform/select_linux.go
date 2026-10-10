//go:build linux

package platform

import (
	"os"

	"github.com/cuihairu/cockpit/core/platform/linux"
)

// 编译期装配：GOOS 决定装哪个实现，运行时零分支。Host 契约在此组装，
// leaf 只出原始事实（基元返回值）——leaf 命名 root 的 Paths 类型会与
// 本文件的 import 成环。
func init() { Register(host{}) }

type host struct{ linux.Host }

func (host) Paths() Paths {
	c, d, l := linux.PathDirs()
	return Paths{ConfigDir: c, DataDir: d, LogDir: l}
}

// Services 服务挂约（P7b）：恒 nil——systemd 服务管理是 systemctl argv
// 业务插件通道（Commander 注入 Linux CI 可测），非平台事实。
func (host) Services() ServiceManager { return nil }

func (host) Signals() []os.Signal { return linux.GracefulSignals() }
