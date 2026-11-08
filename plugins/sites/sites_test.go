package sites

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"godeb/backend/agent"
	"godeb/plugins/domains"
	"godeb/plugins/wireguard"
)

const testConf = "[Interface]\nPrivateKey = yAnz5TF+lXXJte14tji3zlMNq+hd2rYUIgJBgB3fBmk=\nAddress = 10.8.0.2/32\n\n[Peer]\nPublicKey = xTIBA5rboUvnH4htodjb6e697QjLERt1NAB4mZqp8Dg=\nEndpoint = vpn.example.com:51820\nAllowedIPs = 10.8.0.0/24\n"

func useAgentDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GODEB_AGENT_DB", filepath.Join(dir, "agent.db"))
	t.Setenv("GODEB_AGENT_MASTER_KEY", filepath.Join(dir, "agent.key"))
}

// fakeHub replaces every system access with a hub that has one gateway
// domain and records what Add changes.
type fakeHub struct {
	clients   []wireguard.Client
	relay     string
	published []domains.Need
	request   wireguard.PeerRequest
}

func newFakeHub(t *testing.T) *fakeHub {
	t.Helper()
	useAgentDB(t)
	h := &fakeHub{}
	oldCreate, oldLoad, oldRole, oldDomains, oldRelay, oldPublish, oldToken, oldObtain :=
		createPeer, loadClients, currentRole, relayDomains, setRelay, publish, tokenSet, obtainCert
	t.Cleanup(func() {
		createPeer, loadClients, currentRole, relayDomains, setRelay, publish, tokenSet, obtainCert =
			oldCreate, oldLoad, oldRole, oldDomains, oldRelay, oldPublish, oldToken, oldObtain
	})
	currentRole = func() (wireguard.RoleInfo, error) { return wireguard.RoleInfo{Role: wireguard.RoleHub}, nil }
	relayDomains = func() ([]string, error) { return []string{"example.com", "example.org"}, nil }
	setRelay = func(_ context.Context, host string) error { h.relay = host; return nil }
	publish = func(_ context.Context, n domains.Need) error { h.published = append(h.published, n); return nil }
	tokenSet = func() bool { return true }
	obtainCert = func(context.Context, string, string) (domains.Certificate, error) {
		return domains.Certificate{}, errors.New("no ACME in tests")
	}
	loadClients = func() ([]wireguard.Client, error) { return h.clients, nil }
	createPeer = func(_ context.Context, req wireguard.PeerRequest) (wireguard.PeerResult, error) {
		h.request = req
		p, err := req.Profile.Normalize()
		if err != nil {
			return wireguard.PeerResult{}, err
		}
		h.clients = append(h.clients, wireguard.Client{Name: req.Name, Site: true, Profile: p, Addresses: []string{"10.8.0.2/32"}})
		var res wireguard.PeerResult
		res.Peer.Name = req.Name
		res.Peer.AllowedIPs = []string{"10.8.0.2/32"}
		res.ClientConfig = testConf
		return res, nil
	}
	return h
}

