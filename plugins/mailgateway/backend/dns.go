package mailgateway

import (
	"context"
	"net"
	"strings"
)

type txtResolver interface {
	LookupTXT(ctx context.Context, name string) ([]string, error)
	LookupAddr(ctx context.Context, addr string) ([]string, error)
}

type netResolver struct{}

func (netResolver) LookupTXT(ctx context.Context, name string) ([]string, error) {
	return net.DefaultResolver.LookupTXT(ctx, name)
}

func (netResolver) LookupAddr(ctx context.Context, addr string) ([]string, error) {
	return net.DefaultResolver.LookupAddr(ctx, addr)
}

// dnsCheck is one traffic-light row in the anti-blacklisting panel.
type dnsCheck struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Found    string `json:"found"`
	Expected string `json:"expected"`
	Record   string `json:"record"`
}

func evaluateDNS(ctx context.Context, domain, selector, publicIP, dkimP string, look txtResolver) []dnsCheck {
	if selector == "" {
		selector = "godeb"
	}
	spfExpected := "v=spf1 mx a -all"
	if publicIP != "" {
		spfExpected = "v=spf1 mx a ip4:" + publicIP + " -all"
	}
	dkimExpected := ""
	if dkimP != "" {
		dkimExpected = "v=DKIM1; h=sha256; k=rsa; p=" + dkimP
	}
	dmarcExpected := "v=DMARC1; p=quarantine; rua=mailto:dmarc@" + domain

	return []dnsCheck{
		checkPTR(ctx, domain, publicIP, look),
		checkTXT(ctx, look, "spf", domain, "v=spf1", spfExpected, classifySPF(publicIP)),
		checkTXT(ctx, look, "dkim", selector+"._domainkey."+domain, "v=DKIM1", dkimExpected, classifyDKIM(dkimP)),
		checkTXT(ctx, look, "dmarc", "_dmarc."+domain, "v=DMARC1", dmarcExpected, classifyDMARC),
	}
}

func checkPTR(ctx context.Context, domain, publicIP string, look txtResolver) dnsCheck {
	item := dnsCheck{ID: "ptr", Status: "bad", Expected: domain}
	if publicIP == "" {
		item.Found = "keine öffentliche IPv4-Adresse erkannt"
		item.Record = domain + "."
		return item
	}
	names, err := look.LookupAddr(ctx, publicIP)
	if err != nil || len(names) == 0 {
		item.Found = "kein PTR für " + publicIP
		item.Record = publicIP + " -> " + domain + "."
		return item
	}
	item.Found = strings.TrimSuffix(names[0], ".")
	item.Record = publicIP + " -> " + item.Found + "."
	if strings.EqualFold(item.Found, domain) || strings.HasSuffix(strings.ToLower(item.Found), "."+domain) {
		item.Status = "ok"
		return item
	}
	item.Status = "warn"
	return item
}

type txtClass func(found string) string

func checkTXT(ctx context.Context, look txtResolver, id, name, prefix, expected string, class txtClass) dnsCheck {
	item := dnsCheck{ID: id, Status: "bad", Expected: expected, Record: name + " TXT \"" + expected + "\""}
	records, err := look.LookupTXT(ctx, name)
	if err != nil || len(records) == 0 {
		item.Found = "kein TXT"
		return item
	}
	var match string
	for _, record := range records {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(record)), strings.ToLower(prefix)) {
			match = strings.TrimSpace(record)
			break
		}
	}
	if match == "" {
		item.Found = records[0]
		return item
	}
	item.Found = match
	item.Status = class(match)
	return item
}

func classifySPF(publicIP string) txtClass {
	return func(found string) string {
		if strings.Contains(found, "+all") {
			return "warn"
		}
		if publicIP != "" && !strings.Contains(found, publicIP) && !strings.Contains(found, "mx") && !strings.Contains(found, " a") {
			return "warn"
		}
		return "ok"
	}
}

func classifyDKIM(public string) txtClass {
	return func(found string) string {
		if public == "" {
			return "warn"
		}
		got := dkimPValue(found)
		if got == "" {
			return "bad"
		}
		if got == public {
			return "ok"
		}
		return "warn"
	}
}

func classifyDMARC(found string) string {
	lower := strings.ToLower(found)
	if strings.Contains(lower, "p=reject") || strings.Contains(lower, "p=quarantine") {
		return "ok"
	}
	if strings.Contains(lower, "p=none") {
		return "warn"
	}
	return "bad"
}

func dkimPValue(record string) string {
	for _, part := range strings.Split(record, ";") {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(strings.ToLower(part), "p=") {
			return strings.TrimSpace(part[2:])
		}
	}
	return ""
}
