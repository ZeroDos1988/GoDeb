import { useGet } from "@/lib/useAsync.ts";
import type { Status as MailStatus } from "../../mail/ui/shared.tsx";

// Everything else GoDeb runs for a domain: mail domain, websites and
// certificates. Each source is optional; a switched-off plugin answers
// 404 and simply contributes nothing.

export type Site = { name: string; domain: string; mode: string; tls: boolean; php: boolean };
export type Cert = { name: string; notAfter: string };

export type Related = {
  mail?: { installed: boolean; accounts: number; suspended: boolean };
  sites: Site[];
  certs: (Cert & { days: number })[];
};

export function useRelated() {
  const mail = useGet<MailStatus>("/api/plugins/mail/status");
  const sites = useGet<{ sites?: Site[] }>("/api/plugins/websites/status");
  const certs = useGet<{ certificates?: Cert[] }>("/api/plugins/certbot/certificates");

  function forDomain(name: string): Related {
    const inZone = (host: string) => host === name || host.endsWith(`.${name}`);
    const mailDomain = mail.data?.domains?.find((d) => d.name === name);
    return {
      mail: mail.data
        ? {
            installed: mail.data.installed,
            accounts: mailDomain ? mailDomain.accounts : -1,
            suspended: mailDomain?.suspended ?? false,
          }
        : undefined,
      sites: (sites.data?.sites ?? []).filter((s) => inZone(s.domain)),
      certs: (certs.data?.certificates ?? [])
        .filter((c) => inZone(c.name))
        .map((c) => ({ ...c, days: daysUntil(c.notAfter) }))
        .sort((a, b) => a.days - b.days),
    };
  }

  return {
    forDomain,
    mailStatus: mail.data,
    hasMail: Boolean(mail.data),
    hasSites: Boolean(sites.data),
    hasCerts: Boolean(certs.data),
    reloadMail: mail.reload,
  };
}

export function daysUntil(date: string) {
  const ms = new Date(date).getTime() - Date.now();
  return Number.isNaN(ms) ? Infinity : Math.floor(ms / 86_400_000);
}
