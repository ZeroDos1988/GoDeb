package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"godeb/backend/core"
	"godeb/plugins/wireguard"
)

// runSite handles `godeb site add|sync` on the hub (VPS). A site is a GoDeb
// host behind the hub, typically the home VM. The peer is created through
// wireguard.CreatePeer with Site set, exactly like "Standort" in the web UI.
func runSite(args []string, stdout, stderr io.Writer) int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(stderr, "godeb site muss als root laufen: sudo godeb site …")
		return 1
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	switch {
	case len(args) >= 2 && args[0] == "add":
		return siteAdd(ctx, args[1], args[2:], stdout, stderr)
	case len(args) == 1 && args[0] == "sync":
		if err := wireguard.SyncHubRoutes(ctx); err != nil {
			fmt.Fprintf(stderr, "Weiterleitungen nicht abgeglichen: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, "Weiterleitungen zu den Web-UIs der Standorte sind abgeglichen.")
		n, err := wireguard.SyncClientTrust(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "Trusted Endpoints nicht abgeglichen: %v\n", err)
			return 1
		}
		fmt.Fprintf(stdout, "Trusted Endpoints in %d Client-Datei(en) aktualisiert. Betroffene Geräte: QR-Code neu scannen.\n", n)
		return 0
	default:
		fmt.Fprint(stderr, usage)
		return 2
	}
}

func siteAdd(ctx context.Context, name string, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("site add", flag.ContinueOnError)
	fs.SetOutput(stderr)
	address := fs.String("address", "10.8.0.2", "feste Tunnel-Adresse des Standorts (leer = nächste freie)")
	endpoint := fs.String("endpoint", "", "öffentlicher Host oder IP dieses Hubs (Standard: Adresse der Default-Route)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	role, err := wireguard.CurrentRole()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if role.Role != wireguard.RoleHub {
		fmt.Fprintln(stderr, "Dieser Server ist kein WireGuard-Hub. Zuerst `sudo godeb setup` (ohne --join) ausführen.")
		return 1
	}
	host := *endpoint
	if host == "" {
		host = detectHost()
	}
	if host == "" {
		fmt.Fprintln(stderr, "Öffentliche Adresse nicht erkannt; --endpoint angeben.")
		return 1
	}
	if _, _, err := net.SplitHostPort(host); err != nil {
		host = net.JoinHostPort(host, strconv.Itoa(listenPort()))
	}
	res, err := wireguard.CreatePeer(ctx, wireguard.PeerRequest{Name: name, Endpoint: host, Address: *address, Site: true})
	var reqErr *wireguard.RequestError
	if errors.As(err, &reqErr) {
		fmt.Fprintf(stderr, "%s\n", reqErr.Msg)
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "Standort anlegen: %v\n", err)
		return 1
	}
	addr := res.Peer.AllowedIPs[0]
	fmt.Fprintf(stdout, "Standort %s angelegt: %s, Hub-Endpoint %s\n", name, addr, host)
	if res.Detail != "" {
		fmt.Fprintf(stdout, "Hinweis: %s\n", res.Detail)
	}
	file := "/etc/wireguard/clients/" + name + ".conf"
	fmt.Fprintf(stdout, `
Weiter auf dem Standort (Heim-VM), GoDeb dort installiert:
  1. Auf dem Hub eine Kopie ins eigene Home legen (enthält den privaten
     Schlüssel des Standorts):
       sudo install -m 600 -o "$USER" %s ~/
  2. Auf dem Standort holen, auf dem Hub löschen und beitreten:
       scp <benutzer>@<hub>:%[2]s.conf ~/ && ssh <benutzer>@<hub> rm %[2]s.conf
       sudo godeb setup --join ~/%[2]s.conf && rm ~/%[2]s.conf
  3. Web-UI des Standorts vom Handy über den Hub: http://%[3]s:8080
`, file, name, stripMask(addr))
	return 0
}

// listenPort reads ListenPort from wg0.conf, defaulting to 51820.
func listenPort() int {
	data, err := core.ReadAdminFile(wireguard.ConfigPath())
	if err == nil {
		if p, err := strconv.Atoi(wireguard.ParseConfig(string(data)).Interface.ListenPort); err == nil {
			return p
		}
	}
	return 51820
}

func stripMask(cidr string) string {
	if ip, _, err := net.ParseCIDR(cidr); err == nil {
		return ip.String()
	}
	return cidr
}