func TestAddHomeServer(t *testing.T) {
	h := newFakeHub(t)
	res, err := Add(t.Context(), AddRequest{Name: "heim", Profile: "home", Endpoint: "vpn.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if h.request.Endpoint != "vpn.example.com:51820" || !h.request.Site {
		t.Fatalf("peer request %+v", h.request)
	}
	p := res.Site.Profile
	if p.Kind != "home" || p.MailHost != "postfach.example.com" || len(p.MailDomains) != 2 {
		t.Fatalf("profile %+v", p)
	}
	if h.relay != "10.8.0.2" {
		t.Fatalf("gateway relay %q", h.relay)
	}
	if len(h.published) != 1 || h.published[0].Name != "postfach.example.com" || h.published[0].Value != "10.8.0.2" || h.published[0].Zone != "example.com" {
		t.Fatalf("published %+v", h.published)
	}
	if res.ScriptName != "godeb-home-heim.sh" || !strings.Contains(res.Script, "apt-get install -y --no-install-recommends godeb\n") {
		t.Fatalf("script %s", res.ScriptName)
	}
	waitIssue(t, "postfach.example.com")
}

func TestAddPlayAndRejections(t *testing.T) {
	h := newFakeHub(t)
	res, err := Add(t.Context(), AddRequest{Name: "spiele", Profile: "play", Endpoint: "vpn.example.com:51820"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Site.Profile.MailHost != "" || h.relay != "" || len(h.published) != 0 {
		t.Fatalf("play site touched mail: %+v %q %v", res.Site.Profile, h.relay, h.published)
	}
	if !strings.Contains(res.Script, "godeb-play setup --join") {
		t.Fatal("play script does not run godeb-play")
	}
	if _, err := Add(t.Context(), AddRequest{Name: "x", Profile: "root", Endpoint: "vpn.example.com"}); err == nil {
		t.Fatal("unknown profile accepted")
	}
	currentRole = func() (wireguard.RoleInfo, error) { return wireguard.RoleInfo{Role: wireguard.RolePeer}, nil }
	if _, err := Add(t.Context(), AddRequest{Name: "x", Profile: "play", Endpoint: "vpn.example.com"}); err == nil {
		t.Fatal("site created on a peer")
	}
}

func TestAddWithoutTokenNeedsManualRecord(t *testing.T) {
	h := newFakeHub(t)
	tokenSet = func() bool { return false }
	res, err := Add(t.Context(), AddRequest{Name: "heim", Profile: "home", Endpoint: "vpn.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if len(h.published) != 0 || !strings.Contains(strings.Join(res.Notes, " "), "von Hand") {
		t.Fatalf("notes %v published %v", res.Notes, h.published)
	}
}

func waitIssue(t *testing.T, host string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for issuing(host) {
		if time.Now().After(deadline) {
			t.Fatal("issue did not finish")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestScriptIsValidShell(t *testing.T) {
	for _, kind := range []string{"home", "play"} {
		script, err := Script(kind, "heim", "vpn.example.com:51820", wireguard.JoinCode(testConf), time.Unix(0, 0))
		if err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(t.TempDir(), "s.sh")
		if err := os.WriteFile(path, []byte(script), 0o600); err != nil {
			t.Fatal(err)
		}
		if out, err := exec.Command("sh", "-n", path).CombinedOutput(); err != nil {
			t.Fatalf("%s: sh -n: %v\n%s", kind, err, out)
		}
		for _, want := range []string{"set -eu", "VERSION_CODENAME:-}\" = trixie", "BEGIN PGP PUBLIC KEY BLOCK", "Signed-By: /usr/share/keyrings/", `rm -f "$self"`} {
			if !strings.Contains(script, want) {
				t.Fatalf("%s: %q missing", kind, want)
			}
		}
		if strings.Contains(script, "PRIVATE KEY BLOCK") {
			t.Fatal("secret OpenPGP key in the script")
		}
	}
	if _, err := Script("home", "heim", "vpn.example.com:51820", "godeb-join:abc", time.Now()); err == nil {
		t.Fatal("broken join code accepted")
	}
}

// The script must carry exactly the key and sources the package ships, or
// dpkg asks about the conffile on the first update.
func TestEmbeddedKeysMatchPackage(t *testing.T) {
	for file, embedded := range map[string]string{
		"../../package/apt/godeb-archive-keyring.asc":               godebKey,
		"../../package/debian/etc/apt/sources.list.d/godeb.sources": godebSources,
	} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != embedded {
			t.Errorf("%s differs from the copy in plugins/sites/keys", file)
		}
	}
	if !strings.Contains(playSources, "https://zerodos1988.github.io/godeb-play") || !strings.Contains(playKey, "AC9E 656F 2DCE 6FFE 8E80  1C89 BDBE BCEE 6171 B3FB") {
		t.Error("GoDeb Play key or sources unexpected")
	}
}

func certRequest(host, ip string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, certPath+"?host="+url.QueryEscape(host), nil)
	if ip != "" {
		r.Header.Set(agent.ClientIPHeader, ip)
	}
	return r
}

func TestCertOnlyForItsSite(t *testing.T) {
	h := newFakeHub(t)
	p, _ := wireguard.SiteProfile{Kind: "home", MailHost: "postfach.example.com", MailDomains: []string{"example.com"}}.Normalize()
	h.clients = []wireguard.Client{
		{Name: "heim", Site: true, Profile: p, Addresses: []string{"10.8.0.2/32"}},
		{Name: "spiele", Site: true, Profile: wireguard.SiteProfile{Kind: "play"}, Addresses: []string{"10.8.0.3/32"}},
		{Name: "handy", Addresses: []string{"10.8.0.4/32"}},
	}
	rec := httptest.NewRecorder()
	handleCert(rec, certRequest("postfach.example.com", "10.8.0.2"))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("before issue: %d", rec.Code)
	}
	if err := saveCert(certRow{Host: "postfach.example.com", Site: "heim", CertPEM: "CERT", CertKey: "KEY", NotAfter: "2027-01-01T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct{ host, ip string }{
		{"postfach.example.com", "10.8.0.3"}, // another site
		{"postfach.example.com", "10.8.0.4"}, // a phone
		{"postfach.example.com", "127.0.0.1"},
		{"postfach.example.com", ""},
		{"other.example.com", "10.8.0.2"},
		{"", "10.8.0.2"},
	} {
		rec := httptest.NewRecorder()
		handleCert(rec, certRequest(c.host, c.ip))
		if rec.Code != http.StatusNotFound || strings.Contains(rec.Body.String(), "KEY") {
			t.Errorf("%s from %q: %d %s", c.host, c.ip, rec.Code, rec.Body.String())
		}
	}
	rec = httptest.NewRecorder()
	handleCert(rec, certRequest("Postfach.Example.com", "10.8.0.2"))
	var got certResponse
	if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &got) != nil || got.KeyPEM != "KEY" || got.CertPEM != "CERT" {
		t.Fatalf("own site: %d %s", rec.Code, rec.Body.String())
	}
	// On a peer the route does not exist.
	currentRole = func() (wireguard.RoleInfo, error) { return wireguard.RoleInfo{Role: wireguard.RolePeer}, nil }
	rec = httptest.NewRecorder()
	handleCert(rec, certRequest("postfach.example.com", "10.8.0.2"))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("peer answered %d", rec.Code)
	}
}

func TestFetchCertificate(t *testing.T) {
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.URL.Query().Get("host")
		if r.URL.Path != certPath {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(certResponse{CertPEM: "CERT", KeyPEM: "KEY"})
	}))
	defer srv.Close()
	u, _ := url.Parse(srv.URL)
	ap := netip.MustParseAddrPort(u.Host)
	oldHub, oldInstall, oldBehind, oldPort := hubAddr, installCert, mailBehindHub, hubWebPortFor
	t.Cleanup(func() { hubAddr, installCert, mailBehindHub, hubWebPortFor = oldHub, oldInstall, oldBehind, oldPort })
	hubAddr = func() (netip.Addr, bool) { return ap.Addr(), true }
	hubWebPortFor = func() int { return int(ap.Port()) }
	mailBehindHub = func() string { return "postfach.example.com" }
	var cert, key string
	installCert = func(_ context.Context, c, k []byte) error { cert, key = string(c), string(k); return nil }
	if err := FetchCertificate(t.Context()); err != nil {
		t.Fatal(err)
	}
	if gotHost != "postfach.example.com" || cert != "CERT" || key != "KEY" {
		t.Fatalf("host %q cert %q key %q", gotHost, cert, key)
	}
	mailBehindHub = func() string { return "" }
	if err := FetchCertificate(t.Context()); err == nil {
		t.Fatal("fetched without a mail server behind the hub")
	}
}
