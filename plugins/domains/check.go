package domains

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

type resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

var dnsResolver resolver = net.DefaultResolver

// zoneResolver asks the zone's authoritative name servers directly. Right
// after a record was created, the local resolver may still cache "does not
// exist" for up to the SOA minimum TTL (Hetzner: one hour), so the check
// reported records as missing that were already published. Falls back to
// dnsResolver when the name servers cannot be found, and in tests that
// replace dnsResolver.
func zoneResolver(ctx context.Context, zone string, cache map[string]resolver) resolver {
	if def, ok := dnsResolver.(*net.Resolver); !ok || def != net.DefaultResolver || zone == "" {
		return dnsResolver
	}
	if r, ok := cache[zone]; ok {
		return r
	}
	cache[zone] = dnsResolver
	nss, err := net.DefaultResolver.LookupNS(ctx, zone)
	if err != nil {
		return dnsResolver
	}
	var addrs []string
	for _, ns := range nss {
		ips, err := net.DefaultResolver.LookupHost(ctx, ns.Host)
		if err != nil {
			continue
		}
		for _, ip := range ips {
			if net.ParseIP(ip).To4() != nil {
				addrs = append(addrs, net.JoinHostPort(ip, "53"))
				break
			}
		}
	}
	if len(addrs) == 0 {
		return dnsResolver
	}
	r := &authResolver{auth: &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		var last error
		for _, a := range addrs {
			c, err := d.DialContext(ctx, network, a)
			if err == nil {
				return c, nil
			}
			last = err
		}
		return nil, last
	}}}
	cache[zone] = r
	return r
}

// authResolver asks the authoritative servers and falls back to the local
// resolver when they do not answer (blocked port 53, timeout).
type authResolver struct{ auth *net.Resolver }

func fallback(err error) bool {
	var dnsErr *net.DNSError
	return err != nil && !(errors.As(err, &dnsErr) && dnsErr.IsNotFound)
}

func (a *authResolver) LookupMX(ctx context.Context, name string) ([]*net.MX, error) {
	v, err := a.auth.LookupMX(ctx, name)
	if fallback(err) {
		return dnsResolver.LookupMX(ctx, name)
	}
	return v, err
}

func (a *authResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	v, err := a.auth.LookupTXT(ctx, name)
	if fallback(err) {
		return dnsResolver.LookupTXT(ctx, name)
	}
	return v, err
}

func (a *authResolver) LookupHost(ctx context.Context, host string) ([]string, error) {
	v, err := a.auth.LookupHost(ctx, host)
	if fallback(err) {
		return dnsResolver.LookupHost(ctx, host)
	}
	return v, err
}

func (a *authResolver) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	return dnsResolver.LookupAddr(ctx, addr)
}

func recordsFor(ctx context.Context, needs []Need) []Record {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out := make([]Record, 0, len(needs))
	cache := map[string]resolver{}
	for _, n := range needs {
		res := zoneResolver(ctx, n.Zone, cache)
		r := Record{Need: n, Status: "unknown"}
		v := n.Value
		if n.Type == "TXT" {
			v = splitTXT(v)
		}
		r.ZoneLine = n.Name + ". IN " + n.Type + " " + v
		switch n.Kind {
		case "mx":
			checkMX(ctx, res, &r)
		case "spf", "dkim", "dmarc", "mtasts", "tlsrpt":
			checkTXT(ctx, res, &r)
		case "a", "mtasts-host", "autoconfig", "autodiscover":
			checkA(ctx, res, &r)
		case "ptr":
			checkPTR(ctx, res, &r)
		default:
			r.Status = "unknown"
		}
		out = append(out, r)
	}
	return out
}

func checkMX(ctx context.Context, res resolver, r *Record) {
	mxs, err := res.LookupMX(ctx, r.Name)
	want := strings.TrimSuffix(strings.Fields(r.Value)[1], ".")
	found := []string{}
	r.Status = statusFrom(err)
	for _, mx := range mxs {
		host := strings.TrimSuffix(mx.Host, ".")
		found = append(found, host)
		if strings.EqualFold(host, want) {
			r.Status = "ok"
		}
	}
	if r.Status != "ok" && len(found) > 0 {
		r.Status = "wrong"
	}
	r.Found = strings.Join(found, ", ")
}

func checkTXT(ctx context.Context, res resolver, r *Record) {
	txts, err := res.LookupTXT(ctx, r.Name)
	r.Status = statusFrom(err)
	prefix := map[string]string{
		"spf": "v=spf1", "dkim": "v=DKIM1", "dmarc": "v=DMARC1",
		"mtasts": "v=STSv1", "tlsrpt": "v=TLSRPTv1",
	}[r.Kind]
	for _, t := range txts {
		if !strings.HasPrefix(strings.TrimSpace(t), prefix) {
			continue
		}
		r.Found = t
		r.Status = "wrong"
		if txtMatches(r.Kind, t, r.Value) {
			r.Status = "ok"
		}
	}
}

func checkA(ctx context.Context, res resolver, r *Record) {
	ips, err := res.LookupHost(ctx, r.Name)
	r.Status = statusFrom(err)
	r.Found = strings.Join(ips, ", ")
	for _, ip := range ips {
		if ip == r.Value {
			r.Status = "ok"
		}
	}
	if r.Status != "ok" && len(ips) > 0 {
		r.Status = "wrong"
	}
}

func checkPTR(ctx context.Context, res resolver, r *Record) {
	names, err := res.LookupAddr(ctx, r.Name)
	r.Status = statusFrom(err)
	r.Found = strings.Join(names, ", ")
	want := strings.TrimSuffix(r.Value, ".")
	for _, n := range names {
		if strings.EqualFold(strings.TrimSuffix(n, "."), want) {
			r.Status = "ok"
		}
	}
	if r.Status != "ok" && len(names) > 0 {
		r.Status = "wrong"
	}
}

func statusFrom(err error) string {
	var dnsErr *net.DNSError
	if err == nil || (errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
		return "missing"
	}
	return "unknown"
}

func txtMatches(kind, found, want string) bool {
	norm := func(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }
	if kind == "dkim" {
		key := func(s string) string {
			i := strings.Index(s, "p=")
			if i < 0 {
				return ""
			}
			return strings.TrimRight(strings.ReplaceAll(s[i+2:], " ", ""), ";")
		}
		return key(found) != "" && key(found) == key(want)
	}
	if kind == "spf" {
		f := norm(found)
		return strings.Contains(f, " mx") && (strings.HasSuffix(f, "-all") || strings.HasSuffix(f, "~all"))
	}
	if kind == "mtasts" {
		f := norm(found)
		return strings.HasPrefix(f, "v=stsv1;") && strings.Contains(f, "id=")
	}
	if kind == "tlsrpt" {
		f := norm(found)
		return strings.HasPrefix(f, "v=tlsrptv1;") && strings.Contains(f, "rua=mailto:")
	}
	return strings.HasPrefix(norm(found), "v=dmarc1; p=quarantine") || strings.HasPrefix(norm(found), "v=dmarc1; p=reject") || norm(found) == norm(want)
}

// splitTXT wraps long TXT values the way zone files expect.
func splitTXT(v string) string {
	const chunk = 255
	if len(v) <= chunk {
		return `"` + v + `"`
	}
	var b strings.Builder
	for i := 0; i < len(v); i += chunk {
		end := i + chunk
		if end > len(v) {
			end = len(v)
		}
		if i > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('"')
		b.WriteString(v[i:end])
		b.WriteByte('"')
	}
	return b.String()
}
