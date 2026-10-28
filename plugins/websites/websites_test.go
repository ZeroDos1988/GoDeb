package websites

import (
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
