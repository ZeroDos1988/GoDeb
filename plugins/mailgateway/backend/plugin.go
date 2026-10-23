package mailgateway

import (
	"context"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"godeb/backend/core"
)

func init() {
	core.Register("mailgateway", Mount)
}

// Mount registers the mail gateway routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins/mailgateway/status", handleStatus)
	mux.HandleFunc("POST /api/plugins/mailgateway/setup", handleSetup)
	mux.HandleFunc("GET /api/plugins/mailgateway/queue", handleQueue)
	mux.HandleFunc("POST /api/plugins/mailgateway/queue/flush", handleFlush)
	mux.HandleFunc("GET /api/plugins/mailgateway/dns-check", handleDNS)
}

var runCommand = func(ctx context.Context, name string, args ...string) core.CommandResult {
	return core.RunAdmin(ctx, name, args...)
}

type setupRequest struct {
	Domain    string `json:"domain"`
	RelayHost string `json:"relayHost"`
	Selector  string `json:"selector"`
}

type statusResponse struct {
	RelayHost   string         `json:"relayHost"`
	Domains     []storedDomain `json:"domains"`
	TunnelUp    bool           `json:"tunnelUp"`
	LocalTunnel bool           `json:"localTunnel"`
	Mode        string         `json:"mode"`
	PublicIP    string         `json:"publicIP"`
	core.CommandEnvelope
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	conn, err := openStore()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	relay, err := loadRelay(conn)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	domains, err := listDomains(conn)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if domains == nil {
		domains = []storedDomain{}
	}
	probe := runCommand(r.Context(), "postconf", "-h", "relayhost")
	env := core.CommandEnvelope{Command: probe.Command}
	if probe.Err != nil {
		env.Mocked = true
		env.Detail = probe.Detail()
	}
	up := tunnelReachable(r.Context(), relay)
	mode := "idle"
	if len(domains) > 0 {
		if up {
			mode = "relay"
		} else {
			mode = "buffer"
		}
	}
	core.WriteJSON(w, http.StatusOK, statusResponse{
		RelayHost:       relay,
		Domains:         domains,
		TunnelUp:        up,
		LocalTunnel:     localTunnel(r.Context()),
		Mode:            mode,
		PublicIP:        publicIPv4(r.Context()),
		CommandEnvelope: env,
	})
}

func handleSetup(w http.ResponseWriter, r *http.Request) {
	var req setupRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	domain, err := normalizeDomain(req.Domain)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	relay, err := normalizeRelay(req.RelayHost)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	selector, err := normalizeSelector(req.Selector)
	if err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	conn, err := openStore()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	existing, _, err := findDomain(conn, domain)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	privatePEM, publicB64, err := loadOrCreateKey(domain, selector, existing)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	domains, err := listDomains(conn)
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	files := make([]dkimFiles, 0, len(domains)+1)
	seen := false
	names := make([]string, 0, len(domains)+1)
	for _, item := range domains {
		if item.Domain == domain {
			seen = true
			item.Selector = selector
			item.DKIMPublic = publicB64
		}
		pemText, err := os.ReadFile(privateKeyPath(item.Domain, item.Selector))
		if item.Domain == domain {
			pemText = []byte(privatePEM)
			err = nil
		}
		if err != nil {
			core.WriteError(w, http.StatusBadGateway, "DKIM-Schlüssel für "+item.Domain+" fehlt")
			return
		}
		files = append(files, dkimFiles{
			domain: item.Domain, selector: item.Selector, privatePEM: string(pemText), publicB64: item.DKIMPublic,
		})
		names = append(names, item.Domain)
	}
	if !seen {
		files = append(files, dkimFiles{domain: domain, selector: selector, privatePEM: privatePEM, publicB64: publicB64})
		names = append(names, domain)
	}
	if err := writeOpenDKIM(files); err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	applied := runCommand(r.Context(), "postconf", append([]string{"-e"}, postfixSettings(relay, names)...)...)
	if applied.Err != nil {
		core.WriteError(w, http.StatusBadGateway, applied.Detail())
		return
	}
	created := existing.CreatedAt
	if created == "" {
		created = time.Now().UTC().Format(time.RFC3339)
	}
	if err := saveDomain(conn, storedDomain{Domain: domain, Selector: selector, DKIMPublic: publicB64, CreatedAt: created}); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := saveRelay(conn, relay); err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	reload := runCommand(r.Context(), "systemctl", "reload", "postfix")
	opendkim := runCommand(r.Context(), "systemctl", "reload", "opendkim")
	detail := strings.TrimSpace(reload.Detail() + " " + opendkim.Detail())
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"domain":     domain,
		"relayHost":  relay,
		"selector":   selector,
		"dkimPublic": publicB64,
		"dkimRecord": dkimTXT(selector, domain, publicB64),
		"command":    applied.Command,
		"detail":     detail,
	})
}

