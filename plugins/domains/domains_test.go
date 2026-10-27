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
	if got := relativeName("example.com", "example.com"); got != "@" {
		t.Fatalf("apex: %q", got)
	}
	if got := relativeName("mail.example.com", "example.com"); got != "mail" {
		t.Fatalf("host: %q", got)
	}
	if got := relativeName("sel._domainkey.example.com", "example.com"); got != "sel._domainkey" {
		t.Fatalf("dkim: %q", got)
	}
}

func TestCollectNeedsAndApply(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))
	resetNeedsForTest()
	t.Cleanup(resetNeedsForTest)

	RegisterNeeds(func() ([]Need, error) {
		return []Need{
			{Zone: "example.com", Name: "example.com", Type: "MX", Value: "10 mail.example.com.", Kind: "mx", Source: "mail"},
			{Zone: "example.com", Name: "example.com", Type: "TXT", Value: "v=spf1 mx -all", Kind: "spf", Source: "mail"},
			{Zone: "example.com", Name: "mail.example.com", Type: "A", Value: "203.0.113.10", Kind: "a", Source: "mail"},
		}, nil
	})

	if err := addDomain("example.com", ModeHetzner); err != nil {
		t.Fatal(err)
	}
	token := "test-token-abcdefghijklmnopqrstuvwxyz"
	if err := upsertSettings(&token, nil); err != nil {
		t.Fatal(err)
	}

	var calls []string
	httpDo = func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.Method+" "+req.URL.Path)
		body := "{}"
		path := req.URL.Path
		switch {
		case req.Method == http.MethodGet && path == "/v1/zones":
			body = `{"zones":[{"id":1,"name":"example.com"}]}`
		case req.Method == http.MethodGet && strings.HasSuffix(path, "/rrsets"):
			body = `{"rrsets":[]}`
		case req.Method == http.MethodPost && strings.HasSuffix(path, "/rrsets"):
			raw, _ := io.ReadAll(req.Body)
			var payload map[string]any
			_ = json.Unmarshal(raw, &payload)
			labels, _ := payload["labels"].(map[string]any)
			if labels[godebLabel] != godebLabelOK {
				t.Fatalf("missing godeb label: %v", payload)
			}
			body = `{"rrset":{"name":"@","type":"MX"},"action":{"id":1,"status":"success"}}`
		default:
			t.Fatalf("unexpected %s %s", req.Method, req.URL.String())
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	}
	t.Cleanup(func() {
		httpDo = func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		}
	})

	needs, err := collectNeeds("example.com")
	if err != nil || len(needs) != 3 {
		t.Fatalf("needs: %d %v", len(needs), err)
	}
	recs, err := applyZone(context.Background(), token, "example.com", needs, 0, "203.0.113.10")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 {
		t.Fatalf("recs: %d", len(recs))
	}
	for _, r := range recs {
		if r.Status != "ok" || !r.Managed {
			t.Fatalf("record %+v", r)
		}
	}
	if len(calls) < 4 {
		t.Fatalf("calls: %v", calls)
	}
}

func TestConflictLeavesForeignRRSet(t *testing.T) {
	httpDo = func(req *http.Request) (*http.Response, error) {
		body := "{}"
		path := req.URL.Path
		switch {
		case path == "/v1/zones":
			body = `{"zones":[{"id":1,"name":"example.com"}]}`
		case strings.HasSuffix(path, "/rrsets") && req.Method == http.MethodGet:
			body = `{"rrsets":[{"id":"@/MX","name":"@","type":"MX","labels":{},"records":[{"value":"10 other.example.com."}]}]}`
		case req.Method == http.MethodPut || req.Method == http.MethodPost:
			t.Fatal("must not change unmanaged RRSet")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
	}
	t.Cleanup(func() {
		httpDo = func(req *http.Request) (*http.Response, error) {
			return http.DefaultClient.Do(req)
		}
	})
	needs := []Need{{Zone: "example.com", Name: "example.com", Type: "MX", Value: "10 mail.example.com.", Kind: "mx", Source: "mail"}}
	recs, err := applyZone(context.Background(), "tok", "example.com", needs, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 1 || recs[0].Status != "conflict" {
		t.Fatalf("%+v", recs)
	}
}

func TestHandlers(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_DB", filepath.Join(dir, "godeb.db"))
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_MASTER_KEY", filepath.Join(dir, "master.key"))

	mux := http.NewServeMux()
	Mount(mux)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"/status", nil))
	if rec.Code != 200 {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, base+"/domains", strings.NewReader(`{"name":"Example.COM","mode":"manual"}`)))
	if rec.Code != 200 {
		t.Fatalf("add: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, base+"/domains/example.com", nil))
	if rec.Code != 200 {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}
}
