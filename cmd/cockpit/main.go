package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cuihairu/cockpit/internal/server"
)

const Version = "0.1.0"

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
	addr := flag.String("addr", "0.0.0.0:9000", "监听地址")
	dataDir := flag.String("data", "./data", "数据目录")
	showVersion := flag.Bool("version", false, "显示版本信息")
	flag.Parse()

	if *showVersion {
		printVersion()
		os.Exit(0)
	}

	s := server.NewServer(server.Config{
		Addr:    *addr,
		DataDir: *dataDir,
	})

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
	fmt.Println("  -addr string         # 监听地址 (默认 \"0.0.0.0:9000\")")
	fmt.Println("  -data string         # 数据目录 (默认 \"./data\")")
	fmt.Println("  -version             # 显示版本信息")
}

func printVersion() {
	fmt.Printf("Cockpit v%s\n", Version)
}

func handleServer() {
	cmd := flag.NewFlagSet("server", flag.ExitOnError)
	addr := cmd.String("addr", "0.0.0.0:9000", "监听地址")
	dataDir := cmd.String("data", "./data", "数据目录")
	help := cmd.Bool("h", false, "显示帮助")

	cmd.Parse(os.Args[2:])

	if *help {
		fmt.Println("启动 Cockpit Server")
		fmt.Println()
		cmd.PrintDefaults()
		os.Exit(0)
	}

	s := server.NewServer(server.Config{
		Addr:    *addr,
		DataDir: *dataDir,
	})

	if err := s.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleAgent() {
	fmt.Println("Agent command coming soon...")
	os.Exit(1)
}
