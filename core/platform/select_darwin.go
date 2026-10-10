//go:build darwin

package platform

import (
	"os"

	"github.com/cuihairu/cockpit/core/platform/darwin"
)

// 编译期装配：GOOS 决定装哪个实现，运行时零分支。Host 契约在此组装，
// leaf 只出原始事实（基元返回值）——leaf 命名 root 的 Paths 类型会与
// 本文件的 import 成环。
func init() { Register(host{}) }

type host struct{ darwin.Host }

func (host) Paths() Paths {
	c, d, l := darwin.PathDirs()
	return Paths{ConfigDir: c, DataDir: d, LogDir: l}
}

// Services 服务挂约（P7b）：恒 nil——launchd 交互在 leaf 是 launchctl argv
// + plist 目录原语，解析留业务层保 Linux CI 可测；launchctl 动词语义
// （bootout/kickstart）与 SCM 五动词不同构，不硬套 ServiceManager。
func (host) Services() ServiceManager { return nil }

func (host) Signals() []os.Signal { return darwin.GracefulSignals() }
