import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { PanelFrame } from "@/components/PanelFrame.tsx";
import { More, SectionHeader, StatGrid } from "@/components/Section.tsx";
import * as ui from "@/components/ui.ts";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";

type Status = {
  enabled: boolean;
  label: string;
  zone: string;
  host: string;
  port: number;
  url?: string;
  address?: string;
  addressError?: string;
  zones: string[];
  tokenSet: boolean;
  notAfter?: string;
  lastError?: string;
  lastRun?: string;
  running: boolean;
};

const base = "/api/plugins/apphttps";

function date(value?: string): string {
  return value ? new Date(value).toLocaleDateString() : "—";
}

export default function Panel() {
  const { t } = useTranslation();
  const status = useGet<Status>(`${base}/status`);
  const [current, setCurrent] = useState<Status | null>(null);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const [adding, setAdding] = useState(false);
  const view = current ?? status.data;

  useEffect(() => setCurrent(null), [status.data]);

  // Follow a running ACME order until it ends.
  const running = view?.running ?? false;
  useEffect(() => {
    if (!running) {
      return;
    }
    const id = window.setInterval(() => {
      api<Status>(`${base}/status`)
        .then(setCurrent)
        .catch(() => undefined);
    }, 3000);
    return () => window.clearInterval(id);
  }, [running]);

  async function post(action: string, body?: object) {
    setBusy(action);
    setError("");
    try {
      setCurrent(await api<Status>(`${base}/${action}`, { method: "POST", body: body ? JSON.stringify(body) : undefined }));
      setAdding(false);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("apphttps.failed"));
    } finally {
      setBusy("");
    }
  }

  const ready = view ? view.tokenSet && view.zones.length > 0 && !view.addressError : false;
  const state = !view
    ? ""
    : view.running
      ? t("apphttps.running")
      : view.enabled && view.url
        ? t("apphttps.on")
        : view.enabled
          ? t("apphttps.pending")
          : t("apphttps.off");

  return (
    <PanelFrame
      title={t("apphttps.title")}
      description={t("apphttps.description")}
      loading={status.loading}
      error={status.error}
      onRefresh={status.reload}
    >
      {view ? (
        <div className="space-y-4">
          <StatGrid
            items={[
              { label: t("apphttps.state"), value: state, tone: view.enabled && view.url ? "text-emerald-300" : "text-amber-200" },
              { label: t("apphttps.address"), value: view.url ? view.host : "—", mono: true },
              { label: t("apphttps.validUntil"), value: date(view.notAfter) },
              { label: t("apphttps.tunnel"), value: view.address ?? "—", mono: true },
            ]}
          />

          {view.lastError ? <p className="text-sm text-debian break-words">{view.lastError}</p> : null}
          {error ? <p className="text-sm text-debian break-words">{error}</p> : null}

          {view.enabled ? (
            <div className={ui.card}>
              <h3 className={ui.cardTitle}>{t("apphttps.installTitle")}</h3>
              {view.url ? (
                <>
                  <a href={view.url} className="mt-2 block font-mono text-sm break-all text-white underline-offset-2 hover:underline">
                    {view.url}
                  </a>
                  <p className={ui.hint}>{t("apphttps.installHint")}</p>
                </>
              ) : (
                <p className={ui.hint}>{view.running ? t("apphttps.runningHint") : t("apphttps.pendingHint")}</p>
              )}
              <div className="mt-4 flex flex-wrap gap-2">
                <button type="button" className={ui.button} disabled={busy !== "" || view.running} onClick={() => void post("renew")}>
                  {busy === "renew" ? t("apphttps.working") : t("apphttps.renew")}
                </button>
                <button type="button" className={ui.danger} disabled={busy !== "" || view.running} onClick={() => void post("disable")}>
                  {busy === "disable" ? t("apphttps.working") : t("apphttps.disable")}
                </button>
              </div>
            </div>
          ) : (
            <>
              <SectionHeader title={t("apphttps.setupTitle")} hint={t("apphttps.setupHint")}>
                <button type="button" className={adding ? ui.button : ui.primary} disabled={!ready} onClick={() => setAdding((v) => !v)}>
                  {adding ? t("common.cancel") : `+ ${t("apphttps.setup")}`}
                </button>
              </SectionHeader>
              {!ready ? <Missing status={view} /> : null}
              {adding && ready ? <SetupForm status={view} busy={busy === "enable"} onSubmit={(body) => void post("enable", body)} /> : null}
            </>
          )}

          <More title={t("apphttps.details")}>
            <ul className="list-disc space-y-1 ps-5 text-sm text-white/70">
              <li>{t("apphttps.detailPort", { port: view.port })}</li>
              <li>{t("apphttps.detailDns")}</li>
              <li>{t("apphttps.detailRenew")}</li>
              <li>{t("apphttps.detailSite")}</li>
            </ul>
            {view.lastRun ? <p className={ui.hint}>{t("apphttps.lastRun", { date: new Date(view.lastRun).toLocaleString() })}</p> : null}
          </More>
        </div>
      ) : null}
    </PanelFrame>
  );
}

function Missing({ status }: { status: Status }) {
  const { t } = useTranslation();
  return (
    <div className={ui.card}>
      <ul className="list-disc space-y-1 ps-5 text-sm text-white/70">
        {status.addressError ? <li>{t("apphttps.needTunnel")}</li> : null}
        {!status.tokenSet ? <li>{t("apphttps.needToken")}</li> : null}
        {status.zones.length === 0 ? <li>{t("apphttps.needZone")}</li> : null}
      </ul>
      {!status.tokenSet || status.zones.length === 0 ? (
        <a href="#/domains" className={`${ui.button} mt-4`}>
          {t("apphttps.openDomains")}
        </a>
      ) : null}
    </div>
  );
}

function SetupForm({ status, busy, onSubmit }: { status: Status; busy: boolean; onSubmit: (body: { label: string; zone: string }) => void }) {
  const { t } = useTranslation();
  const [label, setLabel] = useState(status.label || "godeb");
  const [zone, setZone] = useState(status.zone && status.zones.includes(status.zone) ? status.zone : (status.zones[0] ?? ""));

  return (
    <form
      className={ui.card}
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit({ label: label.trim().toLowerCase(), zone });
      }}
    >
      <div className="grid gap-3 md:grid-cols-2">
        <label className="block text-sm">
          <span className={ui.label}>{t("apphttps.name")}</span>
          <input className={ui.input} value={label} required pattern="[A-Za-z0-9]([A-Za-z0-9-]{0,61}[A-Za-z0-9])?" onChange={(e) => setLabel(e.target.value)} />
        </label>
        <label className="block text-sm">
          <span className={ui.label}>{t("apphttps.domain")}</span>
          <select className={ui.input} value={zone} required onChange={(e) => setZone(e.target.value)}>
            {status.zones.map((z) => (
              <option key={z} value={z}>
                {z}
              </option>
            ))}
          </select>
        </label>
      </div>
      <p className={ui.hint}>
        {t("apphttps.preview", { url: `https://${label.trim().toLowerCase() || "…"}.${zone}:${status.port}`, ip: status.address ?? "" })}
      </p>
      <div className="mt-3">
        <button type="submit" className={ui.primary} disabled={busy || !label.trim() || !zone}>
          {busy ? t("apphttps.working") : t("apphttps.enable")}
        </button>
      </div>
    </form>
  );
}
