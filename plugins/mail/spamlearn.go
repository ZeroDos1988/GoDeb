package mail

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
)

const (
	learnSpamSieve = dovecotDir + "/learn-spam.sieve"
	learnHamSieve  = dovecotDir + "/learn-ham.sieve"
	sievePipeDir   = dovecotDir + "/bin"
	learnSpamBin   = sievePipeDir + "/learn-spam"
	learnHamBin    = sievePipeDir + "/learn-ham"
	rspamdCtrlPass = dovecotDir + "/rspamd-controller.password"
	rspamdCtrlConf = rspamdLocal + "/worker-controller.inc"
)

const learnSpamSieveScript = `require ["vnd.dovecot.pipe"];
pipe "learn-spam";
`

const learnHamSieveScript = `require ["vnd.dovecot.pipe"];
pipe "learn-ham";
`

// learnPipeScript calls rspamc with the controller password file.
func learnPipeScript(action string) string {
	return `#!/bin/sh
# Written by GoDeb (plugin mail). Changes are overwritten.
set -eu
PASSFILE=` + rspamdCtrlPass + `
PASS=$(tr -d '\n' < "$PASSFILE")
exec /usr/bin/rspamc -h 127.0.0.1:11334 -P "$PASS" ` + action + `
`
}

// loadOrMakeRspamdPassword returns the controller password, generating one
// when the file is missing (caller writes it via writeFile).
func loadOrMakeRspamdPassword() (string, error) {
	if b, err := os.ReadFile(root(rspamdCtrlPass)); err == nil {
		p := strings.TrimSpace(string(b))
		if len(p) >= 16 {
			return p, nil
		}
	}
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func rspamdControllerConfig(password string) string {
	return header + fmt.Sprintf(`bind_socket = "127.0.0.1:11334";
password = %q;
enable_password = %q;
`, password, password)
}
