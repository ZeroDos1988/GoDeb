import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import type { CommandEnvelope } from "@/lib/types.ts";
import { useGet } from "@/lib/useAsync.ts";

type Timer = {
  unit: string;
  next: string;
  activates: string;
  managed: boolean;
};

type CronFile = { name: string; preview: string };

type Status = CommandEnvelope & {
  timers: Timer[];
  cronFiles: CronFile[];
  allowlist: {
    binaries: string[];
    scriptDirs: string[];
    calendarHint: string;
  };
};

const input =
  "w-full rounded-2xl border border-white/10 bg-white/5 px-3 py-2 font-mono text-sm text-white outline-none";
const primary =
  "min-h-11 rounded-2xl bg-debian px-4 py-2 text-sm font-medium text-white enabled:hover:opacity-90 disabled:opacity-50";
const card =
  "rounded-2xl border border-white/10 bg-slate-900/60 p-4 text-white backdrop-blur-xl";

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<Status>("/api/plugins/cron/status");
  const [name, setName] = useState("");
  const [calendar, setCalendar] = useState("*-*-* 03:00:00");
  const [command, setCommand] = useState("/usr/bin/true");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function create(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/api/plugins/cron/jobs", {
        method: "POST",
        body: JSON.stringify({
          name,
          calendar,
          description,
          command: command.trim().split(/\s+/).filter(Boolean),
        }),
      });
      setName("");
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("cron.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function remove(unit: string) {
    const n = unit.replace(/^godeb-cron-/, "").replace(/\.timer$/, "");
    if (!window.confirm(t("cron.confirmDelete", { name: n }))) return;
    setBusy(true);
    setError("");
    try {
      await api(`/api/plugins/cron/jobs/${encodeURIComponent(n)}/delete`, {
        method: "POST",
        body: "{}",
      });
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("cron.failed"));
    } finally {
      setBusy(false);
    }
  }

  const timers = query.data?.timers ?? [];
  const files = query.data?.cronFiles ?? [];

  return (
    <PanelFrame
      title={t("cron.title")}
      description={t("cron.description")}
      command={query.data?.command}
      mocked={query.data?.mocked}
      detail={query.data?.detail}
      loading={query.loading}
      error={query.error}
      onRefresh={query.reload}
    >
      {error ? <p className="mb-4 text-sm text-debian">{error}</p> : null}

      <section className={`${card} mb-4 space-y-3`}>
        <h2 className="text-sm font-semibold">{t("cron.timers")}</h2>
        {timers.length === 0 ? (
          <p className="text-sm text-muted">{t("cron.noTimers")}</p>
        ) : (
          <DataTable
            mobileCards
            head={[t("cron.unit"), t("cron.next"), t("cron.activates"), ""]}
          >
            {timers.map((tm) => (
              <tr key={tm.unit}>
                <td className="px-3 py-2 font-mono text-xs">
                  {tm.unit}
                  {tm.managed ? (
                    <span className="ms-2 text-[10px] tracking-wide text-emerald-300 uppercase">
                      GoDeb
                    </span>
                  ) : null}
                </td>
                <td className="px-3 py-2 text-xs">{tm.next || "–"}</td>
                <td className="px-3 py-2 font-mono text-xs">{tm.activates || "–"}</td>
                <td className="px-3 py-2 text-right">
                  {tm.managed ? (
                    <button
                      type="button"
                      className="min-h-11 text-xs text-rose-300 hover:underline"
                      disabled={busy}
                      onClick={() => void remove(tm.unit)}
                    >
                      {t("cron.delete")}
                    </button>
                  ) : (
                    "–"
                  )}
                </td>
              </tr>
            ))}
          </DataTable>
        )}
      </section>

      <form onSubmit={(e) => void create(e)} className={`${card} mb-4 max-w-2xl space-y-3`}>
        <h2 className="text-sm font-semibold">{t("cron.add")}</h2>
        <p className="text-xs text-white/50">{t("cron.addHint")}</p>
        <label className="block text-xs text-white/60">
          {t("cron.name")}
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            pattern="[a-z0-9][a-z0-9-]{0,31}"
            className={`${input} mt-1`}
            placeholder="nightly-logrotate"
          />
        </label>
        <label className="block text-xs text-white/60">
          {t("cron.calendar")}
          <input
            value={calendar}
            onChange={(e) => setCalendar(e.target.value)}
            required
            className={`${input} mt-1`}
            placeholder={query.data?.allowlist.calendarHint}
          />
        </label>
        <label className="block text-xs text-white/60">
          {t("cron.command")}
          <input
            value={command}
            onChange={(e) => setCommand(e.target.value)}
            required
            className={`${input} mt-1`}
            placeholder="/usr/sbin/logrotate /etc/logrotate.conf"
          />
        </label>
        <label className="block text-xs text-white/60">
          {t("cron.jobDescription")}
          <input
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            maxLength={120}
            className={`${input} mt-1`}
          />
        </label>
        <button type="submit" className={primary} disabled={busy}>
          {t("cron.create")}
        </button>
      </form>

      <section className={`${card} space-y-3`}>
        <h2 className="text-sm font-semibold">{t("cron.cronD")}</h2>
        <p className="text-xs text-white/50">{t("cron.cronDHint")}</p>
        {files.length === 0 ? (
          <p className="text-sm text-muted">{t("cron.noCronD")}</p>
        ) : (
          <ul className="space-y-2">
            {files.map((f) => (
              <li key={f.name} className="rounded-xl border border-white/10 bg-slate-950/40 p-3">
                <div className="font-mono text-xs text-white/80">{f.name}</div>
                <pre className="mt-2 overflow-x-auto font-mono text-[11px] whitespace-pre-wrap text-white/50">
                  {f.preview}
                </pre>
              </li>
            ))}
          </ul>
        )}
      </section>
    </PanelFrame>
  );
}
