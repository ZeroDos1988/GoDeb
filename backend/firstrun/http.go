package firstrun

import (
	"context"
	"encoding/base64"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strconv"
	"strings"
	"time"

	"godeb/backend/auth"
	"godeb/backend/core"
	"godeb/plugins/wireguard"
)

// TokenHeader carries the one-time setup token.
const TokenHeader = "X-GoDeb-Setup-Token"

// Mount registers the wizard routes. /api/setup/status and /api/setup/admin
// are public (see auth.protected); admin needs the setup token. The other
// routes need the session the wizard just created with password and 2FA.
// Every route except status refuses once the setup is done.
func Mount(mux *http.ServeMux) {
	MountPublic(mux)
	MountPrivileged(mux)
}

// MountPublic registers the routes the web process serves itself: status
// and creating the first account (database only).
func MountPublic(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/setup/status", handleStatus)
	mux.HandleFunc("POST /api/setup/admin", onlyDuringSetup(handleAdmin))
}

// MountPrivileged registers the steps that change the system (WireGuard,
// ufw, trusted endpoint). With a root agent they run there.
func MountPrivileged(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/setup/info", onlyDuringSetup(handleInfo))
	mux.HandleFunc("POST /api/setup/hub", onlyDuringSetup(handleHub))
	mux.HandleFunc("POST /api/setup/client", onlyDuringSetup(handleClient))
	mux.HandleFunc("POST /api/setup/join", onlyDuringSetup(handleJoin))
	mux.HandleFunc("POST /api/setup/finish", onlyDuringSetup(handleFinish))
	// The overview stays available after the setup (login required).
	mux.HandleFunc("GET /api/setup/summary", handleSummary)
}

func onlyDuringSetup(fn http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !Required() {
			core.WriteError(w, http.StatusGone, "Die Einrichtung ist abgeschlossen")
			return
		}
		fn(w, r)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	users, _ := auth.HasUsers()
	core.WriteJSON(w, http.StatusOK, map[string]any{"required": Required(), "hasAdmin": users})
}

type adminRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func handleAdmin(w http.ResponseWriter, r *http.Request) {
	if !CheckToken(r.Header.Get(TokenHeader)) {
		core.WriteError(w, http.StatusForbidden, "Einrichtungs-Token fehlt oder ist falsch (godeb setup-url zeigt ihn)")
		return
	}
	var req adminRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	users, err := auth.HasUsers()
	if err != nil {
		core.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if users {
		core.WriteError(w, http.StatusConflict, "Es gibt bereits einen Administrator; bitte anmelden")
		return
	}
	if err := CreateAdmin(req.Username, req.Password); err != nil {
		core.WriteError(w, http.StatusBadRequest, err.Error())
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "username": req.Username})
}

// CreateAdmin creates the first account; 2FA is set up at its first login.
func CreateAdmin(username, password string) error {
	if !auth.ValidUsername(username) {
		return auth.ErrInvalidUsername
	}
	if err := auth.SetPassword(username, password); err != nil {
		return err
	}
	return markStarted()
}

type infoResponse struct {
	Hostname    string             `json:"hostname"`
	PublicIP    string             `json:"publicIp"`
	LAN         []core.LANAddress  `json:"lan"`
	SSHPorts    []int              `json:"sshPorts"`
	Role        wireguard.RoleInfo `json:"role"`
	UFW         string             `json:"ufw"` // missing, inactive, active
	SetupPort   int                `json:"setupPort"`
	Fingerprint string             `json:"fingerprint"`
}

func handleInfo(w http.ResponseWriter, r *http.Request) {
	host, _ := os.Hostname()
	role, _ := wireguard.CurrentRole()
	resp := infoResponse{
		Hostname: host, PublicIP: core.DefaultRoute(r.Context()).Src, LAN: core.LANAddresses(),
		SSHPorts: core.SSHPorts(r.Context()), Role: role, SetupPort: SetupPort(),
	}
	if resp.SSHPorts == nil {
		resp.SSHPorts = []int{}
	}
	switch st := core.RunAdmin(r.Context(), "ufw", "status"); {
	case st.Err != nil:
		resp.UFW = "missing"
	case strings.Contains(st.Stdout, "Status: active"):
		resp.UFW = "active"
	default:
		resp.UFW = "inactive"
	}
	if fp, err := CertFingerprint(); err == nil {
		resp.Fingerprint = fp
	}
	core.WriteJSON(w, http.StatusOK, resp)
}

// HubSpec creates this host as WireGuard hub and optionally a first client.
type HubSpec struct {
	Address    string `json:"address"`    // e.g. 10.8.0.1/24
	Port       int    `json:"port"`       // UDP
	Endpoint   string `json:"endpoint"`   // public host or IP for the clients
	Client     string `json:"client"`     // first client name, empty for none
	FullTunnel bool   `json:"fullTunnel"` // NAT so clients may send all traffic through this hub
}

// HubResult is the hub plus the first client.
type HubResult struct {
	PublicKey string        `json:"publicKey"`
	Endpoint  string        `json:"endpoint"`
	First     *ClientResult `json:"first,omitempty"`
}

