package domains

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeDomainAndRelativeName(t *testing.T) {
	d, err := NormalizeDomain("Example.COM.")
	if err != nil || d != "example.com" {
		t.Fatalf("NormalizeDomain: %q %v", d, err)
	}
	if _, err := NormalizeDomain("no spaces"); err == nil {
		t.Fatal("expected error")
	}
	for fqdn, want := range map[string]string{
		"example.com":                "@",
		"mail.example.com":           "mail",
		"sel._domainkey.example.com": "sel._domainkey",
	} {
		if got := relativeName(fqdn, "example.com"); got != want {
			t.Fatalf("relativeName(%q) = %q, want %q", fqdn, got, want)
		}
	}
}

func TestHetznerValueQuotesTXT(t *testing.T) {
	long := "v=DKIM1; k=rsa; p=" + strings.Repeat("A", 400)
	got := hetznerValue(Need{Type: "TXT", Value: long})
	if !strings.HasPrefix(got, `"`) || !strings.Contains(got, `" "`) {
		t.Fatalf("TXT not chunked and quoted: %q", got)
	}
	if unquoteTXT(got) != long {
		t.Fatal("chunks do not join to the original value")
	}
	if got := hetznerValue(Need{Type: "MX", Value: "10 mail.example.com"}); got != "10 mail.example.com." {
		t.Fatalf("MX: %q", got)
	}
}

// fakeHetzner mimics the parts of the Cloud API GoDeb uses: PUT on an RRSet
// changes labels only, values change through actions/set_records, and TXT
// values must be quoted.
type fakeHetzner struct {
	t      *testing.T
	rrsets map[string]hzRRSet // "name/type"
	writes []string
}

func (f *fakeHetzner) do(req *http.Request) (*http.Response, error) {
	const zone = "/v1/zones/example.com"
	p := req.URL.Path
	reply := func(code int, body any) (*http.Response, error) {
		raw, _ := json.Marshal(body)
		return &http.Response{StatusCode: code, Body: io.NopCloser(strings.NewReader(string(raw))), Header: make(http.Header)}, nil
	}
	notFound := map[string]any{"error": map[string]string{"code": "not_found", "message": "not found"}}
	var payload struct {
		Name    string            `json:"name"`
		Type    string            `json:"type"`
		Labels  map[string]string `json:"labels"`
		Records []hzRecord        `json:"records"`
	}
	if req.Body != nil {
		raw, _ := io.ReadAll(req.Body)
		_ = json.Unmarshal(raw, &payload)
	}
	checkTXT := func(typ string, recs []hzRecord) {
		for _, r := range recs {
			if typ == "TXT" && !strings.HasPrefix(r.Value, `"`) {
				f.t.Fatalf("TXT value not quoted: %q", r.Value)
			}
		}
	}
	switch {
	case req.Method == http.MethodGet && p == zone:
		return reply(200, map[string]any{"zone": hzZone{ID: 1, Name: "example.com"}})
	case strings.HasPrefix(p, "/v1/zones/") && !strings.HasPrefix(p, zone):
		return reply(404, notFound)
	case req.Method == http.MethodPost && p == zone+"/rrsets":
		checkTXT(payload.Type, payload.Records)
		key := payload.Name + "/" + payload.Type
		if _, ok := f.rrsets[key]; ok {
			return reply(409, map[string]any{"error": map[string]string{"code": "uniqueness_error", "message": "exists"}})
		}
		f.rrsets[key] = hzRRSet{Name: payload.Name, Type: payload.Type, Labels: payload.Labels, Records: payload.Records}
		f.writes = append(f.writes, "create "+key)
		return reply(201, map[string]any{})
	case strings.HasPrefix(p, zone+"/rrsets/"):
		rest := strings.TrimPrefix(p, zone+"/rrsets/")
		action := ""
		if i := strings.Index(rest, "/actions/"); i >= 0 {
			rest, action = rest[:i], rest[i+len("/actions/"):]
		}
		cur, ok := f.rrsets[rest]
		if !ok {
			return reply(404, notFound)
		}
		switch {
		case req.Method == http.MethodGet && action == "":
			return reply(200, map[string]any{"rrset": cur})
		case req.Method == http.MethodPost && action == "set_records":
			checkTXT(cur.Type, payload.Records)
			cur.Records = payload.Records
			f.rrsets[rest] = cur
			f.writes = append(f.writes, "set "+rest)
			return reply(201, map[string]any{})
		}
	}
	f.t.Fatalf("unexpected %s %s", req.Method, req.URL.String())
	return nil, nil
}

func useFake(t *testing.T, f *fakeHetzner) {
	old := httpDo
	httpDo = f.do
	t.Cleanup(func() { httpDo = old })
}

