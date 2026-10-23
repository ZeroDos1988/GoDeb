package auth

import (
	"errors"
	"log/slog"
	"net/http"
	"path"
	"strings"
	"time"

	"godeb/backend/core"
	"godeb/backend/logging"
)

// StepUpHeader carries a TOTP or backup code that confirms one request.
const StepUpHeader = "X-GoDeb-TOTP"

// elevationTTL is how long a confirmed code covers ordinary admin actions.
const elevationTTL = 5 * time.Minute

type stepUpLevel int

const (
	// stepUpNone: reading, or a change that cannot affect the host.
	stepUpNone stepUpLevel = iota
	// stepUpWindow: a code confirmed within elevationTTL is enough.
	stepUpWindow
	// stepUpFresh: every request needs its own code. Used where one request
	// can open access to the host or install software.
	stepUpFresh
)

// criticalPrefixes always need a fresh code.
var criticalPrefixes = []string{
	"/api/plugin-manager/",      // install, enable and disable plugins (APT, systemd)
	"/api/auth/oidc/providers/", // who may sign in
	"/api/plugins/ufw/",         // firewall rules
	"/api/plugins/systemd/",     // stop/restart any allowed unit, sshd and wg-quick included
	"/api/plugins/portforward/", // exposes ports of a tunnel peer to the internet
	"/api/plugins/mailgateway/setup",
}

// criticalGameActions always need a fresh code below /api/plugins/steam/games/
// {game}/: they install packages, create accounts and open firewall ports.
var criticalGameActions = map[string]bool{"install": true, "update": true}

// criticalExact always needs a fresh code.
var criticalExact = map[string]bool{
	"/api/plugins/wireguard/peers": true, // a new peer is a new way into the network
	"/api/plugins/wireguard/join":  true, // replaces how this host reaches the tunnel
	"/api/plugins/wireguard/trust": true, // decides where phones switch their tunnel off
}

// noStepUp lists state-changing routes that skip the second factor.
var noStepUp = map[string]bool{
	"/api/user/settings":       true, // theme and language of the own account only
	"/api/plugin-manager/scan": true, // re-reads plugin.json files, changes nothing
}

// stepUpFor decides how fresh the second factor must be. Every state-changing
// /api request needs at least the window unless it is listed in noStepUp, so
// new plugin routes are covered without opting in. Reads are free except for
// downloads that contain private keys.
func stepUpFor(method, raw string) stepUpLevel {
	p := path.Clean("/" + raw)
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		if strings.HasPrefix(p, "/api/plugins/wireguard/peers/") && (strings.HasSuffix(p, "/config") || strings.HasSuffix(p, "/qr")) {
			return stepUpWindow
		}
		return stepUpNone
	}
	if noStepUp[p] {
		return stepUpNone
	}
	if criticalExact[p] {
		return stepUpFresh
	}
	if rest, ok := strings.CutPrefix(p, "/api/plugins/steam/games/"); ok {
		if _, action, ok := strings.Cut(rest, "/"); ok && criticalGameActions[action] {
			return stepUpFresh
		}
	}
	for _, prefix := range criticalPrefixes {
		if p == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(p, prefix) {
			return stepUpFresh
		}
	}
	return stepUpWindow
}

type stepUpResponse struct {
	Error          string `json:"error"`
	StepUpRequired bool   `json:"step_up_required"`
	Critical       bool   `json:"critical"`
	Remaining      int    `json:"remaining,omitempty"`
}

// checkStepUp enforces the level for s. It writes the response and returns
// false when the request must not proceed. Wrong codes count against the
// session; the last allowed failure ends it.
func checkStepUp(w http.ResponseWriter, r *http.Request, s session, level stepUpLevel) bool {
	if level == stepUpNone {
		return true
	}
	code := strings.TrimSpace(r.Header.Get(StepUpHeader))
	critical := level == stepUpFresh
	if code == "" {
		if !critical && now().Before(s.ElevatedUntil) {
			return true
		}
		noStore(w)
		logging.Annotate(r.Context(), slog.String("2fa", "verlangt"))
		logging.Expected(r.Context())
		core.WriteJSON(w, http.StatusForbidden, stepUpResponse{
			Error:          "Bestätigung mit 2FA-Code erforderlich",
			StepUpRequired: true,
			Critical:       critical,
		})
		return false
	}
	noStore(w)
	ok, err := secondFactor.Validate(s.Username, code, s.ID)
	if errors.Is(err, ErrCodesLocked) {
		logging.Annotate(r.Context(), slog.String("2fa", "gesperrt"))
		core.WriteError(w, http.StatusTooManyRequests, err.Error())
		return false
	}
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, "2FA-Daten nicht lesbar")
		return false
	}
	if !ok {
		logging.Annotate(r.Context(), slog.String("2fa", "falsch"))
		onFailure(r, s.Username)
		left, err := recordFailure(s.ID)
		if errors.Is(err, errTooManyAttempts) {
			clearCookie(w, r, SessionCookie, "/")
			core.WriteError(w, http.StatusUnauthorized, "Zu viele falsche Codes. Die Sitzung wurde beendet.")
			return false
		}
		if err != nil {
			core.WriteError(w, http.StatusInternalServerError, "Sitzung nicht lesbar")
			return false
		}
		core.WriteJSON(w, http.StatusForbidden, stepUpResponse{
			Error:          "Code ungültig oder bereits verwendet",
			StepUpRequired: true,
			Critical:       critical,
			Remaining:      left,
		})
		return false
	}
	logging.Annotate(r.Context(), slog.String("2fa", "bestätigt"))
	if err := elevate(s.ID, now().Add(elevationTTL)); err != nil {
		core.WriteError(w, http.StatusInternalServerError, "Sitzung nicht gespeichert")
		return false
	}
	return true
}
