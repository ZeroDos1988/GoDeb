package portforward

import (
	"context"
	"errors"
	"net/http"
	"net/netip"
	"os/exec"
	"strings"

	"godeb/backend/core"
	"godeb/plugins/wireguard"
)

func init() {
	core.Register("portforward", Mount)
}

// Mount registers the port forwarding routes. Changes need a fresh 2FA code
// (criticalPrefixes in backend/auth/stepup.go).
func Mount(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/plugins/portforward/status", handleStatus)
	mux.HandleFunc("POST /api/plugins/portforward/rules", handleAdd)
	mux.HandleFunc("POST /api/plugins/portforward/rules/{name}/delete", handleDelete)
}

type target struct {
	Address string `json:"address"`
	Name    string `json:"name"`
	Site    bool   `json:"site"`
}

type statusResponse struct {
	core.CommandEnvelope
	Rules      []Rule   `json:"rules"`
	PublicIf   string   `json:"publicIf"`
	PublicIP   string   `json:"publicIp"`
	HubAddress string   `json:"hubAddress"`
	Network    string   `json:"network"`
	Targets    []target `json:"targets"`
	Forwarding bool     `json:"forwarding"`
	TableLive  bool     `json:"tableLive"`
	Unit       string   `json:"unit"`
}

var sampleStatus = statusResponse{
	Rules:      []Rule{{Name: "enshrouded", Proto: "udp", From: 15636, To: 15637, Target: "10.8.0.2"}},
	PublicIf:   "eth0",
	PublicIP:   "203.0.113.7",
	HubAddress: "10.8.0.1",
	Network:    "10.8.0.0/24",
	Targets:    []target{{Address: "10.8.0.2", Name: "heim-vm", Site: true}},
}

func status(ctx context.Context) statusResponse {
	resp := statusResponse{}
	resp.Command = []string{"nft", "list", "table", "ip", tableName}
	env, err := discover(ctx)
	rules, rerr := loadRules()
	if err == nil {
		err = rerr
	}
	if _, lookErr := exec.LookPath("nft"); err == nil && lookErr != nil {
		err = errors.New("nft fehlt (Paket nftables)")
	}
	if err != nil {
		resp = sampleStatus
		resp.Command = []string{"nft", "list", "table", "ip", tableName}
		resp.Mocked = true
		resp.Detail = err.Error()
		return resp
	}
	resp.Rules = rules
	resp.PublicIf, resp.PublicIP = env.PublicIf, env.PublicIP
	resp.HubAddress, resp.Network = env.HubAddr.String(), env.Prefix.String()
	resp.Forwarding = core.IPv4Forwarding()
	resp.TableLive = core.RunAdmin(ctx, "nft", "list", "table", "ip", tableName).Err == nil
	resp.Unit = strings.TrimSpace(core.Run(ctx, "systemctl", "is-enabled", unitName).Stdout)
	resp.Targets = targets(env)
	return resp
}

// targets lists the configured peers, sites first, with names from the
// client files.
func targets(env hostEnv) []target {
	names := map[string]target{}
	if clients, err := wireguard.LoadClients(); err == nil {
		for _, c := range clients {
			for _, a := range c.Addresses {
				if p, err := netip.ParsePrefix(strings.TrimSpace(a)); err == nil {
					names[p.Addr().String()] = target{Address: p.Addr().String(), Name: c.Name, Site: c.Site}
				}
			}
		}
	}
	out := []target{}
	for addr := range env.Peers {
		t, ok := names[addr.String()]
		if !ok {
			t = target{Address: addr.String()}
		}
		out = append(out, t)
	}
	sortTargets(out)
	return out
}

func sortTargets(list []target) {
	for i := 1; i < len(list); i++ {
		for j := i; j > 0 && less(list[j], list[j-1]); j-- {
			list[j], list[j-1] = list[j-1], list[j]
		}
	}
}

func less(a, b target) bool {
	if a.Site != b.Site {
		return a.Site
	}
	x, _ := netip.ParseAddr(a.Address)
	y, _ := netip.ParseAddr(b.Address)
	return x.Less(y)
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, status(r.Context()))
}

func handleAdd(w http.ResponseWriter, r *http.Request) {
	var rule Rule
	if !core.DecodeJSON(w, r, &rule) {
		return
	}
	rule.Name = strings.TrimSpace(rule.Name)
	rule.Target = strings.TrimSpace(rule.Target)
	if rule.To == 0 {
		rule.To = rule.From
	}
	change(w, r, func(rules []Rule) ([]Rule, error) {
		for _, existing := range rules {
			if existing.Name == rule.Name {
				return nil, &requestError{msg: "Weiterleitung " + rule.Name + " existiert bereits"}
			}
		}
		return append(rules, rule), nil
	})
}

func handleDelete(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	change(w, r, func(rules []Rule) ([]Rule, error) {
		out := []Rule{}
		found := false
		for _, existing := range rules {
			if existing.Name == name {
				found = true
				continue
			}
			out = append(out, existing)
		}
		if !found {
			return nil, &requestError{msg: "Weiterleitung nicht gefunden"}
		}
		return out, nil
	})
}

// change loads the rules, lets edit modify them and applies the result.
func change(w http.ResponseWriter, r *http.Request, edit func([]Rule) ([]Rule, error)) {
	applyMu.Lock()
	defer applyMu.Unlock()
	rules, err := loadRules()
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	next, err := edit(append([]Rule(nil), rules...))
	if err == nil {
		err = apply(r.Context(), next)
	}
	if err != nil {
		code := http.StatusBadGateway
		var reqErr *requestError
		if errors.As(err, &reqErr) {
			code = http.StatusBadRequest
		}
		core.WriteError(w, code, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, status(r.Context()))
}
