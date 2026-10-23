package fail2ban

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestParseJailList(t *testing.T) {
	got := ParseJailList(sampleStatus)
	if len(got) != 2 || got[0] != "godeb" || got[1] != "sshd" {
		t.Fatalf("jails = %#v", got)
	}
	if got := ParseJailList("Status\n`- Jail list:\t\n"); len(got) != 0 {
		t.Fatalf("empty = %#v", got)
	}
	if got := ParseJailList("`- Jail list:\tok, bad;rm -rf, ../x\n"); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("unsafe names = %#v", got)
	}
}

func TestParseJail(t *testing.T) {
	j := ParseJail("sshd", sampleJails["sshd"])
	if j.TotalFailed != 12 || j.CurrentlyBanned != 2 || j.TotalBanned != 4 {
		t.Fatalf("jail = %#v", j)
	}
	if len(j.BannedIPs) != 2 || j.BannedIPs[0] != "198.51.100.4" || j.BannedIPs[1] != "2001:db8::7" {
		t.Fatalf("ips = %#v", j.BannedIPs)
	}
	g := ParseJail("godeb", sampleJails["godeb"])
	if g.CurrentlyFailed != 1 || len(g.BannedIPs) != 1 {
		t.Fatalf("godeb = %#v", g)
	}
}

func TestUnbanRejectsInvalidIP(t *testing.T) {
	mux := http.NewServeMux()
	Mount(mux)
	for _, body := range []string{`{"ip":"1.2.3.4; reboot"}`, `{"ip":"--help"}`, `{"ip":""}`} {
		req := httptest.NewRequest(http.MethodPost, "/api/plugins/fail2ban/unban", strings.NewReader(body))
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%s status = %d", body, rec.Code)
		}
	}
}
