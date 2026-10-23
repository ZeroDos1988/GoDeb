package mailgateway

import (
	"database/sql"
	"strings"
	"time"

	"godeb/backend/db"
)

type storedDomain struct {
	Domain     string `json:"domain"`
	Selector   string `json:"selector"`
	DKIMPublic string `json:"dkimPublic"`
	CreatedAt  string `json:"created"`
}

func openStore() (*sql.DB, error) {
	conn, err := db.AgentOpen()
	if err != nil {
		return nil, err
	}
	if err := ensureMailSchema(conn); err != nil {
		return nil, err
	}
	return conn, nil
}

func ensureMailSchema(conn *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS mail_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			relay_host TEXT NOT NULL DEFAULT '10.8.0.2',
			updated_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS mail_domains (
			domain TEXT PRIMARY KEY,
			selector TEXT NOT NULL DEFAULT 'godeb',
			dkim_public TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS mail_queue_events (
			id INTEGER PRIMARY KEY,
			queue_id TEXT NOT NULL DEFAULT '',
			sender TEXT NOT NULL DEFAULT '',
			recipients TEXT NOT NULL DEFAULT '',
			action TEXT NOT NULL DEFAULT '',
			detail TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT ''
		)`,
	}
	for _, statement := range statements {
		if _, err := conn.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func loadRelay(conn *sql.DB) (string, error) {
	var host string
	err := conn.QueryRow(`SELECT relay_host FROM mail_settings WHERE id = 1`).Scan(&host)
	if err == sql.ErrNoRows {
		return defaultRelay, nil
	}
	if err != nil {
		return "", err
	}
	if host == "" {
		return defaultRelay, nil
	}
	return host, nil
}

func saveRelay(conn *sql.DB, host string) error {
	_, err := conn.Exec(
		`INSERT INTO mail_settings (id, relay_host, updated_at) VALUES (1, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET relay_host = excluded.relay_host, updated_at = excluded.updated_at`,
		host, time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func listDomains(conn *sql.DB) ([]storedDomain, error) {
	rows, err := conn.Query(`SELECT domain, selector, dkim_public, created_at FROM mail_domains ORDER BY domain`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var domains []storedDomain
	for rows.Next() {
		var domain storedDomain
		if err := rows.Scan(&domain.Domain, &domain.Selector, &domain.DKIMPublic, &domain.CreatedAt); err != nil {
			return nil, err
		}
		domains = append(domains, domain)
	}
	return domains, rows.Err()
}

func findDomain(conn *sql.DB, name string) (storedDomain, bool, error) {
	var domain storedDomain
	err := conn.QueryRow(
		`SELECT domain, selector, dkim_public, created_at FROM mail_domains WHERE domain = ?`, name,
	).Scan(&domain.Domain, &domain.Selector, &domain.DKIMPublic, &domain.CreatedAt)
	if err == sql.ErrNoRows {
		return storedDomain{}, false, nil
	}
	if err != nil {
		return storedDomain{}, false, err
	}
	return domain, true, nil
}

func saveDomain(conn *sql.DB, domain storedDomain) error {
	_, err := conn.Exec(
		`INSERT INTO mail_domains (domain, selector, dkim_public, created_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(domain) DO UPDATE SET selector = excluded.selector, dkim_public = excluded.dkim_public`,
		domain.Domain, domain.Selector, domain.DKIMPublic, domain.CreatedAt,
	)
	return err
}

func rememberQueue(conn *sql.DB, messages []queueMessage) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, message := range messages {
		if message.ID == "" {
			continue
		}
		_, err := conn.Exec(
			`INSERT INTO mail_queue_events (queue_id, sender, recipients, action, detail, created_at)
			 SELECT ?, ?, ?, 'seen', ?, ?
			 WHERE NOT EXISTS (
			   SELECT 1 FROM mail_queue_events WHERE queue_id = ? AND action = 'seen'
			 )`,
			message.ID, message.Sender, strings.Join(message.Recipients, ", "), clip(message.Reason, 500), now, message.ID,
		)
		if err != nil {
			return err
		}
	}
	return nil
}

func rememberFlush(conn *sql.DB, detail string) error {
	_, err := conn.Exec(
		`INSERT INTO mail_queue_events (queue_id, sender, recipients, action, detail, created_at)
		 VALUES ('', '', '', 'flush', ?, ?)`,
		clip(detail, 500), time.Now().UTC().Format(time.RFC3339),
	)
	return err
}

func clip(value string, limit int) string {
	value = strings.TrimSpace(value)
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
