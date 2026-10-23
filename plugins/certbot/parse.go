package certbot

import (
	"strings"
)

// Certificate is public metadata for one live certificate lineage.
type Certificate struct {
	Name      string `json:"name"`
	Subject   string `json:"subject,omitempty"`
	NotBefore string `json:"notBefore,omitempty"`
	NotAfter  string `json:"notAfter,omitempty"`
	Path      string `json:"path"`
}

func sampleCertificates() []Certificate {
	return []Certificate{{
		Name:      "example.com",
		Subject:   "CN = example.com",
		NotBefore: "Jan  1 00:00:00 2026 GMT",
		NotAfter:  "Apr  1 00:00:00 2026 GMT",
		Path:      "/etc/letsencrypt/live/example.com",
	}}
}

// ParseOpenSSL reads `openssl x509 -noout -subject -startdate -enddate`.
func ParseOpenSSL(out string) (subject, notBefore, notAfter string) {
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		val = strings.TrimSpace(val)
		switch strings.TrimSpace(key) {
		case "subject":
			subject = val
		case "notBefore":
			notBefore = val
		case "notAfter":
			notAfter = val
		}
	}
	return subject, notBefore, notAfter
}
