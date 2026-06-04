package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// InitCmd init command
type InitCmd struct {
	Dir     string
	Config  string
	Example bool
}

// Run executes initialization
func (c *InitCmd) Run() error {
	// Determine target directory
	targetDir := c.Dir
	if targetDir == "" {
		targetDir = "."
	}

	// Create directory structure
	dirs := []string{
		filepath.Join(targetDir, "config"),
		filepath.Join(targetDir, "data"),
		filepath.Join(targetDir, "inventory"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
		fmt.Printf("✓ Created directory: %s\n", dir)
	}

	// Create config file
	configPath := c.Config
	if configPath == "" {
		configPath = filepath.Join(targetDir, "config", "cockpit.yaml")
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		if err := writeConfig(configPath); err != nil {
			return fmt.Errorf("write config file: %w", err)
		}
		fmt.Printf("✓ Created config: %s\n", configPath)
	} else {
		fmt.Printf("⊙ Config exists: %s\n", configPath)
	}

	// Create example inventory
	if c.Example {
		exampleInvPath := filepath.Join(targetDir, "inventory", "example.yaml")
		if _, err := os.Stat(exampleInvPath); os.IsNotExist(err) {
			if err := writeExampleInventory(exampleInvPath); err != nil {
				return fmt.Errorf("write example inventory: %w", err)
			}
			fmt.Printf("✓ Created example inventory: %s\n", exampleInvPath)
		}
	}

	fmt.Println("\nInitialization complete!")
	fmt.Printf("Config directory: %s\n", filepath.Join(targetDir, "config"))
	fmt.Printf("Data directory: %s\n", filepath.Join(targetDir, "data"))
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Edit config: " + configPath)
	fmt.Println("  2. Run server: ./cockpit server -config " + configPath)
	fmt.Println("  3. Sync inventory: ./cockpit sync -inventory inventory/example.yaml -db data/cockpit.db")

	return nil
}

func writeConfig(path string) error {
	yaml := `# Cockpit Configuration
server:
  host: 127.0.0.1
  port: 9000
  static_dir: ./web/dist

database:
  path: ./data/cockpit.db

inventory:
  path: ./inventory/example.yaml
  watch: false

jwt:
  secret: change-me-in-production
  expiration: 24h

# Optional email notification
# email:
#   enabled: true
#   smtp:
#     host: smtp.example.com
#     port: 587
#     username: your-email@example.com
#     password: your-password

# Optional notification service
# notification:
#   enabled: true
#   herald:
#     base_url: http://localhost:8080
`
	return os.WriteFile(path, []byte(yaml), 0644)
}

func writeExampleInventory(path string) error {
	yaml := `# Cockpit Infrastructure Inventory Example
version: v1

metadata:
  name: "My Home Lab"
  description: "Personal hybrid infrastructure"
  labels:
    environment: home

regions:
  home:
    name: "Home"
    description: "Home network infrastructure"
    zones:
      datacenter:
        name: "Data Center"
        agents:
          server01:
            name: "Server 01"
            hostname: "server01.home.local"
            ip: "192.168.1.10"
            capabilities:
              - docker
              - system

domains:
  example-local:
    id: "example-local"
    domain: "example.local"
    provider: "internal"

# Compute instances (VMs, containers, baremetal)
computeInstances:
  vm-web01:
    id: "vm-web01"
    name: "Web Server VM"
    type: "vm"
    agent: "server01"
    region: "home"
    zone: "datacenter"
    cpu: 2
    memory: 2048
    disk: 40

# Services to monitor
services:
  web-service:
    id: "web-service"
    name: "Web Service"
    type: "http"
    agent: "server01"
    url: "http://192.168.1.10:8080"
    interval: 60

# Gateways and routers
gateways:
  main-router:
    id: "main-router"
    name: "Main Router"
    type: "openwrt"
    agent: "server01"
    ipv4: "192.168.1.1"

# Storage
storages:
  nas-storage:
    id: "nas-storage"
    name: "NAS Storage"
    type: "nfs"
    agent: "server01"
    path: "/mnt/nas"
`
	return os.WriteFile(path, []byte(yaml), 0644)
}
