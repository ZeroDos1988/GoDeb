package websites

import (
	"regexp"
	"strings"
	"testing"
)

// Every line GoDeb writes into a vhost, with the user values reduced to
// what cleanSite allows. Anything else in the output means input escaped
// into the nginx configuration.
var nginxLine = regexp.MustCompile(`^(?:` + strings.Join([]string{
	`#.*`,
	`server \{`, `\}`,
	`listen (?:\[::\]:)?(?:80|443 ssl http2);`,
	`server_name [a-z0-9.-]+;`,
	`return 301 https://\$host\$request_uri;`,
	`ssl_certificate(?:_key)? /etc/letsencrypt/live/[a-z0-9.-]+/(?:fullchain|privkey)\.pem;`,
	`location / \{`,
	`proxy_pass https?://[0-9a-zA-Z.:\[\]]+(?:/[A-Za-z0-9._~-]*)*;`,
	`proxy_http_version 1\.1;`,
	`proxy_set_header (?:Host \$host|X-Real-IP \$remote_addr|X-Forwarded-For \$proxy_add_x_forwarded_for|X-Forwarded-Proto \$scheme);`,
	`root /var/www/godeb/[a-z0-9-]+;`,
	`index index\.html index\.php;`,
	`try_files \$uri \$uri/ =404;`,
	`location ~ /\\\.\(\?!well-known/\) \{`,
	`deny all;`,
	`location ~ \\\.php\$ \{`,
	`include snippets/fastcgi-php\.conf;`,
	`fastcgi_pass unix:/run/php/php[0-9.]+-fpm\.sock;`,
}, "|") + `)$`)

func FuzzSiteConfig(f *testing.F) {
	f.Add("blog", "blog.example.com", "static", true, "", false)
	f.Add("api", "api.example.com", "proxy", false, "http://127.0.0.1:8080/app", true)
	f.Add("x", "x.example.com", "proxy", false, "http://127.0.0.1:8080/a;}server{listen 81;", false)
	f.Add("x", "x.example.com\nlisten 81", "static", false, "", false)
	f.Add("x", "x.example.com", "proxy", false, "http://[::1]:9000/$host", true)
	f.Fuzz(func(t *testing.T, name, domain, mode string, php bool, proxy string, tls bool) {
		s, err := cleanSite(Site{Name: name, Domain: domain, Mode: mode, PHP: php, ProxyURL: proxy, TLS: tls})
		if err != nil {
			return
		}
		conf := renderNginx(s)
		depth := 0
		for _, line := range strings.Split(strings.TrimSpace(conf), "\n") {
			line = strings.TrimSpace(line)
			if !nginxLine.MatchString(line) {
				t.Fatalf("unexpected nginx line %q from %+v", line, s)
			}
			depth += strings.Count(line, "{") - strings.Count(line, "}")
			if depth < 0 || depth > 2 {
				t.Fatalf("braces out of balance at %q", line)
			}
		}
		if depth != 0 {
			t.Fatalf("braces not closed:\n%s", conf)
		}
	})
}
