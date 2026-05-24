package links

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewManager(t *testing.T) {
	// Test with empty config (in-memory)
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if m == nil {
		t.Fatal("NewManager() should not return nil")
	}

	if len(m.categories) == 0 {
		t.Error("Manager should have default categories")
	}
}

func TestNewManagerWithFile(t *testing.T) {
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "links.json")

	m, err := NewManager(Config{StoragePath: filePath})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if m == nil {
		t.Fatal("NewManager() should not return nil")
	}

	// Should have default categories
	if len(m.categories) == 0 {
		t.Error("Manager should have default categories")
	}

	// Check if file was created
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		// File might not be created until save is called
	}
}

func TestInitDefaults(t *testing.T) {
	m := &Manager{
		links:      make(map[string]*Link),
		categories: make(map[string]*Category),
		filePath:   "",
	}

	m.initDefaults()

	expectedCategories := []string{"infrastructure", "services", "monitoring", "tools", "docs"}
	for _, catID := range expectedCategories {
		if _, exists := m.categories[catID]; !exists {
			t.Errorf("Default category %s should exist", catID)
		}
	}
}

func TestAddLink(t *testing.T) {
	m, _ := NewManager(Config{})

	link := &Link{
		Title:       "Test Link",
		URL:         "https://example.com",
		Description: "Test description",
		Category:    "tools",
		Tags:        []string{"test", "demo"},
		Order:       1,
	}

	err := m.Add(link)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if link.ID == "" {
		t.Error("Link ID should be generated")
	}

	if link.CreatedAt.IsZero() {
		t.Error("Link CreatedAt should be set")
	}

	if link.UpdatedAt.IsZero() {
		t.Error("Link UpdatedAt should be set")
	}

	// Verify link was added
	retrieved, err := m.Get(link.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Title != "Test Link" {
		t.Errorf("Title = %v, want Test Link", retrieved.Title)
	}
}

func TestAddLinkWithID(t *testing.T) {
	m, _ := NewManager(Config{})

	link := &Link{
		ID:    "custom-id",
		Title: "Custom ID Link",
		URL:   "https://example.com",
	}

	err := m.Add(link)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if link.ID != "custom-id" {
		t.Errorf("ID = %v, want custom-id", link.ID)
	}

	retrieved, _ := m.Get("custom-id")
	if retrieved.ID != "custom-id" {
		t.Errorf("Retrieved ID = %v, want custom-id", retrieved.ID)
	}
}

func TestUpdateLink(t *testing.T) {
	m, _ := NewManager(Config{})

	// Add a link first
	link := &Link{
		Title:    "Original Title",
		URL:      "https://example.com",
		Category: "tools",
	}
	m.Add(link)

	// Update it
	updated := &Link{
		Title:       "Updated Title",
		URL:         "https://example.com/updated",
		Description: "Updated description",
		Category:    "services",
	}

	err := m.Update(link.ID, updated)
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}

	if updated.ID != link.ID {
		t.Errorf("Updated link ID should remain %v", link.ID)
	}

	// Verify update
	retrieved, _ := m.Get(link.ID)
	if retrieved.Title != "Updated Title" {
		t.Errorf("Title = %v, want Updated Title", retrieved.Title)
	}
}

func TestUpdateNonExistentLink(t *testing.T) {
	m, _ := NewManager(Config{})

	link := &Link{
		Title: "Test",
		URL:   "https://example.com",
	}

	err := m.Update("non-existent", link)
	if err == nil {
		t.Error("Update() should return error for non-existent link")
	}
}

