import type { ReactNode } from "react";
import { useTranslation } from "react-i18next";

export type SetupLink = { kind: string; label: string; url: string; note?: string; command?: string };

export type SetupSummaryData = {
  hostname: string;
  version: string;
  publicIp: string;
  generated: string;
  completedAt?: string;
  role: "hub" | "peer" | "none";
  access: "tunnel" | "lan" | "local";
  urls: SetupLink[];
  tunnel?: {
    interface: string;
    address: string;
    network?: string;
    listenPort?: number;
    endpoint?: string;
    publicKey?: string;
    hub?: string;
    hubAddress?: string;
    clients?: string[];
    sites?: { name: string; address: string; url: string }[];
  };
  lanCert?: string;
  trust?: string;
  sshPorts: number[];
  firewall: "active" | "inactive" | "missing";
  rules: string[];
  commands: SetupLink[];
};

// SetupSummary is the overview after the wizard and on #/setup-summary. It
// prints on its own (print stylesheet in index.css), so "Save as PDF" in
// the print dialog gives a document to keep.
export function SetupSummary({ data, actions }: { data: SetupSummaryData; actions?: ReactNode }) {
  const { t, i18n } = useTranslation();
  const date = (iso?: string) => (iso ? new Date(iso).toLocaleString(i18n.language) : "–");
  const tn = data.tunnel;

  return (
    <div className="gd-print space-y-4 text-white">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h1 className="text-xl font-semibold tracking-tight">{t("summary.title", { host: data.hostname })}</h1>
          <p className="text-xs text-white/60">{t("summary.generated", { date: date(data.generated), version: data.version })}</p>
        </div>
        <div className="gd-noprint flex flex-wrap gap-2">
          <button
            type="button"
            onClick={() => window.print()}
            className="rounded-2xl border border-white/10 bg-white/10 px-3 py-1.5 text-sm text-white transition-all duration-200 hover:bg-white/15"
          >
            {t("summary.print")}
          </button>
          {actions}
        </div>
      </div>

      <Block title={t("summary.reach")}>
        <ul className="space-y-3">
          {data.urls.map((l) => (
            <li key={l.url + l.label}>
              <div className="text-xs text-white/60">{t(`summary.links.${l.kind}.label`, { defaultValue: l.label })}</div>
              <div className="font-mono text-base font-semibold break-all">{l.url}</div>
              {l.note ? (
                <div className="text-xs break-words text-white/60">
                  {t(`summary.links.${l.kind}.note`, { defaultValue: l.note, cmd: l.command ?? "" })}
                </div>
              ) : null}
            </li>
          ))}
        </ul>
        {data.lanCert ? <Row label={t("summary.lanCert")} value={data.lanCert} mono /> : null}
      </Block>

      <Block title={t("summary.server")}>
        <Row label={t("summary.hostname")} value={data.hostname} />
        <Row label={t("summary.publicIp")} value={data.publicIp || "–"} mono />
        <Row label={t("summary.role")} value={t(`summary.roles.${data.role}`)} />
        <Row label={t("summary.access")} value={t(`summary.accessModes.${data.access}`)} />
        <Row label={t("summary.ssh")} value={data.sshPorts.join(", ") || "–"} mono />
        <Row label={t("summary.completed")} value={date(data.completedAt)} />
      </Block>

      {tn ? (
        <Block title={t("summary.tunnel")}>
          <Row label={t("summary.tunnelAddress")} value={`${tn.address} (${tn.interface})`} mono />
          {tn.network ? <Row label={t("summary.tunnelNetwork")} value={tn.network} mono /> : null}
          {tn.endpoint ? <Row label={t("summary.endpoint")} value={`${tn.endpoint} UDP`} mono /> : null}
          {tn.hub ? <Row label={t("summary.hub")} value={tn.hubAddress ? `${tn.hub} · ${tn.hubAddress}` : tn.hub} mono /> : null}
          {tn.publicKey ? <Row label={t("summary.publicKey")} value={tn.publicKey} mono /> : null}
          {tn.sites?.map((s) => (
            <Row key={s.name} label={t("summary.site", { name: s.name })} value={s.url} mono />
          ))}
          {tn.clients && tn.clients.length > 0 ? <Row label={t("summary.clients")} value={tn.clients.join(", ")} /> : null}
          {data.trust ? <Row label={t("summary.trust")} value={data.trust} mono /> : null}
        </Block>
      ) : null}

      <Block title={t("summary.firewall", { status: t(`summary.fw.${data.firewall}`) })}>
        {data.rules.length > 0 ? (
          <pre className="overflow-x-auto font-mono text-xs whitespace-pre-wrap text-white/80">{data.rules.join("\n")}</pre>
        ) : (
          <p className="text-sm text-white/60">{t("summary.noRules")}</p>
        )}
      </Block>

      <Block title={t("summary.commands")}>
        {data.commands.map((c) => (
          <Row key={c.url} label={t(`summary.cmd.${c.kind}`, { defaultValue: c.label })} value={c.url} mono />
        ))}
      </Block>
      <p className="text-xs text-white/50">{t("summary.noSecrets")}</p>
    </div>
  );
}

function Block({ title, children }: { title: string; children: ReactNode }) {
  return (
    <section className="gd-print-block rounded-2xl border border-white/10 bg-slate-900/60 p-4 backdrop-blur-xl">
      <h2 className="mb-3 text-sm font-semibold">{title}</h2>
      {children}
    </section>
  );
}

function Row({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="grid gap-0.5 border-b border-white/10 py-1.5 last:border-b-0 sm:grid-cols-[12rem_1fr] sm:gap-4">
      <div className="text-xs text-white/60">{label}</div>
      <div className={`min-w-0 text-sm break-all ${mono ? "font-mono" : ""}`}>{value}</div>
    </div>
  );
}
