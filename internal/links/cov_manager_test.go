package links

import (
	"os"
	"path/filepath"
	"testing"
)

// covNewManager 创建指向临时文件的 Manager
func covNewManager(t *testing.T) *Manager {
	t.Helper()
	path := filepath.Join(t.TempDir(), "links.json")
	m, err := NewManager(Config{StoragePath: path})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return m
}

func TestCovSaveMkdirAllError(t *testing.T) {
	dir := t.TempDir()
	// 在普通文件路径下再嵌一层：MkdirAll 因 ENOTDIR 失败
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	m := &Manager{
		links:      make(map[string]*Link),
		categories: make(map[string]*Category),
		filePath:   filepath.Join(blocker, "sub", "links.json"),
	}

	err := m.Add(&Link{Title: "cov", URL: "https://cov.example.com"})
	if err == nil {
		t.Fatal("Add() should fail when parent directory cannot be created")
	}
}

func TestCovSaveWriteFileError(t *testing.T) {
	dir := t.TempDir()
	// StoragePath 本身是目录：MkdirAll(dir) 成功，WriteFile(dir) 失败
	m := &Manager{
		links:      make(map[string]*Link),
		categories: make(map[string]*Category),
		filePath:   dir,
	}

	err := m.Add(&Link{Title: "cov", URL: "https://cov.example.com"})
	if err == nil {
		t.Fatal("Add() should fail when storage path is a directory")
	}
}

func TestCovSaveNoFilePath(t *testing.T) {
	m, err := NewManager(Config{})
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := m.save(); err != nil {
		t.Errorf("save() with empty filePath should be a no-op, got %v", err)
	}
}

func TestCovAddCategoryGeneratesID(t *testing.T) {
	m := covNewManager(t)

	cat := &Category{Name: "Cov Category"}
	if err := m.AddCategory(cat); err != nil {
		t.Fatalf("AddCategory() error = %v", err)
	}
	if cat.ID == "" {
		t.Fatal("AddCategory() should generate an ID when missing")
	}
	got, err := m.GetCategory(cat.ID)
	if err != nil || got == nil || got.Name != "Cov Category" {
		t.Errorf("GetCategory(%q) = %+v, err %v, want stored category", cat.ID, got, err)
	}
}

func TestCovUpdateCategoryNotFound(t *testing.T) {
	m := covNewManager(t)

	err := m.UpdateCategory("cov-missing", &Category{Name: "X"})
	if err == nil {
		t.Fatal("UpdateCategory() should fail for unknown category")
	}
}

func TestCovUpdateCategorySuccess(t *testing.T) {
	m := covNewManager(t)

	if err := m.AddCategory(&Category{ID: "cov-cat", Name: "Old"}); err != nil {
		t.Fatal(err)
	}
	if err := m.UpdateCategory("cov-cat", &Category{Name: "New", Order: 9}); err != nil {
		t.Fatalf("UpdateCategory() error = %v", err)
	}

	got, gErr := m.GetCategory("cov-cat")
	if gErr != nil || got == nil || got.Name != "New" {
		t.Errorf("GetCategory() = %+v, err %v, want updated name New", got, gErr)
	}
	if got.Order != 9 {
		t.Errorf("Order = %d, want 9", got.Order)
	}
	if got.ID != "cov-cat" {
		t.Errorf("ID = %q, want cov-cat (forced)", got.ID)
	}
}

func TestCovDeleteCategoryNotFound(t *testing.T) {
	m := covNewManager(t)

	if err := m.DeleteCategory("cov-missing"); err == nil {
		t.Fatal("DeleteCategory() should fail for unknown category")
	}
}

func TestCovDeleteCategorySuccess(t *testing.T) {
	m := covNewManager(t)

	if err := m.AddCategory(&Category{ID: "cov-cat", Name: "Tmp"}); err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteCategory("cov-cat"); err != nil {
		t.Fatalf("DeleteCategory() error = %v", err)
	}
	if got, gErr := m.GetCategory("cov-cat"); gErr == nil && got != nil {
		t.Errorf("GetCategory() = %+v, want missing after delete", got)
	}
}

func TestCovImportInvalidJSON(t *testing.T) {
	m := covNewManager(t)

	if err := m.Import([]byte("not-json")); err == nil {
		t.Fatal("Import() should fail for invalid JSON")
	}
}

func TestCovImportGeneratesIDs(t *testing.T) {
	m := covNewManager(t)

	data := `{
  "links": [{"title": "Cov Link", "url": "https://cov.example.com"}],
  "categories": [{"name": "Cov Imported"}]
}`
	if err := m.Import([]byte(data)); err != nil {
		t.Fatalf("Import() error = %v", err)
	}

	links := m.List()
	if len(links) != 1 {
		t.Fatalf("List() = %v, want 1 imported link", links)
	}
	if links[0].ID == "" {
		t.Error("Import should generate link ID when missing")
	}

	cats := m.ListCategories()
	found := false
	for _, c := range cats {
		if c.Name == "Cov Imported" {
			found = c.ID != ""
		}
	}
	if !found {
		t.Error("Import should store category with generated ID")
	}
}
