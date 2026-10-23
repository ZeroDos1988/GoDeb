package portforward

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"sync"

	"godeb/backend/core"
	"godeb/plugins/ufw"
	"godeb/plugins/wireguard"
)

// applyMu serialises changes to the rules file, the nft table and ufw.
var applyMu sync.Mutex

// loadRules reads /etc/godeb/portforward.json; a missing file means no rules.
func loadRules() ([]Rule, error) {
	data, err := core.ReadAdminFile(rulesPath)
	if errors.Is(err, fs.ErrNotExist) {
		return []Rule{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f rulesFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s: %w", rulesPath, err)
	}
	if f.Rules == nil {
		f.Rules = []Rule{}
	}
	return f.Rules, nil
}

// discover reads the hub's tunnel network, peers, public interface and the
// ports that must never be forwarded.
func discover(ctx context.Context) (hostEnv, error) {
	env := hostEnv{Peers: map[netip.Addr]bool{}, Reserved: map[string]map[int]string{"tcp": {}, "udp": {}}}
	data, err := core.ReadAdminFile(wireguard.ConfigPath())
	if err != nil {
		return env, fmt.Errorf("%s nicht lesbar: %w", wireguard.ConfigPath(), err)
	}
	if role := wireguard.DetectRole(string(data)); role.Role != wireguard.RoleHub {
		return env, errors.New("Portweiterleitung gibt es nur auf dem Hub; dieser Server ist Peer von " + role.Hub)
	}
	cfg := wireguard.ParseConfig(string(data))
	for _, raw := range cfg.Interface.Addresses {
		if p, err := netip.ParsePrefix(strings.TrimSpace(raw)); err == nil && p.Addr().Is4() && p.Bits() < 31 {
			env.Prefix, env.HubAddr = p.Masked(), p.Addr()
			break
		}
	}
	if !env.HubAddr.IsValid() {
		return env, errors.New("wg0.conf hat keine IPv4-Adresse mit Subnetz")
	}
	for _, peer := range cfg.Peers {
		for _, a := range peer.AllowedIPs {
			if p, err := netip.ParsePrefix(strings.TrimSpace(a)); err == nil && p.Bits() == 32 && env.Prefix.Contains(p.Addr()) {
				env.Peers[p.Addr()] = true
			}
		}
	}
	route := core.DefaultRoute(ctx)
	env.PublicIf, env.PublicIP = route.Dev, route.Src
	for _, p := range core.SSHPorts(ctx) {
		env.Reserved["tcp"][p] = "SSH"
	}
	env.Reserved["tcp"][22] = "SSH"
	listen := 51820
	if p, err := strconv.Atoi(cfg.Interface.ListenPort); err == nil {
		listen = p
	}
	env.Reserved["udp"][listen] = "WireGuard"
	env.Reserved["tcp"][8080] = "die GoDeb-Oberfläche"
	return env, nil
}

// apply validates rules, replaces the nft table, syncs ufw and saves the
// rules. If ufw fails, the previous nft table is restored.
func apply(ctx context.Context, rules []Rule) error {
	env, err := discover(ctx)
	if err != nil {
		return err
	}
	sortRules(rules)
	if err := validate(rules, env); err != nil {
		return &requestError{msg: err.Error()}
	}
	previous, _ := core.ReadAdminFile(nftPath)
	script := renderNft(rules, env)
	if err := core.WriteAdminFile(nftPath+".new", []byte(script), 0o600); err != nil {
		return err
	}
	defer os.Remove(nftPath + ".new")
	if res := core.RunAdmin(ctx, "nft", "-c", "-f", nftPath+".new"); res.Err != nil {
		return fmt.Errorf("nftables lehnt die Regeln ab: %s", res.Detail())
	}
	if len(rules) > 0 {
		if err := core.EnableIPv4Forwarding(ctx); err != nil {
			return fmt.Errorf("IP-Forwarding: %w", err)
		}
	}
	if err := core.WriteAdminFile(nftPath, []byte(script), 0o600); err != nil {
		return err
	}
	if res := core.RunAdmin(ctx, "nft", "-f", nftPath); res.Err != nil {
		restore(ctx, previous)
		return fmt.Errorf("nft -f: %s", res.Detail())
	}
	if err := ufw.SyncRoutes(ctx, routeTag, routeRules(rules, env)); err != nil {
		restore(ctx, previous)
		return fmt.Errorf("ufw: %w", err)
	}
	raw, err := json.MarshalIndent(rulesFile{Rules: rules}, "", "  ")
	if err != nil {
		return err
	}
	if err := core.WriteAdminFile(rulesPath, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	// The unit re-applies the nft file at boot; ufw keeps its own rules.
	if res := core.RunAdmin(ctx, "systemctl", "enable", unitName); res.Err != nil {
		return fmt.Errorf("Regeln aktiv, aber %s nicht aktiviert: %s", unitName, res.Detail())
	}
	return nil
}

func restore(ctx context.Context, previous []byte) {
	if previous == nil {
		previous = []byte(renderNft(nil, hostEnv{}))
	}
	if core.WriteAdminFile(nftPath, previous, 0o600) == nil {
		core.RunAdmin(ctx, "nft", "-f", nftPath)
	}
}

type requestError struct{ msg string }

func (e *requestError) Error() string { return e.msg }
