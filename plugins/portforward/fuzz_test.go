package portforward

import (
	"regexp"
	"strings"
	"testing"
)

// nftLine is every line renderNft may write; names, ports and addresses
// are reduced to what validate allows.
var nftLine = regexp.MustCompile(`^(?:# Managed by GoDeb \(plugins/portforward\)\. Changes are overwritten\.|` +
	`table ip godeb_portforward(?: \{)?|delete table ip godeb_portforward|\}|` +
	`chain (?:prerouting|postrouting) \{|` +
	`type nat hook (?:prerouting priority dstnat|postrouting priority srcnat); policy accept;|` +
	`iifname "eth0" (?:udp|tcp) dport [0-9]{4,5}(?:-[0-9]{4,5})? dnat to 10\.8\.0\.[0-9]+ comment "[a-z0-9-]+"|` +
	`oifname "wg0" ip daddr 10\.8\.0\.[0-9]+ (?:udp|tcp) dport [0-9]{4,5}(?:-[0-9]{4,5})? snat to 10\.8\.0\.1 comment "[a-z0-9-]+")$`)

var (
	routeComment = regexp.MustCompile(`^GoDeb-PF [a-z0-9-]+-[0-9a-f]{6}$`)
	routePort    = regexp.MustCompile(`^[0-9]{4,5}(?::[0-9]{4,5})?$`)
)

func FuzzRules(f *testing.F) {
	f.Add("enshrouded", "udp", 15636, 15637, "10.8.0.2", "web", "tcp", 8443, 8443, "10.8.0.10")
	f.Add("a\" accept", "udp", 2000, 2000, "10.8.0.2", "b", "tcp", 22, 22, "10.8.0.2")
	f.Add("x", "udp", 3000, 2000, "10.8.0.1", "y", "icmp", 1, 70000, "8.8.8.8")
	f.Fuzz(func(t *testing.T, n1, p1 string, f1, t1 int, a1, n2, p2 string, f2, t2 int, a2 string) {
		rules := []Rule{{Name: n1, Proto: p1, From: f1, To: t1, Target: a1}, {Name: n2, Proto: p2, From: f2, To: t2, Target: a2}}
		if validate(rules, testEnv()) != nil {
			return
		}
		depth := 0
		for _, line := range strings.Split(strings.TrimSpace(renderNft(rules, testEnv())), "\n") {
			line = strings.TrimSpace(line)
			if !nftLine.MatchString(line) {
				t.Fatalf("unexpected nft line %q for %+v", line, rules)
			}
			depth += strings.Count(line, "{") - strings.Count(line, "}")
		}
		if depth != 0 {
			t.Fatal("braces not balanced")
		}
		for _, r := range routeRules(rules, testEnv()) {
			if !routeComment.MatchString(r.Comment) || !routePort.MatchString(r.Port) || !strings.HasPrefix(r.To, "10.8.0.") {
				t.Fatalf("unsafe ufw route rule %+v", r)
			}
		}
	})
}
