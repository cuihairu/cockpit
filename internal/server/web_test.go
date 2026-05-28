package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cuihairu/cockpit/internal/config"
)

func TestSPAHandlerNoStaticDir(t *testing.T) {
	s := &Server{cfg: &config.Config{Server: &config.ServerConfig{}}}
	handler := s.spaHandler()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	var resp map[string]string
	json.Unmarshal(rec.Body.Bytes(), &resp)
	if resp["message"] == "" {
		t.Error("should return API message when no static dir")
	}
}

func TestSPAHandlerWithStaticDir(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/index.html", []byte("<html>test</html>"), 0644)

	s := &Server{cfg: &config.Config{Server: &config.ServerConfig{StaticDir: dir}}}
	handler := s.spaHandler()

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "<html>test</html>" {
		t.Errorf("body = %q, want index.html content", rec.Body.String())
	}
}

func TestSPAHandlerSPAFallback(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/index.html", []byte("<html>fallback</html>"), 0644)

	s := &Server{cfg: &config.Config{Server: &config.ServerConfig{StaticDir: dir}}}
	handler := s.spaHandler()

	req := httptest.NewRequest("GET", "/some/spa/route", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (SPA fallback)", rec.Code)
	}
	if rec.Body.String() != "<html>fallback</html>" {
		t.Errorf("body = %q, want fallback index.html", rec.Body.String())
	}
}

func TestSPAHandlerStaticFile(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(dir+"/index.html", []byte("index"), 0644)
	os.WriteFile(dir+"/app.js", []byte("console.log(1)"), 0644)

	s := &Server{cfg: &config.Config{Server: &config.ServerConfig{StaticDir: dir}}}
	handler := s.spaHandler()

	req := httptest.NewRequest("GET", "/app.js", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
	if rec.Body.String() != "console.log(1)" {
		t.Errorf("body = %q, want app.js content", rec.Body.String())
	}
}

func TestStaticDirFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STATIC_DIR", dir)

	s := &Server{cfg: &config.Config{Server: &config.ServerConfig{}}}
	if got := s.staticDir(); got != dir {
		t.Errorf("staticDir() = %q, want %q", got, dir)
	}
}

func TestStaticDirFromConfig(t *testing.T) {
	s := &Server{cfg: &config.Config{Server: &config.ServerConfig{StaticDir: "/some/path"}}}
	if got := s.staticDir(); got != "/some/path" {
		t.Errorf("staticDir() = %q, want /some/path", got)
	}
}

func TestStaticDirEmpty(t *testing.T) {
	s := &Server{cfg: &config.Config{Server: &config.ServerConfig{}}}
	if got := s.staticDir(); got != "" {
		t.Errorf("staticDir() = %q, want empty", got)
	}
}
