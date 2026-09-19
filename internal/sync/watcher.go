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
	mu            sync.RWMutex
	inventoryPath string
	db            *storage.DB
	watcher       *fsnotify.Watcher
	ctx           context.Context
	cancel        context.CancelFunc
	lastModTime   time.Time
	onReload      func(*inventory.Inventory) error
	strict        bool
}

// Config watcher configuration
type Config struct {
	InventoryPath string
	DB            *storage.DB
	OnReload      func(*inventory.Inventory) error
	Strict        bool
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
		db:            cfg.DB,
		watcher:       watcher,
		ctx:           ctx,
		cancel:        cancel,
		onReload:      cfg.OnReload,
		strict:        cfg.Strict,
	}, nil
}

// Start starts watching
func (w *Watcher) Start() error {
	// Add watch to directory
	dir := filepath.Dir(w.inventoryPath)
	if err := w.watcher.Add(dir); err != nil {
		w.Stop()
		return err
	}

	// Initial load
	if err := w.loadInventory(); err != nil {
		if w.strict {
			w.Stop()
			return err
		}
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
	// 立即停掉 NewTimer(0) 的初始触发，避免首轮 select 就收到一次多余 tick
	// （go1.23 起 Stop 保证之后不会送达旧值，无需再排空通道）。
	debounceTimer := time.NewTimer(0)
	debounceTimer.Stop()

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

	// Apply to database（无失败路径，见 applyInventory 注释）
	w.applyInventory(inv)

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

// applyInventory applies inventory to database via the shared Syncer.
// 复用 inventory.Syncer 统一同步逻辑，避免 CLI sync 与 watch 两套并行实现。
// Syncer 内部对单条落库失败只计数不上抛，因此这里没有可失败路径。
func (w *Watcher) applyInventory(inv *inventory.Inventory) {
	if w.db == nil || inv == nil {
		return
	}

	result := inventory.NewSyncer(w.db).Sync(w.ctx, inv)

	log.Printf("Inventory applied: agents=%d domains=%d certificates=%d compute=%d services=%d gateways=%d storages=%d",
		countResult(result.Agents),
		countResult(result.Domains),
		countResult(result.Certificates),
		countResult(result.ComputeInstances),
		countResult(result.Services),
		countResult(result.Gateways),
		countResult(result.Storages))
}

// countResult 汇总 ResourceResult 的处理总数（Created+Updated+Errors）
func countResult(r *inventory.ResourceResult) int {
	if r == nil {
		return 0
	}
	return r.Created + r.Updated + r.Errors
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
	return NewManagerWithConfig(Config{
		InventoryPath: inventoryPath,
		DB:            db,
	})
}

// NewManagerWithConfig creates sync manager with full watcher configuration.
func NewManagerWithConfig(cfg Config) (*Manager, error) {
	watcher, err := NewWatcher(cfg)
	if err != nil {
		return nil, err
	}

	return &Manager{
		watcher: watcher,
		db:      cfg.DB,
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

// Consistency inventory 声明与库内实报的一致性比对（drift-design.md M6 D28）
func (m *Manager) Consistency() (*inventory.ConsistencyReport, error) {
	inv, err := m.watcher.GetInventory()
	if err != nil {
		return nil, err
	}
	agents, err := m.db.ListAgents()
	if err != nil {
		return nil, err
	}
	return inventory.CompareAgents(inv, agents), nil
}

// Validate validates inventory file
func (m *Manager) Validate() error {
	_, err := m.watcher.GetInventory()
	return err
}
