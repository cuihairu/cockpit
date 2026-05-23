package sync

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/cuihairu/cockpit/internal/inventory"
	"github.com/cuihairu/cockpit/internal/storage"
	"github.com/fsnotify/fsnotify"
)

// Watcher watches inventory file for changes
type Watcher struct {
	mu           sync.RWMutex
	inventoryPath string
	db           *storage.DB
	watcher      *fsnotify.Watcher
	ctx          context.Context
	cancel       context.CancelFunc
	lastModTime  time.Time
	onReload     func(*inventory.Inventory) error
}

// Config watcher configuration
type Config struct {
	InventoryPath string
	DB           *storage.DB
	OnReload     func(*inventory.Inventory) error
}

// NewWatcher creates file watcher
func NewWatcher(cfg Config) (*Watcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Watcher{
		inventoryPath: cfg.InventoryPath,
		db:           cfg.DB,
		watcher:      watcher,
		ctx:          ctx,
		cancel:       cancel,
		onReload:     cfg.OnReload,
	}, nil
}

// Start starts watching
func (w *Watcher) Start() error {
	// Add watch to directory
	dir := filepath.Dir(w.inventoryPath)
	if err := w.watcher.Add(dir); err != nil {
		return err
	}

	// Initial load
	if err := w.loadInventory(); err != nil {
		log.Printf("Initial load failed: %v", err)
	}

	// Start watch loop
	go w.watchLoop()

	log.Printf("Started watching inventory file: %s", w.inventoryPath)
	return nil
}

// Stop stops watching
func (w *Watcher) Stop() {
	w.cancel()
	if w.watcher != nil {
		w.watcher.Close()
	}
	log.Println("Stopped inventory watcher")
}

// watchLoop watches for file changes
func (w *Watcher) watchLoop() {
	debounceTimer := time.NewTimer(0)
	if !debounceTimer.Stop() {
		<-debounceTimer.C
	}

	for {
		select {
		case <-w.ctx.Done():
			return

		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}

			if filepath.Clean(event.Name) != filepath.Clean(w.inventoryPath) {
				continue
			}

			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
				log.Printf("Inventory file modified: %s", event.Name)
				debounceTimer.Reset(500 * time.Millisecond)
			}

		case _, ok := <-w.watcher.Errors:
			if !ok {
				return
			}

		case <-debounceTimer.C:
			w.loadInventory()
		}
	}
}

// loadInventory loads and applies inventory
func (w *Watcher) loadInventory() error {
	info, err := os.Stat(w.inventoryPath)
	if err != nil {
		return err
	}

	w.mu.RLock()
	if !info.ModTime().After(w.lastModTime) {
		w.mu.RUnlock()
		return nil
	}
	w.mu.RUnlock()

	// Read file
	data, err := os.ReadFile(w.inventoryPath)
	if err != nil {
		return err
	}

	// Parse inventory
	inv, err := inventory.Parse(data)
	if err != nil {
		return err
	}

	// Apply to database
	if err := w.applyInventory(inv); err != nil {
		log.Printf("Apply inventory error: %v", err)
	}

	// Call custom reload handler
	if w.onReload != nil {
		if err := w.onReload(inv); err != nil {
			log.Printf("Reload handler error: %v", err)
		}
	}

	w.mu.Lock()
	w.lastModTime = info.ModTime()
	w.mu.Unlock()

	regionCount := len(inv.Regions)
	agentCount := countAgents(inv)
	log.Printf("Inventory loaded: %d regions, %d agents", regionCount, agentCount)

	return nil
}

// applyInventory applies inventory to database
func (w *Watcher) applyInventory(inv *inventory.Inventory) error {
	if w.db == nil {
		return nil
	}

	// Sync regions and zones
	for _, region := range inv.Regions {
		// Sync zones
		for _, zone := range region.Zones {
			// Sync agents
			for _, agent := range zone.Agents {
				dbAgent := &storage.Agent{
					ID:       agent.ID,
					Hostname: agent.Hostname,
					IP:       agent.IP,
					Region:   region.Name,
					Zone:     zone.Name,
				}

				// Set capabilities
				if len(agent.Capabilities) > 0 {
					for _, cap := range agent.Capabilities {
						dbAgent.Capabilities = append(dbAgent.Capabilities, storage.Capability{
							Type: cap,
						})
					}
				} else {
					// Detect from config
					if _, ok := agent.Config["pve"]; ok {
						dbAgent.Capabilities = append(dbAgent.Capabilities, storage.Capability{Type: "pve"})
					}
					if _, ok := agent.Config["docker"]; ok {
						dbAgent.Capabilities = append(dbAgent.Capabilities, storage.Capability{Type: "docker"})
					}
					if _, ok := agent.Config["openwrt"]; ok {
						dbAgent.Capabilities = append(dbAgent.Capabilities, storage.Capability{Type: "openwrt"})
					}
				}

				if err := w.db.UpsertAgent(dbAgent); err != nil {
					log.Printf("Failed to sync agent %s: %v", agent.ID, err)
				}
			}
		}
	}

	return nil
}

// ForceReload forces a reload of inventory
func (w *Watcher) ForceReload() error {
	return w.loadInventory()
}

// GetInventory gets current inventory
func (w *Watcher) GetInventory() (*inventory.Inventory, error) {
	data, err := os.ReadFile(w.inventoryPath)
	if err != nil {
		return nil, err
	}

	return inventory.Parse(data)
}

// GetLastModTime returns last modification time
func (w *Watcher) GetLastModTime() time.Time {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.lastModTime
}

// countAgents counts agents in inventory
func countAgents(inv *inventory.Inventory) int {
	count := 0
	for _, region := range inv.Regions {
		for _, zone := range region.Zones {
			count += len(zone.Agents)
		}
	}
	return count
}

// Manager manages inventory sync
type Manager struct {
	watcher *Watcher
	db      *storage.DB
}

// NewManager creates sync manager
func NewManager(inventoryPath string, db *storage.DB) (*Manager, error) {
	cfg := Config{
		InventoryPath: inventoryPath,
		DB:           db,
	}

	watcher, err := NewWatcher(cfg)
	if err != nil {
		return nil, err
	}

	return &Manager{
		watcher: watcher,
		db:      db,
	}, nil
}

// Start starts the manager
func (m *Manager) Start() error {
	return m.watcher.Start()
}

// Stop stops the manager
func (m *Manager) Stop() {
	m.watcher.Stop()
}

// Reload reloads inventory
func (m *Manager) Reload() error {
	return m.watcher.ForceReload()
}

// Validate validates inventory file
func (m *Manager) Validate() error {
	inv, err := m.watcher.GetInventory()
	if err != nil {
		return err
	}

	// Validate structure
	if len(inv.Regions) == 0 && len(inv.Domains) == 0 {
		return nil // Empty inventory is valid
	}

	// Validate regions
	for regionKey, region := range inv.Regions {
		if region.ID == "" {
			region.ID = regionKey
		}
		for zoneKey, zone := range region.Zones {
			if zone.ID == "" {
				zone.ID = zoneKey
			}
			for agentKey, agent := range zone.Agents {
				if agent.ID == "" {
					agent.ID = agentKey
				}
			}
		}
	}

	return nil
}
