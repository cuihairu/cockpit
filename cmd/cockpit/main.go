package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"

	"github.com/cuihairu/cockpit/internal/cli"
	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/server"
)

const Version = "0.1.0"

var defaultConfigPaths = []string{
	"./config/cockpit.yaml",
	"./cockpit.yaml",
	"/etc/cockpit/config.yaml",
}

// loadConfig 按显式路径加载；否则按默认路径探测；都缺失时返回默认配置。
// （不使用 log.Fatal，交由调用方决定退出方式——可测。）
func loadConfig(configPath string, stdout io.Writer) (*config.Config, error) {
	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			return nil, err
		}
		log.Printf("Loaded config: %s", configPath)
		return cfg, nil
	}

	for _, path := range defaultConfigPaths {
		if _, err := os.Stat(path); err == nil {
			cfg, err := config.Load(path)
			if err != nil {
				log.Printf("Warning: config exists but failed to load %s: %v", path, err)
				continue
			}
			log.Printf("Loaded config: %s", path)
			return cfg, nil
		}
	}

	fmt.Fprintln(stdout, "No config found, using defaults")
	return config.LoadOrDefault(""), nil
}

func main() {
	os.Exit(run(os.Args, os.Stdout))
}

// run 命令分发，返回进程退出码（main 只做薄壳，逻辑全在此可测）
func run(args []string, stdout io.Writer) int {
	if len(args) < 2 {
		return runServerDefault(args[1:], stdout)
	}

	command := args[1]

	switch command {
	case "server":
		return runServer(args[2:], stdout)
	case "agent":
		return runAgent(args[2:], stdout)
	case "init":
		return runInit(args[2:], stdout)
	case "sync":
		return runSync(args[2:], stdout)
	case "status":
		return runStatus(args[2:], stdout)
	case "version", "-v", "--version":
		printVersion(stdout)
		return 0
	default:
		if args[1][0] == '-' {
			return runServerDefault(args[1:], stdout)
		}
		fmt.Fprintf(stdout, "Unknown command: %s\n\n", command)
		printUsage(stdout)
		return 1
	}
}

// runServerDefault 无命令/直接跟 flag：按 server 启动（兼容旧用法 `cockpit -config x`）
func runServerDefault(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("cockpit", flag.ExitOnError)
	configPath := fs.String("config", "", "Config file path")
	showVersion := fs.Bool("version", false, "Show version")
	fs.Parse(args)

	if *showVersion {
		printVersion(stdout)
		return 0
	}

	return startServer(*configPath, stdout)
}

// startServer 加载配置并启动 server（阻塞直到出错）
func startServer(configPath string, stdout io.Writer) int {
	cfg, err := loadConfig(configPath, stdout)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		return 1
	}
	s := server.NewServer(cfg)

	if err := s.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		return 1
	}
	return 0
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, "Cockpit - Personal Hybrid Infrastructure Console")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  cockpit [command] [options]")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  init       Initialize configuration and directories")
	fmt.Fprintln(w, "  server     Start Cockpit Server")
	fmt.Fprintln(w, "  agent      Start Cockpit Agent")
	fmt.Fprintln(w, "  sync       Sync inventory to database")
	fmt.Fprintln(w, "  status     Show status")
	fmt.Fprintln(w, "  version    Show version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Server options:")
	fmt.Fprintln(w, "  -config string       Config file path (default \"./config/cockpit.yaml\")")
	fmt.Fprintln(w, "  -version             Show version")
}

func printVersion(w io.Writer) {
	fmt.Fprintf(w, "Cockpit v%s\n", Version)
}

// runServer `cockpit server [-config path]`
func runServer(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := fs.String("config", "", "Config file path")
	help := fs.Bool("h", false, "Show help")

	fs.Parse(args)

	if *help {
		fmt.Fprintln(stdout, "Start Cockpit Server")
		fmt.Fprintln(stdout)
		fs.PrintDefaults()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Examples:")
		fmt.Fprintln(stdout, "  cockpit server")
		fmt.Fprintln(stdout, "  cockpit server -config /path/to/config.yaml")
		return 0
	}

	return startServer(*configPath, stdout)
}

