package mail

import (
	"fmt"
	"sort"
	"strings"
)

// Files GoDeb writes. Nothing else is touched (own allowlist, see apply.go).
const (
	postfixMapDir = "/etc/postfix/godeb"
	mapDomains    = postfixMapDir + "/domains"
	mapMailboxes  = postfixMapDir + "/mailboxes"
	mapAliases    = postfixMapDir + "/aliases"
	mapSenders    = postfixMapDir + "/senders"

	dovecotConf  = "/etc/dovecot/dovecot.conf"
	dovecotDir   = "/etc/dovecot/godeb"
	dovecotUsers = dovecotDir + "/users"
	dovecotDeny  = dovecotDir + "/deny"
	spamSieve    = dovecotDir + "/spam.sieve"

	rspamdLocal = "/etc/rspamd/local.d"
	dkimDir     = "/var/lib/rspamd/dkim"

	fail2banJail = "/etc/fail2ban/jail.d/godeb-mail.conf"

	vmailDir = "/var/vmail"
	// milter is Rspamd's proxy worker in milter mode (Debian default).
	milter = "inet:localhost:11332"
)

const header = "# Written by GoDeb (plugin mail). Changes are overwritten.\n"

// postfixMain is the main.cf part, applied with `postconf -e`. Port 25
// accepts mail for the hosted domains only; sending needs SASL over TLS on
// 465/587 and only as one's own address (reject_sender_login_mismatch).
func postfixMain(s Settings) []string {
	return []string{
		"myhostname=" + s.Hostname,
		"myorigin=$myhostname",
		"mydestination=localhost",
		"mynetworks=127.0.0.0/8 [::1]/128",
		"inet_interfaces=all",
		"relayhost=",
		"relay_domains=",
		"smtpd_banner=$myhostname ESMTP",
		"biff=no",
		"append_dot_mydomain=no",
		"recipient_delimiter=+",
		"message_size_limit=52428800",
		"mailbox_size_limit=0",
		"disable_vrfy_command=yes",
		"smtpd_helo_required=yes",
		"strict_rfc821_envelopes=yes",

		"virtual_mailbox_domains=texthash:" + mapDomains,
		"virtual_mailbox_maps=texthash:" + mapMailboxes,
		"virtual_alias_maps=texthash:" + mapAliases,
		"virtual_transport=lmtp:unix:private/dovecot-lmtp",
		"smtpd_sender_login_maps=texthash:" + mapSenders,

		"smtpd_sasl_type=dovecot",
		"smtpd_sasl_path=private/auth",
		"smtpd_sasl_auth_enable=no",
		"smtpd_sasl_security_options=noanonymous",
		"smtpd_tls_auth_only=yes",

		"smtpd_tls_cert_file=" + s.CertFile,
		"smtpd_tls_key_file=" + s.KeyFile,
		"smtpd_tls_security_level=may",
		"smtpd_tls_protocols=>=TLSv1.2",
		"smtpd_tls_mandatory_protocols=>=TLSv1.2",
		"smtpd_tls_received_header=yes",
		"smtpd_tls_loglevel=1",
		"smtp_tls_security_level=may",
		"smtp_tls_protocols=>=TLSv1.2",
		"smtp_tls_CApath=/etc/ssl/certs",
		"smtp_tls_loglevel=1",

		"smtpd_helo_restrictions=permit_mynetworks, permit_sasl_authenticated, reject_invalid_helo_hostname, reject_non_fqdn_helo_hostname",
		"smtpd_sender_restrictions=permit_mynetworks, reject_non_fqdn_sender, reject_unknown_sender_domain",
		"smtpd_relay_restrictions=permit_mynetworks, permit_sasl_authenticated, defer_unauth_destination",
		"smtpd_recipient_restrictions=permit_mynetworks, reject_non_fqdn_recipient, reject_unknown_recipient_domain, reject_unauth_destination, reject_unlisted_recipient",
		"smtpd_data_restrictions=reject_unauth_pipelining",

		"milter_protocol=6",
		"milter_default_action=accept",
		"milter_mail_macros=i {mail_addr} {client_addr} {client_name} {auth_authen}",
		"smtpd_milters=" + milter,
		"non_smtpd_milters=" + milter,
	}
}

// submissionOverrides are the -o options of the two submission services:
// TLS mandatory, login mandatory, sender must belong to the login.
var submissionOverrides = []string{
	"smtpd_sasl_auth_enable=yes",
	"smtpd_tls_auth_only=yes",
	"smtpd_client_restrictions=permit_sasl_authenticated,reject",
	"smtpd_helo_restrictions=",
	"smtpd_sender_restrictions=reject_sender_login_mismatch,permit_sasl_authenticated,reject",
	"smtpd_recipient_restrictions=permit_sasl_authenticated,reject",
	"smtpd_relay_restrictions=permit_sasl_authenticated,reject",
	"milter_macro_daemon_name=ORIGINATING",
}

