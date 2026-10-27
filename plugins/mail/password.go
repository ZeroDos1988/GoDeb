package mail

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters for mailbox passwords (OWASP: 19 MiB, t=2, p=1).
// Dovecot verifies the PHC string with libsodium; mail clients log in
// often, so the cost stays moderate and Dovecot's auth cache does the rest.
const (
	argonMemory  = 19456 // KiB
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16

	minPassword = 10
	maxPassword = 128
)

var errPassword = fmt.Errorf("Passwort: %d bis %d Zeichen", minPassword, maxPassword)

// HashPassword returns the Dovecot passdb value {ARGON2ID}$argon2id$….
// GoDeb never stores the plain password.
func HashPassword(password string) (string, error) {
	if err := checkPassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("{ARGON2ID}$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads, b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

func checkPassword(password string) error {
	n := utf8.RuneCountInString(password)
	if n < minPassword || n > maxPassword || !utf8.ValidString(password) {
		return errPassword
	}
	for _, r := range password {
		// Control characters (and ':' or newline would also break the
		// passwd-file line, though only the hash is written there).
		if r < 0x20 || r == 0x7f {
			return errors.New("Passwort enthält Steuerzeichen")
		}
	}
	return nil
}