// ClientSpec is one more tunnel client created in the wizard on a hub: a
// device (phone, laptop) or a site, i.e. another GoDeb host such as the
// home VM.
type ClientSpec struct {
	Name       string `json:"name"`
	Site       bool   `json:"site"`
	Address    string `json:"address"`  // empty: next free (sites prefer 10.8.0.2)
	Endpoint   string `json:"endpoint"` // public host or host:port of this hub
	FullTunnel bool   `json:"fullTunnel,omitempty"` // device AllowedIPs 0.0.0.0/0 (hub NAT must be on)
}

// ClientResult carries everything the admin needs to set up the client:
// the file, a QR code for phones and, for sites, the join code to paste on
// the other GoDeb. All of it contains the client's private key.
type ClientResult struct {
	Name    string `json:"name"`
	Site    bool   `json:"site"`
	Address string `json:"address"`
	Config  string `json:"config"`
	QR      string `json:"qr,omitempty"` // data URL
	Code    string `json:"code,omitempty"`
	Detail  string `json:"detail,omitempty"`
}

// CreateHub writes wg0.conf as server (unless it exists), starts the tunnel
// and creates the first client through wireguard.CreatePeer.
func CreateHub(ctx context.Context, spec HubSpec) (HubResult, error) {
	var res HubResult
	if spec.Address == "" {
		spec.Address = "10.8.0.1/24"
	}
	if spec.Port == 0 {
		spec.Port = 51820
	}
	prefix, err := netip.ParsePrefix(strings.TrimSpace(spec.Address))
	if err != nil {
		return res, errors.New("Tunnel-Adresse ist kein Netz wie 10.8.0.1/24")
	}
	configured, err := wireguard.ServerConfigured()
	if err != nil {
		return res, err
	}
	if configured {
		if role, _ := wireguard.CurrentRole(); role.Role != wireguard.RoleHub {
			return res, errors.New("wg0 ist bereits als Peer eingerichtet")
		}
	} else if res.PublicKey, err = wireguard.InitServer(ctx, wireguard.ServerSpec{Address: prefix, ListenPort: spec.Port}); err != nil {
		return res, err
	}
	if err := wireguard.StartTunnel(ctx); err != nil {
		return res, err
	}
	if spec.FullTunnel {
		if err := wireguard.ApplyVolltunnel(ctx, true); err != nil {
			return res, err
		}
	}
	res.Endpoint = hubEndpoint(spec.Endpoint, spec.Port)
	if strings.TrimSpace(spec.Client) == "" {
		return res, nil
	}
	first, err := AddClient(ctx, ClientSpec{Name: spec.Client, Endpoint: res.Endpoint, FullTunnel: spec.FullTunnel})
	if err != nil {
		return res, err
	}
	res.First = &first
	return res, nil
}

// hubEndpoint adds the WireGuard port to a bare host or IP.
func hubEndpoint(host string, port int) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if _, _, err := net.SplitHostPort(host); err == nil {
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}

// AddClient creates one more client on this hub through
// wireguard.CreatePeer, the same function as the WireGuard view and
// `godeb site add`. Devices get the tunnel network only, like the first
// client; sites are created with Site set.
func AddClient(ctx context.Context, spec ClientSpec) (ClientResult, error) {
	var res ClientResult
	role, err := wireguard.CurrentRole()
	if err != nil {
		return res, err
	}
	if role.Role != wireguard.RoleHub {
		return res, errors.New("Weitere Clients gibt es nur auf einem WireGuard-Server")
	}
	endpoint := hubEndpoint(spec.Endpoint, listenPort())
	if endpoint == "" {
		endpoint = wireguard.DefaultEndpoint()
	}
	if endpoint == "" {
		return res, errors.New("Öffentliche Adresse des Servers fehlt")
	}
	req := wireguard.PeerRequest{Name: spec.Name, Endpoint: endpoint, Address: spec.Address, Site: spec.Site}
	if !spec.Site {
		if spec.FullTunnel {
			req.AllowedIPs = "0.0.0.0/0, ::/0"
		} else {
			prefix, err := wireguard.TunnelPrefix()
			if err != nil {
				return res, err
			}
			req.AllowedIPs = prefix.Masked().String()
		}
	}
	peer, err := wireguard.CreatePeer(ctx, req)
	if err != nil {
		return res, err
	}
	res = ClientResult{Name: peer.Peer.Name, Site: spec.Site, Config: peer.ClientConfig, Detail: peer.Detail}
	if len(peer.Peer.AllowedIPs) > 0 {
		res.Address = strings.TrimSuffix(peer.Peer.AllowedIPs[0], "/32")
	}
	if spec.Site {
		res.Code = wireguard.JoinCode(peer.ClientConfig)
	} else if png, err := wireguard.RenderQR(ctx, []byte(peer.ClientConfig)); err == nil {
		res.QR = "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
	}
	return res, nil
}

