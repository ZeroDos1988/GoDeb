package domains

import (
	"context"
	"strings"
)

// applyZone creates or updates the Hetzner RRSets for the zone's needs.
// RRSets without the godeb label are left alone and reported as conflict.
// PTR records go through change_dns_ptr of the server with that IP.
func applyZone(ctx context.Context, token string, zoneName string, needs []Need, serverID int64) ([]Record, error) {
	zone, err := findZone(ctx, token, zoneName)
	if err != nil {
		return nil, err
	}
	out := make([]Record, 0, len(needs))
	for _, n := range needs {
		rec := Record{Need: n, ZoneLine: zoneLine(n), Status: "unknown"}
		if n.Kind == "ptr" {
			if err := applyPTR(ctx, token, &serverID, n); err != nil {
				rec.Status = "wrong"
				rec.Found = err.Error()
			} else {
				rec.Status, rec.Found, rec.Managed = "ok", n.Value, true
			}
			out = append(out, rec)
			continue
		}
		rel := relativeName(n.Name, zoneName)
		value := hetznerValue(n)
		cur, exists, err := getRRSet(ctx, token, zone, rel, n.Type)
		if err != nil {
			rec.Found = err.Error()
			out = append(out, rec)
			continue
		}
		if exists && !isManaged(cur) {
			rec.Status, rec.Found = "conflict", valuesOf(cur)
			out = append(out, rec)
			continue
		}
		switch {
		case exists && valuesMatch(cur, value):
			err = nil
		case exists:
			err = setRecords(ctx, token, zone, rel, n.Type, value)
		default:
			err = createRRSet(ctx, token, zone, rel, n.Type, value)
		}
		if err != nil {
			rec.Status, rec.Found = "wrong", err.Error()
		} else {
			rec.Status, rec.Found, rec.Managed = "ok", value, true
		}
		out = append(out, rec)
	}
	return out, nil
}

func applyPTR(ctx context.Context, token string, serverID *int64, n Need) error {
	if *serverID == 0 {
		id, err := findServerID(ctx, token, n.Name)
		if err != nil {
			return err
		}
		*serverID = id
		_ = upsertSettings(nil, &id)
	}
	return changePTR(ctx, token, *serverID, n.Name, n.Value)
}

func zoneLine(n Need) string {
	v := n.Value
	if n.Type == "TXT" {
		v = splitTXT(v)
	}
	return n.Name + ". IN " + n.Type + " " + v
}

// hetznerValue formats a value for the Cloud API: TXT as quoted strings of
// at most 255 characters, host names in MX/CNAME/PTR with a final dot.
func hetznerValue(n Need) string {
	switch n.Type {
	case "TXT":
		return splitTXT(n.Value)
	case "MX":
		fields := strings.Fields(n.Value)
		if len(fields) == 2 && !strings.HasSuffix(fields[1], ".") {
			return fields[0] + " " + fields[1] + "."
		}
		return n.Value
	case "CNAME", "PTR":
		if !strings.HasSuffix(n.Value, ".") {
			return n.Value + "."
		}
		return n.Value
	default:
		return n.Value
	}
}

func valuesOf(rr hzRRSet) string {
	parts := make([]string, 0, len(rr.Records))
	for _, r := range rr.Records {
		parts = append(parts, r.Value)
	}
	return strings.Join(parts, ", ")
}

// valuesMatch: the RRSet holds exactly the wanted value. TXT compares the
// joined content of the quoted strings, so chunking differences don't count.
func valuesMatch(rr hzRRSet, want string) bool {
	if len(rr.Records) != 1 {
		return false
	}
	norm := func(s string) string {
		if strings.HasPrefix(strings.TrimSpace(s), `"`) {
			s = unquoteTXT(s)
		}
		return strings.ToLower(strings.Join(strings.Fields(s), " "))
	}
	return norm(rr.Records[0].Value) == norm(want)
}

// unquoteTXT joins `"a" "b"` to `ab`.
func unquoteTXT(s string) string {
	var b strings.Builder
	in := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '\\' && in && i+1 < len(s):
			i++
			b.WriteByte(s[i])
		case c == '"':
			in = !in
		case in:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// markManaged flags records whose Hetzner RRSet carries the godeb label.
func markManaged(ctx context.Context, token string, zoneName string, records []Record) []Record {
	zone, err := findZone(ctx, token, zoneName)
	if err != nil {
		return records
	}
	for i := range records {
		r := &records[i]
		if r.Kind == "ptr" {
			continue
		}
		if cur, ok, err := getRRSet(ctx, token, zone, relativeName(r.Name, zoneName), r.Type); err == nil && ok {
			r.Managed = isManaged(cur)
		}
	}
	return records
}
