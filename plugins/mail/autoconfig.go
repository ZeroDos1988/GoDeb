package mail

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	autoWWW     = "/var/www/godeb/mail"
	autoNginx   = "/etc/nginx/sites-available"
	autoEnabled = "/etc/nginx/sites-enabled"
)

func autoconfigXML(s Settings, domain string) string {
	host := strings.TrimSuffix(s.Hostname, ".")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<clientConfig version="1.1">
  <emailProvider id="%s">
    <domain>%s</domain>
    <displayName>%s</displayName>
    <displayShortName>%s</displayShortName>
    <incomingServer type="imap">
      <hostname>%s</hostname>
      <port>993</port>
      <socketType>SSL</socketType>
      <authentication>password-cleartext</authentication>
      <username>%%EMAILADDRESS%%</username>
    </incomingServer>
    <outgoingServer type="smtp">
      <hostname>%s</hostname>
      <port>465</port>
      <socketType>SSL</socketType>
      <authentication>password-cleartext</authentication>
      <username>%%EMAILADDRESS%%</username>
    </outgoingServer>
  </emailProvider>
</clientConfig>
`, domain, domain, domain, domain, host, host)
}

func autodiscoverXML(s Settings, domain string) string {
	host := strings.TrimSuffix(s.Hostname, ".")
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<Autodiscover xmlns="http://schemas.microsoft.com/exchange/autodiscover/responseschema/2006">
  <Response xmlns="http://schemas.microsoft.com/exchange/autodiscover/outlook/responseschema/2006a">
    <Account>
      <AccountType>email</AccountType>
      <Action>settings</Action>
      <Protocol>
        <Type>IMAP</Type>
        <Server>%s</Server>
        <Port>993</Port>
        <DomainRequired>off</DomainRequired>
        <LoginName>%%EMAILADDRESS%%</LoginName>
        <SPA>off</SPA>
        <SSL>on</SSL>
        <AuthRequired>on</AuthRequired>
      </Protocol>
      <Protocol>
        <Type>SMTP</Type>
        <Server>%s</Server>
        <Port>465</Port>
        <DomainRequired>off</DomainRequired>
        <LoginName>%%EMAILADDRESS%%</LoginName>
        <SPA>off</SPA>
        <SSL>on</SSL>
        <AuthRequired>on</AuthRequired>
      </Protocol>
    </Account>
  </Response>
</Autodiscover>
`, host, host)
}

func autoconfigNginx(domain string) string {
	root := filepath.Join(autoWWW, domain)
	policy := mtaSTSFilePath(domain)
	return header + fmt.Sprintf(`server {
  listen 80;
  listen [::]:80;
  server_name %s autoconfig.%s autodiscover.%s mta-sts.%s;
  root %s;

  location = /.well-known/mta-sts.txt {
    alias %s;
    default_type text/plain;
  }
  location /.well-known/autoconfig/mail/ {
    try_files $uri =404;
  }
  location /mail/ {
    try_files $uri =404;
  }
  location /autodiscover/ {
    try_files $uri =404;
  }
  location /Autodiscover/ {
    try_files $uri =404;
  }
}
`, domain, domain, domain, domain, root, policy)
}

func syncAutoconfig(s Settings, domains []Domain) error {
	if _, err := os.Stat("/usr/sbin/nginx"); err != nil {
		return nil // websites/nginx not installed yet
	}
	want := map[string]bool{}
	for _, d := range domains {
		if d.Suspended {
			continue
		}
		if _, err := NormalizeDomain(d.Name); err != nil {
			continue
		}
		root := filepath.Join(autoWWW, d.Name)
		if err := os.MkdirAll(filepath.Join(root, ".well-known/autoconfig/mail"), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(root, "mail"), 0o755); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(root, "autodiscover"), 0o755); err != nil {
			return err
		}
		xml := autoconfigXML(s, d.Name)
		if err := os.WriteFile(filepath.Join(root, ".well-known/autoconfig/mail/config-v1.1.xml"), []byte(xml), 0o644); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(root, "mail/config-v1.1.xml"), []byte(xml), 0o644); err != nil {
			return err
		}
		ad := autodiscoverXML(s, d.Name)
		if err := os.WriteFile(filepath.Join(root, "autodiscover/autodiscover.xml"), []byte(ad), 0o644); err != nil {
			return err
		}
		name := "godeb-mail-" + d.Name + ".conf"
		path := filepath.Join(autoNginx, name)
		want[path] = true
		if err := os.MkdirAll(autoNginx, 0o755); err != nil {
			return err
		}
		if err := writeAs(path, autoconfigNginx(d.Name), fileSpec{0o644, ""}); err != nil {
			return err
		}
		link := filepath.Join(autoEnabled, name)
		_ = os.Remove(link)
		if err := os.Symlink(path, link); err != nil && !os.IsExist(err) {
			return err
		}
	}
	return nil
}
