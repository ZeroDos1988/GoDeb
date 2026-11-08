package sites

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"godeb/backend/agent"
	"godeb/backend/core"
	"godeb/backend/db"
	"godeb/plugins/domains"
	"godeb/plugins/mail"
	"godeb/plugins/wireguard"
)

// The hub issues the certificate of a site's mail host by DNS-01 (the
// Hetzner token never leaves the hub) and renews it. The site fetches it
// over the tunnel from GET certPath; the hub answers only the tunnel
// address of the site the name is assigned to.

const (
	certPath    = "/api/plugins/sites/cert"
	renewBefore = 30 * 24 * time.Hour
	// hubWebPort is GoDeb's web port on the hub's wg0 (GODEB_ADDR default).
	hubWebPort   = 8080
	maxCertBytes = 64 << 10
)

// certRow lives in the agent database of the hub.
type certRow struct {
	Host      string `gorm:"primaryKey"`
	Site      string
	CertPEM   string
	CertKey   string `vault:"secret"`
	NotAfter  string // RFC 3339
	LastRun   string
	LastError string
}

func (certRow) TableName() string { return "sites_certs" }

// acmeRow holds the ACME account of the hub for site certificates.
type acmeRow struct {
	ID         int    `gorm:"primaryKey"`
	AccountKey string `vault:"secret"`
}

func (acmeRow) TableName() string { return "sites_acme" }

func init() {
	db.RegisterAgentSecret("sites_certs", "host", "cert_key")
	db.RegisterAgentSecret("sites_acme", "id", "account_key")
}

// CertState is the certificate of a mail host as the UI shows it.
type CertState struct {
	NotAfter  string `json:"notAfter,omitempty"`
	LastRun   string `json:"lastRun,omitempty"`
	LastError string `json:"lastError,omitempty"`
	Running   bool   `json:"running"`
}

var (
	migrateMu sync.Mutex
	migrated  = map[string]bool{}
	rowMu     sync.Mutex
)

func store() (*gorm.DB, error) {
	gdb, err := db.AgentGorm()
	if err != nil {
		return nil, err
	}
	path := db.AgentPath()
	migrateMu.Lock()
	defer migrateMu.Unlock()
	if !migrated[path] {
		if err := gdb.AutoMigrate(&certRow{}, &acmeRow{}); err != nil {
			return nil, err
		}
		migrated[path] = true
	}
	return gdb, nil
}

func loadCert(host string) (certRow, bool, error) {
	gdb, err := store()
	if err != nil {
		return certRow{}, false, err
	}
	var row certRow
	res := gdb.Where("host = ?", host).Limit(1).Find(&row)
	return row, res.RowsAffected > 0, res.Error
}

func loadCerts() (map[string]CertState, error) {
	gdb, err := store()
	if err != nil {
		return nil, err
	}
	var rows []certRow
	if err := gdb.Find(&rows).Error; err != nil {
		return nil, err
	}
	out := map[string]CertState{}
	for _, r := range rows {
		out[r.Host] = CertState{NotAfter: r.NotAfter, LastRun: r.LastRun, LastError: r.LastError, Running: issuing(r.Host)}
	}
	return out, nil
}

func saveCert(row certRow) error {
	rowMu.Lock()
	defer rowMu.Unlock()
	gdb, err := store()
	if err != nil {
		return err
	}
	return gdb.Save(&row).Error
}

func accountKey() (string, error) {
	rowMu.Lock()
	defer rowMu.Unlock()
	gdb, err := store()
	if err != nil {
		return "", err
	}
	var row acmeRow
	if err := gdb.Where("id = 1").Limit(1).Find(&row).Error; err != nil {
		return "", err
	}
	if row.AccountKey != "" {
		return row.AccountKey, nil
	}
	_, pemKey, err := domains.ACMEAccountKey("")
	if err != nil {
		return "", err
	}
	return pemKey, gdb.Save(&acmeRow{ID: 1, AccountKey: pemKey}).Error
}

// Hooks for tests.
var (
	obtainCert = domains.ObtainCertificate
	issueMu    sync.Mutex
	issueRun   = map[string]bool{}
)

func issuing(host string) bool {
	issueMu.Lock()
	defer issueMu.Unlock()
	return issueRun[host]
}

// startIssue orders a certificate for host in the background; false when
// an order for it is already running.
func startIssue(host, site string) bool {
	issueMu.Lock()
	if issueRun[host] {
		issueMu.Unlock()
		return false
	}
	issueRun[host] = true
	issueMu.Unlock()
	go func() {
		defer func() {
			issueMu.Lock()
			delete(issueRun, host)
			issueMu.Unlock()
		}()
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		defer cancel()
		if err := issue(ctx, host, site); err != nil {
			slog.Error("Standort: Zertifikat nicht erhalten", "name", host, "fehler", err)
		}
	}()
	return true
}

