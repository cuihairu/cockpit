package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/cuihairu/cockpit/internal/server"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
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
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Cockpit - 个人混合基础设施控制台")
	fmt.Println()
	fmt.Println("用法:")
	fmt.Println("  cockpit <command> [options]")
	fmt.Println()
	fmt.Println("命令:")
	fmt.Println("  server    启动 Cockpit Server")
	fmt.Println("  agent     启动 Cockpit Agent")
	fmt.Println("  version   显示版本信息")
	fmt.Println()
	fmt.Println("使用 'cockpit <command> -h' 查看具体命令的帮助")
}

func printVersion() {
	fmt.Println("Cockpit v0.1.0")
}

func handleServer() {
	cmd := flag.NewFlagSet("server", flag.ExitOnError)
	addr := cmd.String("addr", "0.0.0.0:8080", "监听地址")
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
