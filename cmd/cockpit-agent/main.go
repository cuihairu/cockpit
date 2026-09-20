package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/cuihairu/cockpit/internal/agent"
)

func main() {
	os.Exit(run(os.Args, os.Stdout))
}

// run 命令分发，返回进程退出码（main 只做薄壳，逻辑全在此可测）
func run(args []string, stdout io.Writer) int {
	if len(args) < 2 {
		printUsage(stdout)
		return 1
	}

	command := args[1]

	switch command {
	case "start":
		return handleStart(args[2:], stdout)
	case "version", "-v", "--version":
		printVersion(stdout)
		return 0
	default:
		fmt.Fprintf(stdout, "Unknown command: %s\n\n", command)
		printUsage(stdout)
		return 1
	}
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Cockpit Agent - 个人混合基础设施监控代理")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "用法:")
	fmt.Fprintln(w, "  cockpit-agent <command> [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "命令:")
	fmt.Fprintln(w, "  start     启动 Agent")
	fmt.Fprintln(w, "  version   显示版本信息")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "使用 'cockpit-agent <command> -h' 查看具体命令的帮助")
}

func printVersion(w io.Writer) {
	fmt.Fprintln(w, "Cockpit Agent v0.1.0")
}

// handleStart `cockpit-agent start [-server ws://...]`
func handleStart(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("start", flag.ExitOnError)
	startCmd := &agent.StartCmd{}
	startCmd.BindWithUsage(fs, agent.StartUsage{
		Server: "Server WebSocket 地址 (必需)",
		ID:     "Agent ID (可选，默认自动生成)",
		Secret: "Agent 认证密钥 (可选，但推荐使用)",
		Region: "地域 (可选)",
		Zone:   "可用区 (可选)",
		Labels: "标签 (可选)，格式: key1=value1,key2=value2,key3=[a,b,c]",
	})
	help := fs.Bool("h", false, "显示帮助")

	fs.Parse(args)

	if *help {
		fmt.Fprintln(stdout, "启动 Cockpit Agent")
		fmt.Fprintln(stdout)
		fs.PrintDefaults()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "示例:")
		fmt.Fprintln(stdout, "  cockpit-agent start -server ws://localhost:9000/ws")
		fmt.Fprintln(stdout, "  cockpit-agent start -server wss://example.com:9000/ws -region jiangsu-huaian -zone datacenter-a")
		fmt.Fprintln(stdout, "  cockpit-agent start -server ws://localhost:9000/ws -labels env=prod,services=[docker,k8s],gpu=true")
		fmt.Fprintln(stdout, "  cockpit-agent start -server ws://localhost:9000/ws -secret YOUR_SECRET_HERE")
		return 0
	}

	if err := startCmd.Validate(); err != nil {
		fmt.Fprintf(stdout, "错误: %v\n", err)
		fs.PrintDefaults()
		return 1
	}

	if err := startCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	return 0
}
