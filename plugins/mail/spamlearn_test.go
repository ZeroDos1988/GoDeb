package mail

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOrMakeRspamdPasswordStable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GODEB_MAIL_ROOT", dir)
	if err := os.MkdirAll(filepath.Join(dir, "etc/dovecot/godeb"), 0o755); err != nil {
		t.Fatal(err)
	}
	p1, err := loadOrMakeRspamdPassword()
	if err != nil || len(p1) < 16 {
		t.Fatalf("%q %v", p1, err)
	}
	if err := writeFile(rspamdCtrlPass, p1+"\n"); err != nil {
		t.Fatal(err)
	}
	p2, err := loadOrMakeRspamdPassword()
	if err != nil || p2 != p1 {
		t.Fatalf("got %q want %q (%v)", p2, p1, err)
	}
	cfg := rspamdControllerConfig(p1)
	if !strings.Contains(cfg, `password = "`+p1+`"`) || !strings.Contains(cfg, "127.0.0.1:11334") {
		t.Fatalf("%s", cfg)
	}
}
