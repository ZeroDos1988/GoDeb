package firstrun

import (
	"context"
	"errors"
	"net/netip"
	"path/filepath"
	"strings"
	"testing"

	"godeb/plugins/mail"
	"godeb/plugins/wireguard"
)

func fakeHome(t *testing.T, profile wireguard.SiteProfile, memKB int) *[]string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_AGENT_MASTER_KEY", filepath.Join(dir, "agent.key"))
	var calls []string
	oldRole, oldHub, oldStart, oldWait, oldAdd, oldMem := currentRole, hubTunnelAddr, startBehindHub, waitMail, addMailDomain, memTotalKB
	t.Cleanup(func() {
		currentRole, hubTunnelAddr, startBehindHub, waitMail, addMailDomain, memTotalKB = oldRole, oldHub, oldStart, oldWait, oldAdd, oldMem
	})
	currentRole = func() (wireguard.RoleInfo, error) {
		return wireguard.RoleInfo{Role: wireguard.RolePeer, Address: "10.8.0.2", Profile: profile}, nil
	}
	hubTunnelAddr = func() (netip.Addr, bool) { return netip.MustParseAddr("10.8.0.1"), true }
	startBehindHub = func(host, hub string, clamav bool) error {
		calls = append(calls, "start "+host+" "+hub+" "+map[bool]string{true: "clamav", false: "noclamav"}[clamav])
		return nil
	}
	waitMail = func(context.Context) error { calls = append(calls, "wait"); return nil }
	addMailDomain = func(_ context.Context, d string, _ mail.DomainOptions) (mail.DomainSetup, error) {
		calls = append(calls, "domain "+d)
		if d == "example.org" {
			return mail.DomainSetup{}, mail.ErrDomainExists
		}
		return mail.DomainSetup{Postmaster: "postmaster@" + d, Password: "pw", Aliases: []string{"abuse"}}, nil
	}
	memTotalKB = func() int { return memKB }
	return &calls
}

func TestSetupHomeServer(t *testing.T) {
	p, err := wireguard.SiteProfile{Kind: "home", MailHost: "postfach.example.com", MailDomains: []string{"example.com", "example.org"}}.Normalize()
	if err != nil {
		t.Fatal(err)
	}
	calls := fakeHome(t, p, 8<<20)
	res, err := SetupHomeServer(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := "start postfach.example.com 10.8.0.1 clamav|wait|domain example.com|domain example.org"
	if got := strings.Join(*calls, "|"); got != want {
		t.Fatalf("calls %s", got)
	}
	if len(res.Mailboxes) != 1 || res.Mailboxes[0].Address != "postmaster@example.com" || !res.ClamAV {
		t.Fatalf("result %+v", res)
	}
}

func TestSetupHomeServerSmallAndWithoutMail(t *testing.T) {
	p, _ := wireguard.SiteProfile{Kind: "home", MailHost: "postfach.example.com", MailDomains: []string{"example.com"}}.Normalize()
	calls := fakeHome(t, p, 2<<20)
	res, err := SetupHomeServer(t.Context())
	if err != nil || res.ClamAV || !strings.Contains(strings.Join(*calls, "|"), "noclamav") {
		t.Fatalf("small host: %+v %v %v", res, err, *calls)
	}
	calls = fakeHome(t, wireguard.SiteProfile{Kind: "home"}, 8<<20)
	if res, err := SetupHomeServer(t.Context()); err != nil || len(*calls) != 0 || len(res.Notes) == 0 {
		t.Fatalf("no mail host: %+v %v %v", res, err, *calls)
	}
	fakeHome(t, wireguard.SiteProfile{Kind: "play"}, 8<<20)
	if _, err := SetupHomeServer(t.Context()); err == nil {
		t.Fatal("play site set up as home server")
	}
	fakeHome(t, p, 8<<20)
	startBehindHub = func(string, string, bool) error { return errors.New("apt kaputt") }
	if _, err := SetupHomeServer(t.Context()); err == nil {
		t.Fatal("install error swallowed")
	}
}
