package steam

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"godeb/backend/core"
	"godeb/backend/pluginmgr"
	"godeb/plugins/wireguard"
)

const steamcmdURL = "https://steamcdn-a.akamaihd.net/client/installer/steamcmd_linux.tar.gz"

// renderUnit is the systemd unit of a game server. It runs as the game's own
// account and may only write below the game's base directory.
func renderUnit(g *Game) string {
	base := g.baseDir()
	lines := []string{
		"# Managed by GoDeb (Steam game servers). Changes are overwritten on update.",
		"[Unit]",
		"Description=" + g.Name + " dedicated server (GoDeb)",
		"After=network-online.target",
		"Wants=network-online.target",
		"",
		"[Service]",
		"Type=simple",
		"User=" + g.user(),
		"Group=" + g.user(),
		"WorkingDirectory=" + g.serverDir(),
		"Environment=HOME=" + base,
	}
	exec := g.executable()
	if g.Platform == Windows {
		lines = append(lines,
			"Environment=WINEPREFIX="+g.winePrefix(),
			"Environment=WINEDEBUG=-all",
			"Environment=WINEDLLOVERRIDES=mscoree,mshtml=",
		)
		exec = "/usr/bin/wine " + exec
	}
	if len(g.Args) > 0 {
		exec += " " + strings.Join(g.Args, " ")
	}
	lines = append(lines,
		"ExecStart="+exec,
		"Restart=on-failure",
		"RestartSec=10",
		"TimeoutStopSec=60",
		"KillSignal="+g.StopSignal,
		"",
		"NoNewPrivileges=yes",
		"ProtectSystem=strict",
		"ReadWritePaths="+base,
		"ProtectHome=yes",
		"PrivateTmp=yes",
		"PrivateDevices=yes",
		"ProtectKernelTunables=yes",
		"ProtectKernelModules=yes",
		"ProtectKernelLogs=yes",
		"ProtectControlGroups=yes",
		"ProtectClock=yes",
		"ProtectHostname=yes",
		"RestrictSUIDSGID=yes",
		"RestrictRealtime=yes",
		"RestrictNamespaces=yes",
		"RestrictAddressFamilies=AF_UNIX AF_INET AF_INET6",
		"CapabilityBoundingSet=",
		"",
		"[Install]",
		"WantedBy=multi-user.target",
		"",
	)
	return strings.Join(lines, "\n")
}

func requireRoot() error {
	if os.Geteuid() != 0 {
		return errors.New("Installation braucht root; der GoDeb-Dienst läuft normalerweise als root")
	}
	return nil
}

// requireAmd64 stops early on other architectures: SteamCMD and the Wine
// servers it installs only exist for x86, and apt would otherwise fail with
// a missing lib32gcc-s1.
func requireAmd64() error {
	if runtime.GOARCH != "amd64" {
		return fmt.Errorf("Spielserver über SteamCMD laufen nur auf amd64, dieser Server ist %s", runtime.GOARCH)
	}
	return nil
}