func TestApplyZoneCreatesUpdatesAndSkipsForeign(t *testing.T) {
	f := &fakeHetzner{t: t, rrsets: map[string]hzRRSet{
		// managed but outdated: must be updated via set_records
		"@/MX": {Name: "@", Type: "MX", Labels: map[string]string{godebLabel: godebLabelOK}, Records: []hzRecord{{Value: "10 old.example.com."}}},
		// foreign: must stay untouched
		"_dmarc/TXT": {Name: "_dmarc", Type: "TXT", Labels: map[string]string{}, Records: []hzRecord{{Value: `"v=DMARC1; p=none"`}}},
		// managed and current (chunked differently): no write
		"mail/A": {Name: "mail", Type: "A", Labels: map[string]string{godebLabel: godebLabelOK}, Records: []hzRecord{{Value: "203.0.113.10"}}},
	}}
	useFake(t, f)
	needs := []Need{
		{Zone: "example.com", Name: "example.com", Type: "MX", Value: "10 mail.example.com.", Kind: "mx"},
		{Zone: "example.com", Name: "example.com", Type: "TXT", Value: "v=spf1 mx -all", Kind: "spf"},
		{Zone: "example.com", Name: "_dmarc.example.com", Type: "TXT", Value: "v=DMARC1; p=quarantine", Kind: "dmarc"},
		{Zone: "example.com", Name: "mail.example.com", Type: "A", Value: "203.0.113.10", Kind: "a"},
	}
	recs, err := applyZone(context.Background(), "tok", "example.com", needs, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"mx": "ok", "spf": "ok", "dmarc": "conflict", "a": "ok"}
	for _, r := range recs {
		if r.Status != want[r.Kind] {
			t.Errorf("%s: status %q (%s), want %q", r.Kind, r.Status, r.Found, want[r.Kind])
		}
	}
	if got := strings.Join(f.writes, ","); got != "set @/MX,create @/TXT" {
		t.Fatalf("writes = %s", got)
	}
	if v := f.rrsets["@/MX"].Records[0].Value; v != "10 mail.example.com." {
		t.Fatalf("MX not updated: %q", v)
	}
	if v := f.rrsets["_dmarc/TXT"].Records[0].Value; v != `"v=DMARC1; p=none"` {
		t.Fatalf("foreign DMARC changed: %q", v)
	}

	// A second run changes nothing.
	f.writes = nil
	if _, err := applyZone(context.Background(), "tok", "example.com", needs, 0); err != nil {
		t.Fatal(err)
	}
	if len(f.writes) != 0 {
		t.Fatalf("second run wrote %v", f.writes)
	}
}

func TestApplyZoneUnknownZone(t *testing.T) {
	useFake(t, &fakeHetzner{t: t, rrsets: map[string]hzRRSet{}})
	if _, err := applyZone(context.Background(), "tok", "other.org", nil, 0); err == nil || !strings.Contains(err.Error(), "nicht in diesem Hetzner-Projekt") {
		t.Fatalf("err = %v", err)
	}
}

func TestCollectNeedsFiltersZone(t *testing.T) {
	resetNeedsForTest()
	t.Cleanup(resetNeedsForTest)
	RegisterNeeds(func() ([]Need, error) {
		return []Need{{Zone: "example.com", Kind: "mx"}, {Zone: "other.org", Kind: "mx"}}, nil
	})
	needs, err := collectNeeds("example.com")
	if err != nil || len(needs) != 1 {
		t.Fatalf("needs = %v, %v", needs, err)
	}
}

func TestHandlersAndTokenSealed(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))

	mux := http.NewServeMux()
	Mount(mux)
	do := func(method, path, body string) *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
		return rec
	}
	if rec := do(http.MethodGet, base+"/status", ""); rec.Code != 200 {
		t.Fatalf("status: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/domains", `{"name":"Example.COM","mode":"hetzner"}`); rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body)
	}
	if rec := do(http.MethodPost, base+"/domains", `{"name":"example.com"}`); rec.Code != http.StatusConflict {
		t.Fatalf("duplicate: %d", rec.Code)
	}
	if rec := do(http.MethodPost, base+"/domains", `{"name":"bad name"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid: %d", rec.Code)
	}
	if rec := do(http.MethodPost, base+"/token", `{"token":"bad token!"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("bad token: %d", rec.Code)
	}

	token := "abcdefghijklmnopqrstuvwxyz0123456789"
	if err := upsertSettings(&token, nil); err != nil {
		t.Fatal(err)
	}
	gdb, err := store()
	if err != nil {
		t.Fatal(err)
	}
	var raw string
	if err := gdb.Raw(`SELECT token FROM domains_settings WHERE id = 1`).Scan(&raw).Error; err != nil {
		t.Fatal(err)
	}
	if raw == token || !strings.HasPrefix(raw, "v1:") {
		t.Fatalf("token stored unsealed: %q", raw)
	}
	rec := do(http.MethodGet, base+"/status", "")
	if strings.Contains(rec.Body.String(), token) {
		t.Fatal("status leaks the token")
	}
	useFake(t, &fakeHetzner{t: t, rrsets: map[string]hzRRSet{}})
	if rec := do(http.MethodPost, base+"/domains", `{"name":"other.org","mode":"hetzner"}`); rec.Code != 200 {
		t.Fatalf("add other: %d %s", rec.Code, rec.Body)
	}
	var view struct {
		HetznerError string `json:"hetznerError"`
		ZoneMissing  bool   `json:"zoneMissing"`
	}
	rec = do(http.MethodGet, base+"/domains/other.org", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || !view.ZoneMissing || view.HetznerError == "" {
		t.Fatalf("zone missing view: %d %s", rec.Code, rec.Body)
	}
	rec = do(http.MethodGet, base+"/domains/example.com", "")
	view.HetznerError, view.ZoneMissing = "", false
	if err := json.Unmarshal(rec.Body.Bytes(), &view); err != nil || view.ZoneMissing || view.HetznerError != "" {
		t.Fatalf("known zone view: %d %s", rec.Code, rec.Body)
	}

	if rec := do(http.MethodPost, base+"/domains/example.com/delete", "{}"); rec.Code != 200 {
		t.Fatalf("delete: %d", rec.Code)
	}
}

func TestZoneFor(t *testing.T) {
	list := []Domain{{Name: "example.com"}, {Name: "sub.example.com"}}
	for host, want := range map[string]string{
		"example.com":          "example.com",
		"www.example.com":      "example.com",
		"a.sub.example.com":    "sub.example.com",
		"WWW.Other.org.":       "other.org",
		"other.org":            "other.org",
		"deep.www.unknown.net": "www.unknown.net",
	} {
		if got := zoneFor(host, list); got != want {
			t.Errorf("zoneFor(%q) = %q, want %q", host, got, want)
		}
	}
}
