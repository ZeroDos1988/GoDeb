import { useState, type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";
import * as ui from "@/components/ui.ts";
import { More, SectionHeader } from "@/components/Section.tsx";
import { Help } from "@/components/Help.tsx";
import { DomainView as MailDomainView } from "../../mail/ui/DomainView.tsx";
import { useRelated } from "./related.ts";

type Settings = {
  tokenSet: boolean;
  serverId?: number;
  tokenHint?: string;
};

type Domain = {
  name: string;
  mode: "manual" | "hetzner";
  created: string;
};

type Record = {
  zone: string;
  name: string;
  type: string;
  value: string;
  kind: string;
  source: string;
  zoneLine: string;
  status: string;
  found?: string;
  managed: boolean;
};

type Status = {
  settings: Settings;
  domains: Domain[];
  publicIp: string;
};

const base = "/api/plugins/domains";
const card = `${ui.card} mb-6`;
const button = ui.button;
const primary = ui.primary;
const input = ui.input;

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<Status>(`${base}/status`);
  const data = query.data;
  const [selected, setSelected] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const [adding, setAdding] = useState(false);
  const related = useRelated();

  if (selected) {
    return (
      <DomainView
        name={selected}
        mode={
          data?.domains.find((d) => d.name === selected)?.mode ?? "manual"
        }
        tokenSet={data?.settings.tokenSet ?? false}
        related={related}
        onBack={() => setSelected(null)}
        onChanged={query.reload}
      />
    );
  }

  async function addDomain(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains`, {
        method: "POST",
        body: JSON.stringify({
          name: String(fd.get("name") ?? ""),
          mode: String(fd.get("mode") ?? "manual"),
        }),
      });
      e.currentTarget.reset();
      setAdding(false);
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function saveToken(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api(`${base}/token`, {
        method: "POST",
        body: JSON.stringify({ token: String(fd.get("token") ?? "") }),
      });
      e.currentTarget.reset();
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function clearToken() {
    setBusy(true);
    setError("");
    try {
      await api(`${base}/token`, {
        method: "POST",
        body: JSON.stringify({ clear: true }),
      });
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  const tokenCard = (
  <section className={ui.card}>
    <h2 className="mb-3 flex items-center gap-2 text-base font-semibold">
      {t("domains.hetzner")}
      <Help>{t("domains.hetznerHint")}</Help>
    </h2>
    {data?.settings.tokenSet ? (
      <div className="mb-3 flex flex-wrap items-center gap-3 text-sm">
        <span>
          {t("domains.tokenSet", { hint: data.settings.tokenHint ?? "····" })}
        </span>
        <button type="button" className={button} disabled={busy} onClick={clearToken}>
          {t("domains.tokenClear")}
        </button>
      </div>
    ) : null}
    <form onSubmit={saveToken} className="grid gap-3 md:grid-cols-[1fr_auto]">
      <input
        name="token"
        type="password"
        autoComplete="off"
        required={!data?.settings.tokenSet}
        placeholder={t("domains.tokenPlaceholder")}
        className={input}
      />
      <button type="submit" className={primary} disabled={busy}>
        {t("domains.tokenSave")}
      </button>
    </form>
  </section>
  );

  return (
    <PanelFrame
      title={t("domains.title")}
      description={t("domains.description")}
      loading={query.loading && !data}
      error={error || query.error}
      onRefresh={query.reload}
    >
      <SectionHeader title={t("domains.list")}>
        <button type="button" className={adding ? ui.button : ui.primary} onClick={() => setAdding((v) => !v)}>
          {adding ? t("common.cancel") : `+ ${t("domains.add")}`}
        </button>
      </SectionHeader>
      {adding ? (
        <form onSubmit={addDomain} className={`${card} grid gap-3 md:grid-cols-[1fr_auto_auto]`}>
          <input name="name" required placeholder="example.com" className={input} autoComplete="off" />
          <select name="mode" className={input} defaultValue="manual">
            <option value="manual">{t("domains.modes.manual")}</option>
            <option value="hetzner">{t("domains.modes.hetzner")}</option>
          </select>
          <button type="submit" className={primary} disabled={busy}>
            {t("domains.add")}
          </button>
        </form>
      ) : null}
      {(data?.domains ?? []).length === 0 ? (
        <p className="text-sm text-muted">{t("domains.empty")}</p>
      ) : (
        <DataTable
          mobileCards
          head={[t("domains.name"), "DNS", t("domains.tabs.mail"), t("domains.tabs.web"), t("domains.cert"), ""]}
        >
          {(data?.domains ?? []).map((d) => {
            const r = related.forDomain(d.name);
            return (
              <tr key={d.name} className="cursor-pointer" onClick={() => setSelected(d.name)}>
                <td className="px-3 py-3 font-mono font-medium">{d.name}</td>
                <td className="px-3 py-3 text-sm">{t(`domains.modes.${d.mode}`)}</td>
                <td className="px-3 py-3 text-sm">
                  {!r.mail?.installed ? "–" : r.mail.accounts < 0 ? "–" : t("domains.mailboxes", { count: r.mail.accounts })}
                </td>
                <td className="px-3 py-3 text-sm">{r.sites.length ? t("domains.sites", { count: r.sites.length }) : "–"}</td>
                <td className="px-3 py-3 text-sm">
                  <CertState certs={r.certs} />
                </td>
                <td className="px-3 py-3 text-right">
                  <button type="button" className={ui.button} onClick={() => setSelected(d.name)}>
                    {t("domains.open")}
                  </button>
                </td>
              </tr>
            );
          })}
        </DataTable>
      )}
      {data?.settings.tokenSet ? (
        <More title={t("domains.hetzner")}>
          {tokenCard}
        </More>
      ) : (
        <div className="mt-6">
          {tokenCard}
        </div>
      )}
    </PanelFrame>
  );
}

type Tab = "overview" | "dns" | "mail" | "web";

// DomainView follows HestiaCP: one page per domain with everything GoDeb
// runs for it, the DNS records, the mail domain and the websites.
function DomainView({
  name,
  mode,
  tokenSet,
  related,
  onBack,
  onChanged,
}: {
  name: string;
  mode: string;
  tokenSet: boolean;
  related: ReturnType<typeof useRelated>;
  onBack: () => void;
  onChanged: () => void;
}) {
  const [tab, setTab] = useState<Tab>("overview");
  const r = related.forDomain(name);
  const { t } = useTranslation();
  const query = useGet<{
    domain: Domain;
    records: Record[];
    hetznerError?: string;
    zoneMissing?: boolean;
  }>(
    `${base}/domains/${encodeURIComponent(name)}`,
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const records = query.data?.records ?? [];

  async function setMode(next: string) {
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${encodeURIComponent(name)}`, {
        method: "POST",
        body: JSON.stringify({ mode: next }),
      });
      query.reload();
      onChanged();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function apply() {
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${encodeURIComponent(name)}/apply`, {
        method: "POST",
        body: "{}",
      });
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!window.confirm(t("domains.deleteConfirm", { name }))) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${encodeURIComponent(name)}/delete`, {
        method: "POST",
        body: "{}",
      });
      onChanged();
      onBack();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  const tabs: Tab[] = ["overview", "dns", ...(related.hasMail ? (["mail"] as const) : []), ...(related.hasSites ? (["web"] as const) : [])];
  const okRecords = records.filter((x) => x.status === "ok").length;
  const currentMode = query.data?.domain.mode ?? mode;

  return (
    <>
    <button type="button" className="mb-2 text-sm text-muted hover:text-brand-cyan" onClick={onBack}>
      ← {t("domains.title")}
    </button>
    <PanelFrame
      title={name}
      description={t("domains.domainLead")}
      loading={query.loading && !query.data}
      error={error || query.error}
      onRefresh={query.reload}
      tabs={
        <div className="inline-flex rounded-xl border border-white/10 bg-slate-900/60 p-1 text-sm text-white">
          {tabs.map((id) => (
            <button
              key={id}
              type="button"
              aria-pressed={tab === id}
              onClick={() => setTab(id)}
              className={`min-h-9 shrink-0 rounded-lg px-3 transition-all duration-200 ease-in-out ${tab === id ? "bg-white/15 font-medium" : "text-white/70 hover:text-white"}`}
            >
              {t(`domains.tabs.${id}`)}
            </button>
          ))}
        </div>
      }
    >
      {tab === "overview" ? (
        <div className="grid gap-4 md:grid-cols-2">
          <Summary
            title="DNS"
            value={records.length ? t("domains.recordsOk", { ok: okRecords, total: records.length }) : t("domains.noRecords")}
            tone={records.length && okRecords < records.length ? "warning" : "ok"}
            detail={t(`domains.modes.${currentMode}`)}
            onOpen={() => setTab("dns")}
          />
          {related.hasMail ? (
            <Summary
              title={t("domains.tabs.mail")}
              value={
                !r.mail?.installed || r.mail.accounts < 0
                  ? t("domains.noMail")
                  : t("domains.mailboxes", { count: r.mail.accounts })
              }
              tone={r.mail?.suspended ? "warning" : undefined}
              onOpen={() => setTab("mail")}
            />
          ) : null}
          {related.hasSites ? (
            <Summary
              title={t("domains.tabs.web")}
              value={r.sites.length ? t("domains.sites", { count: r.sites.length }) : t("domains.noSites")}
              detail={r.sites.map((x) => x.domain).join(", ")}
              onOpen={() => setTab("web")}
            />
          ) : null}
          {related.hasCerts ? (
            <Summary title={t("domains.cert")} value={<CertState certs={r.certs} />} onOpen={() => (window.location.hash = "#/certbot")} />
          ) : null}
        </div>
      ) : null}

      {tab === "dns" ? (
        <>
          <section className={card}>
            <div className="flex flex-wrap items-center gap-3">
              <label className="text-sm text-white/70">{t("domains.mode")}</label>
              <select
                className={input + " max-w-xs"}
                value={currentMode}
                disabled={busy}
                onChange={(e) => setMode(e.target.value)}
              >
                <option value="manual">{t("domains.modes.manual")}</option>
                <option value="hetzner">{t("domains.modes.hetzner")}</option>
              </select>
              {currentMode === "hetzner" ? (
                <button type="button" className={primary} disabled={busy || !tokenSet} onClick={apply}>
                  {t("domains.apply")}
                </button>
              ) : null}
              <Help>{t("domains.recordsHint")}</Help>
            </div>
            {!tokenSet && currentMode === "hetzner" ? (
              <p className="mt-3 text-sm text-amber-200">{t("domains.needToken")}</p>
            ) : null}
            {query.data?.zoneMissing ? (
              <p className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-100">
                {t("domains.zoneMissing", { name })}
              </p>
            ) : query.data?.hetznerError ? (
              <p className="mt-3 rounded-xl border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-100">
                {t("domains.hetznerUnreachable", { error: query.data.hetznerError })}
              </p>
            ) : null}
          </section>
          {records.length === 0 ? (
            <p className="text-sm text-muted">{t("domains.noRecords")}</p>
          ) : (
            <DataTable
              mobileCards
              head={[t("domains.dnsType"), t("domains.dnsName"), t("domains.dnsValue"), t("domains.state")]}
            >
              {records.map((rec) => (
                <tr key={`${rec.kind}-${rec.name}-${rec.type}`}>
                  <td className="px-3 py-2 align-top">
                    <span className="font-mono text-sm">{rec.type}</span>
                    {rec.kind.toUpperCase() !== rec.type ? (
                      <span className="block text-xs text-white/50">{t(`domains.kinds.${rec.kind}`, rec.kind)}</span>
                    ) : null}
                  </td>
                  <td className="px-3 py-2 align-top font-mono text-xs break-all">{rec.name}</td>
                  <td className="max-w-md px-3 py-2 align-top font-mono text-xs break-all">
                    {rec.value}
                    {rec.found && rec.status !== "ok" ? (
                      <span className="mt-1 block text-amber-200">
                        {t("domains.dnsFound")}: {rec.found}
                      </span>
                    ) : null}
                  </td>
                  <td className="px-3 py-2 align-top whitespace-nowrap">
                    <StatusBadge status={rec.status} />
                    {rec.managed ? <span className="ms-1 text-xs text-emerald-300">{t("domains.managed")}</span> : null}
                  </td>
                </tr>
              ))}
            </DataTable>
          )}
          <More title={t("common.more")}>
            <button type="button" className={ui.danger} disabled={busy} onClick={remove}>
              {t("domains.delete")}
            </button>
          </More>
        </>
      ) : null}

      {tab === "mail" && related.mailStatus ? (
        related.mailStatus.installed && (r.mail?.accounts ?? -1) >= 0 ? (
          <MailDomainView
            embedded
            name={name}
            status={related.mailStatus}
            onBack={() => setTab("overview")}
            onChanged={related.reloadMail}
          />
        ) : (
          <div className={card}>
            <p className="text-sm">{related.mailStatus.installed ? t("domains.noMailDomain") : t("domains.mailNotInstalled")}</p>
            <a href="#/mail" className={`${ui.primary} mt-3`}>
              {t("domains.toMail")}
            </a>
          </div>
        )
      ) : null}

      {tab === "web" ? (
        <>
          {r.sites.length === 0 ? (
            <p className="text-sm text-muted">{t("domains.noSites")}</p>
          ) : (
            <DataTable mobileCards head={[t("domains.siteName"), t("domains.siteHost"), t("domains.siteMode")]}>
              {r.sites.map((x) => (
                <tr key={x.name}>
                  <td className="px-3 py-2 font-mono text-xs">{x.name}</td>
                  <td className="px-3 py-2 font-mono text-xs">
                    {x.domain}
                    {x.tls ? " · TLS" : ""}
                    {x.php ? " · PHP" : ""}
                  </td>
                  <td className="px-3 py-2 text-xs">{x.mode}</td>
                </tr>
              ))}
            </DataTable>
          )}
          <a href="#/websites" className={`${ui.button} mt-4`}>
            {t("domains.toWebsites")}
          </a>
        </>
      ) : null}
    </PanelFrame>
    </>
  );
}