func handleQueue(w http.ResponseWriter, r *http.Request) {
	messages, env := readQueue(r.Context())
	if messages == nil {
		messages = []queueMessage{}
	}
	if !env.Mocked {
		if conn, err := openStore(); err == nil {
			_ = rememberQueue(conn, messages)
		}
	}
	core.WriteJSON(w, http.StatusOK, struct {
		core.CommandEnvelope
		Messages []queueMessage `json:"messages"`
		Count    int            `json:"count"`
	}{CommandEnvelope: env, Messages: messages, Count: len(messages)})
}

func handleFlush(w http.ResponseWriter, r *http.Request) {
	res := runCommand(r.Context(), "postqueue", "-f")
	if res.Err != nil {
		core.WriteError(w, http.StatusBadGateway, res.Detail())
		return
	}
	if conn, err := openStore(); err == nil {
		_ = rememberFlush(conn, res.Stdout)
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"command": res.Command,
		"output":  strings.TrimSpace(res.Stdout),
	})
}

func handleDNS(w http.ResponseWriter, r *http.Request) {
	conn, err := openStore()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	domain := strings.TrimSpace(r.URL.Query().Get("domain"))
	selector := "godeb"
	public := ""
	if domain == "" {
		domains, err := listDomains(conn)
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if len(domains) == 0 {
			core.WriteError(w, http.StatusBadRequest, "Keine Relay-Domain eingerichtet")
			return
		}
		domain = domains[0].Domain
		selector = domains[0].Selector
		public = domains[0].DKIMPublic
	} else {
		domain, err = normalizeDomain(domain)
		if err != nil {
			core.WriteError(w, http.StatusBadRequest, err.Error())
			return
		}
		if stored, ok, err := findDomain(conn, domain); err != nil {
			core.WriteError(w, http.StatusInternalServerError, err.Error())
			return
		} else if ok {
			selector = stored.Selector
			public = stored.DKIMPublic
		}
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"domain":   domain,
		"publicIP": publicIPv4(r.Context()),
		"checks":   evaluateDNS(r.Context(), domain, selector, publicIPv4(r.Context()), public, netResolver{}),
	})
}

func readQueue(ctx context.Context) ([]queueMessage, core.CommandEnvelope) {
	jsonQueue := runCommand(ctx, "postqueue", "-j")
	if jsonQueue.Err == nil {
		if messages, err := parsePostqueueJSON(jsonQueue.Stdout); err == nil {
			return messages, core.CommandEnvelope{Command: jsonQueue.Command}
		}
	}
	textQueue := runCommand(ctx, "mailq")
	if textQueue.Err != nil {
		detail := jsonQueue.Detail()
		if detail == "" {
			detail = textQueue.Detail()
		}
		return []queueMessage{}, core.CommandEnvelope{Command: textQueue.Command, Mocked: true, Detail: detail}
	}
	return parseMailq(textQueue.Stdout), core.CommandEnvelope{Command: textQueue.Command}
}

func loadOrCreateKey(domain, selector string, existing storedDomain) (string, string, error) {
	if existing.Domain == domain && existing.Selector == selector && existing.DKIMPublic != "" {
		if raw, err := os.ReadFile(privateKeyPath(domain, selector)); err == nil {
			return string(raw), existing.DKIMPublic, nil
		}
	}
	return generateDKIM()
}

func privateKeyPath(domain, selector string) string {
	return filepath.Join(etcPath("opendkim", "keys", domain), selector+".private")
}

func tunnelReachable(ctx context.Context, host string) bool {
	dialer := net.Dialer{Timeout: 1500 * time.Millisecond}
	conn, err := dialer.DialContext(ctx, "tcp", net.JoinHostPort(host, "25"))
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func localTunnel(ctx context.Context) bool {
	res := core.Run(ctx, "ip", "-4", "addr", "show")
	if res.Err != nil {
		return false
	}
	for _, field := range strings.Fields(res.Stdout) {
		if !strings.Contains(field, "/") {
			continue
		}
		ip, network, err := net.ParseCIDR(field)
		if err != nil || ip.To4() == nil || network == nil {
			continue
		}
		v4 := ip.To4()
		if v4[0] == 10 && v4[1] == 8 && v4[2] == 0 {
			return true
		}
	}
	return false
}

func publicIPv4(ctx context.Context) string {
	res := core.Run(ctx, "ip", "-4", "route", "get", "1.1.1.1")
	if res.Err != nil {
		return ""
	}
	fields := strings.Fields(res.Stdout)
	for i, field := range fields {
		if field == "src" && i+1 < len(fields) {
			ip := net.ParseIP(fields[i+1])
			if ip == nil || ip.To4() == nil || ip.IsPrivate() || ip.IsLoopback() {
				return ""
			}
			return ip.String()
		}
	}
	return ""
}
