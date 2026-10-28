package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVacationSieve(t *testing.T) {
	a := Account{
		Domain: "example.com", Local: "info", Vacation: true,
		VacationSubject: `Abwesend / "Büro"`, VacationBody: "Bin weg.\nBis bald.",
		VacationDays: 5, Aliases: []string{"kontakt"},
	}
	s := vacationSieve(a)
	for _, want := range []string{
		`require ["vacation"];`,
		`:days 5`,
		`:subject "Abwesend / \"Büro\""`,
		`:addresses ["info@example.com", "kontakt@example.com"]`,
		`"Bin weg.\nBis bald."`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("script lacks %q:\n%s", want, s)
		}
	}
	if vacationSieve(Account{Domain: "example.com", Local: "x"}) != "" {
		t.Fatal("off vacation must yield empty script")
	}
	if vacationSieve(Account{Domain: "example.com", Local: "x", Vacation: true, ForwardOnly: true, Forward: []string{"a@b.c"}}) != "" {
		t.Fatal("forward-only must not write vacation")
	}
}

func TestCleanVacation(t *testing.T) {
	a, err := cleanAccount(Account{
		Domain: "example.com", Local: "info", Vacation: true, VacationBody: "Hallo", VacationDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if a.VacationSubject != "Abwesend" || !a.Vacation {
		t.Fatalf("%+v", a)
	}
	if _, err := cleanAccount(Account{Domain: "example.com", Local: "info", Vacation: true, VacationDays: 7}); err == nil {
		t.Fatal("empty body must fail")
	}
	if _, err := cleanAccount(Account{
		Domain: "example.com", Local: "info", Vacation: true, VacationBody: "x", VacationDays: 99,
	}); err == nil {
		t.Fatal("days out of range must fail")
	}
	fwd, err := cleanAccount(Account{
		Domain: "example.com", Local: "shop", ForwardOnly: true, Forward: []string{"a@b.co"},
		Vacation: true, VacationBody: "x", VacationDays: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if fwd.Vacation {
		t.Fatal("forward-only clears vacation")
	}
}

func TestWriteVacation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_MAIL_ROOT", dir)
	a := Account{
		Domain: "example.com", Local: "info", Vacation: true,
		VacationSubject: "Weg", VacationBody: "Später", VacationDays: 3,
	}
	if err := writeVacation(a); err != nil {
		t.Fatal(err)
	}
	path := personalSievePath(a.Domain, a.Local)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `:days 3`) {
		t.Fatalf("%s", body)
	}
	a.Vacation = false
	if err := writeVacation(a); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected removal, got %v", err)
	}
	// Path stays under the test root.
	home := mailboxHome(a.Domain, a.Local)
	if !strings.HasPrefix(home, filepath.Clean(dir)) {
		t.Fatalf("home %q not under %q", home, dir)
	}
}
