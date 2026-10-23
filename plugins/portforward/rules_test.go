package portforward

import (
	"net/netip"
	"strings"
	"testing"
)

func testEnv() hostEnv {
	return hostEnv{
		PublicIf: "eth0",
		HubAddr:  netip.MustParseAddr("10.8.0.1"),
		Prefix:   netip.MustParsePrefix("10.8.0.0/24"),
		Peers:    map[netip.Addr]bool{netip.MustParseAddr("10.8.0.2"): true, netip.MustParseAddr("10.8.0.10"): true},
		Reserved: map[string]map[int]string{"tcp": {22: "SSH", 2222: "SSH", 8080: "GoDeb"}, "udp": {51820: "WireGuard"}},
	}
}

var enshrouded = Rule{Name: "enshrouded", Proto: "udp", From: 15636, To: 15637, Target: "10.8.0.2"}

func TestValidateAcceptsEnshrouded(t *testing.T) {
	if err := validate([]Rule{enshrouded, {Name: "web", Proto: "tcp", From: 8443, To: 8443, Target: "10.8.0.2"}}, testEnv()); err != nil {
		t.Fatal(err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string][]Rule{
		"ssh":           {{Name: "ssh", Proto: "tcp", From: 2222, To: 2222, Target: "10.8.0.2"}},
		"wireguard":     {{Name: "wg", Proto: "udp", From: 51000, To: 52000, Target: "10.8.0.2"}},
		"ui":            {{Name: "ui", Proto: "tcp", From: 8000, To: 8100, Target: "10.8.0.2"}},
		"low port":      {{Name: "smtp", Proto: "tcp", From: 25, To: 25, Target: "10.8.0.2"}},
		"range too big": {{Name: "big", Proto: "udp", From: 20000, To: 21000, Target: "10.8.0.2"}},
		"reversed":      {{Name: "rev", Proto: "udp", From: 16000, To: 15000, Target: "10.8.0.2"}},
		"hub itself":    {{Name: "hub", Proto: "udp", From: 15000, To: 15000, Target: "10.8.0.1"}},
		"outside net":   {{Name: "lan", Proto: "udp", From: 15000, To: 15000, Target: "192.168.1.10"}},
		"not a peer":    {{Name: "ghost", Proto: "udp", From: 15000, To: 15000, Target: "10.8.0.3"}},
		"bad name":      {{Name: "Bad Name", Proto: "udp", From: 15000, To: 15000, Target: "10.8.0.2"}},
		"injection":     {{Name: "x\"; flush ruleset", Proto: "udp", From: 15000, To: 15000, Target: "10.8.0.2"}},
		"bad proto":     {{Name: "icmp", Proto: "icmp", From: 15000, To: 15000, Target: "10.8.0.2"}},
		"overlap":       {enshrouded, {Name: "other", Proto: "udp", From: 15637, To: 15640, Target: "10.8.0.10"}},
		"duplicate":     {enshrouded, enshrouded},
	}
	for name, rules := range cases {
		if err := validate(rules, testEnv()); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	// Same ports on the other protocol do not overlap.
	if err := validate([]Rule{enshrouded, {Name: "tcp", Proto: "tcp", From: 15636, To: 15637, Target: "10.8.0.2"}}, testEnv()); err != nil {
		t.Errorf("tcp and udp on the same ports: %v", err)
	}
	bad := testEnv()
	bad.PublicIf = "wg0"
	if err := validate([]Rule{enshrouded}, bad); err == nil {
		t.Error("tunnel interface accepted as public interface")
	}
}

func TestRenderNft(t *testing.T) {
	out := renderNft([]Rule{enshrouded}, testEnv())
	for _, want := range []string{
		"table ip godeb_portforward\ndelete table ip godeb_portforward\n",
		"type nat hook prerouting priority dstnat; policy accept;",
		`iifname "eth0" udp dport 15636-15637 dnat to 10.8.0.2 comment "enshrouded"`,
		`oifname "wg0" ip daddr 10.8.0.2 udp dport 15636-15637 snat to 10.8.0.1 comment "enshrouded"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}
	empty := renderNft(nil, testEnv())
	if strings.Contains(empty, "chain") || !strings.Contains(empty, "delete table ip godeb_portforward") {
		t.Errorf("empty rule set must only remove the table:\n%s", empty)
	}
}

func TestRouteRules(t *testing.T) {
	env := testEnv()
	a := routeRules([]Rule{enshrouded}, env)
	if len(a) != 1 || a[0].In != "eth0" || a[0].Out != "wg0" || a[0].Port != "15636:15637" || a[0].Proto != "udp" || !strings.HasPrefix(a[0].Comment, "GoDeb-PF enshrouded-") {
		t.Fatalf("route=%+v", a)
	}
	env.PublicIf = "ens3"
	if b := routeRules([]Rule{enshrouded}, env); b[0].Comment == a[0].Comment {
		t.Fatal("a new public interface must change the comment so the old ufw rule is replaced")
	}
}