func handleClient(w http.ResponseWriter, r *http.Request) {
	var spec ClientSpec
	if !core.DecodeJSON(w, r, &spec) {
		return
	}
	res, err := AddClient(r.Context(), spec)
	if err != nil {
		writeErr(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	core.WriteJSON(w, http.StatusOK, res)
}

func handleHub(w http.ResponseWriter, r *http.Request) {
	var spec HubSpec
	if !core.DecodeJSON(w, r, &spec) {
		return
	}
	res, err := CreateHub(r.Context(), spec)
	if err != nil {
		writeErr(w, err)
		return
	}
	core.WriteJSON(w, http.StatusOK, res)
}

type joinRequest struct {
	Config string `json:"config"`
}

func handleJoin(w http.ResponseWriter, r *http.Request) {
	var req joinRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	res, err := wireguard.JoinHub(r.Context(), req.Config)
	if err != nil {
		writeErr(w, err)
		return
	}
	resp := map[string]any{"ok": true, "address": res.Address, "hub": res.Hub, "notes": res.Notes}
	if err := wireguard.StartTunnel(r.Context()); err != nil {
		resp["detail"] = err.Error()
	} else {
		resp["handshake"] = wireguard.WaitHandshake(r.Context(), 10*time.Second)
	}
	core.WriteJSON(w, http.StatusOK, resp)
}

// FinishRequest is the last wizard step.
type FinishRequest struct {
	Access     string `json:"access"`
	LANAddress string `json:"lanAddress"`
	LANPort    int    `json:"lanPort"`
	Trust      bool   `json:"trust"`
	Firewall   bool   `json:"firewall"`
}

// Finish stores the access mode, sets up the trusted endpoint and the
// firewall and marks the setup done. The public setup listener closes.
func Finish(ctx context.Context, req FinishRequest) ([]string, error) {
	access, err := ResolveAccess(req.Access, req.LANAddress, req.LANPort)
	if err != nil {
		return nil, err
	}
	role, _ := wireguard.CurrentRole()
	if access.Mode == AccessTunnel && role.Role == wireguard.RoleNone {
		return nil, errors.New("Ohne WireGuard ist „nur über den Tunnel“ nicht erreichbar; Heimnetz oder nur lokal wählen")
	}
	var log []string
	if req.Trust {
		addr := req.LANAddress
		if addr == "" {
			addr = core.DefaultRoute(ctx).Src
		}
		if _, err := wireguard.SetTrust(ctx, true, addr, 0); err != nil {
			log = append(log, "Trusted Endpoint nicht eingerichtet: "+err.Error())
		} else {
			log = append(log, "Trusted Endpoint aktiv: "+wireguard.LocalTrust().URL)
		}
	}
	plan := FirewallPlan{SSHPorts: core.SSHPorts(ctx), UIPort: 8080, SetupPort: SetupPort(), Enable: req.Firewall}
	if role.Role == wireguard.RoleHub {
		plan.WireGuard = listenPort()
	}
	if role.Role == wireguard.RoleNone {
		plan.UIPort = 0
	}
	if access.Mode == AccessLAN {
		plan.LANIface, plan.LANAddress, plan.LANPrefix, plan.LANPort = access.Iface, access.Address, access.Prefix, access.Port
	}
	fw, err := ApplyFirewall(ctx, plan)
	log = append(log, fw...)
	if err != nil {
		return log, err
	}
	if err := SaveAccess(access); err != nil {
		return log, err
	}
	return log, MarkDone(string(role.Role), access.Mode)
}

func handleFinish(w http.ResponseWriter, r *http.Request) {
	var req FinishRequest
	if !core.DecodeJSON(w, r, &req) {
		return
	}
	log, err := Finish(r.Context(), req)
	if err != nil {
		core.WriteJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error(), "log": log})
		return
	}
	core.WriteJSON(w, http.StatusOK, map[string]any{"ok": true, "log": log, "access": LoadAccess(), "summary": BuildSummary(r.Context())})
}

func listenPort() int {
	data, err := core.ReadAdminFile(wireguard.ConfigPath())
	if err == nil {
		if p, err := strconv.Atoi(wireguard.ParseConfig(string(data)).Interface.ListenPort); err == nil {
			return p
		}
	}
	return 51820
}

func writeErr(w http.ResponseWriter, err error) {
	var reqErr *wireguard.RequestError
	if errors.As(err, &reqErr) {
		core.WriteError(w, reqErr.Status, reqErr.Msg)
		return
	}
	core.WriteError(w, http.StatusBadRequest, err.Error())
}

// SetupAddr is where the public setup listener runs (GODEB_SETUP_ADDR).
func SetupAddr() string {
	if v := strings.TrimSpace(os.Getenv("GODEB_SETUP_ADDR")); v != "" {
		return v
	}
	return "0.0.0.0:8443"
}

// SetupPort is the port of SetupAddr.
func SetupPort() int {
	_, port, err := net.SplitHostPort(SetupAddr())
	if err != nil {
		return 0
	}
	n, _ := strconv.Atoi(port)
	return n
}
