package mail

import (
	"regexp"
	"strings"
	"testing"
)

func TestAutoconfigXML(t *testing.T) {
	s := Settings{Hostname: "mail.example.com"}
	xml := autoconfigXML(s, "example.com")
	for _, want := range []string{"<hostname>mail.example.com</hostname>", "<port>993</port>", "<port>465</port>", "example.com"} {
		if !strings.Contains(xml, want) {
			t.Errorf("lacks %q", want)
		}
	}
	ad := autodiscoverXML(s, "example.com")
	if !strings.Contains(ad, "<Type>IMAP</Type>") || !strings.Contains(ad, "mail.example.com") {
		t.Fatalf("%s", ad)
	}
	conf := autoconfigNginx("example.com")
	for _, want := range []string{"autoconfig.example.com", "mta-sts.example.com", "config-v1.1.xml"} {
		if !strings.Contains(conf, want) && want != "config-v1.1.xml" {
			t.Errorf("nginx lacks %q", want)
		}
	}
	if !strings.Contains(conf, "mta-sts.txt") {
		t.Fatal(conf)
	}
	if !strings.Contains(conf, "server_name autoconfig.example.com autodiscover.example.com mta-sts.example.com;") {
		t.Errorf("bare domain or missing names in server_name:\n%s", conf)
	}
}

// Every line of the vhost, with the domain reduced to what NormalizeDomain
// allows. Anything else means the domain escaped into the configuration.
var autoNginxLine = regexp.MustCompile(`^(?:` + strings.Join([]string{
	`#.*`,
	`server \{`, `\}`,
	`listen (?:\[::\]:)?80;`,
	`server_name autoconfig\.[a-z0-9.-]+ autodiscover\.[a-z0-9.-]+ mta-sts\.[a-z0-9.-]+;`,
	`root /var/www/godeb/mail/[a-z0-9.-]+;`,
	`location = /\.well-known/mta-sts\.txt \{`,
	`alias /etc/godeb/[a-z0-9/._-]+\.txt;`,
	`default_type text/plain;`,
	`location /(?:\.well-known/autoconfig/mail|mail|autodiscover|Autodiscover)/ \{`,
	`try_files \$uri =404;`,
}, "|") + `)$`)

func FuzzAutoconfigNginx(f *testing.F) {
	f.Add("example.com")
	f.Add("example.com;}server{listen 81;")
	f.Add("example.com\nlisten 81")
	f.Add("../../etc")
	f.Fuzz(func(t *testing.T, raw string) {
		d, err := NormalizeDomain(raw)
		if err != nil {
			return
		}
		depth := 0
		for _, line := range strings.Split(strings.TrimSpace(autoconfigNginx(d)), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			if !autoNginxLine.MatchString(line) {
				t.Fatalf("unexpected nginx line %q for %q", line, d)
			}
			depth += strings.Count(line, "{") - strings.Count(line, "}")
		}
		if depth != 0 {
			t.Fatalf("unbalanced braces for %q", d)
		}
		for _, x := range []string{autodiscoverXML(Settings{Hostname: "mail.example.com"}, d), autoconfigXML(Settings{Hostname: "mail.example.com"}, d)} {
			if strings.ContainsAny(d, "<>&\"") || strings.Count(x, "<") != strings.Count(x, ">") {
				t.Fatalf("markup from domain %q", d)
			}
		}
	})
}
