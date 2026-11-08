package wireguard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"sync"

	"godeb/backend/core"
	"godeb/plugins/ufw"
)

// The home gateway lets devices in the home network reach the tunnel
// addresses without a WireGuard client of their own (a phone at home whose
// app switched the tunnel off). The router gets one static route,
// <tunnel network> → <LAN address of this site>; this site forwards from the
// LAN to wg0 and rewrites the source to its own tunnel address, so the hub
// answers without a route back. Every internal name points to a tunnel
// address, at home and on the road: no split DNS, no second proxy.

const (
	gatewayStatePath = "/etc/godeb/gateway.json"
	gatewayNftPath   = "/etc/godeb/gateway.nft"
	gatewayUnit      = "godeb-gateway.service"
	gatewayTable     = "godeb_gateway"
	gatewayRouteTag  = "GoDeb-GW"
)

var gatewayMu sync.Mutex

type gatewayState struct {
	Enabled bool   `json:"enabled"`
	LANIf   string `json:"lanIf,omitempty"`
	LAN     string `json:"lan,omitempty"` // LAN prefix, e.g. 192.168.178.0/24
	Address string `json:"address,omitempty"`
}

// GatewayInfo is the state shown in the UI, with the route for the router.
type GatewayInfo struct {
	core.CommandEnvelope
	Available  bool   `json:"available"` // only on a peer with a LAN address
	Enabled    bool   `json:"enabled"`
	LANIf      string `json:"lanIf,omitempty"`
	LAN        string `json:"lan,omitempty"`
	Address    string `json:"address,omitempty"`
	Tunnel     string `json:"tunnel,omitempty"`
	Forwarding bool   `json:"forwarding"`
	TableLive  bool   `json:"tableLive"`
}

// Hooks for tests.
var (
	lanAddresses = core.LANAddresses
	defaultRoute = core.DefaultRoute
)