// postfixMaster returns `postconf -M` service lines and `postconf -P`
// parameters for submission (587, STARTTLS) and submissions (465, TLS).
func postfixMaster() (services []string, params []string) {
	services = []string{
		"submission/inet=submission inet n - y - - smtpd",
		"submissions/inet=submissions inet n - y - - smtpd",
	}
	params = []string{
		"submission/inet/syslog_name=postfix/submission",
		"submission/inet/smtpd_tls_security_level=encrypt",
		"submissions/inet/syslog_name=postfix/submissions",
		"submissions/inet/smtpd_tls_wrappermode=yes",
	}
	for _, svc := range []string{"submission", "submissions"} {
		for _, o := range submissionOverrides {
			params = append(params, svc+"/inet/"+o)
		}
	}
	return services, params
}

// active returns the domains that receive mail and their accounts.
func active(domains []Domain, accounts []Account) (map[string]Domain, []Account) {
	on := map[string]Domain{}
	for _, d := range domains {
		if !d.Suspended {
			on[d.Name] = d
		}
	}
	out := []Account{}
	for _, a := range accounts {
		if _, ok := on[a.Domain]; ok {
			out = append(out, a)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Address() < out[j].Address() })
	return on, out
}

// postfixMaps renders the four texthash maps.
func postfixMaps(domains []Domain, accounts []Account) (dom, mailboxes, aliases, senders string) {
	on, accts := active(domains, accounts)
	names := make([]string, 0, len(on))
	for n := range on {
		names = append(names, n)
	}
	sort.Strings(names)
	var d, m, a, s strings.Builder
	for _, w := range []*strings.Builder{&d, &m, &a, &s} {
		w.WriteString(header)
	}
	for _, n := range names {
		fmt.Fprintf(&d, "%s OK\n", n)
	}
	taken := map[string]bool{}
	for _, acc := range accts {
		addr := acc.Address()
		taken[addr] = true
		if !acc.ForwardOnly {
			fmt.Fprintf(&m, "%s %s/%s/\n", addr, acc.Domain, acc.Local)
		}
		// Every address maps at least to itself, so a catch-all of the
		// domain does not swallow existing mailboxes.
		targets := []string{}
		if !acc.ForwardOnly {
			targets = append(targets, addr)
		}
		targets = append(targets, acc.Forward...)
		fmt.Fprintf(&a, "%s %s\n", addr, strings.Join(targets, ", "))
		fmt.Fprintf(&s, "%s %s\n", addr, addr)
	}
	for _, acc := range accts {
		for _, al := range acc.Aliases {
			alias := al + "@" + acc.Domain
			if taken[alias] {
				continue
			}
			taken[alias] = true
			fmt.Fprintf(&a, "%s %s\n", alias, acc.Address())
			// The owner may send as the alias.
			fmt.Fprintf(&s, "%s %s\n", alias, acc.Address())
		}
	}
	for _, n := range names {
		if c := on[n].CatchAll; c != "" {
			fmt.Fprintf(&a, "@%s %s\n", n, c)
		}
	}
	return d.String(), m.String(), a.String(), s.String()
}

// dovecotUsersFile renders the passwd-file: address, hash, and the quota
// as userdb extra field. uid, gid and home come from dovecot.conf.
// Forward-only addresses have no mailbox and no login.
func dovecotUsersFile(domains []Domain, accounts []Account) (users, deny string) {
	var u, d strings.Builder
	u.WriteString(header)
	d.WriteString(header)
	suspended := map[string]bool{}
	for _, dom := range domains {
		if dom.Suspended {
			suspended[dom.Name] = true
		}
	}
	sorted := append([]Account(nil), accounts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Address() < sorted[j].Address() })
	for _, a := range sorted {
		if a.ForwardOnly || a.Hash == "" {
			continue
		}
		extra := ""
		if a.QuotaMB > 0 {
			extra = fmt.Sprintf("userdb_quota_storage_size=%dM", a.QuotaMB)
		}
		fmt.Fprintf(&u, "%s:%s::::::%s\n", a.Address(), a.Hash, extra)
		// Suspended mailboxes still receive mail (LMTP needs the userdb
		// entry) but cannot log in or send.
		if a.Suspended || suspended[a.Domain] {
			fmt.Fprintf(&d, "%s:\n", a.Address())
		}
	}
	return u.String(), d.String()
}

