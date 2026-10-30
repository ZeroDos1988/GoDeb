import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import type { CommandEnvelope } from "@/lib/types.ts";
import { useGet } from "@/lib/useAsync.ts";
import * as ui from "@/components/ui.ts";
import { More, SectionHeader } from "@/components/Section.tsx";
import { Help } from "@/components/Help.tsx";

type Timer = {
  unit: string;
  next: string; // RFC 3339, "" if none
  last: string; // RFC 3339, "" if never ran
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

const input = `${ui.input} font-mono`;
const primary = ui.primary;
const card = ui.card;

export default function Panel() {
  const { t, i18n } = useTranslation();
  const query = useGet<Status>("/api/plugins/cron/status");
  const [adding, setAdding] = useState(false);
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
      setAdding(false);
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

      <SectionHeader title={t("cron.timers")}>
        <button type="button" className={adding ? ui.button : ui.primary} onClick={() => setAdding((v) => !v)}>
          {adding ? t("common.cancel") : `+ ${t("cron.add")}`}
        </button>
      </SectionHeader>
      {adding ? (
      <form onSubmit={(e) => void create(e)} className={`${card} mb-4 space-y-3`}>
        <h3 className="flex items-center gap-2 text-base font-semibold">
          {t("cron.add")}
          <Help>{t("cron.addHint")}</Help>
        </h3>
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
      ) : null}
      {timers.length === 0 ? (
          <p className="text-sm text-muted">{t("cron.noTimers")}</p>
        ) : (
          <DataTable
            mobileCards
            head={[t("cron.unit"), t("cron.next"), t("cron.last"), ""]}
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
                <td className="px-3 py-2 text-xs">{when(tm.next, i18n.language)}</td>
                <td className="px-3 py-2 text-xs text-white/60">{when(tm.last, i18n.language)}</td>
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


      <More title={t("cron.cronD")}>
        <p className="text-sm text-muted">{t("cron.cronDHint")}</p>
        {files.length === 0 ? (
          <p className="text-sm text-muted">{t("cron.noCronD")}</p>
        ) : (
          <ul className="space-y-2">
            {files.map((f) => (
              <li key={f.name} className="rounded-xl border border-white/10 bg-slate-950/40 p-3">
                <details>
                  <summary className="cursor-pointer font-mono text-xs text-white/80">{f.name}</summary>
                  <pre className="mt-2 overflow-x-auto font-mono text-[11px] whitespace-pre-wrap text-white/50">{f.preview}</pre>
                </details>
              </li>
            ))}
          </ul>
        )}
      </More>
    </PanelFrame>
  );
}

// when formats a timer time in the user's language; "–" for none.
function when(iso: string, lang: string): string {
  if (!iso) return "–";
  const d = new Date(iso);
  return Number.isNaN(d.getTime()) ? "–" : d.toLocaleString(lang, { dateStyle: "medium", timeStyle: "short" });
}
