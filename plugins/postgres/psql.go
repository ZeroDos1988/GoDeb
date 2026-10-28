package postgres

import (
	"context"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"strings"

	"godeb/backend/core"
)

// psqlArgs runs psql as the postgres user. runuser ships with util-linux
// (essential on Debian), sudo may be missing on minimal installs.
func psqlArgs(extra ...string) []string {
	args := []string{"-u", "postgres", "--", "psql", "-X", "-v", "ON_ERROR_STOP=1", "-At"}
	return append(args, extra...)
}

// psql sends sql on stdin, never as an argument: arguments are visible in
// /proc/*/cmdline and end up in the log when the command fails.
func psql(ctx context.Context, sql string) (string, error) {
	res := core.RunAdminInput(ctx, sql, "runuser", psqlArgs()...)
	if res.Err != nil {
		return "", fmt.Errorf("%s", res.Detail())
	}
	return res.Stdout, nil
}

func psqlDB(ctx context.Context, dbname, sql string) (string, error) {
	if _, err := cleanIdent(dbname, "Datenbank"); err != nil {
		return "", err
	}
	res := core.RunAdminInput(ctx, sql, "runuser", psqlArgs("-d", dbname)...)
	if res.Err != nil {
		return "", fmt.Errorf("%s", res.Detail())
	}
	return res.Stdout, nil
}

func listDatabases(ctx context.Context) ([]string, error) {
	out, err := psql(ctx, `SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY 1;`)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func listRoles(ctx context.Context) ([]string, error) {
	out, err := psql(ctx, `SELECT rolname FROM pg_roles WHERE rolcanlogin ORDER BY 1;`)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func listTables(ctx context.Context, dbname string) ([]string, error) {
	out, err := psqlDB(ctx, dbname, `SELECT tablename FROM pg_tables WHERE schemaname = 'public' ORDER BY 1;`)
	if err != nil {
		return nil, err
	}
	return nonEmptyLines(out), nil
}

func browseTable(ctx context.Context, dbname, table string) ([]string, [][]string, error) {
	if _, err := cleanIdent(dbname, "Datenbank"); err != nil {
		return nil, nil, err
	}
	tbl, err := cleanIdent(table, "Tabelle")
	if err != nil {
		return nil, nil, err
	}
	sql := fmt.Sprintf(`SELECT * FROM "%s" LIMIT 100;`, tbl)
	// Unaligned with header but without the "(n rows)" footer; -t would
	// drop the column names.
	args := []string{"-u", "postgres", "--", "psql", "-X", "-v", "ON_ERROR_STOP=1", "-A", "-P", "footer=off", "-F", "\t", "-d", dbname}
	res := core.RunAdminInput(ctx, sql, "runuser", args...)
	if res.Err != nil {
		return nil, nil, fmt.Errorf("%s", res.Detail())
	}
	lines := nonEmptyLines(res.Stdout)
	if len(lines) == 0 {
		return []string{}, [][]string{}, nil
	}
	head := strings.Split(lines[0], "\t")
	rows := make([][]string, 0, len(lines)-1)
	for _, l := range lines[1:] {
		rows = append(rows, strings.Split(l, "\t"))
	}
	return head, rows, nil
}

func createDatabase(ctx context.Context, name, owner string) error {
	n, err := cleanIdent(name, "Datenbank")
	if err != nil {
		return err
	}
	o, err := cleanIdent(owner, "Benutzer")
	if err != nil {
		return err
	}
	_, err = psql(ctx, fmt.Sprintf(`CREATE DATABASE "%s" OWNER "%s";`, n, o))
	return err
}

func createRole(ctx context.Context, name, password string) error {
	n, err := cleanIdent(name, "Benutzer")
	if err != nil {
		return err
	}
	if err := cleanPassword(password); err != nil {
		return err
	}
	verifier, err := scramVerifier(password)
	if err != nil {
		return err
	}
	// Only the SCRAM verifier reaches PostgreSQL, so neither its log nor
	// pg_stat_statements ever holds the clear password.
	_, err = psql(ctx, fmt.Sprintf(`CREATE ROLE "%s" LOGIN PASSWORD '%s';`, n, verifier))
	return err
}

const scramIterations = 4096

// scramVerifier builds the SCRAM-SHA-256 secret PostgreSQL stores in
// pg_authid (RFC 5802/7677). cleanPassword limits passwords to printable
// ASCII, where SASLprep is the identity, so clients compute the same keys.
func scramVerifier(password string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return scramVerifierSalt(password, salt)
}

func scramVerifierSalt(password string, salt []byte) (string, error) {
	salted, err := pbkdf2.Key(sha256.New, password, salt, scramIterations, sha256.Size)
	if err != nil {
		return "", err
	}
	clientKey := hmacSHA256(salted, "Client Key")
	storedKey := sha256.Sum256(clientKey)
	serverKey := hmacSHA256(salted, "Server Key")
	b64 := base64.StdEncoding.EncodeToString
	return fmt.Sprintf("SCRAM-SHA-256$%d:%s$%s:%s", scramIterations, b64(salt), b64(storedKey[:]), b64(serverKey)), nil
}

func hmacSHA256(key []byte, msg string) []byte {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(msg))
	return m.Sum(nil)
}

func dropDatabase(ctx context.Context, name string) error {
	n, err := cleanIdent(name, "Datenbank")
	if err != nil {
		return err
	}
	_, err = psql(ctx, fmt.Sprintf(`DROP DATABASE IF EXISTS "%s";`, n))
	return err
}

func dropRole(ctx context.Context, name string) error {
	n, err := cleanIdent(name, "Benutzer")
	if err != nil {
		return err
	}
	_, err = psql(ctx, fmt.Sprintf(`DROP ROLE IF EXISTS "%s";`, n))
	return err
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" {
			out = append(out, l)
		}
	}
	return out
}
