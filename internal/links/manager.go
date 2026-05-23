package links

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Link quick access link
type Link struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	URL         string    `json:"url"`
	Description string    `json:"description,omitempty"`
	Icon        string    `json:"icon,omitempty"`
	Category    string    `json:"category,omitempty"`
	Tags        []string  `json:"tags,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Order       int       `json:"order"`
}

// Category link category
type Category struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Icon        string   `json:"icon,omitempty"`
	Description string   `json:"description,omitempty"`
	Order       int      `json:"order"`
}

// Manager links manager
type Manager struct {
	mu       sync.RWMutex
	links    map[string]*Link
	categories map[string]*Category
	filePath string
}

// Config manager configuration
type Config struct {
	StoragePath string
}

// NewManager creates links manager
func NewManager(cfg Config) (*Manager, error) {
	m := &Manager{
		links:      make(map[string]*Link),
		categories: make(map[string]*Category),
		filePath:   cfg.StoragePath,
	}

	if cfg.StoragePath != "" {
		if err := m.load(); err != nil {
			// Initialize with default categories if file doesn't exist
			if os.IsNotExist(err) {
				m.initDefaults()
			} else {
				return nil, fmt.Errorf("load links: %w", err)
			}
		}
	} else {
		m.initDefaults()
	}

	return m, nil
}

// initDefaults initializes default categories
func (m *Manager) initDefaults() {
	defaultCategories := []*Category{
		{ID: "infrastructure", Name: "Infrastructure", Icon: "server", Order: 1},
		{ID: "services", Name: "Services", Icon: "appstore", Order: 2},
		{ID: "monitoring", Name: "Monitoring", Icon: "dashboard", Order: 3},
		{ID: "tools", Name: "Tools", Icon: "tool", Order: 4},
		{ID: "docs", Name: "Documentation", Icon: "book", Order: 5},
	}

	for _, cat := range defaultCategories {
		m.categories[cat.ID] = cat
	}
}

// load loads links from file
func (m *Manager) load() error {
	if m.filePath == "" {
		return nil
	}

	data, err := os.ReadFile(m.filePath)
	if err != nil {
		return err
	}

	var store struct {
		Links      []*Link      `json:"links"`
		Categories []*Category `json:"categories"`
	}

	if err := json.Unmarshal(data, &store); err != nil {
		return err
	}

	m.links = make(map[string]*Link)
	for _, link := range store.Links {
		m.links[link.ID] = link
	}

	m.categories = make(map[string]*Category)
	for _, cat := range store.Categories {
		m.categories[cat.ID] = cat
	}

	return nil
}

// save saves links to file
func (m *Manager) save() error {
	if m.filePath == "" {
		return nil
	}

	// Ensure directory exists
	dir := filepath.Dir(m.filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	links := make([]*Link, 0, len(m.links))
	for _, link := range m.links {
		links = append(links, link)
	}

	categories := make([]*Category, 0, len(m.categories))
	for _, cat := range m.categories {
		categories = append(categories, cat)
	}

	store := struct {
		Links      []*Link      `json:"links"`
		Categories []*Category `json:"categories"`
	}{
		Links:      links,
		Categories: categories,
	}

	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	if err := os.WriteFile(m.filePath, data, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// Add adds a new link
func (m *Manager) Add(link *Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if link.ID == "" {
		link.ID = generateID()
	}

	if link.CreatedAt.IsZero() {
		link.CreatedAt = time.Now()
	}
	link.UpdatedAt = time.Now()

	m.links[link.ID] = link

	return m.save()
}

// Update updates a link
func (m *Manager) Update(id string, link *Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.links[id]; !exists {
		return fmt.Errorf("link not found: %s", id)
	}

	link.ID = id
	link.UpdatedAt = time.Now()

	m.links[id] = link

	return m.save()
}

// Delete deletes a link
func (m *Manager) Delete(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.links[id]; !exists {
		return fmt.Errorf("link not found: %s", id)
	}

	delete(m.links, id)

	return m.save()
}

// Get gets a link by ID
func (m *Manager) Get(id string) (*Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	link, exists := m.links[id]
	if !exists {
		return nil, fmt.Errorf("link not found: %s", id)
	}

	// Return a copy
	copy := *link
	return &copy, nil
}

// List lists all links
func (m *Manager) List() []*Link {
	m.mu.RLock()
	defer m.mu.RUnlock()

	links := make([]*Link, 0, len(m.links))
	for _, link := range m.links {
		copy := *link
		links = append(links, &copy)
	}

	return links
}

// ListByCategory lists links by category
func (m *Manager) ListByCategory(category string) []*Link {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var links []*Link
	for _, link := range m.links {
		if link.Category == category {
			copy := *link
			links = append(links, &copy)
		}
	}

	return links
}

// ListByTag lists links by tag
func (m *Manager) ListByTag(tag string) []*Link {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var links []*Link
	for _, link := range m.links {
		for _, t := range link.Tags {
			if t == tag {
				copy := *link
				links = append(links, &copy)
				break
			}
		}
	}

	return links
}

// Search searches links by title or URL
func (m *Manager) Search(query string) []*Link {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var links []*Link
	for _, link := range m.links {
		if contains(link.Title, query) || contains(link.URL, query) || contains(link.Description, query) {
			copy := *link
			links = append(links, &copy)
		}
	}

	return links
}

// AddCategory adds a category
func (m *Manager) AddCategory(cat *Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if cat.ID == "" {
		cat.ID = generateID()
	}

	m.categories[cat.ID] = cat

	return m.save()
}

// UpdateCategory updates a category
func (m *Manager) UpdateCategory(id string, cat *Category) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.categories[id]; !exists {
		return fmt.Errorf("category not found: %s", id)
	}

	cat.ID = id
	m.categories[id] = cat

	return m.save()
}

// DeleteCategory deletes a category
func (m *Manager) DeleteCategory(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.categories[id]; !exists {
		return fmt.Errorf("category not found: %s", id)
	}

	delete(m.categories, id)

	return m.save()
}

// GetCategory gets a category by ID
func (m *Manager) GetCategory(id string) (*Category, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cat, exists := m.categories[id]
	if !exists {
		return nil, fmt.Errorf("category not found: %s", id)
	}

	copy := *cat
	return &copy, nil
}

// ListCategories lists all categories
func (m *Manager) ListCategories() []*Category {
	m.mu.RLock()
	defer m.mu.RUnlock()

	categories := make([]*Category, 0, len(m.categories))
	for _, cat := range m.categories {
		copy := *cat
		categories = append(categories, &copy)
	}

	return categories
}

// GetCategoriesWithLinks returns categories with their links
func (m *Manager) GetCategoriesWithLinks() map[string][]*Link {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string][]*Link)
	for _, cat := range m.categories {
		result[cat.ID] = []*Link{}
	}

	for _, link := range m.links {
		if link.Category != "" {
			copy := *link
			result[link.Category] = append(result[link.Category], &copy)
		}
	}

	return result
}

// SetLinkOrder sets the order of links within a category
func (m *Manager) SetLinkOrder(linkIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, id := range linkIDs {
		if link, exists := m.links[id]; exists {
			link.Order = i
		}
	}

	return m.save()
}

// SetCategoryOrder sets the order of categories
func (m *Manager) SetCategoryOrder(categoryIDs []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for i, id := range categoryIDs {
		if cat, exists := m.categories[id]; exists {
			cat.Order = i
		}
	}

	return m.save()
}

// Import imports links from JSON
func (m *Manager) Import(data []byte) error {
	var store struct {
		Links      []*Link      `json:"links"`
		Categories []*Category `json:"categories"`
	}

	if err := json.Unmarshal(data, &store); err != nil {
		return fmt.Errorf("unmarshal: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for _, link := range store.Links {
		if link.ID == "" {
			link.ID = generateID()
		}
		m.links[link.ID] = link
	}

	for _, cat := range store.Categories {
		if cat.ID == "" {
			cat.ID = generateID()
		}
		m.categories[cat.ID] = cat
	}

	return m.save()
}

// Export exports links to JSON
func (m *Manager) Export() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	links := make([]*Link, 0, len(m.links))
	for _, link := range m.links {
		links = append(links, link)
	}

	categories := make([]*Category, 0, len(m.categories))
	for _, cat := range m.categories {
		categories = append(categories, cat)
	}

	store := struct {
		Links      []*Link      `json:"links"`
		Categories []*Category `json:"categories"`
		ExportedAt time.Time    `json:"exported_at"`
	}{
		Links:      links,
		Categories: categories,
		ExportedAt: time.Now(),
	}

	return json.MarshalIndent(store, "", "  ")
}

// Stats returns statistics about links
func (m *Manager) Stats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	tagCounts := make(map[string]int)
	for _, link := range m.links {
		for _, tag := range link.Tags {
			tagCounts[tag]++
		}
	}

	return map[string]interface{}{
		"total_links":    len(m.links),
		"total_categories": len(m.categories),
		"tags":           tagCounts,
	}
}

// generateID generates a unique ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// contains checks if string contains substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && containsIgnoreCase(s, substr))
}

func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}
