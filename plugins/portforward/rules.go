// Package portforward exposes ports of a tunnel peer (a site such as the home
// VM) on the hub's public interface. NAT lives in an nftables table of its
// own; ufw only gets matching `ufw route allow` rules for its FORWARD chain.
package portforward

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"godeb/plugins/ufw"
)

const (
	rulesPath = "/etc/godeb/portforward.json"
	nftPath   = "/etc/godeb/portforward.nft"
	unitName  = "godeb-portforward.service"
	tableName = "godeb_portforward"
	routeTag  = "GoDeb-PF"
	tunnelIf  = "wg0"
	// minPort keeps system services (SSH, SMTP, HTTP) out of reach; the mail
	// gateway relays port 25 itself instead of forwarding it.
	minPort  = 1024
	maxRange = 1000
	maxRules = 32
)

var (
	namePattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,23}$`)
	ifacePattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,14}$`)
)

// Rule forwards From..To of Proto on the public interface to the same ports
// on Target, a peer address inside the tunnel network.
type Rule struct {
	Name   string `json:"name"`
	Proto  string `json:"proto"`
	From   int    `json:"from"`
	To     int    `json:"to"`
	Target string `json:"target"`
}

type rulesFile struct {
	Rules []Rule `json:"rules"`
}

// hostEnv is what validation and rendering need to know about the hub.
type hostEnv struct {
	PublicIf string
	PublicIP string
	HubAddr  netip.Addr
	Prefix   netip.Prefix
	// Peers are the /32 addresses configured in wg0.conf.
	Peers map[netip.Addr]bool
	// Reserved ports per protocol with the reason, e.g. SSH and WireGuard.
	Reserved map[string]map[int]string
}

func (r Rule) ports(sep string) string {
	if r.From == r.To {
		return strconv.Itoa(r.From)
	}
	return strconv.Itoa(r.From) + sep + strconv.Itoa(r.To)
}

// validate checks every rule against the hub and against each other.
func validate(rules []Rule, env hostEnv) error {
	if len(rules) > maxRules {
		return fmt.Errorf("höchstens %d Weiterleitungen", maxRules)
	}
	if !ifacePattern.MatchString(env.PublicIf) || env.PublicIf == tunnelIf {
		return fmt.Errorf("öffentliches Interface %q nicht erkannt", env.PublicIf)
	}
	names := map[string]bool{}
	for i, r := range rules {
		if !namePattern.MatchString(r.Name) {
			return fmt.Errorf("Name %q: 1–24 Zeichen, Kleinbuchstaben, Ziffern und -", r.Name)
		}
		if names[r.Name] {
			return fmt.Errorf("Name %s ist doppelt", r.Name)
		}
		names[r.Name] = true
		if r.Proto != "udp" && r.Proto != "tcp" {
			return fmt.Errorf("%s: Protokoll muss udp oder tcp sein", r.Name)
		}
		if r.From < minPort || r.To > 65535 || r.From > r.To {
			return fmt.Errorf("%s: Ports müssen zwischen %d und 65535 liegen, von ≤ bis", r.Name, minPort)
		}
		if r.To-r.From >= maxRange {
			return fmt.Errorf("%s: höchstens %d Ports pro Regel", r.Name, maxRange)
		}
		for p := r.From; p <= r.To; p++ {
			if why, ok := env.Reserved[r.Proto][p]; ok {
				return fmt.Errorf("%s: Port %d/%s ist für %s reserviert", r.Name, p, r.Proto, why)
			}
		}
		target, err := netip.ParseAddr(r.Target)
		if err != nil || !target.Is4() {
			return fmt.Errorf("%s: Ziel muss eine IPv4-Adresse sein", r.Name)
		}
		if !env.Prefix.Contains(target) || target == env.HubAddr {
			return fmt.Errorf("%s: Ziel %s liegt nicht im Tunnelnetz %s oder ist der Hub selbst", r.Name, target, env.Prefix.Masked())
		}
		if !env.Peers[target] {
			return fmt.Errorf("%s: %s ist kein Peer in wg0.conf", r.Name, target)
		}
		for _, o := range rules[:i] {
			if o.Proto == r.Proto && r.From <= o.To && o.From <= r.To {
				return fmt.Errorf("%s überschneidet sich mit %s", r.Name, o.Name)
			}
		}
	}
	return nil
}

// renderNft produces an nft script that replaces the GoDeb table atomically.
// Without rules it only removes the table.
func renderNft(rules []Rule, env hostEnv) string {
	var b strings.Builder
	b.WriteString("# Managed by GoDeb (plugins/portforward). Changes are overwritten.\n")
	fmt.Fprintf(&b, "table ip %s\ndelete table ip %s\n", tableName, tableName)
	if len(rules) == 0 {
		return b.String()
	}
	fmt.Fprintf(&b, "table ip %s {\n", tableName)
	b.WriteString("\tchain prerouting {\n\t\ttype nat hook prerouting priority dstnat; policy accept;\n")
	for _, r := range rules {
		fmt.Fprintf(&b, "\t\tiifname %q %s dport %s dnat to %s comment %q\n", env.PublicIf, r.Proto, r.ports("-"), r.Target, r.Name)
	}
	b.WriteString("\t}\n")
	// SNAT to the hub's tunnel address so replies return through the tunnel
	// instead of leaving the site via its home router.
	b.WriteString("\tchain postrouting {\n\t\ttype nat hook postrouting priority srcnat; policy accept;\n")
	for _, r := range rules {
		fmt.Fprintf(&b, "\t\toifname %q ip daddr %s %s dport %s snat to %s comment %q\n", tunnelIf, r.Target, r.Proto, r.ports("-"), env.HubAddr, r.Name)
	}
	b.WriteString("\t}\n}\n")
	return b.String()
}

// routeRules are the ufw FORWARD rules matching the NAT rules. The comment
// carries a hash of the rule, so a changed rule (or a new public interface)
// replaces the old ufw rule on the next sync.
func routeRules(rules []Rule, env hostEnv) []ufw.RouteRule {
	out := make([]ufw.RouteRule, 0, len(rules))
	for _, r := range rules {
		sum := sha256.Sum256([]byte(strings.Join([]string{r.Name, r.Proto, r.ports(":"), r.Target, env.PublicIf}, "|")))
		out = append(out, ufw.RouteRule{
			In: env.PublicIf, Out: tunnelIf, To: r.Target, Port: r.ports(":"), Proto: r.Proto,
			Comment: routeTag + " " + r.Name + "-" + hex.EncodeToString(sum[:3]),
		})
	}
	return out
}

func sortRules(rules []Rule) {
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].From != rules[j].From {
			return rules[i].From < rules[j].From
		}
		return rules[i].Proto < rules[j].Proto
	})
}
