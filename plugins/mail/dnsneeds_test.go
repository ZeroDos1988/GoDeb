package mail

import "testing"

func TestZoneOfHost(t *testing.T) {
	list := []Domain{{Name: "example.com"}, {Name: "sub.example.com"}, {Name: "other.org"}}
	cases := map[string]string{
		"mail.example.com":   "example.com",
		"mx.sub.example.com": "sub.example.com",
		"mail.unrelated.net": "unrelated.net",
		"Mail.Example.COM.":  "example.com",
		"example.com":        "example.com",
		"mail.foreign.co.uk": "foreign.co.uk",
		"localhost.localnet": "localhost.localnet",
	}
	for host, want := range cases {
		if got := zoneOfHost(host, list); got != want {
			t.Errorf("zoneOfHost(%q) = %q, want %q", host, got, want)
		}
	}
}
