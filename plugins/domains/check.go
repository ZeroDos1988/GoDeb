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

func recordsFor(ctx context.Context, needs []Need) []Record {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	out := make([]Record, 0, len(needs))
	for _, n := range needs {
		r := Record{Need: n, Status: "unknown"}
		v := n.Value
		if n.Type == "TXT" {
			v = splitTXT(v)
		}
		r.ZoneLine = n.Name + ". IN " + n.Type + " " + v
		switch n.Kind {
		case "mx":
			checkMX(ctx, &r)
		case "spf", "dkim", "dmarc", "mtasts", "tlsrpt":
			checkTXT(ctx, &r)
		case "a", "mtasts-host", "autoconfig", "autodiscover":
			checkA(ctx, &r)
		case "ptr":
			checkPTR(ctx, &r)
		default:
			r.Status = "unknown"
		}
		out = append(out, r)
	}
	return out
}

func checkMX(ctx context.Context, r *Record) {
	mxs, err := dnsResolver.LookupMX(ctx, r.Name)
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

func checkTXT(ctx context.Context, r *Record) {
	txts, err := dnsResolver.LookupTXT(ctx, r.Name)
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

func checkA(ctx context.Context, r *Record) {
	ips, err := dnsResolver.LookupHost(ctx, r.Name)
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

func checkPTR(ctx context.Context, r *Record) {
	names, err := dnsResolver.LookupAddr(ctx, r.Name)
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
