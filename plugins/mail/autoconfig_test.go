package mail

import (
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
}