func loadGatewayState() gatewayState {
	var s gatewayState
	if data, err := core.ReadAdminFile(gatewayStatePath); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

// gatewayPlan finds the LAN of the default route and the tunnel network
// from wg0.conf of a peer.
func gatewayPlan(ctx context.Context, content string) (lan core.LANAddress, tunnel netip.Prefix, err error) {
	role := DetectRole(content)
	if role.Role != RolePeer {
		return lan, tunnel, errors.New("Das Heimnetz-Gateway gibt es nur auf einem Standort (Peer eines Hubs)")
	}
	own, err := netip.ParseAddr(role.Address)
	if err != nil {
		return lan, tunnel, errors.New("Tunnel-Adresse dieses Standorts nicht gefunden")
	}
	for _, p := range ParseConfig(content).Peers {
		for _, a := range p.AllowedIPs {
			if pre, err := netip.ParsePrefix(strings.TrimSpace(a)); err == nil && pre.Addr().Is4() && pre.Contains(own) && pre.Bits() < 32 {
				tunnel = pre.Masked()
			}
		}
	}
	if !tunnel.IsValid() {
		return lan, tunnel, errors.New("Tunnelnetz in wg0.conf nicht gefunden")
	}
	dev := defaultRoute(ctx).Dev
	for _, l := range lanAddresses() {
		if l.Iface == dev {
			lan = l
			break
		}
	}
	if lan.Iface == "" || !volltunnelIface.MatchString(lan.Iface) {
		return lan, tunnel, fmt.Errorf("Heimnetz-Schnittstelle nicht erkannt (Default-Route über %q)", dev)
	}
	lp, err := netip.ParsePrefix(lan.Prefix)
	if err != nil || lp.Overlaps(tunnel) {
		return lan, tunnel, fmt.Errorf("Heimnetz %s und Tunnelnetz %s überschneiden sich", lan.Prefix, tunnel)
	}
	return lan, tunnel, nil
}

// GatewayStatus reads the gateway state of this host.
func GatewayStatus(ctx context.Context) GatewayInfo {
	st := loadGatewayState()
	info := GatewayInfo{Enabled: st.Enabled, LANIf: st.LANIf, LAN: st.LAN, Address: st.Address, Forwarding: core.IPv4Forwarding()}
	info.Command = []string{"nft", "list", "table", "ip", gatewayTable}
	data, err := core.ReadAdminFile(configPath)
	if err != nil {
		info.Mocked = true
		info.Detail = err.Error()
		return info
	}
	lan, tunnel, err := gatewayPlan(ctx, string(data))
	if err != nil {
		info.Detail = err.Error()
		return info
	}
	info.Available, info.Tunnel = true, tunnel.String()
	if !st.Enabled {
		info.LANIf, info.LAN, info.Address = lan.Iface, lan.Prefix, lan.Address
	}
	info.TableLive = core.RunAdminQuiet(ctx, "nft", "list", "table", "ip", gatewayTable).Err == nil
	return info
}

// ApplyGateway switches the home gateway on or off: IP forwarding, an
// nftables table that masquerades LAN traffic into wg0 (only from the LAN
// prefix), a ufw route allow from the LAN interface to wg0, and the unit
// that restores the table at boot.
func ApplyGateway(ctx context.Context, enabled bool) (GatewayInfo, error) {
	gatewayMu.Lock()
	defer gatewayMu.Unlock()
	data, err := core.ReadAdminFile(configPath)
	if err != nil {
		return GatewayInfo{}, fmt.Errorf("%s nicht lesbar: %w", configPath, err)
	}
	lan, tunnel, err := gatewayPlan(ctx, string(data))
	if err != nil && enabled {
		return GatewayInfo{}, err
	}
	st := gatewayState{Enabled: enabled}
	if enabled {
		st.LANIf, st.LAN, st.Address = lan.Iface, lan.Prefix, lan.Address
	}
	previous, _ := core.ReadAdminFile(gatewayNftPath)
	script := renderGatewayNft(st, tunnel)
	if err := core.WriteAdminFile(gatewayNftPath+".new", []byte(script), 0o600); err != nil {
		return GatewayInfo{}, err
	}
	defer os.Remove(gatewayNftPath + ".new")
	if res := core.RunAdmin(ctx, "nft", "-c", "-f", gatewayNftPath+".new"); res.Err != nil {
		return GatewayInfo{}, fmt.Errorf("nftables lehnt die Regeln ab: %s", res.Detail())
	}
	if enabled {
		if err := core.EnableIPv4Forwarding(ctx); err != nil {
			return GatewayInfo{}, fmt.Errorf("IP-Forwarding: %w", err)
		}
	}
	if err := core.WriteAdminFile(gatewayNftPath, []byte(script), 0o600); err != nil {
		return GatewayInfo{}, err
	}
	if res := core.RunAdmin(ctx, "nft", "-f", gatewayNftPath); res.Err != nil {
		restoreNft(ctx, gatewayNftPath, previous, renderGatewayNft(gatewayState{}, tunnel))
		return GatewayInfo{}, fmt.Errorf("nft -f: %s", res.Detail())
	}
	want := []ufw.RouteRule{}
	if enabled {
		want = append(want, ufw.RouteRule{In: st.LANIf, Out: iface, Comment: gatewayRouteTag + " heimnetz"})
	}
	if err := ufw.SyncRoutes(ctx, gatewayRouteTag, want); err != nil {
		restoreNft(ctx, gatewayNftPath, previous, renderGatewayNft(gatewayState{}, tunnel))
		return GatewayInfo{}, fmt.Errorf("ufw: %w", err)
	}
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return GatewayInfo{}, err
	}
	if err := core.WriteAdminFile(gatewayStatePath, append(raw, '\n'), 0o600); err != nil {
		return GatewayInfo{}, err
	}
	if enabled {
		if res := core.RunAdmin(ctx, "systemctl", "enable", gatewayUnit); res.Err != nil {
			return GatewayInfo{}, fmt.Errorf("Gateway aktiv, aber %s nicht aktiviert: %s", gatewayUnit, res.Detail())
		}
	} else {
		core.RunAdmin(ctx, "systemctl", "disable", gatewayUnit)
	}
	return GatewayStatus(ctx), nil
}

func restoreNft(ctx context.Context, path string, previous []byte, empty string) {
	if previous == nil {
		previous = []byte(empty)
	}
	if core.WriteAdminFile(path, previous, 0o600) == nil {
		core.RunAdmin(ctx, "nft", "-f", path)
	}
}

// renderGatewayNft replaces the gateway table atomically. Only packets from
// the LAN prefix that enter on the LAN interface and leave through wg0 to
// the tunnel network are rewritten.
func renderGatewayNft(st gatewayState, tunnel netip.Prefix) string {
	var b strings.Builder
	b.WriteString("# Managed by GoDeb (plugins/wireguard gateway). Changes are overwritten.\n")
	fmt.Fprintf(&b, "table ip %s\ndelete table ip %s\n", gatewayTable, gatewayTable)
	if !st.Enabled {
		return b.String()
	}
	fmt.Fprintf(&b, "table ip %s {\n", gatewayTable)
	b.WriteString("\tchain postrouting {\n\t\ttype nat hook postrouting priority srcnat; policy accept;\n")
	fmt.Fprintf(&b, "\t\tiifname %q oifname %q ip saddr %s ip daddr %s masquerade\n", st.LANIf, iface, st.LAN, tunnel)
	b.WriteString("\t}\n}\n")
	return b.String()
}

type gatewayRequest struct {
	Enabled bool `json:"enabled"`
}

func handleGatewayGet(w http.ResponseWriter, r *http.Request) {
	core.WriteJSON(w, http.StatusOK, GatewayStatus(r.Context()))
}

func handleGatewaySet(w http.ResponseWriter, r *http.Request) {
	var req gatewayRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	info, err := ApplyGateway(r.Context(), req.Enabled)
	if err != nil {
		core.WriteError(w, http.StatusBadGateway, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, info)
}
