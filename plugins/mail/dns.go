package mail

import (
	"context"
	"errors"
	"net"
	"strings"
	"time"
)

// Record is one DNS entry the admin has to create, with the live result.
type Record struct {
	Kind   string `json:"kind"` // mx, spf, dkim, dmarc, mtasts, tlsrpt, a, mtasts-host, ptr
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Zone   string `json:"zone"`   // value as a zone file line (TXT split)
	Status string `json:"status"` // ok, missing, wrong, unknown
	Found  string `json:"found,omitempty"`
}

// resolver is replaced in tests.
type resolver interface {
	LookupMX(ctx context.Context, name string) ([]*net.MX, error)
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupHost(ctx context.Context, host string) ([]string, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

var dnsResolver resolver = net.DefaultResolver

// expectedRecords lists what a domain needs. SPF allows only the MX to
// send (-all), DMARC asks receivers to quarantine what fails.
func expectedRecords(s Settings, d Domain, publicIP string) []Record {
	recs := []Record{
		{Kind: "mx", Name: d.Name, Type: "MX", Value: "10 " + s.Hostname + "."},
		{Kind: "spf", Name: d.Name, Type: "TXT", Value: "v=spf1 mx -all"},
	}
	if d.DKIMPublic != "" {
		recs = append(recs, Record{Kind: "dkim", Name: d.Selector + "._domainkey." + d.Name, Type: "TXT", Value: dkimRecordValue(d.DKIMPublic)})
	}
	recs = append(recs,
		Record{Kind: "dmarc", Name: "_dmarc." + d.Name, Type: "TXT", Value: "v=DMARC1; p=quarantine; adkim=s; aspf=s"},
		Record{Kind: "mtasts", Name: "_mta-sts." + d.Name, Type: "TXT", Value: mtaSTSRecord(d.Name, s)},
		Record{Kind: "tlsrpt", Name: "_smtp._tls." + d.Name, Type: "TXT", Value: tlsRPTRecord(d.Name)},
	)
	if publicIP != "" {
		recs = append(recs,
			Record{Kind: "a", Name: s.Hostname, Type: "A", Value: publicIP},
			Record{Kind: "mtasts-host", Name: "mta-sts." + d.Name, Type: "A", Value: publicIP},
			Record{Kind: "ptr", Name: publicIP, Type: "PTR", Value: s.Hostname + "."},
		)
	}
	for i := range recs {
		r := &recs[i]
		v := r.Value
		if r.Type == "TXT" {
			v = splitTXT(v)
		}
		r.Zone = r.Name + ". IN " + r.Type + " " + v
		r.Status = "unknown"
	}
	return recs
}

// checkRecords looks every record up and sets Status and Found.
func checkRecords(ctx context.Context, recs []Record) []Record {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	for i := range recs {
		r := &recs[i]
		switch r.Kind {
		case "mx":
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
		case "spf", "dkim", "dmarc", "mtasts", "tlsrpt":
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
		case "a", "mtasts-host":
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
		case "ptr":
			names, err := dnsResolver.LookupAddr(ctx, r.Name)
			r.Status = statusFrom(err)
			r.Found = strings.Join(names, ", ")
			for _, n := range names {
				if strings.EqualFold(strings.TrimSuffix(n, "."), strings.TrimSuffix(r.Value, ".")) {
					r.Status = "ok"
				}
			}
			if r.Status != "ok" && len(names) > 0 {
				r.Status = "wrong"
			}
		}
	}
	return recs
}

// statusFrom: no error but no matching record, or NXDOMAIN, is "missing";
// a resolver failure is "unknown".
func statusFrom(err error) string {
	var dnsErr *net.DNSError
	if err == nil || (errors.As(err, &dnsErr) && dnsErr.IsNotFound) {
		return "missing"
	}
	return "unknown"
}

// txtMatches compares what is published with what is expected. DKIM
// compares the key only (providers reformat the rest), SPF and DMARC the
// whole value without case and spacing differences.
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
		// Any SPF that ends in -all and allows the MX counts; admins often
		// add other senders.
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
