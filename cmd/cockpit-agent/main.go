package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

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
	secret := cmd.String("secret", "", "Agent 认证密钥 (可选，但推荐使用)")
	region := cmd.String("region", "", "地域 (可选)")
	zone := cmd.String("zone", "", "可用区 (可选)")
	labelsStr := cmd.String("labels", "", "标签 (可选)，格式: key1=value1,key2=value2,key3=[a,b,c]")
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
		fmt.Println("  cockpit-agent start -server ws://localhost:8080 -labels env=prod,services=[docker,k8s],gpu=true")
		fmt.Println("  cockpit-agent start -server ws://localhost:8080 -secret YOUR_SECRET_HERE")
		os.Exit(0)
	}

	if *server == "" {
		fmt.Println("错误: 必须指定 -server 参数")
		cmd.PrintDefaults()
		os.Exit(1)
	}

	// 解析标签
	labels := parseLabels(*labelsStr)

	cfg := agent.Config{
		ServerURL: *server,
		AgentID:   *agentID,
		Secret:    *secret,
		Region:    *region,
		Zone:      *zone,
		Labels:    labels,
	}

	a := agent.NewAgent(cfg)

	if err := a.Start(); err != nil {
		log.Fatalf("Agent error: %v", err)
	}
}

// parseLabels 解析标签字符串
// 格式: key1=value1,key2=[a,b,c],key3=true
func parseLabels(labelsStr string) map[string]interface{} {
	labels := make(map[string]interface{})
	if labelsStr == "" {
		return labels
	}

	for _, part := range strings.Split(labelsStr, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// 查找等号
		idx := strings.Index(part, "=")
		if idx == -1 {
			log.Printf("警告: 无效的标签格式: %s", part)
			continue
		}

		key := strings.TrimSpace(part[:idx])
		valueStr := strings.TrimSpace(part[idx+1:])

		// 解析值
		value := parseLabelValue(valueStr)
		labels[key] = value
	}

	return labels
}

// parseLabelValue 解析标签值
func parseLabelValue(valueStr string) interface{} {
	// 数组格式: [a,b,c]
	if strings.HasPrefix(valueStr, "[") && strings.HasSuffix(valueStr, "]") {
		inner := strings.TrimSuffix(strings.TrimPrefix(valueStr, "["), "]")
		if inner == "" {
			return []string{}
		}
		parts := strings.Split(inner, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	}

	// 布尔值
	lower := strings.ToLower(valueStr)
	if lower == "true" {
		return true
	}
	if lower == "false" {
		return false
	}

	// 数字
	if num, err := strconv.Atoi(valueStr); err == nil {
		return num
	}

	// 默认为字符串
	return valueStr
}
