package domains

import (
	"context"
	"fmt"
	"strings"
)

// applyZone creates or updates Hetzner RRSets for every non-PTR need of the
// zone. Existing RRSets without the godeb label are left alone and reported
// as conflict. PTR is applied separately via change_dns_ptr.
func applyZone(ctx context.Context, token string, zoneName string, needs []Need, serverID int64, publicIP string) ([]Record, error) {
	zone, err := findZone(ctx, token, zoneName)
	if err != nil {
		return nil, err
	}
	existing, err := listRRSets(ctx, token, zone)
	if err != nil {
		return nil, err
	}
	byKey := map[string]hzRRSet{}
	for _, rr := range existing {
		byKey[rr.Name+"/"+rr.Type] = rr
	}

	out := make([]Record, 0, len(needs))
	for _, n := range needs {
		rec := Record{Need: n, Status: "unknown"}
		v := n.Value
		if n.Type == "TXT" {
			rec.ZoneLine = n.Name + ". IN TXT " + splitTXT(v)
		} else {
			rec.ZoneLine = n.Name + ". IN " + n.Type + " " + v
		}

		if n.Kind == "ptr" {
			if err := applyPTR(ctx, token, &serverID, publicIP, n, &rec); err != nil {
				rec.Status = "wrong"
				rec.Found = err.Error()
			}
			out = append(out, rec)
			continue
		}

		rel := relativeName(n.Name, zoneName)
		apiValue := hetznerValue(n)
		key := rel + "/" + n.Type
		if cur, ok := byKey[key]; ok {
			rec.Managed = isManaged(cur)
			if !rec.Managed {
				rec.Status = "conflict"
				rec.Found = valuesOf(cur)
				out = append(out, rec)
				continue
			}
			if valuesMatch(cur, apiValue) {
				rec.Status = "ok"
				rec.Found = apiValue
				out = append(out, rec)
				continue
			}
			if err := updateRRSet(ctx, token, zone, rel, n.Type, apiValue, cur.Labels); err != nil {
				rec.Status = "wrong"
				rec.Found = err.Error()
			} else {
				rec.Status = "ok"
				rec.Found = apiValue
				rec.Managed = true
			}
			out = append(out, rec)
			continue
		}
		if err := createRRSet(ctx, token, zone, rel, n.Type, apiValue); err != nil {
			rec.Status = "wrong"
			rec.Found = err.Error()
		} else {
			rec.Status = "ok"
			rec.Found = apiValue
			rec.Managed = true
		}
		out = append(out, rec)
	}
	return out, nil
}

func applyPTR(ctx context.Context, token string, serverID *int64, publicIP string, n Need, rec *Record) error {
	ip := n.Name
	if publicIP != "" && ip != publicIP {
		// Still allow applying the PTR the mail plugin asked for.
	}
	id := *serverID
	if id == 0 {
		found, err := findServerID(ctx, token, ip)
		if err != nil {
			return err
		}
		id = found
		*serverID = id
		_ = upsertSettings(nil, &id)
	}
	if err := changePTR(ctx, token, id, ip, n.Value); err != nil {
		return err
	}
	rec.Status = "ok"
	rec.Found = n.Value
	rec.Managed = true
	return nil
}

// hetznerValue formats the record for the Cloud API.
func hetznerValue(n Need) string {
	switch n.Type {
	case "MX":
		// "10 mail.example.com." — Hetzner expects priority and host.
		fields := strings.Fields(n.Value)
		if len(fields) >= 2 {
			host := fields[1]
			if !strings.HasSuffix(host, ".") {
				host += "."
			}
			return fields[0] + " " + host
		}
		return n.Value
	case "TXT":
		return n.Value
	case "A":
		return n.Value
	default:
		v := n.Value
		if n.Type == "CNAME" || n.Type == "PTR" {
			if !strings.HasSuffix(v, ".") {
				v += "."
			}
		}
		return v
	}
}

func valuesOf(rr hzRRSet) string {
	parts := make([]string, 0, len(rr.Records))
	for _, r := range rr.Records {
		parts = append(parts, r.Value)
	}
	return strings.Join(parts, ", ")
}

func valuesMatch(rr hzRRSet, want string) bool {
	wantNorm := strings.ToLower(strings.Join(strings.Fields(want), " "))
	for _, r := range rr.Records {
		got := strings.ToLower(strings.Join(strings.Fields(r.Value), " "))
		if got == wantNorm {
			return true
		}
		// TXT: providers may quote; compare without quotes.
		if strings.Trim(got, `"`) == strings.Trim(wantNorm, `"`) {
			return true
		}
	}
	return false
}

func markManaged(ctx context.Context, token string, zoneName string, records []Record) []Record {
	zone, err := findZone(ctx, token, zoneName)
	if err != nil {
		return records
	}
	existing, err := listRRSets(ctx, token, zone)
	if err != nil {
		return records
	}
	byKey := map[string]hzRRSet{}
	for _, rr := range existing {
		byKey[rr.Name+"/"+rr.Type] = rr
	}
	for i := range records {
		r := &records[i]
		if r.Kind == "ptr" {
			continue
		}
		rel := relativeName(r.Name, zoneName)
		if cur, ok := byKey[rel+"/"+r.Type]; ok {
			r.Managed = isManaged(cur)
		}
	}
	return records
}

func validateToken(ctx context.Context, token string) error {
	_, err := listZones(ctx, token)
	if err != nil {
		return fmt.Errorf("Token ungültig oder ohne DNS-Recht: %w", err)
	}
	return nil
}