// runAgent `cockpit agent [start] [-server ws://...]`（与 cockpit-agent start 共用 cli.AgentStartCmd）
func runAgent(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("agent", flag.ExitOnError)
	startCmd := &cli.AgentStartCmd{}
	startCmd.Bind(fs)
	help := fs.Bool("h", false, "Show help")

	if len(args) > 0 && args[0] == "start" {
		args = args[1:]
	}

	fs.Parse(args)

	if *help {
		fmt.Fprintln(stdout, "Start Cockpit Agent")
		fmt.Fprintln(stdout)
		fs.PrintDefaults()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Examples:")
		fmt.Fprintln(stdout, "  cockpit agent -server ws://localhost:9000/ws")
		fmt.Fprintln(stdout, "  cockpit agent -server wss://example.com:9000/ws -region home -zone dc-a")
		fmt.Fprintln(stdout, "  cockpit agent start -server ws://localhost:9000/ws")
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Compatibility:")
		fmt.Fprintln(stdout, "  cockpit-agent start ... remains supported")
		return 0
	}

	if err := startCmd.Validate(); err != nil {
		fmt.Fprintf(stdout, "Error: %v\n", err)
		fs.PrintDefaults()
		return 1
	}

	if err := startCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		return 1
	}
	return 0
}

// runInit `cockpit init [-dir path] [-config path] [-example]`
func runInit(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("init", flag.ExitOnError)
	dir := fs.String("dir", "", "Target directory (default: current)")
	configPath := fs.String("config", "", "Config file path")
	example := fs.Bool("example", false, "Create example inventory")
	help := fs.Bool("h", false, "Show help")

	fs.Parse(args)

	if *help {
		fmt.Fprintln(stdout, "Initialize Cockpit configuration")
		fmt.Fprintln(stdout)
		fs.PrintDefaults()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Examples:")
		fmt.Fprintln(stdout, "  cockpit init")
		fmt.Fprintln(stdout, "  cockpit init -dir /path/to/project -example")
		return 0
	}

	initCmd := &cli.InitCmd{
		Dir:     *dir,
		Config:  *configPath,
		Example: *example,
	}

	if err := initCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Init failed: %v\n", err)
		return 1
	}
	return 0
}

// runSync `cockpit sync [-config path] [-inventory path] [-db path]`
func runSync(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("sync", flag.ExitOnError)
	configPath := fs.String("config", "", "Config file path")
	inventoryPath := fs.String("inventory", "", "Inventory file path")
	dbPath := fs.String("db", "", "Database path (overrides config)")
	help := fs.Bool("h", false, "Show help")

	fs.Parse(args)

	if *help {
		fmt.Fprintln(stdout, "Sync inventory to database")
		fmt.Fprintln(stdout)
		fs.PrintDefaults()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Examples:")
		fmt.Fprintln(stdout, "  cockpit sync -config config/cockpit.yaml -inventory inventory/example.yaml")
		fmt.Fprintln(stdout, "  cockpit sync -inventory inventory/example.yaml -db /path/to/cockpit.db")
		return 0
	}

	syncCmd := &cli.SyncCmd{
		Config:    *configPath,
		Inventory: *inventoryPath,
		DBPath:    *dbPath,
	}

	if err := syncCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Sync failed: %v\n", err)
		return 1
	}
	return 0
}

// runStatus `cockpit status [-db path]`
func runStatus(args []string, stdout io.Writer) int {
	fs := flag.NewFlagSet("status", flag.ExitOnError)
	dbPath := fs.String("db", "", "Database file path")
	help := fs.Bool("h", false, "Show help")

	fs.Parse(args)

	if *help {
		fmt.Fprintln(stdout, "Show Cockpit status")
		fmt.Fprintln(stdout)
		fs.PrintDefaults()
		fmt.Fprintln(stdout)
		fmt.Fprintln(stdout, "Examples:")
		fmt.Fprintln(stdout, "  cockpit status")
		fmt.Fprintln(stdout, "  cockpit status -db /path/to/cockpit.db")
		return 0
	}

	statusCmd := &cli.StatusCmd{
		DBPath: *dbPath,
	}

	if err := statusCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Status query failed: %v\n", err)
		return 1
	}
	return 0
}
