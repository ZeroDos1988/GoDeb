package wireguard

import (
	"strings"
	"testing"
	"time"
)

func homeProfile() SiteProfile {
	return SiteProfile{Kind: ProfileHome, MailHost: "Postfach.Example.com.", MailDomains: []string{"example.com", "EXAMPLE.org", "example.com"}}
}

func TestProfileNormalize(t *testing.T) {
	p, err := homeProfile().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	if p.MailHost != "postfach.example.com" || strings.Join(p.MailDomains, ",") != "example.com,example.org" {
		t.Fatalf("normalized %+v", p)
	}
	bad := []SiteProfile{
		{Kind: "root"},
		{Kind: ProfilePlay, MailHost: "postfach.example.com"},
		{Kind: ProfileHome, MailHost: "postfach.example.com"},
		{Kind: ProfileHome, MailHost: "postfach", MailDomains: []string{"example.com"}},
		{Kind: ProfileHome, MailHost: "postfach.example.com", MailDomains: []string{"example.com; PostUp = id"}},
		{Kind: ProfileHome, MailHost: "a b.example.com", MailDomains: []string{"example.com"}},
		{Kind: ProfileHome, MailHost: "-x.example.com", MailDomains: []string{"example.com"}},
	}
	for _, b := range bad {
		if _, err := b.Normalize(); err == nil {
			t.Errorf("accepted %+v", b)
		}
	}
	many := SiteProfile{Kind: ProfileHome, MailHost: "postfach.example.com"}
	for i := 0; i <= maxMailDomains; i++ {
		many.MailDomains = append(many.MailDomains, "d"+strings.Repeat("x", i)+".example.com")
	}
	if _, err := many.Normalize(); err == nil {
		t.Error("accepted too many domains")
	}
}

// The profile travels hub client file → join code → site wg0.conf.
func TestProfileRoundTrip(t *testing.T) {
	p, err := homeProfile().Normalize()
	if err != nil {
		t.Fatal(err)
	}
	conf := RenderClient(ClientSpec{
		Name: "heim-vm", Keys: Keypair{Private: testPriv, Public: testPub}, PresharedKey: testPSK,
		Address: mustAddr("10.8.0.2"), ServerKey: testPub, Endpoint: "203.0.113.7:51820",
		AllowedIPs: []string{"10.8.0.0/24"}, Created: time.Unix(0, 0), Site: true, Profile: p,
	})
	if c := ParseClient("heim-vm", conf); c.Profile.MailHost != "postfach.example.com" {
		t.Fatalf("client file profile %+v", c.Profile)
	}
	jc, _, err := parseJoinConfig(JoinCode(conf))
	if err == nil {
		t.Fatal("parseJoinConfig must not decode join codes itself")
	}
	content, err := decodeJoinInput(JoinCode(conf))
	if err != nil {
		t.Fatal(err)
	}
	if jc, _, err = parseJoinConfig(content); err != nil {
		t.Fatal(err)
	}
	role := DetectRole(renderPeerConfig(jc))
	if role.Role != RolePeer || role.Profile.Kind != ProfileHome || len(role.Profile.MailDomains) != 2 {
		t.Fatalf("site wg0.conf role %+v", role)
	}
	// A plain site file stays without profile.
	if DetectRole(renderPeerConfig(mustJoin(t, siteClientConf()))).Profile.Kind != "" {
		t.Fatal("plain site got a profile")
	}
}

func TestJoinRejectsBadProfile(t *testing.T) {
	for _, extra := range []string{
		"# GoDeb-Profile: admin\n",
		"# GoDeb-Profile: home\n# GoDeb-Mail-Host: postfach.example.com\n# GoDeb-Mail-Domains: exa mple.com\n",
		"# GoDeb-Profile: play\n# GoDeb-Mail-Host: postfach.example.com\n",
	} {
		if _, _, err := parseJoinConfig(extra + siteClientConf()); err == nil {
			t.Errorf("accepted %q", extra)
		}
	}
}

func mustJoin(t *testing.T, conf string) joinConfig {
	t.Helper()
	jc, _, err := parseJoinConfig(conf)
	if err != nil {
		t.Fatal(err)
	}
	return jc
}