// install sets up a game with one click: APT packages, the service account,
// SteamCMD, the server files, a Wine prefix for Windows servers, a default
// configuration, the systemd unit and the firewall.
func install(ctx context.Context, g *Game, j *jobHandle) error {
	if err := requireRoot(); err != nil {
		return err
	}
	if err := requireAmd64(); err != nil {
		return err
	}
	j.step("Systempakete")
	if err := ensurePackages(ctx, g, j); err != nil {
		return err
	}
	j.step("Dienstkonto " + g.user())
	uid, gid, err := ensureUser(ctx, g, j)
	if err != nil {
		return err
	}
	dirs := []string{rootDir(), g.baseDir(), g.serverDir(), g.steamcmdDir()}
	if g.Platform == Windows {
		dirs = append(dirs, g.winePrefix())
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
		if dir == rootDir() {
			// Shared parent: root-owned, every game account may only enter.
			if err := os.Chmod(dir, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.Chown(dir, uid, gid); err != nil {
			return err
		}
	}
	j.step("SteamCMD")
	if err := ensureSteamCMD(ctx, g, j, uid, gid); err != nil {
		return err
	}
	j.step("Serverdateien (App " + g.AppID + ")")
	if err := adoptServerFiles(g, j, uid, gid); err != nil {
		return err
	}
	if err := steamUpdate(ctx, g, j); err != nil {
		return err
	}
	if g.Platform == Windows {
		j.step("Wine-Umgebung")
		if err := j.run(ctx, 10*time.Minute, nil, "runuser", "-u", g.user(), "--",
			"/usr/bin/env", "HOME="+g.baseDir(), "WINEPREFIX="+g.winePrefix(), "WINEDEBUG=-all",
			"WINEDLLOVERRIDES=mscoree,mshtml=", "wineboot", "--init"); err != nil {
			j.logf("wineboot meldet einen Fehler; der Server legt die Umgebung beim Start notfalls selbst an.")
		}
	}
	var cfg map[string]any
	if g.ConfigFile != "" {
		j.step("Konfiguration")
		if cfg, err = ensureConfig(g, uid, gid); err != nil {
			return err
		}
	}
	j.step("systemd-Dienst " + g.unit())
	if err := writeUnit(ctx, g); err != nil {
		return err
	}
	j.step("Firewall")
	openFirewall(ctx, g, j, g.Ports(cfg))
	j.step("Server starten")
	if res := core.RunAdmin(ctx, "systemctl", "enable", "--now", g.unit()); res.Err != nil {
		return fmt.Errorf("systemctl enable --now: %s", res.Detail())
	}
	return nil
}

// update pulls the newest server build and restarts the server if it ran.
func update(ctx context.Context, g *Game, j *jobHandle) error {
	if err := requireRoot(); err != nil {
		return err
	}
	wasActive := serviceState(ctx, g) == "active"
	if wasActive {
		j.step("Server stoppen")
		if res := core.RunAdmin(ctx, "systemctl", "stop", g.unit()); res.Err != nil {
			return fmt.Errorf("systemctl stop: %s", res.Detail())
		}
	}
	j.step("Serverdateien aktualisieren")
	uid, gid, err := ensureUser(ctx, g, j)
	if err != nil {
		return err
	}
	if err := adoptServerFiles(g, j, uid, gid); err != nil {
		return err
	}
	if err := steamUpdate(ctx, g, j); err != nil {
		return err
	}
	if err := writeUnit(ctx, g); err != nil {
		return err
	}
	if wasActive {
		j.step("Server starten")
		if res := core.RunAdmin(ctx, "systemctl", "start", g.unit()); res.Err != nil {
			return fmt.Errorf("systemctl start: %s", res.Detail())
		}
	}
	return nil
}

// ensurePackages installs the plugin's packages from plugin.json plus the
// game's own (e.g. wine) when they are missing.
func ensurePackages(ctx context.Context, g *Game, j *jobHandle) error {
	root, err := core.RepoRoot()
	if err != nil {
		return err
	}
	desc, err := pluginmgr.Load(filepath.Join(root, "plugins"), "steam")
	if err != nil {
		return fmt.Errorf("plugin.json: %w", err)
	}
	wanted := append(append([]string{}, desc.AptDependencies...), g.Packages...)
	missing := []string{}
	for _, pkg := range wanted {
		res := core.Run(ctx, "dpkg-query", "-W", "-f", "${db:Status-Status}", pkg)
		if strings.TrimSpace(res.Stdout) != "installed" {
			missing = append(missing, pkg)
		}
	}
	if len(missing) == 0 {
		j.logf("Alle Pakete vorhanden: %s", strings.Join(wanted, ", "))
		return nil
	}
	if err := j.run(ctx, 10*time.Minute, nil, "apt-get", "update"); err != nil {
		return err
	}
	args := append([]string{"install", "-y", "--no-install-recommends"}, missing...)
	return j.run(ctx, 30*time.Minute, nil, "apt-get", args...)
}

func ensureUser(ctx context.Context, g *Game, j *jobHandle) (int, int, error) {
	if _, err := user.Lookup(g.user()); err != nil {
		if err := j.run(ctx, time.Minute, nil, "useradd", "--system", "--user-group",
			"--home-dir", g.baseDir(), "--no-create-home", "--shell", "/usr/sbin/nologin", g.user()); err != nil {
			return 0, 0, err
		}
	}
	return lookupUser(g)
}

func lookupUser(g *Game) (int, int, error) {
	u, err := user.Lookup(g.user())
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(u.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(u.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}

func ensureSteamCMD(ctx context.Context, g *Game, j *jobHandle, uid, gid int) error {
	script := filepath.Join(g.steamcmdDir(), "steamcmd.sh")
	if _, err := os.Stat(script); err == nil {
		j.logf("SteamCMD vorhanden")
		return nil
	}
	j.logf("Lade %s", steamcmdURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, steamcmdURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("SteamCMD herunterladen: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SteamCMD herunterladen: HTTP %d", resp.StatusCode)
	}
	if err := extractTarGz(io.LimitReader(resp.Body, 64<<20), g.steamcmdDir(), uid, gid); err != nil {
		return fmt.Errorf("SteamCMD entpacken: %w", err)
	}
	if _, err := os.Stat(script); err != nil {
		return errors.New("steamcmd.sh fehlt im Archiv")
	}
	return nil
}

// extractTarGz unpacks regular files and directories below dest and rejects
// entries that would escape it.
func extractTarGz(r io.Reader, dest string, uid, gid int) error {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		target := filepath.Join(dest, hdr.Name)
		if target != dest && !strings.HasPrefix(target, dest+string(os.PathSeparator)) {
			return fmt.Errorf("Pfad %q verlässt das Zielverzeichnis", hdr.Name)
		}
		mode := os.FileMode(hdr.Mode).Perm() &^ 0o022
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, mode|0o700); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 256<<20)); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default:
			continue
		}
		if os.Geteuid() == 0 {
			if err := os.Lchown(target, uid, gid); err != nil {
				return err
			}
		}
	}
}

