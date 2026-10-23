package fail2ban

import (
	"context"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"godeb/backend/core"
)

func init() {
	core.Register("fail2ban", Mount)
}

var jailPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

// Mount registers the fail2ban routes.
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins/fail2ban/status", handleStatus)
	mux.HandleFunc("POST /api/plugins/fail2ban/unban", handleUnban)
}

type statusResponse struct {
	core.CommandEnvelope
	Jails []Jail `json:"jails"`
}

type unbanRequest struct {
	IP string `json:"ip"`
}

// readAdmin runs a fail2ban-client query as root and falls back to the sample.
func readAdmin(ctx context.Context, sample string, args ...string) (string, core.CommandEnvelope) {
	res := core.RunAdmin(ctx, "fail2ban-client", args...)
	env := core.CommandEnvelope{Command: append([]string{"fail2ban-client"}, args...)}
	if res.Err != nil {
		env.Mocked = true
		env.Detail = res.Detail()
		return sample, env
	}
	return res.Stdout, env
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	out, env := readAdmin(r.Context(), sampleStatus, "status")
	names := ParseJailList(out)
	jails := make([]Jail, 0, len(names))
	for _, name := range names {
		var jailOut string
		if env.Mocked {
			jailOut = sampleJails[name]
		} else {
			var jailEnv core.CommandEnvelope
			jailOut, jailEnv = readAdmin(r.Context(), "", "status", name)
			if jailEnv.Mocked {
				env.Detail = strings.TrimSpace(env.Detail + " " + name + ": " + jailEnv.Detail)
			}
		}
		jails = append(jails, ParseJail(name, jailOut))
	}
	sort.SliceStable(jails, func(i, j int) bool {
		if (jails[i].Name == "godeb") != (jails[j].Name == "godeb") {
			return jails[i].Name == "godeb"
		}
		return jails[i].Name < jails[j].Name
	})
	core.WriteJSON(w, http.StatusOK, statusResponse{CommandEnvelope: env, Jails: jails})
}

func handleUnban(w http.ResponseWriter, r *http.Request) {
	var req unbanRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	ip := net.ParseIP(strings.TrimSpace(req.IP))
	if ip == nil {
		core.WriteError(w, http.StatusBadRequest, "Keine gültige IP-Adresse")
		return
	}
	res := core.RunAdmin(r.Context(), "fail2ban-client", "unban", ip.String())
	if res.Err != nil {
		core.WriteError(w, http.StatusBadGateway, res.Detail())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"command": res.Command,
		"output":  strings.TrimSpace(res.Stdout),
	})
}

func validJail(name string) bool {
	return jailPattern.MatchString(name)
}
