package mail

import (
	"regexp"
	"strings"
	"testing"
)

// readSieveString reads a quoted Sieve string at the start of s (RFC 5228
// 2.4.2: only \\ and \" are escapes, no line breaks) and returns its value
// and the rest.
func readSieveString(t *testing.T, s string) (string, string) {
	t.Helper()
	if !strings.HasPrefix(s, `"`) {
		t.Fatalf("expected a quoted string at %q", s)
	}
	var b strings.Builder
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
			if i >= len(s) || (s[i] != '\\' && s[i] != '"') {
				t.Fatalf("bad escape in %q", s)
			}
			b.WriteByte(s[i])
		case '"':
			return b.String(), s[i+1:]
		case '\n', '\r':
			t.Fatalf("line break inside a quoted string: %q", s)
		default:
			b.WriteByte(s[i])
		}
	}
	t.Fatalf("unterminated string: %q", s)
	return "", ""
}

func FuzzVacationSieve(f *testing.F) {
	f.Add("Weg", "Hallo,\nbis bald.", 7, "kontakt")
	f.Add(`Abwesend "Büro" \`, ".\n..\n.punkt\r\n.", 3, "info")
	f.Add("a\"; discard; #", "text:\n.\n;\nkeep;", 1, "x")
	f.Fuzz(func(t *testing.T, subject, body string, days int, alias string) {
		a := Account{Domain: "example.com", Local: "info", Vacation: true,
			VacationSubject: subject, VacationBody: body, VacationDays: days}
		if _, err := NormalizeLocal(alias); err == nil {
			a.Aliases = []string{alias}
		}
		a, err := cleanVacation(a)
		if err != nil {
			return
		}
		script := vacationSieve(a)
		rest, ok := strings.CutPrefix(script, header+`require ["vacation"];`+"\n")
		if !ok {
			t.Fatalf("unexpected start:\n%s", script)
		}
		m := regexp.MustCompile(`^vacation :days ([0-9]+) :subject `).FindStringSubmatch(rest)
		if m == nil {
			t.Fatalf("unexpected vacation line: %q", rest)
		}
		subj, rest := readSieveString(t, rest[len(m[0]):])
		wantSubj := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(a.VacationSubject)
		if subj != wantSubj {
			t.Fatalf("subject %q, want %q", subj, wantSubj)
		}
		rest, ok = strings.CutPrefix(rest, " :addresses [")
		if !ok {
			t.Fatalf("addresses missing: %q", rest)
		}
		for {
			_, rest = readSieveString(t, rest)
			if r, ok := strings.CutPrefix(rest, ", "); ok {
				rest = r
				continue
			}
			break
		}
		rest, ok = strings.CutPrefix(rest, "] text:\n")
		if !ok {
			t.Fatalf("text block missing: %q", rest)
		}
		var lines []string
		for {
			line, after, found := strings.Cut(rest, "\n")
			if !found {
				t.Fatalf("text block not terminated: %q", rest)
			}
			rest = after
			if line == "." {
				break
			}
			lines = append(lines, strings.TrimPrefix(line, "."))
		}
		if rest != ";\n" {
			t.Fatalf("something follows the vacation command: %q", rest)
		}
		wantBody := strings.NewReplacer("\r\n", "\n", "\r", "\n").Replace(a.VacationBody)
		if got := strings.Join(lines, "\n"); got != wantBody {
			t.Fatalf("body %q, want %q", got, wantBody)
		}
	})
}

// Addresses end up in Postfix maps and Dovecot's passwd-file, where
// whitespace, ':' or ',' would start a new field or entry.
func FuzzMailAddress(f *testing.F) {
	for _, s := range []string{"info@example.com", "a.b+c@sub.example.org", "x y@example.com", "a:b@example.com", "\"a\"@example.com", "a@b@example.com"} {
		f.Add(s)
	}
	safe := regexp.MustCompile(`^[a-z0-9._+-]+@[a-z0-9.-]+$`)
	f.Fuzz(func(t *testing.T, v string) {
		if a, err := NormalizeAddress(v); err == nil && !safe.MatchString(a) {
			t.Fatalf("NormalizeAddress(%q) = %q", v, a)
		}
		if l, err := NormalizeLocal(v); err == nil && !regexp.MustCompile(`^[a-z0-9._+-]+$`).MatchString(l) {
			t.Fatalf("NormalizeLocal(%q) = %q", v, l)
		}
		if d, err := NormalizeDomain(v); err == nil && !regexp.MustCompile(`^[a-z0-9.-]+$`).MatchString(d) {
			t.Fatalf("NormalizeDomain(%q) = %q", v, d)
		}
	})
}
