package cli

import (
	"fmt"
	"time"

	"github.com/cuihairu/cockpit/internal/storage"
)

// StatusCmd status command
type StatusCmd struct {
	DBPath string
}

// Run executes status query
func (c *StatusCmd) Run() error {
	dbPath := c.DBPath
	if dbPath == "" {
		dbPath = "./data/cockpit.db"
	}

	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	fmt.Println("=== Cockpit Status ===")
	fmt.Println()
	fmt.Printf("Database: %s\n\n", dbPath)

	// Agent summary
	agents, err := db.ListAgents()
	if err != nil {
		return fmt.Errorf("list agents: %w", err)
	}

	onlineCount := 0
	for _, a := range agents {
		if a.Status == "online" || isRecentlyActive(a.LastSeen) {
			onlineCount++
		}
	}

	fmt.Printf("Agents: %d (online: %d, offline: %d)\n", len(agents), onlineCount, len(agents)-onlineCount)

	// Domain summary
	domains, err := db.ListDomains()
	if err != nil {
		return fmt.Errorf("list domains: %w", err)
	}
	fmt.Printf("Domains: %d\n", len(domains))

	// Compute instances summary
	compute, err := db.ListComputeInstances(nil)
	if err == nil {
		fmt.Printf("Compute Instances: %d\n", len(compute))
	}

	// Services summary
	services, err := db.ListServices()
	if err == nil {
		upCount := 0
		for _, s := range services {
			if s.Status == "up" {
				upCount++
			}
		}
		fmt.Printf("Services: %d (up: %d)\n", len(services), upCount)
	}

	// Gateways summary
	gateways, err := db.ListGateways()
	if err == nil {
		fmt.Printf("Gateways: %d\n", len(gateways))
	}

	// Storages summary
	storages, err := db.ListStorages()
	if err == nil {
		fmt.Printf("Storages: %d\n", len(storages))
	}

	// Certificate summary
	certs, err := db.ListCertificates()
	if err != nil {
		return fmt.Errorf("list certificates: %w", err)
	}

	expiringCount := 0
	for _, c := range certs {
		if c.ExpiresAt.Before(time.Now().AddDate(0, 0, 30)) && c.ExpiresAt.After(time.Now()) {
			expiringCount++
		}
	}
	fmt.Printf("Certificates: %d (expiring in 30 days: %d)\n", len(certs), expiringCount)

	// Proxy summary
	proxies, err := db.ListProxies("")
	if err == nil {
		enabledCount := 0
		for _, p := range proxies {
			if p.Enabled {
				enabledCount++
			}
		}
		fmt.Printf("Proxies: %d (enabled: %d)\n", len(proxies), enabledCount)
	}

	fmt.Println("\nRun './cockpit server' to start the server")

	return nil
}

func isRecentlyActive(t time.Time) bool {
	if t.IsZero() {
		return false
	}
	return time.Since(t) < 2*time.Minute
}
