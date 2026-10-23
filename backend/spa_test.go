package main

import (
	"path/filepath"
	"testing"
)

func TestResolveDistFile(t *testing.T) {
	dist := filepath.Join(string(filepath.Separator), "srv", "godeb", "frontend", "dist")

	index, ok := resolveDistFile(dist, "/")
	if !ok || filepath.Base(index) != "index.html" {
		t.Fatalf("index = %s ok=%v", index, ok)
	}

	asset, ok := resolveDistFile(dist, "/assets/app.js")
	if !ok || asset != filepath.Join(dist, "assets", "app.js") {
		t.Fatalf("asset = %s", asset)
	}

	escaped, ok := resolveDistFile(dist, "/../../go.mod")
	if !ok {
		t.Fatal("cleaned URL should stay inside dist")
	}
	if !filepath.IsAbs(escaped) {
		rel, err := filepath.Rel(dist, escaped)
		if err != nil || len(rel) >= 2 && rel[:2] == ".." {
			t.Fatalf("escaped = %s", escaped)
		}
	}
	rel, err := filepath.Rel(dist, escaped)
	if err != nil || len(rel) >= 2 && rel[:2] == ".." {
		t.Fatalf("rel = %s err=%v", rel, err)
	}
}
