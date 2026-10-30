package domains

import (
	"context"
	"path/filepath"
	"slices"
	"testing"
)

func setupHetzner(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))
	token := "abcdefghijklmnopqrstuvwxyz0123456789"
	if err := upsertSettings(&token, nil); err != nil {
		t.Fatal(err)
	}
	if err := addDomain("example.com", ModeHetzner); err != nil {
		t.Fatal(err)
	}
	if err := addDomain("manual.org", ModeManual); err != nil {
		t.Fatal(err)
	}
}

func TestHetznerZonesOnlyHetznerMode(t *testing.T) {
	setupHetzner(t)
	zones, tokenSet, err := HetznerZones()
	if err != nil || !tokenSet || !slices.Equal(zones, []string{"example.com"}) {
		t.Fatalf("zones = %v token=%v err=%v", zones, tokenSet, err)
	}
}

func TestPublishAndACMEChallenge(t *testing.T) {
	setupHetzner(t)
	f := &fakeHetzner{t: t, rrsets: map[string]hzRRSet{}}
	useFake(t, f)
	ctx := context.Background()

	if err := Publish(ctx, Need{Name: "godeb.example.com", Type: "A", Value: "10.8.0.1", Kind: "a"}); err != nil {
		t.Fatal(err)
	}
	if rr := f.rrsets["godeb/A"]; !isManaged(rr) || rr.Records[0].Value != "10.8.0.1" {
		t.Fatalf("A rrset = %+v", rr)
	}
	if err := Publish(ctx, Need{Name: "godeb.manual.org", Type: "A", Value: "10.8.0.1", Kind: "a"}); err == nil {
		t.Fatal("manual zone must be refused")
	}

	zone, cleanup, err := SetACMEChallenge(ctx, "godeb.example.com", "tokenvalue")
	if err != nil || zone != "example.com" {
		t.Fatalf("challenge: %q %v", zone, err)
	}
	if rr := f.rrsets["_acme-challenge.godeb/TXT"]; !isManaged(rr) || rr.Records[0].Value != `"tokenvalue"` {
		t.Fatalf("TXT rrset = %+v", rr)
	}
	cleanup(ctx)
	if _, ok := f.rrsets["_acme-challenge.godeb/TXT"]; ok {
		t.Fatal("cleanup left the TXT record")
	}

	// A foreign record is never touched.
	f.rrsets["_acme-challenge.godeb/TXT"] = hzRRSet{Name: "_acme-challenge.godeb", Type: "TXT", Records: []hzRecord{{Value: `"x"`}}}
	if _, _, err := SetACMEChallenge(ctx, "godeb.example.com", "tokenvalue"); err == nil {
		t.Fatal("foreign TXT must be a conflict")
	}
	f.rrsets["godeb/A"] = hzRRSet{Name: "godeb", Type: "A", Records: []hzRecord{{Value: "1.2.3.4"}}}
	if err := Publish(ctx, Need{Name: "godeb.example.com", Type: "A", Value: "10.8.0.1", Kind: "a"}); err == nil {
		t.Fatal("foreign A must be a conflict")
	}
}
