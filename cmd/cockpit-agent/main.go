package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cuihairu/cockpit/internal/agent"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "start":
		handleStart()
	case "version", "-v", "--version":
		printVersion()
	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Cockpit Agent - 个人混合基础设施监控代理")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  cockpit-agent <command> [options]")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  start     启动 Agent")
	fmt.Println("  version   显示版本信息")
	fmt.Println()
	fmt.Println("使用 'cockpit-agent <command> -h' 查看具体命令的帮助")
}

func printVersion() {
	fmt.Println("Cockpit Agent v0.1.0")
}

func handleStart() {
	cmd := flag.NewFlagSet("start", flag.ExitOnError)
	server := cmd.String("server", "", "Server WebSocket 地址 (必需)")
	agentID := cmd.String("id", "", "Agent ID (可选，默认自动生成)")
	region := cmd.String("region", "", "地域 (可选)")
	zone := cmd.String("zone", "", "可用区 (可选)")
	help := cmd.Bool("h", false, "显示帮助")

	cmd.Parse(os.Args[2:])

	if *help {
		fmt.Println("启动 Cockpit Agent")
		fmt.Println()
		cmd.PrintDefaults()
		fmt.Println()
		fmt.Println("示例:")
		fmt.Println("  cockpit-agent start -server ws://localhost:8080")
		fmt.Println("  cockpit-agent start -server wss://example.com:8080 -region jiangsu-huaian -zone datacenter-a")
		os.Exit(0)
	}

	if *server == "" {
		fmt.Println("错误: 必须指定 -server 参数")
		cmd.PrintDefaults()
		os.Exit(1)
	}

	cfg := agent.Config{
		ServerURL: *server,
		AgentID:   *agentID,
		Region:    *region,
		Zone:      *zone,
	}

	a := agent.NewAgent(cfg)

	if err := a.Start(); err != nil {
		log.Fatalf("Agent error: %v", err)
	}
}
