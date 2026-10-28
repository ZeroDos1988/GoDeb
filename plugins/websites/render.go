package websites

import (
	"fmt"
	"os"
	"strings"
)

const header = "# Written by GoDeb (plugin websites). Changes are overwritten.\n"

func resolvePHPSock() string {
	for _, v := range []string{"8.4", "8.3", "8.2", "8.1"} {
		p := fmt.Sprintf("/run/php/php%s-fpm.sock", v)
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "/run/php/php8.4-fpm.sock"
}

func renderNginx(s Site) string {
	var b strings.Builder
	b.WriteString(header)
	cert, key := certPaths(s.Domain)
	useTLS := s.TLS
	if useTLS {
		if _, err := os.Stat(cert); err != nil {
			useTLS = false
		}
	}
	if useTLS {
		fmt.Fprintf(&b, `server {
  listen 80;
  listen [::]:80;
  server_name %s;
  return 301 https://$host$request_uri;
}
server {
  listen 443 ssl http2;
  listen [::]:443 ssl http2;
  server_name %s;
  ssl_certificate %s;
  ssl_certificate_key %s;
`, s.Domain, s.Domain, cert, key)
	} else {
		fmt.Fprintf(&b, `server {
  listen 80;
  listen [::]:80;
  server_name %s;
`, s.Domain)
	}
	if s.Mode == ModeProxy {
		fmt.Fprintf(&b, `  location / {
    proxy_pass %s;
    proxy_http_version 1.1;
    proxy_set_header Host $host;
    proxy_set_header X-Real-IP $remote_addr;
    proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
  }
}
`, s.ProxyURL)
		return b.String()
	}
	fmt.Fprintf(&b, `  root %s;
  index index.html index.php;
  location / {
    try_files $uri $uri/ =404;
  }
  # Never serve .git, .env and other dot files; /.well-known stays (ACME).
  location ~ /\.(?!well-known/) {
    deny all;
  }
`, s.Root)
	if s.PHP {
		sock := resolvePHPSock()
		fmt.Fprintf(&b, `  location ~ \.php$ {
    include snippets/fastcgi-php.conf;
    fastcgi_pass unix:%s;
  }
`, sock)
	}
	b.WriteString("}\n")
	return b.String()
}