func issue(ctx context.Context, host, site string) error {
	row, _, err := loadCert(host)
	if err != nil {
		return err
	}
	row.Host, row.Site = host, site
	row.LastRun = time.Now().UTC().Format(time.RFC3339)
	key, err := accountKey()
	if err == nil {
		var c domains.Certificate
		if c, err = obtainCert(ctx, host, key); err == nil {
			row.CertPEM, row.CertKey = c.CertPEM, c.KeyPEM
			row.NotAfter = c.NotAfter.UTC().Format(time.RFC3339)
			slog.Info("Standort: Zertifikat erhalten", "name", host, "gültig_bis", row.NotAfter)
		}
	}
	row.LastError = ""
	if err != nil {
		row.LastError = err.Error()
	}
	if serr := saveCert(row); serr != nil {
		return errors.Join(err, serr)
	}
	return err
}

func needsRenewal(row certRow, now time.Time) bool {
	if row.CertPEM == "" {
		return true
	}
	end, err := time.Parse(time.RFC3339, row.NotAfter)
	return err != nil || end.Sub(now) < renewBefore
}

// renewAll runs on the hub: every mail host of a home site gets its
// certificate, and it is renewed 30 days before it ends. A failed order is
// retried after a day, not every round.
func renewAll(now time.Time) {
	if !tokenSet() {
		return
	}
	list, err := List()
	if err != nil {
		return
	}
	for _, s := range list {
		host := s.Profile.MailHost
		if host == "" {
			continue
		}
		row, _, err := loadCert(host)
		if err != nil || !needsRenewal(row, now) {
			continue
		}
		if last, err := time.Parse(time.RFC3339, row.LastRun); err == nil && row.LastError != "" && now.Sub(last) < 24*time.Hour {
			continue
		}
		startIssue(host, s.Name)
	}
}

type certResponse struct {
	CertPEM  string `json:"certPEM"`
	KeyPEM   string `json:"keyPEM"`
	NotAfter string `json:"notAfter"`
}

// handleCert serves the certificate of a site's mail host to that site.
// The route needs no session (auth.protected): the site has no account on
// the hub. It answers only when the request comes from the tunnel address
// of the site the name is assigned to; WireGuard binds that address to the
// site's key, and the web process sets the address from the connection,
// never from a header. Everyone else gets 404.
func handleCert(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("host")))
	ip := agent.ClientIP(r)
	if role, err := currentRole(); err != nil || role.Role != wireguard.RoleHub || host == "" || ip == "" {
		http.NotFound(w, r)
		return
	}
	s, err := siteFor(host, ip)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	row, ok, err := loadCert(host)
	if err != nil || !ok || row.CertPEM == "" {
		core.WriteError(w, http.StatusServiceUnavailable, "Zertifikat für "+host+" ist noch nicht ausgestellt")
		return
	}
	slog.Info("Standort: Zertifikat abgerufen", "name", host, "standort", s.Name)
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, certResponse{CertPEM: row.CertPEM, KeyPEM: row.CertKey, NotAfter: row.NotAfter})
}

// Hooks for tests.
var (
	hubAddr       = wireguard.HubTunnelAddr
	hubWebPortFor = func() int { return hubWebPort }
	installCert   = mail.InstallHubCertificate
	mailBehindHub = mailHostBehindHub
	certClient    = &http.Client{
		Timeout:       15 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
)

// mailHostBehindHub is the mail host when this host is a home server whose
// mail server runs behind the hub, otherwise "".
func mailHostBehindHub() string {
	role, err := currentRole()
	if err != nil || role.Role != wireguard.RolePeer || role.Profile.Kind != wireguard.ProfileHome {
		return ""
	}
	if !mail.Installed() || !mail.BehindHub() || mail.Hostname() != role.Profile.MailHost {
		return ""
	}
	return role.Profile.MailHost
}

// FetchCertificate runs on a home server: it fetches the certificate of its
// mail host from the hub and hands it to the mail server.
func FetchCertificate(ctx context.Context) error {
	host := mailBehindHub()
	if host == "" {
		return errors.New("Kein Mailserver hinter dem Hub eingerichtet")
	}
	hub, ok := hubAddr()
	if !ok {
		return errors.New("Hub-Adresse im Tunnel nicht gefunden")
	}
	u := url.URL{Scheme: "http", Host: net.JoinHostPort(hub.String(), fmt.Sprint(hubWebPortFor())), Path: certPath, RawQuery: url.Values{"host": {host}}.Encode()}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	resp, err := certClient.Do(req)
	if err != nil {
		return fmt.Errorf("Hub nicht erreichbar: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxCertBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxCertBytes {
		return errors.New("Antwort des Hubs ist zu groß")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("Hub antwortet %d: %s", resp.StatusCode, errorText(body))
	}
	var c certResponse
	if err := json.Unmarshal(body, &c); err != nil {
		return fmt.Errorf("Antwort des Hubs: %w", err)
	}
	return installCert(ctx, []byte(c.CertPEM), []byte(c.KeyPEM))
}

// errorText takes the message out of a JSON error answer of the hub.
func errorText(body []byte) string {
	var e struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &e) == nil && e.Error != "" {
		return e.Error
	}
	return strings.TrimSpace(string(body))
}
