package cli

import (
	"context"
	"fmt"
	"log"

	"github.com/cuihairu/cockpit/internal/config"
	"github.com/cuihairu/cockpit/internal/inventory"
	"github.com/cuihairu/cockpit/internal/storage"
)

// SyncCmd sync command
type SyncCmd struct {
	Config    string
	Inventory string
	DBPath    string
	Verbose   bool
}

// Run executes sync
func (c *SyncCmd) Run() error {
	// Load config
	cfg := loadConfigOrDefault(c.Config)

	// Determine database path
	dbPath := c.DBPath
	if dbPath == "" && cfg.Database != nil {
		dbPath = cfg.Database.Path
	}
	if dbPath == "" {
		dbPath = "./data/cockpit.db"
	}

	// Open database
	db, err := storage.Open(storage.Config{Path: dbPath})
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer db.Close()

	// Parse inventory
	invPath := c.Inventory
	if invPath == "" && cfg.Inventory != nil {
		invPath = cfg.Inventory.Path
	}
	if invPath == "" {
		return fmt.Errorf("inventory path required (-inventory or inventory.path in config)")
	}

	inv, err := inventory.ParseFile(invPath)
	if err != nil {
		return fmt.Errorf("parse inventory: %w", err)
	}

	log.Printf("Syncing inventory: %s", invPath)
	log.Printf("  version: %s", inv.Version)
	if inv.Metadata.Name != "" {
		log.Printf("  name: %s", inv.Metadata.Name)
	}

	// Execute sync
	syncer := inventory.NewSyncer(db)
	result, err := syncer.Sync(context.Background(), inv)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Print result
	fmt.Println("\nSync completed:")
	printResult("Agents", result.Agents)
	printResult("Domains", result.Domains)
	printResult("Certificates", result.Certificates)
	printResult("Compute Instances", result.ComputeInstances)
	printResult("Services", result.Services)
	printResult("Gateways", result.Gateways)
	printResult("Storages", result.Storages)

	if hasSyncErrors(result) {
		return fmt.Errorf("sync had errors")
	}

	return nil
}

func hasSyncErrors(result *inventory.SyncResult) bool {
	if result == nil {
		return false
	}
	for _, r := range []*inventory.ResourceResult{
		result.Agents,
		result.Domains,
		result.Certificates,
		result.ComputeInstances,
		result.Services,
		result.Gateways,
		result.Storages,
	} {
		if r != nil && r.Errors > 0 {
			return true
		}
	}
	return false
}

func printResult(name string, r *inventory.ResourceResult) {
	if r == nil {
		return
	}
	total := r.Created + r.Updated
	if total > 0 {
		fmt.Printf("  %s: +%d ~%d", name, r.Created, r.Updated)
		if r.Deleted > 0 {
			fmt.Printf(" -%d", r.Deleted)
		}
		if r.Errors > 0 {
			fmt.Printf(" (%d errors)", r.Errors)
		}
		fmt.Println()
	}
}

func loadConfigOrDefault(path string) *config.Config {
	if path != "" {
		cfg, err := config.Load(path)
		if err != nil {
			log.Printf("Warning: load config failed: %v, using defaults", err)
			return config.LoadOrDefault("")
		}
		return cfg
	}
	return config.LoadOrDefault("")
}
