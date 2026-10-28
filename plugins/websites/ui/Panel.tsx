import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";

type Site = {
  name: string;
  domain: string;
  mode: "static" | "proxy";
  php: boolean;
  proxyUrl: string;
  tls: boolean;
  public: boolean;
  enabled: boolean;
  root: string;
};

type Status = {
  sites: Site[];
  nginx: string;
  wwwRoot: string;
  mocked?: boolean;
};

const input =
  "w-full rounded-2xl border border-white/10 bg-white/5 px-3 py-2 font-mono text-sm text-white outline-none";
const primary =
  "min-h-11 rounded-2xl bg-debian px-4 py-2 text-sm font-medium text-white disabled:opacity-50";
const button =
  "min-h-11 rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white disabled:opacity-50";
const card =
  "rounded-2xl border border-white/10 bg-slate-900/60 p-4 text-white backdrop-blur-xl";

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<Status>("/api/plugins/websites/status");
  const [name, setName] = useState("");
  const [domain, setDomain] = useState("");
  const [mode, setMode] = useState<"static" | "proxy">("static");
  const [php, setPhp] = useState(true);
  const [proxyUrl, setProxyUrl] = useState("http://127.0.0.1:8080/");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function create(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/api/plugins/websites/sites", {
        method: "POST",
        body: JSON.stringify({
          name,
          domain,
          mode,
          php: mode === "static" && php,
          proxyUrl: mode === "proxy" ? proxyUrl : "",
          public: false,
          tls: false,
          enabled: true,
        }),
      });
      setName("");
      setDomain("");
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("websites.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function act(name: string, path: string, body: unknown = {}) {
    setBusy(true);
    setError("");
    try {
      await api(`/api/plugins/websites/sites/${encodeURIComponent(name)}${path}`, {
        method: "POST",
        body: JSON.stringify(body),
      });
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("websites.failed"));
    } finally {
      setBusy(false);
    }
  }

  const sites = query.data?.sites ?? [];

  return (
    <PanelFrame
      title={t("websites.title")}
      description={t("websites.description")}
      mocked={query.data?.mocked}
      loading={query.loading}
      error={query.error}
      onRefresh={query.reload}
    >
      <p className="mb-4 text-sm text-white/70">
        nginx: <span className="font-mono">{query.data?.nginx ?? "–"}</span> ·{" "}
        {t("websites.root")}: <span className="font-mono">{query.data?.wwwRoot}</span>
      </p>
      {error ? <p className="mb-4 text-sm text-debian">{error}</p> : null}

      <section className={`${card} mb-4`}>
        {sites.length === 0 ? (
          <p className="text-sm text-muted">{t("websites.empty")}</p>
        ) : (
          <DataTable
            mobileCards
            head={[t("websites.name"), t("websites.domain"), t("websites.mode"), ""]}
          >
            {sites.map((s) => (
              <tr key={s.name}>
                <td className="px-3 py-2 font-mono text-xs">{s.name}</td>
                <td className="px-3 py-2 font-mono text-xs">
                  {s.domain}
                  {s.tls ? " · TLS" : ""}
                  {s.php ? " · PHP" : ""}
                </td>
                <td className="px-3 py-2 text-xs">{s.mode}</td>
                <td className="px-3 py-2 text-right whitespace-nowrap">
                  <button
                    type="button"
                    className="me-2 text-xs text-white/80 hover:underline"
                    disabled={busy}
                    onClick={() => void act(s.name, "/certificate", { email: "" })}
                  >
                    {t("websites.cert")}
                  </button>
                  <button
                    type="button"
                    className="text-xs text-rose-300 hover:underline"
                    disabled={busy}
                    onClick={() => {
                      if (window.confirm(t("websites.confirmDelete", { name: s.name }))) {
                        void act(s.name, "/delete");
                      }
                    }}
                  >
                    {t("websites.delete")}
                  </button>
                </td>
              </tr>
            ))}
          </DataTable>
        )}
      </section>

      <form onSubmit={(e) => void create(e)} className={`${card} max-w-2xl space-y-3`}>
        <h2 className="text-sm font-semibold">{t("websites.add")}</h2>
        <p className="text-xs text-white/50">{t("websites.addHint")}</p>
        <label className="block text-xs text-white/60">
          {t("websites.name")}
          <input className={`${input} mt-1`} value={name} required pattern="[a-z0-9][a-z0-9-]{0,31}" onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="block text-xs text-white/60">
          {t("websites.domain")}
          <input className={`${input} mt-1`} value={domain} required onChange={(e) => setDomain(e.target.value)} placeholder="www.example.com" />
        </label>
        <div className="flex flex-wrap gap-4 text-sm">
          <label className="flex items-center gap-2">
            <input type="radio" checked={mode === "static"} onChange={() => setMode("static")} />
            {t("websites.modeStatic")}
          </label>
          <label className="flex items-center gap-2">
            <input type="radio" checked={mode === "proxy"} onChange={() => setMode("proxy")} />
            {t("websites.modeProxy")}
          </label>
        </div>
        {mode === "static" ? (
          <label className="flex items-center gap-2 text-sm">
            <input type="checkbox" checked={php} onChange={(e) => setPhp(e.target.checked)} />
            {t("websites.php")}
          </label>
        ) : (
          <label className="block text-xs text-white/60">
            {t("websites.proxyUrl")}
            <input className={`${input} mt-1`} value={proxyUrl} onChange={(e) => setProxyUrl(e.target.value)} />
          </label>
        )}
        <button type="submit" className={primary} disabled={busy}>
          {t("websites.create")}
        </button>
        <button type="button" className={button} disabled={busy} onClick={() => query.reload()}>
          {t("common.refresh")}
        </button>
      </form>
    </PanelFrame>
  );
}
