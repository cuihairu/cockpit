package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/server"
)

const Version = "0.1.0"

// 默认配置文件搜索路径
var defaultConfigPaths = []string{
	"./config/cockpit.yaml",
	"./cockpit.yaml",
	"/etc/cockpit/config.yaml",
}

func loadConfig(configPath string) *config.Config {
	// 如果指定了配置文件路径，直接加载
	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			log.Fatalf("加载配置文件失败: %v", err)
		}
		log.Printf("已加载配置文件: %s", configPath)
		return cfg
	}

	// 尝试默认路径
	for _, path := range defaultConfigPaths {
		if _, err := os.Stat(path); err == nil {
			cfg, err := config.Load(path)
			if err != nil {
				log.Printf("警告: 配置文件 %s 存在但加载失败: %v", path, err)
				continue
			}
			log.Printf("已加载配置文件: %s", path)
			return cfg
		}
	}

	// 未找到配置文件，使用默认配置
	log.Println("未找到配置文件，使用默认配置")
	return config.LoadOrDefault("")
}

func main() {
	// 默认以 server 模式启动
	if len(os.Args) < 2 {
		handleServerDefault()
		return
	}

	command := os.Args[1]

	switch command {
	case "server":
		handleServer()
	case "agent":
		handleAgent()
	case "version", "-v", "--version":
		printVersion()
	default:
		// 如果是参数形式（如 -addr），则默认启动 server
		if os.Args[1][0] == '-' {
			handleServerDefault()
			return
		}
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

// handleServerDefault 处理默认 server 启动
func handleServerDefault() {
	configPath := flag.String("config", "", "配置文件路径")
	showVersion := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	cfg := loadConfig(*configPath)
	s := server.NewServer(cfg)

	if err := s.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func printUsage() {
	fmt.Println("Cockpit - 个人混合基础设施控制台")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  cockpit [命令] [选项]")
	fmt.Println("  cockpit              # 默认启动 server")
	fmt.Println("  cockpit server       # 启动 Cockpit Server")
	fmt.Println("  cockpit agent        # 启动 Cockpit Agent")
	fmt.Println("  cockpit version      # 显示版本信息")
	fmt.Println()
	fmt.Println("Server 选项:")
	fmt.Println("  -config string       # 配置文件路径 (默认 \"./config/cockpit.yaml\")")
	fmt.Println("  -version             # 显示版本信息")
}

func printVersion() {
	fmt.Printf("Cockpit v%s\n", Version)
}

func handleServer() {
	cmd := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := cmd.String("config", "", "配置文件路径")
	help := cmd.Bool("h", false, "显示帮助")

	cmd.Parse(os.Args[2:])

	if *help {
		fmt.Println("启动 Cockpit Server")
		fmt.Println()
		cmd.PrintDefaults()
		os.Exit(0)
	}

	cfg := loadConfig(*configPath)
	s := server.NewServer(cfg)

	if err := s.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleAgent() {
	fmt.Println("Agent command coming soon...")
	os.Exit(1)
}