// Summary is one area of a domain on its overview tab.
function Summary({
  title,
  value,
  detail,
  tone,
  onOpen,
}: {
  title: string;
  value: ReactNode;
  detail?: string;
  tone?: "ok" | "warning";
  onOpen: () => void;
}) {
  return (
    <button type="button" onClick={onOpen} className={`${ui.card} group text-start transition-all duration-200 hover:border-brand-cyan/50`}>
      <span className="flex items-center justify-between text-sm font-medium text-white/70">
        {title}
        <span aria-hidden className="text-white/40 group-hover:text-brand-cyan">
          →
        </span>
      </span>
      <span className="mt-2 flex items-center gap-2 text-lg font-semibold">
        {tone ? <span className={`h-2.5 w-2.5 rounded-full ${tone === "ok" ? "bg-emerald-500" : "bg-amber-400"}`} aria-hidden /> : null}
        {value}
      </span>
      {detail ? <span className="mt-1 block truncate text-sm text-white/60">{detail}</span> : null}
    </button>
  );
}

// CertState is the nearest expiry of the domain's certificates.
function CertState({ certs }: { certs: { notAfter: string; days: number }[] }) {
  const { t, i18n } = useTranslation();
  if (certs.length === 0) return <span>–</span>;
  const next = certs[0];
  const tone = next.days < 0 ? "text-rose-300" : next.days < 21 ? "text-amber-200" : "";
  return (
    <span className={tone}>
      {next.days < 0
        ? t("domains.certExpired")
        : t("domains.certUntil", { date: new Date(next.notAfter).toLocaleDateString(i18n.language) })}
    </span>
  );
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const tone =
    status === "ok"
      ? "bg-emerald-500/20 text-emerald-200"
      : status === "missing"
        ? "bg-amber-500/20 text-amber-100"
        : status === "conflict" || status === "wrong"
          ? "bg-rose-500/20 text-rose-200"
          : "bg-white/10 text-white/70";
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs ${tone}`}>
      {t(`domains.status.${status}`, status)}
    </span>
  );
}
