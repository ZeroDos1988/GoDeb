package websites

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCleanSite(t *testing.T) {
	s, err := cleanSite(Site{Name: "Blog", Domain: "Blog.Example.COM", Mode: ModeStatic, PHP: true})
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "blog" || s.Domain != "blog.example.com" || !strings.HasSuffix(s.Root, "/blog") {
		t.Fatalf("%+v", s)
	}
	if _, err := cleanSite(Site{Name: "x", Domain: "bad", Mode: ModeStatic}); err == nil {
		t.Fatal("domain")
	}
	p, err := cleanSite(Site{
		Name: "app", Domain: "app.example.com", Mode: ModeProxy,
		ProxyURL: "http://127.0.0.1:8080/api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.PHP || !strings.Contains(p.ProxyURL, "127.0.0.1:8080") {
		t.Fatalf("%+v", p)
	}
	if _, err := cleanSite(Site{
		Name: "x", Domain: "x.example.com", Mode: ModeProxy, ProxyURL: "http://8.8.8.8/",
	}); err == nil {
		t.Fatal("public proxy rejected")
	}
}

func TestRenderNginx(t *testing.T) {
	s, _ := cleanSite(Site{Name: "site", Domain: "site.example.com", Mode: ModeStatic, PHP: true, Enabled: true})
	conf := renderNginx(s)
	for _, want := range []string{"server_name site.example.com", "root /var/www/godeb/site", "fastcgi_pass unix:"} {
		if !strings.Contains(conf, want) {
			t.Errorf("lacks %q:\n%s", want, conf)
		}
	}
	proxy, _ := cleanSite(Site{Name: "p", Domain: "p.example.com", Mode: ModeProxy, ProxyURL: "http://10.8.0.2:9090/"})
	pc := renderNginx(proxy)
	if !strings.Contains(pc, "proxy_pass http://10.8.0.2:9090/") {
		t.Fatalf("%s", pc)
	}
}

func TestCreateDocRootNoFollow(t *testing.T) {
	base := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim")
	if err := os.WriteFile(outside, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Fresh directory: placeholder written, nothing else.
	fresh := filepath.Join(base, "fresh")
	if err := createDocRoot(fresh, "fresh.example.com"); err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(filepath.Join(fresh, "index.html")); err != nil || !strings.Contains(string(b), "fresh.example.com") {
		t.Fatalf("index: %q %v", b, err)
	}

	// Existing directory with a planted symlink: left untouched.
	planted := filepath.Join(base, "planted")
	if err := os.Mkdir(planted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(planted, "index.html")); err != nil {
		t.Fatal(err)
	}
	if err := createDocRoot(planted, "planted.example.com"); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(outside); string(b) != "keep" {
		t.Fatalf("wrote through symlink: %q", b)
	}

	// The docroot itself being a symlink is refused.
	link := filepath.Join(base, "link")
	if err := os.Symlink(t.TempDir(), link); err != nil {
		t.Fatal(err)
	}
	if err := createDocRoot(link, "link.example.com"); err == nil {
		t.Fatal("symlinked docroot accepted")
	}
}

func TestCertEmail(t *testing.T) {
	for _, ok := range []string{"admin@example.com", "A.B+c@Mail.Example.org"} {
		if !emailPattern.MatchString(ok) {
			t.Errorf("rejected %q", ok)
		}
	}
	for _, bad := range []string{"--agree-tos", "a@b", "a b@example.com", "a@example.com\n"} {
		if emailPattern.MatchString(bad) {
			t.Errorf("accepted %q", bad)
		}
	}
}
