package main

import (
	"flag"
	"fmt"
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

func loadConfig(configPath string) *config.Config {
	if configPath != "" {
		cfg, err := config.Load(configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
		log.Printf("Loaded config: %s", configPath)
		return cfg
	}

	for _, path := range defaultConfigPaths {
		if _, err := os.Stat(path); err == nil {
			cfg, err := config.Load(path)
			if err != nil {
				log.Printf("Warning: config exists but failed to load %s: %v", path, err)
				continue
			}
			log.Printf("Loaded config: %s", path)
			return cfg
		}
	}

	log.Println("No config found, using defaults")
	return config.LoadOrDefault("")
}

func main() {
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
	case "init":
		handleInit()
	case "sync":
		handleSync()
	case "status":
		handleStatus()
	case "version", "-v", "--version":
		printVersion()
	default:
		if os.Args[1][0] == '-' {
			handleServerDefault()
			return
		}
		fmt.Printf("Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func handleServerDefault() {
	configPath := flag.String("config", "", "Config file path")
	showVersion := flag.Bool("version", false, "Show version")
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
	fmt.Println("Cockpit - Personal Hybrid Infrastructure Console")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  cockpit [command] [options]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  init       Initialize configuration and directories")
	fmt.Println("  server     Start Cockpit Server")
	fmt.Println("  agent      Start Cockpit Agent (use cockpit-agent instead)")
	fmt.Println("  sync       Sync inventory to database")
	fmt.Println("  status     Show status")
	fmt.Println("  version    Show version")
	fmt.Println()
	fmt.Println("Server options:")
	fmt.Println("  -config string       Config file path (default \"./config/cockpit.yaml\")")
	fmt.Println("  -version             Show version")
}

func printVersion() {
	fmt.Printf("Cockpit v%s\n", Version)
}

func handleServer() {
	cmd := flag.NewFlagSet("server", flag.ExitOnError)
	configPath := cmd.String("config", "", "Config file path")
	help := cmd.Bool("h", false, "Show help")

	cmd.Parse(os.Args[2:])

	if *help {
		fmt.Println("Start Cockpit Server")
		fmt.Println()
		cmd.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  cockpit server")
		fmt.Println("  cockpit server -config /path/to/config.yaml")
		os.Exit(0)
	}

	cfg := loadConfig(*configPath)
	s := server.NewServer(cfg)

	if err := s.Start(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func handleAgent() {
	fmt.Println("Agent functionality has been moved to cockpit-agent")
	fmt.Println()
	fmt.Println("Use the following to start an agent:")
	fmt.Println("  ./cockpit-agent start -server ws://your-server.com:9000")
	fmt.Println()
	fmt.Println("Or download binaries from:")
	fmt.Println("  https://github.com/cuihairu/cockpit/releases")
	fmt.Println()
	os.Exit(1)
}

func handleInit() {
	cmd := flag.NewFlagSet("init", flag.ExitOnError)
	dir := cmd.String("dir", "", "Target directory (default: current)")
	configPath := cmd.String("config", "", "Config file path")
	example := cmd.Bool("example", false, "Create example inventory")
	help := cmd.Bool("h", false, "Show help")

	cmd.Parse(os.Args[2:])

	if *help {
		fmt.Println("Initialize Cockpit configuration")
		fmt.Println()
		cmd.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  cockpit init")
		fmt.Println("  cockpit init -dir /path/to/project -example")
		os.Exit(0)
	}

	initCmd := &cli.InitCmd{
		Dir:     *dir,
		Config:  *configPath,
		Example: *example,
	}

	if err := initCmd.Run(); err != nil {
		log.Fatalf("Init failed: %v", err)
	}
}

func handleSync() {
	cmd := flag.NewFlagSet("sync", flag.ExitOnError)
	configPath := cmd.String("config", "", "Config file path")
	inventoryPath := cmd.String("inventory", "", "Inventory file path")
	dbPath := cmd.String("db", "", "Database path (overrides config)")
	help := cmd.Bool("h", false, "Show help")

	cmd.Parse(os.Args[2:])

	if *help {
		fmt.Println("Sync inventory to database")
		fmt.Println()
		cmd.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  cockpit sync -config config/cockpit.yaml -inventory inventory/example.yaml")
		fmt.Println("  cockpit sync -inventory inventory/example.yaml -db /path/to/cockpit.db")
		os.Exit(0)
	}

	syncCmd := &cli.SyncCmd{
		Config:    *configPath,
		Inventory: *inventoryPath,
		DBPath:    *dbPath,
	}

	if err := syncCmd.Run(); err != nil {
		log.Fatalf("Sync failed: %v", err)
	}
}

func handleStatus() {
	cmd := flag.NewFlagSet("status", flag.ExitOnError)
	dbPath := cmd.String("db", "", "Database file path")
	help := cmd.Bool("h", false, "Show help")

	cmd.Parse(os.Args[2:])

	if *help {
		fmt.Println("Show Cockpit status")
		fmt.Println()
		cmd.PrintDefaults()
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  cockpit status")
		fmt.Println("  cockpit status -db /path/to/cockpit.db")
		os.Exit(0)
	}

	statusCmd := &cli.StatusCmd{
		DBPath: *dbPath,
	}

	if err := statusCmd.Run(); err != nil {
		log.Fatalf("Status query failed: %v", err)
	}
}
