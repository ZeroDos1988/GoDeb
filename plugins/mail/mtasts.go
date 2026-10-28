package mail

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const mtaSTSDir = "/etc/godeb/mta-sts"

// mtaSTSPolicy is the file served later at
// https://mta-sts.<domain>/.well-known/mta-sts.txt (needs the web plugin).
// mode=testing: receivers fetch the policy but do not yet reject failures.
func mtaSTSPolicy(s Settings) string {
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(s.Hostname)), ".")
	return "version: STSv1\nmode: testing\nmx: " + host + "\nmax_age: 86400\n"
}

// mtaSTSID is the DNS id; it changes when the policy body changes so
// receivers refetch.
func mtaSTSID(s Settings) string {
	sum := sha256.Sum256([]byte(mtaSTSPolicy(s)))
	return hex.EncodeToString(sum[:8])
}

func mtaSTSRecord(domain string, s Settings) string {
	return "v=STSv1; id=" + mtaSTSID(s)
}

func tlsRPTRecord(domain string) string {
	return "v=TLSRPTv1; rua=mailto:tls-reports@" + domain
}

func mtaSTSFilePath(domain string) string {
	return filepath.Join(mtaSTSDir, domain+".txt")
}

// syncMTASTS writes the policy file for every mail domain under /etc/godeb.
func syncMTASTS(s Settings, domains []Domain) error {
	if err := os.MkdirAll(root(mtaSTSDir), 0o755); err != nil {
		return err
	}
	want := map[string]bool{}
	policy := mtaSTSPolicy(s)
	for _, d := range domains {
		if d.Suspended {
			continue
		}
		if _, err := NormalizeDomain(d.Name); err != nil {
			return err
		}
		path := mtaSTSFilePath(d.Name)
		want[path] = true
		if err := writeAs(root(path), policy, fileSpec{0o644, ""}); err != nil {
			return fmt.Errorf("MTA-STS %s: %w", d.Name, err)
		}
	}
	entries, err := os.ReadDir(root(mtaSTSDir))
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		full := filepath.Join(mtaSTSDir, e.Name())
		if !want[full] {
			_ = os.Remove(root(full))
		}
	}
	return nil
}
