package firstrun

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanDomains(t *testing.T) {
	got, err := CleanDomains([]string{" Example.COM. ", "", "example.com", "shop.example.org"})
	if err != nil || strings.Join(got, ",") != "example.com,shop.example.org" {
		t.Fatalf("got %v, %v", got, err)
	}
	for _, bad := range [][]string{nil, {" "}, {"exa mple.com"}, {"-bad.com"}, {"example.com;rm"}} {
		if _, err := CleanDomains(bad); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
	many := make([]string, maxSetupDomains+1)
	for i := range many {
		many[i] = "d" + strings.Repeat("x", i+1) + ".de"
	}
	if _, err := CleanDomains(many); err == nil {
		t.Fatal("too many domains accepted")
	}
}

func TestSetupDomainsRefusesRelayWithoutPackages(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))
	_, err := SetupDomains(context.Background(), DomainsSpec{Domains: []string{"example.com"}, MailRelay: true})
	if err == nil || !strings.Contains(err.Error(), "nicht installiert") {
		t.Fatalf("err = %v", err)
	}
}
