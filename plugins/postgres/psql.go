package postgres

import (
	"context"
	"fmt"
	"strings"

	"godeb/backend/core"
)

func psqlArgs(extra ...string) []string {
	args := []string{"-n", "-u", "postgres", "--", "psql", "-v", "ON_ERROR_STOP=1", "-At"}
	return append(args, extra...)
}

func psql(ctx context.Context, sql string) (string, error) {
	res := core.RunAdmin(ctx, "sudo", append(psqlArgs("-c", sql))...)
	if res.Err != nil {
		return "", fmt.Errorf("%s", res.Detail())
	}
	return res.Stdout, nil
}

func psqlDB(ctx context.Context, dbname, sql string) (string, error) {
	if _, err := cleanIdent(dbname, "Datenbank"); err != nil {
		return "", err
	}
	res := core.RunAdmin(ctx, "sudo", append(psqlArgs("-d", dbname, "-c", sql))...)
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
	res := core.RunAdmin(ctx, "sudo", append(psqlArgs("-d", dbname, "-F", "\t", "-A", "-c", sql))...)
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
	tag := "godebpw"
	for strings.Contains(password, tag) {
		tag += "x"
	}
	_, err = psql(ctx, fmt.Sprintf(`CREATE ROLE "%s" LOGIN PASSWORD $%s$%s$%s$;`, n, tag, password, tag))
	return err
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
