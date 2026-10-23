package fail2ban

import (
	"net"
	"strconv"
	"strings"
)

// Jail is the parsed output of `fail2ban-client status <jail>`.
type Jail struct {
	Name            string   `json:"name"`
	CurrentlyFailed int      `json:"currentlyFailed"`
	TotalFailed     int      `json:"totalFailed"`
	CurrentlyBanned int      `json:"currentlyBanned"`
	TotalBanned     int      `json:"totalBanned"`
	BannedIPs       []string `json:"bannedIPs"`
}

const sampleStatus = `Status
|- Number of jail:	2
` + "`" + `- Jail list:	godeb, sshd
`

var sampleJails = map[string]string{
	"godeb": `Status for the jail: godeb
|- Filter
|  |- Currently failed:	1
|  |- Total failed:	7
|  ` + "`" + `- File list:	/var/log/godeb/auth.log
` + "`" + `- Actions
   |- Currently banned:	1
   |- Total banned:	2
   ` + "`" + `- Banned IP list:	203.0.113.25
`,
	"sshd": `Status for the jail: sshd
|- Filter
|  |- Currently failed:	0
|  |- Total failed:	12
|  ` + "`" + `- Journal matches:	_SYSTEMD_UNIT=ssh.service + _COMM=sshd
` + "`" + `- Actions
   |- Currently banned:	2
   |- Total banned:	4
   ` + "`" + `- Banned IP list:	198.51.100.4 2001:db8::7
`,
}

// ParseJailList reads the "Jail list:" line of `fail2ban-client status`.
func ParseJailList(out string) []string {
	jails := []string{}
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := field(line)
		if !ok || key != "jail list" {
			continue
		}
		for _, name := range strings.Split(val, ",") {
			name = strings.TrimSpace(name)
			if validJail(name) {
				jails = append(jails, name)
			}
		}
	}
	return jails
}

// ParseJail reads counters and banned addresses from a jail status.
func ParseJail(name, out string) Jail {
	jail := Jail{Name: name, BannedIPs: []string{}}
	for _, line := range strings.Split(out, "\n") {
		key, val, ok := field(line)
		if !ok {
			continue
		}
		switch key {
		case "currently failed":
			jail.CurrentlyFailed = atoi(val)
		case "total failed":
			jail.TotalFailed = atoi(val)
		case "currently banned":
			jail.CurrentlyBanned = atoi(val)
		case "total banned":
			jail.TotalBanned = atoi(val)
		case "banned ip list":
			for _, ip := range strings.Fields(val) {
				if parsed := net.ParseIP(ip); parsed != nil {
					jail.BannedIPs = append(jail.BannedIPs, parsed.String())
				}
			}
		}
	}
	return jail
}

// field strips the tree drawing characters and splits "Key:\tvalue".
func field(line string) (key, val string, ok bool) {
	line = strings.TrimLeft(line, "|`- \t")
	key, val, ok = strings.Cut(line, ":")
	if !ok {
		return "", "", false
	}
	return strings.ToLower(strings.TrimSpace(key)), strings.TrimSpace(val), true
}

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}
