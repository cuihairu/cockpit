package main

import (
	"flag"
	"io"

	"github.com/cuihairu/cockpit/internal/agent"
)

// Windows 服务面与平台无关的部分：`service install` 的参数绑定与
// BinaryPathName 参数还原。两者是纯函数，故不带构建标签——放这里才能被
// linux 侧 CI 真跑到（service_windows.go 整文件带 windows 标签，CI 无
// windows runner，那边只有 nightly 真机走查覆盖）。
//
// 这段是安装路径的口径中枢：`service install` 的入参 → StartCmd →
// svcArgsFrom → 注册进服务的命令行 → SCM 重启时再解析回来。任一环的参数名
// 或顺序漂移都只在真机装机时暴露，故在此处做往返断言。

// svcBind 把 start 同款参数绑到 StartCmd（service install 透传连接参数）。
// flag 解析错误原样返回（usage 已在此处给定，与 handleStart 的文案一致）。
func svcBind(args []string) (*agent.StartCmd, error) {
	cmd := &agent.StartCmd{Version: version}
	fs := flag.NewFlagSet("service", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cmd.BindWithUsage(fs, agent.StartUsage{
		Server:  "Server WebSocket 地址 (必需)",
		ID:      "Agent ID (可选，默认基于 machine-id 自动生成，重启不变)",
		Secret:  "Agent 认证密钥 (可选，但推荐使用)",
		Region:  "地域 (可选)",
		Zone:    "可用区 (可选)",
		Labels:  "标签 (可选)，格式: key1=value1,key2=value2,key3=[a,b,c]",
		SSHKeys: "SSH 私钥目录 (可选，默认 ~/.ssh/)",
	})
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return cmd, nil
}

// svcArgsFrom 从 StartCmd 还原服务命令行参数（只带非空项，保持 binPath 干净）。
// 首两段 `service run` 是 SCM 入口动词（agentService.Execute 会剥掉）。
func svcArgsFrom(c *agent.StartCmd) []string {
	args := []string{"service", "run", "-server", c.Server}
	if c.ID != "" {
		args = append(args, "-id", c.ID)
	}
	if c.Secret != "" {
		args = append(args, "-secret", c.Secret)
	}
	if c.Region != "" {
		args = append(args, "-region", c.Region)
	}
	if c.Zone != "" {
		args = append(args, "-zone", c.Zone)
	}
	if c.Labels != "" {
		args = append(args, "-labels", c.Labels)
	}
	if c.SSHKeys != "" {
		args = append(args, "-ssh-keys", c.SSHKeys)
	}
	return args
}

// svcStripVerbs 剥掉 SCM 入口动词：注册的命令行首段是 `service run`
// （本文件 svcArgsFrom 产出），而 install.ps1 存量直挂的是 `start`
func svcStripVerbs(args []string) []string {
	for len(args) > 0 && (args[0] == "service" || args[0] == "run" || args[0] == "start") {
		args = args[1:]
	}
	return args
}