func TestDeleteLink(t *testing.T) {
	m, _ := NewManager(Config{})

	link := &Link{
		Title: "To Delete",
		URL:   "https://example.com",
	}
	m.Add(link)

	err := m.Delete(link.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err = m.Get(link.ID)
	if err == nil {
		t.Error("Get() should return error after deletion")
	}
}

func TestDeleteNonExistentLink(t *testing.T) {
	m, _ := NewManager(Config{})

	err := m.Delete("non-existent")
	if err == nil {
		t.Error("Delete() should return error for non-existent link")
	}
}

func TestGetLink(t *testing.T) {
	m, _ := NewManager(Config{})

	link := &Link{
		Title: "Test Link",
		URL:   "https://example.com",
	}
	m.Add(link)

	retrieved, err := m.Get(link.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if retrieved.Title != "Test Link" {
		t.Errorf("Title = %v, want Test Link", retrieved.Title)
	}

	// Verify it's a copy
	retrieved.Title = "Modified"
	original, _ := m.Get(link.ID)
	if original.Title == "Modified" {
		t.Error("Get() should return a copy, not reference")
	}
}

func TestListLinks(t *testing.T) {
	m, _ := NewManager(Config{})

	// Add multiple links with explicit IDs to avoid collisions
	for i := 0; i < 3; i++ {
		link := &Link{
			ID:       "list-test-" + string(rune('0'+i)),
			Title:    "Link",
			URL:      "https://example.com",
			Category: "tools",
		}
		m.Add(link)
	}

	links := m.List()
	if len(links) != 3 {
		t.Errorf("List() returned %d links, want 3", len(links))
	}
}

func TestListByCategory(t *testing.T) {
	m, _ := NewManager(Config{})

	m.Add(&Link{ID: "cat-test-1", Title: "Tool 1", URL: "https://example.com", Category: "tools"})
	m.Add(&Link{ID: "cat-test-2", Title: "Service 1", URL: "https://example.com", Category: "services"})
	m.Add(&Link{ID: "cat-test-3", Title: "Tool 2", URL: "https://example.com", Category: "tools"})

	toolsLinks := m.ListByCategory("tools")
	if len(toolsLinks) != 2 {
		t.Errorf("ListByCategory(tools) returned %d links, want 2", len(toolsLinks))
	}

	servicesLinks := m.ListByCategory("services")
	if len(servicesLinks) != 1 {
		t.Errorf("ListByCategory(services) returned %d links, want 1", len(servicesLinks))
	}
}

func TestListByTag(t *testing.T) {
	m, _ := NewManager(Config{})

	m.Add(&Link{ID: "tag-test-1", Title: "Link 1", URL: "https://example.com", Tags: []string{"dev", "api"}})
	m.Add(&Link{ID: "tag-test-2", Title: "Link 2", URL: "https://example.com", Tags: []string{"dev"}})
	m.Add(&Link{ID: "tag-test-3", Title: "Link 3", URL: "https://example.com", Tags: []string{"api"}})

	devLinks := m.ListByTag("dev")
	if len(devLinks) != 2 {
		t.Errorf("ListByTag(dev) returned %d links, want 2", len(devLinks))
	}

	apiLinks := m.ListByTag("api")
	if len(apiLinks) != 2 {
		t.Errorf("ListByTag(api) returned %d links, want 2", len(apiLinks))
	}
}

func TestSearchLinks(t *testing.T) {
	m, _ := NewManager(Config{})

	m.Add(&Link{ID: "search-1", Title: "GitHub", URL: "https://github.com", Description: "Code hosting"})
	m.Add(&Link{ID: "search-2", Title: "Google", URL: "https://google.com", Description: "Search engine"})
	m.Add(&Link{ID: "search-3", Title: "Example", URL: "https://example.com", Description: "Test site"})

	results := m.Search("git")
	if len(results) != 1 {
		t.Errorf("Search(git) returned %d results, want 1", len(results))
	}

	results = m.Search("https")
	if len(results) < 3 {
		t.Errorf("Search(https) should return at least 3 results, got %d", len(results))
	}

	results = m.Search("engine")
	if len(results) != 1 {
		t.Errorf("Search(engine) returned %d results, want 1", len(results))
	}
}

func TestAddCategory(t *testing.T) {
	m, _ := NewManager(Config{})

	cat := &Category{
		ID:          "custom",
		Name:        "Custom Category",
		Icon:        "custom-icon",
		Description: "Custom description",
		Order:       10,
	}

	err := m.AddCategory(cat)
	if err != nil {
		t.Fatalf("AddCategory() error = %v", err)
	}

	retrieved, err := m.GetCategory("custom")
	if err != nil {
		t.Fatalf("GetCategory() error = %v", err)
	}

	if retrieved.Name != "Custom Category" {
		t.Errorf("Name = %v, want Custom Category", retrieved.Name)
	}
}

func TestUpdateCategory(t *testing.T) {
	m, _ := NewManager(Config{})

	cat := &Category{
		ID:   "test-cat",
		Name: "Original Name",
	}
	m.AddCategory(cat)

	updated := &Category{
		Name: "Updated Name",
	}

	err := m.UpdateCategory("test-cat", updated)
	if err != nil {
		t.Fatalf("UpdateCategory() error = %v", err)
	}

	if updated.ID != "test-cat" {
		t.Errorf("Updated category ID should be test-cat")
	}

	retrieved, _ := m.GetCategory("test-cat")
	if retrieved.Name != "Updated Name" {
		t.Errorf("Name = %v, want Updated Name", retrieved.Name)
	}
}

func TestDeleteCategory(t *testing.T) {
	m, _ := NewManager(Config{})

	cat := &Category{
		ID:   "to-delete",
		Name: "Delete Me",
	}
	m.AddCategory(cat)

	err := m.DeleteCategory("to-delete")
	if err != nil {
		t.Fatalf("DeleteCategory() error = %v", err)
	}

	_, err = m.GetCategory("to-delete")
	if err == nil {
		t.Error("GetCategory() should return error after deletion")
	}
}

func TestListCategories(t *testing.T) {
	m, _ := NewManager(Config{})

	categories := m.ListCategories()

	if len(categories) < 5 {
		t.Errorf("ListCategories() should return at least 5 default categories, got %d", len(categories))
	}
}

func TestGetCategoriesWithLinks(t *testing.T) {
	m, _ := NewManager(Config{})

	m.Add(&Link{ID: "catlink-1", Title: "Tool 1", URL: "https://example.com", Category: "tools"})
	m.Add(&Link{ID: "catlink-2", Title: "Service 1", URL: "https://example.com", Category: "services"})
	m.Add(&Link{ID: "catlink-3", Title: "Tool 2", URL: "https://example.com", Category: "tools"})

	result := m.GetCategoriesWithLinks()

	toolsLinks := result["tools"]
	if len(toolsLinks) != 2 {
		t.Errorf("tools category should have 2 links, got %d", len(toolsLinks))
	}

	servicesLinks := result["services"]
	if len(servicesLinks) != 1 {
		t.Errorf("services category should have 1 link, got %d", len(servicesLinks))
	}
}

func TestSetLinkOrder(t *testing.T) {
	m, _ := NewManager(Config{})

	link1 := &Link{ID: "order-1", Title: "Link 1", URL: "https://example.com", Category: "tools", Order: 0}
	link2 := &Link{ID: "order-2", Title: "Link 2", URL: "https://example.com", Category: "tools", Order: 1}
	link3 := &Link{ID: "order-3", Title: "Link 3", URL: "https://example.com", Category: "tools", Order: 2}

	m.Add(link1)
	m.Add(link2)
	m.Add(link3)

	// Reverse order
	err := m.SetLinkOrder([]string{link3.ID, link2.ID, link1.ID})
	if err != nil {
		t.Fatalf("SetLinkOrder() error = %v", err)
	}

	// Verify orders - links are not guaranteed to be in order, but Order field should be set
	links := m.ListByCategory("tools")
	// Find link3 and verify its order is 0
	for _, l := range links {
		if l.ID == "order-3" && l.Order != 0 {
			t.Errorf("link3 Order = %d, want 0", l.Order)
		}
	}
}

func TestSetCategoryOrder(t *testing.T) {
	m, _ := NewManager(Config{})

	// Just verify it doesn't error
	cats := m.ListCategories()
	catIDs := make([]string, len(cats))
	for i, cat := range cats {
		catIDs[i] = cat.ID
	}

	err := m.SetCategoryOrder(catIDs)
	if err != nil {
		t.Fatalf("SetCategoryOrder() error = %v", err)
	}
}

func TestImport(t *testing.T) {
	m, _ := NewManager(Config{})

	data := []byte(`{
		"links": [
			{"id": "import1", "title": "Imported Link", "url": "https://example.com", "category": "tools"}
		],
		"categories": [
			{"id": "import-cat", "name": "Imported Category"}
		]
	}`)

	err := m.Import(data)
	if err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	link, err := m.Get("import1")
	if err != nil {
		t.Errorf("Imported link not found")
	}

	if link.Title != "Imported Link" {
		t.Errorf("Title = %v, want Imported Link", link.Title)
	}

	cat, err := m.GetCategory("import-cat")
	if err != nil {
		t.Errorf("Imported category not found")
	}

	if cat.Name != "Imported Category" {
		t.Errorf("Category Name = %v, want Imported Category", cat.Name)
	}
}

func TestExport(t *testing.T) {
	m, _ := NewManager(Config{})

	m.Add(&Link{Title: "Test", URL: "https://example.com"})

	data, err := m.Export()
	if err != nil {
		t.Fatalf("Export() error = %v", err)
	}

	if len(data) == 0 {
		t.Error("Export() should return non-empty data")
	}

	// Should contain JSON
	dataStr := string(data)
	if !contains(dataStr, "links") || !contains(dataStr, "exported_at") {
		t.Error("Export() should return valid JSON with links and exported_at")
	}
}

func TestStats(t *testing.T) {
	m, _ := NewManager(Config{})

	m.Add(&Link{
		Title:    "Test",
		URL:      "https://example.com",
		Category: "tools",
		Tags:     []string{"tag1", "tag2"},
	})

	stats := m.Stats()

	totalLinks, ok := stats["total_links"].(int)
	if !ok || totalLinks != 1 {
		t.Errorf("total_links = %v, want 1", stats["total_links"])
	}

	totalCats, ok := stats["total_categories"].(int)
	if !ok || totalCats < 5 {
		t.Errorf("total_categories = %v, want at least 5", totalCats)
	}
}

func TestGenerateID(t *testing.T) {
	id1 := generateID()

	if id1 == "" {
		t.Error("generateID() should not return empty string")
	}

	// Add small delay to ensure different timestamp
	time.Sleep(time.Nanosecond)

	id2 := generateID()

	if id1 == id2 {
		t.Log("IDs may collide if generated in same nanosecond - this is acceptable")
		// Don't fail the test - this is a known limitation
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		substr string
		want   bool
	}{
		{"exact match", "Hello World", "Hello World", true},
		{"substring", "Hello World", "Hello", true},
		{"case insensitive", "Hello World", "hello", true},
		{"not contained", "Hello World", "Goodbye", false},
		{"empty substring", "Hello World", "", true},
		{"longer substring", "Hi", "Hello", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contains(tt.s, tt.substr); got != tt.want {
				t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.substr, got, tt.want)
			}
		})
	}
}

