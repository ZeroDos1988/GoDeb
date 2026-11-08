package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"godeb/backend/core"
	"godeb/plugins/sites"
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
	address := fs.String("address", "", "feste Tunnel-Adresse des Standorts (leer = 10.8.0.2, sonst die nächste freie)")
	endpoint := fs.String("endpoint", "", "öffentlicher Host oder IP dieses Hubs (Standard: Adresse der Default-Route)")
	profile := fs.String("profile", "", "home (Heim-Server: Mail und schwere Dienste) oder play (GoDeb Play): erzeugt ein Einrichtungsskript")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *profile != "" {
		if *address != "" {
			fmt.Fprintln(stderr, "--address und --profile zusammen gehen nicht; die Adresse vergibt der Hub")
			return 2
		}
		host := *endpoint
		if host == "" {
			host = detectHost()
		}
		return siteAddProfile(ctx, name, *profile, host, stdout, stderr)
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
	fmt.Fprintf(stdout, `
Weiter auf dem Standort (Heim-VM), GoDeb dort installiert. Der Beitrittscode
enthält den privaten Schlüssel des Standorts, nur dort einfügen:

%s

  Im Browser: Assistent des Standorts → „Mit Hub verbinden“ → Code einfügen.
  Im Terminal: sudo godeb setup --join -   (Code einfügen, Enter)
  Datei statt Code: /etc/wireguard/clients/%s.conf
  Web-UI des Standorts vom Handy über den Hub: http://%s:8080
`, wireguard.JoinCode(res.ClientConfig), name, stripMask(addr))
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

// siteAddProfile creates a site with a profile through sites.Add, the same
// function as the web UI, and writes the setup script into the current
// directory. It belongs to the account that called sudo, so it can be
// copied with scp right away; it carries the site's private key (0600).
func siteAddProfile(ctx context.Context, name, profile, endpoint string, stdout, stderr io.Writer) int {
	res, err := sites.Add(ctx, sites.AddRequest{Name: name, Profile: profile, Endpoint: endpoint})
	var reqErr *wireguard.RequestError
	if errors.As(err, &reqErr) {
		fmt.Fprintln(stderr, reqErr.Msg)
		return 1
	}
	if err != nil {
		fmt.Fprintf(stderr, "Standort anlegen: %v\n", err)
		return 1
	}
	path, err := filepath.Abs(res.ScriptName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if err := os.WriteFile(path, []byte(res.Script), 0o600); err != nil {
		fmt.Fprintf(stderr, "Skript nicht geschrieben: %v\n", err)
		return 1
	}
	if uid, gid, ok := sudoOwner(); ok {
		_ = os.Chown(path, uid, gid)
	}
	fmt.Fprintf(stdout, "Standort %s angelegt: %s (Profil %s)\n", res.Site.Name, res.Site.Address, profile)
	for _, n := range res.Notes {
		fmt.Fprintf(stdout, "  %s\n", n)
	}
	fmt.Fprintf(stdout, `
Einrichtungsskript: %s
Es enthält den privaten WireGuard-Schlüssel des Standorts. Auf den neuen
Rechner (Debian 13) kopieren und dort ausführen; danach löscht es sich:

  scp %s <konto>@<neuer-rechner>:
  ssh <konto>@<neuer-rechner> sudo sh %s

Web-UI des Standorts danach über den Tunnel: %s
`, path, path, res.ScriptName, res.Site.WebUI)
	return 0
}

// sudoOwner is the account that called sudo (SUDO_UID/SUDO_GID).
func sudoOwner() (int, int, bool) {
	uid, err1 := strconv.Atoi(os.Getenv("SUDO_UID"))
	gid, err2 := strconv.Atoi(os.Getenv("SUDO_GID"))
	if err1 != nil || err2 != nil || uid <= 0 {
		return 0, 0, false
	}
	return uid, gid, true
}
