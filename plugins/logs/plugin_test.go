package logs

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"testing"
)

func TestParseQueryRejectsUnknownValues(t *testing.T) {
	for _, raw := range []string{"source=sshd", "level=crit", "since=-1y", "lines=abc", "lines=0", "source=all%3Brm"} {
		v, _ := url.ParseQuery(raw)
		if _, ok := ParseQuery(v.Get); ok {
			t.Fatalf("%s accepted", raw)
		}
	}
}

func TestQueryArgs(t *testing.T) {
	v, _ := url.ParseQuery("source=agent&level=warning&since=1h&lines=5000")
	q, ok := ParseQuery(v.Get)
	if !ok {
		t.Fatal("rejected")
	}
	want := []string{"--no-pager", "--quiet", "--output=json", "--priority=warning", "--unit=godeb-agent.service", "--since=-1h", "--lines=1000"}
	if !slices.Equal(q.Args(), want) {
		t.Fatalf("args = %q", q.Args())
	}
	v, _ = url.ParseQuery("since=all")
	q, _ = ParseQuery(v.Get)
	if slices.ContainsFunc(q.Args(), func(a string) bool { return len(a) > 8 && a[:8] == "--since=" }) {
		t.Fatalf("since=all must not limit: %q", q.Args())
	}
}

func TestParseJournal(t *testing.T) {
	entries := ParseJournal(sampleJournal + "kein json\n")
	if len(entries) != 7 {
		t.Fatalf("entries = %d", len(entries))
	}
	e := entries[4]
	if e.Message != "Aktion" || e.Unit != "godeb-agent.service" || e.Priority != 6 || e.Time != 1761170180000 {
		t.Fatalf("entry = %+v", e)
	}
	if e.Fields[0] != (Field{"methode", "POST"}) || e.Fields[len(e.Fields)-1] != (Field{"2fa", "bestätigt"}) {
		t.Fatalf("fields = %+v", e.Fields)
	}
	f := entries[5].Fields
	if f[0] != (Field{"befehl", "[systemctl restart enshrouded.service]"}) || f[len(f)-1] != (Field{"fehler", "Job for enshrouded.service failed."}) {
		t.Fatalf("quoted fields = %+v", f)
	}
}

func TestPlainAndBinaryMessages(t *testing.T) {
	entries := ParseJournal(`{"MESSAGE":"panic: a=b kaputt"}` + "\n" + `{"MESSAGE":[104,105,255]}` + "\n")
	if entries[0].Message != "panic: a=b kaputt" || entries[0].Fields == nil || len(entries[0].Fields) != 0 {
		t.Fatalf("plain = %+v", entries[0])
	}
	if entries[1].Message != "hi�" {
		t.Fatalf("binary = %q", entries[1].Message)
	}
}

func TestFilterNewestFirst(t *testing.T) {
	entries := ParseJournal(sampleJournal)
	got := Filter(entries, "", 2)
	if len(got) != 2 || got[0].Priority != 3 {
		t.Fatalf("got = %+v", got)
	}
	got = Filter(entries, "IP=10.8.0.5", 10)
	if len(got) != 3 {
		t.Fatalf("text filter = %d", len(got))
	}
}

func TestEntriesRejectsBadFilter(t *testing.T) {
	mux := http.NewServeMux()
	Mount(mux)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/plugins/logs/entries?source=ssh", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d", rec.Code)
	}
}

// Lines systemd writes for the unit ("Started …") are plain text. They must
// come with an empty field list: null made the log view crash to a blank
// page on real servers.
func TestPlainLinesHaveEmptyFields(t *testing.T) {
	out := `{"__REALTIME_TIMESTAMP":"1761170000000000","PRIORITY":"6","UNIT":"godeb.service","MESSAGE":"Started godeb.service - GoDeb web interface."}
`
	entries := ParseJournal(out)
	if len(entries) != 1 || entries[0].Fields == nil || entries[0].Message != "Started godeb.service - GoDeb web interface." {
		t.Fatalf("entries = %#v", entries)
	}
	data, err := json.Marshal(Filter(entries, "", 10))
	if err != nil || strings.Contains(string(data), "null") {
		t.Fatalf("json = %s %v", data, err)
	}
	if data, _ := json.Marshal(Filter(ParseJournal(""), "", 10)); string(data) != "[]" {
		t.Fatalf("empty journal = %s", data)
	}
}