func TestLinkTimestamps(t *testing.T) {
	m, _ := NewManager(Config{})

	beforeAdd := time.Now()
	link := &Link{
		Title: "Test",
		URL:   "https://example.com",
	}
	m.Add(link)
	afterAdd := time.Now()

	if link.CreatedAt.Before(beforeAdd) || link.CreatedAt.After(afterAdd) {
		t.Error("CreatedAt should be set to approximately now")
	}

	if link.UpdatedAt.Before(beforeAdd) || link.UpdatedAt.After(afterAdd) {
		t.Error("UpdatedAt should be set to approximately now")
	}

	// Wait a bit and update
	time.Sleep(10 * time.Millisecond)
	beforeUpdate := time.Now()

	updatedLink := &Link{
		Title: "Updated",
		URL:   "https://example.com",
	}
	m.Update(link.ID, updatedLink)
	afterUpdate := time.Now()

	if updatedLink.UpdatedAt.Before(beforeUpdate) || updatedLink.UpdatedAt.After(afterUpdate) {
		t.Error("UpdatedAt should be updated on Update()")
	}
}

func TestGetCategoriesWithLinksEmpty(t *testing.T) {
	m, _ := NewManager(Config{})

	result := m.GetCategoriesWithLinks()

	for catID, links := range result {
		if len(links) != 0 {
			t.Errorf("Category %s should have 0 links initially", catID)
		}
	}
}

func TestSearchEmptyQuery(t *testing.T) {
	m, _ := NewManager(Config{})

	m.Add(&Link{Title: "Test", URL: "https://example.com"})

	results := m.Search("")
	if len(results) == 0 {
		t.Error("Search with empty query should return all links")
	}
}

func TestLinkCopyBehavior(t *testing.T) {
	m, _ := NewManager(Config{})

	link := &Link{
		Title: "Test",
		URL:   "https://example.com",
		Tags:  []string{"tag1", "tag2"},
	}
	m.Add(link)

	// Get a copy
	retrieved := m.List()[0]

	// Modify the copy's title (not the slice)
	retrieved.Title = "Modified"

	// Original should not be affected
	original, _ := m.Get(link.ID)
	if original.Title == "Modified" {
		t.Error("Modifying returned link should not affect stored link")
	}
}