// adoptServerFiles hands server files that were copied in by hand (for
// example from a download cache) to the game's account. SteamCMD then only
// validates them and fetches what is missing or changed.
func adoptServerFiles(g *Game, j *jobHandle, uid, gid int) error {
	if !g.installed() {
		return nil
	}
	if id := buildID(g); id != "" {
		j.logf("Serverdateien liegen bereits vor (Build %s). SteamCMD prüft sie nur und lädt Fehlendes nach.", id)
	} else {
		j.logf("Serverdateien liegen bereits vor, aber ohne steamapps/appmanifest_%s.acf. SteamCMD prüft alle Dateien.", g.AppID)
	}
	if err := chownTree(g.serverDir(), uid, gid); err != nil {
		return fmt.Errorf("Serverdateien an %s übergeben: %w", g.user(), err)
	}
	return nil
}

// chownTree sets the owner of root and everything below it. Symlinks are
// changed themselves and never followed.
func chownTree(root string, uid, gid int) error {
	if os.Geteuid() != 0 {
		return nil
	}
	return filepath.WalkDir(root, func(p string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, gid)
	})
}

// steamAttempts bounds SteamCMD runs per install or update. A freshly
// bootstrapped SteamCMD often fails the first app_update with "Missing
// configuration" and succeeds on the next run.
const steamAttempts = 3

// steamArgs is the SteamCMD command line for a game.
func steamArgs(g *Game) []string {
	args := []string{filepath.Join(g.steamcmdDir(), "steamcmd.sh")}
	if g.Platform == Windows {
		args = append(args, "+@sSteamCmdForcePlatformType", "windows")
	}
	return append(args,
		"+force_install_dir", g.serverDir(),
		"+login", "anonymous",
		"+app_update", g.AppID, "validate",
		"+quit")
}

func steamUpdate(ctx context.Context, g *Game, j *jobHandle) error {
	args := append([]string{"-u", g.user(), "--", "/usr/bin/env", "HOME=" + g.baseDir()}, steamArgs(g)...)
	var err error
	for attempt := 1; attempt <= steamAttempts; attempt++ {
		err = j.run(ctx, 90*time.Minute, nil, "runuser", args...)
		if err == nil || ctx.Err() != nil {
			return err
		}
		if attempt < steamAttempts {
			j.logf("SteamCMD-Versuch %d von %d fehlgeschlagen (%v), neuer Versuch", attempt, steamAttempts, err)
		}
	}
	return err
}

func ensureConfig(g *Game, uid, gid int) (map[string]any, error) {
	raw, err := os.ReadFile(g.configPath())
	if errors.Is(err, os.ErrNotExist) {
		raw, err = g.DefaultConfig()
		if err != nil {
			return nil, err
		}
		if err := writeConfig(g, raw, uid, gid); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return g.ValidateConfig(raw)
}

// writeConfig replaces the config atomically. The game account owns it so
// the server can rewrite it; other users cannot read passwords in it.
func writeConfig(g *Game, raw []byte, uid, gid int) error {
	dir := g.serverDir()
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "."+g.ConfigFile+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if os.Geteuid() == 0 {
		if err := os.Chown(tmp.Name(), uid, gid); err != nil {
			return err
		}
	}
	return os.Rename(tmp.Name(), g.configPath())
}

func writeUnit(ctx context.Context, g *Game) error {
	if err := os.WriteFile(g.unitPath(), []byte(renderUnit(g)), 0o644); err != nil {
		return err
	}
	if res := core.RunAdmin(ctx, "systemctl", "daemon-reload"); res.Err != nil {
		return fmt.Errorf("systemctl daemon-reload: %s", res.Detail())
	}
	return nil
}

func openFirewall(ctx context.Context, g *Game, j *jobHandle, ports []int) {
	if res := core.RunAdmin(ctx, "ufw", "status"); res.Err != nil {
		j.logf("ufw nicht verfügbar, Ports selbst freigeben: %v", ports)
		return
	}
	role, _ := wireguard.CurrentRole()
	for _, port := range ports {
		args := ufwArgs(g, role, port)
		if res := core.RunAdmin(ctx, "ufw", args...); res.Err != nil {
			j.logf("ufw %s: %s", strings.Join(args, " "), res.Detail())
		} else {
			j.logf("ufw %s", strings.Join(args[:len(args)-2], " "))
		}
	}
	if role.Role == wireguard.RolePeer {
		j.logf("Dieser Server ist Peer des Hubs %s. Die Ports sind nur im Tunnel offen; öffentlich erreichbar werden sie über eine Portweiterleitung auf dem Hub an %s.", role.Hub, role.Address)
	}
}

// ufwArgs opens a game port. Behind a GoDeb hub the players arrive through
// the tunnel (port forwarding on the hub), so the port opens on wg0 only.
func ufwArgs(g *Game, role wireguard.RoleInfo, port int) []string {
	comment := "GoDeb " + g.Name
	if role.Role == wireguard.RolePeer {
		return []string{"allow", "in", "on", "wg0", "to", "any", "port", strconv.Itoa(port), "proto", "udp", "comment", comment}
	}
	return []string{"allow", strconv.Itoa(port) + "/udp", "comment", comment}
}
