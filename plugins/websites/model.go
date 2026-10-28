package websites

import (
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Mode of a site.
const (
	ModeStatic = "static" // files under DocumentRoot, optional PHP
	ModeProxy  = "proxy"  // reverse proxy to an internal URL
)

// Site is one vhost managed by GoDeb.
type Site struct {
	Name       string `json:"name"`       // directory / unit id
	Domain     string `json:"domain"`     // server_name
	Mode       string `json:"mode"`       // static | proxy
	PHP        bool   `json:"php"`        // enable PHP-FPM for static
	ProxyURL   string `json:"proxyUrl"`   // http://127.0.0.1:8080
	TLS        bool   `json:"tls"`        // use Let's Encrypt cert if present
	Public     bool   `json:"public"`     // open 80/443 in ufw
	Enabled    bool   `json:"enabled"`
	Created    string `json:"created"`
	Root       string `json:"root"`       // resolved document root
	HomeLink   string `json:"homeLink"`   // symlink in the web user's home
}

var (
	namePattern   = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
	domainPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+$`)
)

const (
	wwwRoot     = "/var/www/godeb"
	nginxAvail  = "/etc/nginx/sites-available"
	nginxEnabled = "/etc/nginx/sites-enabled"
	homeWebRel  = "www" // ~/www/<name> -> /var/www/godeb/<name>
)

func cleanSite(s Site) (Site, error) {
	s.Name = strings.ToLower(strings.TrimSpace(s.Name))
	if !namePattern.MatchString(s.Name) {
		return s, errors.New("Name: Kleinbuchstaben, Ziffern und Bindestrich (max. 32)")
	}
	s.Domain = strings.ToLower(strings.TrimSpace(s.Domain))
	if !domainPattern.MatchString(s.Domain) {
		return s, errors.New("Domain ist ungültig")
	}
	switch s.Mode {
	case ModeStatic, ModeProxy:
	case "":
		s.Mode = ModeStatic
	default:
		return s, errors.New("Modus muss static oder proxy sein")
	}
	if s.Mode == ModeProxy {
		s.PHP = false
		u, err := url.Parse(strings.TrimSpace(s.ProxyURL))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return s, errors.New("Proxy-URL muss http(s)://… sein")
		}
		host := u.Hostname()
		ip := net.ParseIP(host)
		if ip == nil {
			// allow localhost hostname
			if !strings.EqualFold(host, "localhost") {
				return s, errors.New("Proxy-Ziel nur IP oder localhost")
			}
		} else if !ip.IsLoopback() && !ip.IsPrivate() {
			return s, errors.New("Proxy-Ziel muss Loopback oder privates Netz sein")
		}
		if u.RawQuery != "" || u.Fragment != "" {
			return s, errors.New("Proxy-URL ohne Query/Fragment")
		}
		s.ProxyURL = u.Scheme + "://" + u.Host + strings.TrimSuffix(u.Path, "/")
		if s.ProxyURL == u.Scheme+"://"+u.Host {
			s.ProxyURL += "/"
		}
	} else {
		s.ProxyURL = ""
	}
	s.Root = filepath.Join(wwwRoot, s.Name)
	s.HomeLink = filepath.Join(homeWebRel, s.Name)
	return s, nil
}

func confName(name string) string { return "godeb-" + name + ".conf" }
func confPath(name string) string { return filepath.Join(nginxAvail, confName(name)) }
func enabledPath(name string) string { return filepath.Join(nginxEnabled, confName(name)) }

func certPaths(domain string) (cert, key string) {
	live := filepath.Join("/etc/letsencrypt/live", domain)
	return filepath.Join(live, "fullchain.pem"), filepath.Join(live, "privkey.pem")
}
