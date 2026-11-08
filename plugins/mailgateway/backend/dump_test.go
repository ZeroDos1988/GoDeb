package mailgateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDumpConfig writes the main.cf settings of the gateway into
// $GODEB_MAILGW_DUMP for scripts/check-mail-config.sh, which runs them
// through the real postconf of Debian 13. Skipped without the variable.
func TestDumpConfig(t *testing.T) {
	dir := os.Getenv("GODEB_MAILGW_DUMP")
	if dir == "" {
		t.Skip("GODEB_MAILGW_DUMP not set")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(postfixSettings("10.8.0.2", []string{"example.com", "example.org"}), "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "postconf-e.txt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