// dovecotConfig is the complete dovecot.conf. It replaces Debian's file,
// which authenticates system users through PAM: on a mail server only
// GoDeb's mailboxes may log in.
func dovecotConfig(s Settings) string {
	return header + `dovecot_config_version = 2.4.0
dovecot_storage_version = 2.4.0

protocols = imap lmtp
listen = *, ::

auth_mechanisms = plain login
auth_allow_cleartext = no
auth_username_format = %{user | lower}
auth_failure_delay = 2 secs

ssl = required
ssl_server_cert_file = ` + s.CertFile + `
ssl_server_key_file = ` + s.KeyFile + `
ssl_min_protocol = TLSv1.2
ssl_server_prefer_ciphers = server

mail_uid = vmail
mail_gid = vmail
first_valid_uid = 100
mail_driver = maildir
mail_home = ` + vmailDir + `/%{user | domain}/%{user | username}
mail_path = ~/Maildir
mailbox_list_index = yes
mail_plugins {
  quota = yes
}

# Suspended mailboxes: listed here, refused before the password check.
passdb deny {
  driver = passwd-file
  passwd_file_path = ` + dovecotDeny + `
  deny = yes
}
passdb passwd-file {
  passwd_file_path = ` + dovecotUsers + `
  default_password_scheme = ARGON2ID
}
userdb passwd-file {
  passwd_file_path = ` + dovecotUsers + `
}

namespace inbox {
  inbox = yes
  separator = /
  mailbox Drafts {
    special_use = \Drafts
    auto = subscribe
  }
  mailbox Junk {
    special_use = \Junk
    auto = subscribe
  }
  mailbox Trash {
    special_use = \Trash
    auto = subscribe
  }
  mailbox Sent {
    special_use = \Sent
    auto = subscribe
  }
}

quota "User quota" {
  quota_storage_size = 0
}

protocol imap {
  mail_plugins {
    imap_quota = yes
  }
}

protocol lmtp {
  mail_plugins {
    sieve = yes
  }
}

# Spam (header X-Spam: Yes from Rspamd) goes to Junk before the user's
# own rules run.
sieve_script spam {
  type = before
  driver = file
  path = ` + spamSieve + `
}
sieve_script personal {
  driver = file
  path = ~/sieve
  active_path = ~/.dovecot.sieve
}

# Absence notes (vacation) for personal scripts written by GoDeb.
sieve_extensions {
  vacation = yes
}

service imap-login {
  inet_listener imap {
    port = 0
  }
  inet_listener imaps {
    port = 993
    ssl = yes
  }
}
service lmtp {
  unix_listener /var/spool/postfix/private/dovecot-lmtp {
    mode = 0600
    user = postfix
    group = postfix
  }
}
service auth {
  unix_listener /var/spool/postfix/private/auth {
    mode = 0660
    user = postfix
    group = postfix
  }
}
`
}

const spamSieveScript = `require ["fileinto"];
if header :contains "X-Spam" "Yes" {
  fileinto "Junk";
  stop;
}
`

// rspamdFiles are the local.d overrides. DKIM keys live in dkimDir as
// <domain>.<selector>.key; a domain without key is not signed.
func rspamdFiles(s Settings, domains []Domain) map[string]string {
	var dkim strings.Builder
	dkim.WriteString(header + `enabled = true;
sign_authenticated = true;
sign_local = true;
use_domain = "header";
use_esld = false;
allow_username_mismatch = true;
allow_hdrfrom_mismatch = false;
try_fallback = false;
domain {
`)
	sorted := append([]Domain(nil), domains...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name })
	for _, d := range sorted {
		if d.Suspended || d.Selector == "" {
			continue
		}
		fmt.Fprintf(&dkim, "  %q {\n    selector = %q;\n    path = %q;\n  }\n", d.Name, d.Selector, dkimKeyPath(d.Name, d.Selector))
	}
	dkim.WriteString("}\n")
	files := map[string]string{
		rspamdLocal + "/dkim_signing.conf": dkim.String(),
		// ARC seals forwarded mail with the same keys.
		rspamdLocal + "/arc.conf": strings.Replace(dkim.String(), "allow_hdrfrom_mismatch = false;\n", "", 1),
		rspamdLocal + "/actions.conf": header + `reject = 15;
add_header = 6;
greylist = 4;
`,
		rspamdLocal + "/milter_headers.conf": header + `use = ["authentication-results", "x-spam-status"];
authenticated_headers = ["authentication-results"];
`,
		rspamdLocal + "/redis.conf": header + `servers = "127.0.0.1:6379";
`,
		rspamdLocal + "/classifier-bayes.conf": header + `autolearn = true;
`,
	}
	av := header + "# ClamAV is off.\n"
	if s.ClamAV {
		av = header + `clamav {
  type = "clamav";
  servers = "/run/clamav/clamd.ctl";
  scan_mime_parts = true;
  symbol = "CLAM_VIRUS";
  action = "reject";
  message = "Virus gefunden: ${VIRUS}";
}
`
	}
	files[rspamdLocal+"/antivirus.conf"] = av
	return files
}

func dkimKeyPath(domain, selector string) string {
	return fmt.Sprintf("%s/%s.%s.key", dkimDir, domain, selector)
}

// fail2banConfig bans after failed logins on SMTP and IMAP.
const fail2banConfig = header + `[postfix]
enabled = true
mode = aggressive
backend = systemd

[dovecot]
enabled = true
backend = systemd
`

// mailPorts are opened in ufw: SMTP, submissions, submission, IMAPS.
var mailPorts = []string{"25", "465", "587", "993"}
