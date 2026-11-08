package firstrun

import (
	"bufio"
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"godeb/backend/pluginmgr"
	"godeb/plugins/mail"
	"godeb/plugins/wireguard"
)

// clamAVMinMemKB: ClamAV needs about 1.5 GB; below 3 GB RAM it stays off.
const clamAVMinMemKB = 3 << 20

// HomeMailbox is a postmaster mailbox created for a domain, with its
// password shown once.
type HomeMailbox struct {
	Address  string   `json:"address"`
	Password string   `json:"password,omitempty"`
	Aliases  []string `json:"aliases,omitempty"`
}

// HomeResult is what SetupHomeServer did.
type HomeResult struct {
	MailHost  string        `json:"mailHost"`
	ClamAV    bool          `json:"clamav"`
	Mailboxes []HomeMailbox `json:"mailboxes"`
	Notes     []string      `json:"notes"`
}

// Hooks for tests.
var (
	currentRole    = wireguard.CurrentRole
	hubTunnelAddr  = wireguard.HubTunnelAddr
	startBehindHub = startMailBehindHub
	waitMail       = WaitMailJob
	addMailDomain  = mail.AddDomain
	memTotalKB     = readMemTotalKB
)

// SetupHomeServer sets up what the hub assigned to this home server in its
// profile: the mail server behind the hub's gateway for the profile's mail
// host and domains, each with a postmaster mailbox. `godeb setup --join`
// and the wizard both call it after joining. Without a mail host it does
// nothing.
func SetupHomeServer(ctx context.Context) (HomeResult, error) {
	role, err := currentRole()
	if err != nil {
		return HomeResult{}, err
	}
	p := role.Profile
	if role.Role != wireguard.RolePeer || p.Kind != wireguard.ProfileHome {
		return HomeResult{}, errors.New("Dieser Server ist kein Heim-Server eines Hubs")
	}
	res := HomeResult{MailHost: p.MailHost, Mailboxes: []HomeMailbox{}, Notes: []string{}}
	if p.MailHost == "" {
		res.Notes = append(res.Notes, "Der Hub hat keinen Mailserver vorgesehen (keine Domains im Mail-Gateway).")
		return res, nil
	}
	hub, ok := hubTunnelAddr()
	if !ok {
		return res, errors.New("Hub-Adresse im Tunnel nicht gefunden")
	}
	res.ClamAV = memTotalKB() >= clamAVMinMemKB
	if !res.ClamAV {
		res.Notes = append(res.Notes, "Weniger als 3 GB RAM: Virenscanner (ClamAV) bleibt aus, später unter E-Mail einschaltbar.")
	}
	if !mail.Installed() {
		if err := startBehindHub(p.MailHost, hub.String(), res.ClamAV); err != nil {
			return res, err
		}
		if err := waitMail(ctx); err != nil {
			return res, err
		}
	} else if !mail.BehindHub() {
		return res, errors.New("Auf diesem Server läuft schon ein Mailserver, der nicht hinter dem Hub eingerichtet ist")
	}
	for _, d := range p.MailDomains {
		setup, err := addMailDomain(ctx, d, mail.DomainOptions{})
		switch {
		case errors.Is(err, mail.ErrDomainExists):
		case err != nil:
			res.Notes = append(res.Notes, d+": "+err.Error())
		case setup.Postmaster != "":
			res.Mailboxes = append(res.Mailboxes, HomeMailbox{Address: setup.Postmaster, Password: setup.Password, Aliases: setup.Aliases})
		}
	}
	return res, nil
}

// startMailBehindHub switches the mail plugin on through the plugin manager
// (as StartMailServer does) and starts the installation behind the hub.
func startMailBehindHub(host, hub string, clamav bool) error {
	if err := pluginmgr.InstallAndEnable(context.Background(), "mail"); err != nil {
		return err
	}
	return mail.StartInstallBehindHub(host, hub, clamav)
}

func readMemTotalKB() int {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if rest, ok := strings.CutPrefix(sc.Text(), "MemTotal:"); ok {
			n, _ := strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(rest), "kB")))
			return n
		}
	}
	return 0
}
