package doctor

import (
	"context"
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Meta is the head of the diagnosis report: what runs where.
type Meta struct {
	Hostname string `json:"hostname"`
	FQDN     string `json:"fqdn,omitempty"`
	Version  string `json:"version"`
	OS       string `json:"os,omitempty"`
	Kernel   string `json:"kernel,omitempty"`
	Uptime   string `json:"uptime,omitempty"`
	Arch     string `json:"arch,omitempty"`
	Role     string `json:"role,omitempty"` // hub, peer or empty (no tunnel)
	Setup    string `json:"setup,omitempty"`
}

// FullReport is what the agent hands the web process for the text report:
// the findings, the head and the names to hide when the admin asks for a
// redacted report (host and domain names the agent knows).
type FullReport struct {
	Report    Report   `json:"report"`
	Meta      Meta     `json:"meta"`
	Sensitive []string `json:"sensitive,omitempty"`
}

// CollectMeta reads the host facts for the report head.
func CollectMeta(ctx context.Context, env Env) Meta {
	m := Meta{Version: env.Version}
	if b, err := env.ReadFile("/etc/hostname"); err == nil {
		m.Hostname = strings.TrimSpace(string(b))
	}
	if res := env.Run(ctx, "hostname", "-f"); res.Err == nil {
		if f := strings.TrimSpace(res.Stdout); f != "" && f != m.Hostname {
			m.FQDN = f
		}
	}
	if b, err := env.ReadFile("/etc/os-release"); err == nil {
		for _, l := range lines(string(b)) {
			if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
				m.OS = strings.Trim(v, `"`)
			}
		}
	}
	if b, err := env.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		m.Kernel = strings.TrimSpace(string(b))
	}
	if b, err := env.ReadFile("/proc/uptime"); err == nil {
		if f := strings.Fields(string(b)); len(f) > 0 {
			if secs, err := strconv.ParseFloat(f[0], 64); err == nil {
				m.Uptime = (time.Duration(secs) * time.Second).Round(time.Minute).String()
			}
		}
	}
	if res := env.Run(ctx, "dpkg", "--print-architecture"); res.Err == nil {
		m.Arch = strings.TrimSpace(res.Stdout)
	}
	return m
}

// Text renders the report as plain text for copying or attaching to an
// issue. It holds no secrets; Redact hides addresses and names on top.
func Text(meta Meta, r Report, at time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "GoDeb-Diagnosebericht – %s\n\n", at.Format("2006-01-02 15:04 MST"))
	row := func(k, v string) {
		if v != "" {
			fmt.Fprintf(&b, "%-10s %s\n", k+":", v)
		}
	}
	row("Host", meta.Hostname)
	row("FQDN", meta.FQDN)
	row("GoDeb", meta.Version)
	row("System", meta.OS)
	row("Kernel", meta.Kernel)
	row("Arch", meta.Arch)
	row("Laufzeit", meta.Uptime)
	row("Rolle", meta.Role)
	row("Setup", meta.Setup)
	b.WriteString("\n")
	area := ""
	for _, f := range r.Findings {
		if f.Area != area {
			area = f.Area
			fmt.Fprintf(&b, "== %s\n", area)
		}
		fmt.Fprintf(&b, "%-5s %s\n", f.Level, f.Text)
		for _, d := range f.Details {
			fmt.Fprintf(&b, "      %s\n", d)
		}
	}
	fmt.Fprintf(&b, "\nErgebnis: %d Fehler, %d Warnungen\n", r.Fails, r.Warns)
	return b.String()
}

// Summary is the short form for a prefilled GitHub issue: head and only
// the failures and warnings.
func Summary(meta Meta, r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "GoDeb %s · %s · %s", meta.Version, meta.OS, meta.Arch)
	if meta.Role != "" {
		fmt.Fprintf(&b, " · Rolle %s", meta.Role)
	}
	fmt.Fprintf(&b, "\n%d Fehler, %d Warnungen\n", r.Fails, r.Warns)
	for _, f := range r.Findings {
		if f.Level == "FAIL" || f.Level == "WARN" {
			fmt.Fprintf(&b, "- %s [%s] %s\n", f.Level, f.Area, f.Text)
		}
	}
	return b.String()
}

var (
	ipv4Pattern = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	// IPv6 candidates: at least two colons and hex groups; netip decides.
	ipv6Pattern = regexp.MustCompile(`(?i)\b[0-9a-f]{0,4}(?::[0-9a-f]{0,4}){2,7}\b`)
)

// keepIP reports addresses that say nothing about the host's identity:
// loopback, private ranges (tunnel, home network), link-local, unspecified
// and documentation ranges.
func keepIP(a netip.Addr) bool {
	if a.IsLoopback() || a.IsPrivate() || a.IsLinkLocalUnicast() || a.IsUnspecified() || a.IsMulticast() {
		return true
	}
	for _, p := range []string{"100.64.0.0/10", "192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24", "2001:db8::/32"} {
		if netip.MustParsePrefix(p).Contains(a) {
			return true
		}
	}
	return false
}

// commonWords are never redacted as names: a host called "godeb" or
// "debian" would otherwise blank out half the report.
var commonWords = map[string]bool{
	"localhost": true, "godeb": true, "debian": true, "trixie": true, "root": true, "server": true,
	"admin": true, "mail": true, "home": true, "wireguard": true, "agent": true, "linux": true,
}

// Redact hides public IP addresses and the given names (host and domain
// names). Each distinct value gets its own placeholder, so the report
// still shows which lines talk about the same address.
func Redact(text string, names []string) string {
	ips := map[string]string{}
	replaceIP := func(s string) string {
		a, err := netip.ParseAddr(s)
		if err != nil || keepIP(a.Unmap()) {
			return s
		}
		if p, ok := ips[a.String()]; ok {
			return p
		}
		p := fmt.Sprintf("<ip-%d>", len(ips)+1)
		ips[a.String()] = p
		return p
	}
	text = ipv4Pattern.ReplaceAllStringFunc(text, replaceIP)
	text = ipv6Pattern.ReplaceAllStringFunc(text, replaceIP)

	// Longest names first, so mail.example.org is not cut by example.org
	// before its own placeholder applies.
	uniq := map[string]bool{}
	var list []string
	for _, n := range names {
		n = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(n), "."))
		if len(n) < 3 || uniq[n] || commonWords[n] {
			continue
		}
		uniq[n] = true
		list = append(list, n)
	}
	sort.Slice(list, func(i, j int) bool { return len(list[i]) > len(list[j]) })
	for i, n := range list {
		re := regexp.MustCompile(`(?i)(^|[^a-z0-9-])` + regexp.QuoteMeta(n) + `($|[^a-z0-9-])`)
		p := fmt.Sprintf("<name-%d>", i+1)
		// Two passes: adjacent matches share a separator.
		for range 2 {
			text = re.ReplaceAllString(text, "${1}"+p+"${2}")
		}
	}
	return text
}
