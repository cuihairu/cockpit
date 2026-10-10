//go:build windows

package platform

import (
	"os"
	"time"

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

// Services 服务挂约（P7b）：SCM 交互层（leaf 原始事实 windows.Service）到
// 挂约实体的适配。
func (host) Services() ServiceManager { return svcMgr{} }

type svcMgr struct{}

func (svcMgr) List() ([]Service, error) {
	svcs, err := windows.List()
	if err != nil {
		return nil, err
	}
	out := make([]Service, 0, len(svcs))
	for _, s := range svcs {
		out = append(out, Service{
			Name:        s.Name,
			DisplayName: s.DisplayName,
			Status:      s.Status,
			StartType:   s.StartType,
		})
	}
	return out, nil
}

func (svcMgr) Action(name, action string, timeout time.Duration) error {
	return windows.Action(name, action, timeout)
}
