package postgres

import "testing"

func TestCleanIdent(t *testing.T) {
	if _, err := cleanIdent("My_App", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := cleanIdent("postgres", "x"); err == nil {
		t.Fatal("reserved")
	}
	if _, err := cleanIdent("1bad", "x"); err == nil {
		t.Fatal("digit")
	}
	if err := cleanPassword("short"); err == nil {
		t.Fatal("short pw")
	}
	if err := cleanPassword("long-enough-secret"); err != nil {
		t.Fatal(err)
	}
}
