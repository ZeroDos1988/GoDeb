package domains

import (
	"path/filepath"
	"testing"
)

func TestWithDefaultsKeepsPluginRecords(t *testing.T) {
	plugin := []Need{
		{Zone: "example.com", Name: "example.com", Type: "MX", Value: "10 mx.example.com.", Kind: "mx", Source: "mail"},
		{Zone: "example.com", Name: "Example.com", Type: "A", Value: "198.51.100.9", Kind: "a", Source: "websites"},
	}
	list := []Domain{{Name: "example.com", Defaults: true}, {Name: "other.org"}}
	got := withDefaults(plugin, list, "203.0.113.5")
	by := map[string]Need{}
	for _, n := range got {
		if n.Zone == "other.org" {
			t.Fatalf("defaults for a domain without them: %+v", n)
		}
		by[n.Name+"|"+n.Type] = n
	}
	if by["example.com|MX"].Value != "10 mx.example.com." {
		t.Fatalf("plugin MX replaced: %+v", by["example.com|MX"])
	}
	if _, dup := by["example.com|A"]; dup {
		t.Fatal("default apex A added next to the website's A record")
	}
	for _, name := range []string{"www.example.com|A", "mail.example.com|A", "webmail.example.com|A", "example.com|TXT", "_dmarc.example.com|TXT"} {
		n, ok := by[name]
		if !ok {
			t.Fatalf("missing default %s in %+v", name, got)
		}
		if n.Type == "A" && n.Value != "203.0.113.5" {
			t.Fatalf("%s = %q", name, n.Value)
		}
	}
	if len(got) != 2+5 {
		t.Fatalf("got %d needs: %+v", len(got), got)
	}
}

func TestDefaultNeedsWithoutIPHasNoARecords(t *testing.T) {
	for _, n := range defaultNeeds("example.com", "") {
		if n.Type == "A" {
			t.Fatalf("A record without an address: %+v", n)
		}
	}
}

func TestEnsureAddsAndUpdates(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))
	if _, err := Ensure("Example.COM", ModeManual, true); err != nil {
		t.Fatal(err)
	}
	d, err := Ensure("example.com", ModeHetzner, false)
	if err != nil {
		t.Fatal(err)
	}
	if d.Mode != ModeHetzner || d.Defaults {
		t.Fatalf("update lost: %+v", d)
	}
	if _, err := Ensure("bad domain", ModeManual, true); err == nil {
		t.Fatal("invalid domain accepted")
	}
	if _, err := Ensure("example.net", "route53", true); err == nil {
		t.Fatal("invalid mode accepted")
	}
}
