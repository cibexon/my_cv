package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func testSite(t *testing.T) *Site {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "en"), 0o755)
	os.WriteFile(filepath.Join(dir, "01-k8s.md"), []byte("# Кубер\n\n| a | b |\n|---|---|\n| 1 | 2 |\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "en", "01-k8s.md"), []byte("# Kube\n\ntext"), 0o644)
	site, err := loadSite(dir)
	if err != nil {
		t.Fatal(err)
	}
	return site
}

func TestTopicRendersMarkdown(t *testing.T) {
	site := testSite(t)
	mux := http.NewServeMux()
	mux.Handle("GET /t/{file}", site.topic("ru"))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/t/01-k8s.md", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<table>") || !strings.Contains(rec.Body.String(), "Кубер") {
		t.Fatalf("code %d, body %s", rec.Code, rec.Body)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/t/missing.md", nil))
	if rec.Code != 404 {
		t.Fatalf("missing topic: code %d", rec.Code)
	}
}

func TestEmptyContentDirFails(t *testing.T) {
	if _, err := loadSite(t.TempDir()); err == nil {
		t.Fatal("expected an error for a directory without markdown")
	}
}

func TestErrorFaultIsCounted(t *testing.T) {
	h := instrument("test", &Faults{ErrorRate: 1}, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	before := testutil.ToFloat64(requests.WithLabelValues("test", "500"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/", nil))
	if rec.Code != 500 || testutil.ToFloat64(requests.WithLabelValues("test", "500")) != before+1 {
		t.Fatalf("code %d, 500s not counted", rec.Code)
	}
}
