import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { MetricRing, StatusPill, formatBytes } from "@/components/Metrics.tsx";
import { useGet } from "@/lib/useAsync.ts";
import type { SystemInfo } from "@/lib/types.ts";

type PeerList = { peers?: { live?: boolean }[]; interface?: { up?: boolean } };
type JailList = { jails?: { bannedIPs?: string[] }[] };
type Health = {
  status: "ok" | "info" | "warning" | "critical";
  findings: { id: string; status: string; title: string }[] | null;
  series: { name: string; value: number }[] | null;
};

type DoctorReport = {
  findings: { area: string; level: string; text: string; details?: string[] }[];
  fails: number;
  warns: number;
  checked: number;
};

// The overview refreshes itself; CPU is a share over time and a single
// reading on page load mostly measures the page load. Doctor is slower,
// so it only reloads on demand and when the page opens.
const REFRESH_MS = 10_000;

const healthDot: Record<Health["status"], string> = {
  ok: "bg-[#0ca30c]",
  info: "bg-[#0ca30c]",
  warning: "bg-[#fab219]",
  critical: "bg-[#d03b3b] animate-pulse",
};

const doctorTone: Record<string, string> = {
  FAIL: "bg-rose-500/20 text-rose-100",
  WARN: "bg-amber-500/20 text-amber-100",
  INFO: "bg-white/10 text-white/70",
  OK: "bg-emerald-500/15 text-emerald-200",
};

export function Overview() {
  const { t } = useTranslation();
  const query = useGet<SystemInfo>("/api/system");
  const peers = useGet<PeerList>("/api/plugins/wireguard/peers");
  const bans = useGet<JailList>("/api/plugins/fail2ban/status");
  // Missing when the monitor plugin is switched off; the card then hides.
  const health = useGet<Health>("/api/plugins/monitor/summary");
  const doctor = useGet<DoctorReport>("/api/doctor");
  const reloadSystem = query.reload;
  const reloadHealth = health.reload;
  useEffect(() => {
    const id = window.setInterval(() => {
      reloadSystem();
      reloadHealth();
    }, REFRESH_MS);
    return () => window.clearInterval(id);
  }, [reloadSystem, reloadHealth]);
  const info = query.data;
  // The monitor samples every 10 s in the background; prefer its value.
  const monitorCPU = health.data?.series?.find((s) => s.name === "cpu")?.value;
  const cpu = monitorCPU ?? info?.cpu ?? 0;
  const livePeers = (peers.data?.peers ?? []).filter((peer) => peer.live).length;
  const blocked = (bans.data?.jails ?? []).reduce((sum, jail) => sum + (jail.bannedIPs?.length ?? 0), 0);
  const tunnelUp = Boolean(peers.data?.interface?.up);
  const doctorIssues = (doctor.data?.findings ?? []).filter((f) => f.level === "FAIL" || f.level === "WARN");

  return (
    <section>
      <h1 className="text-2xl font-semibold tracking-tight">{t("overview.title")}</h1>
      <p className="mt-1 max-w-2xl text-sm text-muted">{t("overview.lead")}</p>

      {query.loading && !info ? <p className="mt-6 text-sm text-muted">{t("overview.loading")}</p> : null}
      {query.error ? (
        <p className="mt-6 max-w-xl rounded-2xl border border-white/10 bg-rose-950/40 px-4 py-3 text-sm text-rose-100 backdrop-blur-xl">
          {t("overview.unreachable", { error: query.error })}
        </p>
      ) : null}

      {health.data ? (
        <a
          href="#/monitor"
          className="mt-6 flex max-w-3xl items-center gap-4 rounded-3xl border border-white/10 bg-slate-900/60 p-5 text-white backdrop-blur-xl transition-all duration-200 ease-in-out hover:bg-white/5"
        >
          <span className={`h-3 w-3 shrink-0 rounded-full ${healthDot[health.data.status]}`} aria-hidden="true" />
          <span className="min-w-0 flex-1">
            <span className="block text-xs tracking-wide text-white/60 uppercase">{t("monitor.tile")}</span>
            <span className="block text-lg font-semibold">{t(`monitor.status.${health.data.status}`)}</span>
            {(health.data.findings ?? [])
              .filter((f) => f.status !== "info")
              .slice(0, 3)
              .map((f) => (
                <span key={f.id} className="block truncate text-sm text-white/70">
                  {f.title}
                </span>
              ))}
          </span>
          <span className="text-sm text-white/60">{t("monitor.tileOpen")} →</span>
        </a>
      ) : null}

      {doctor.data || doctor.loading || doctor.error ? (
        <article className="mt-4 max-w-3xl rounded-3xl border border-white/10 bg-slate-900/60 p-5 text-white backdrop-blur-xl">
          <div className="flex flex-wrap items-start justify-between gap-3">
            <div>
              <h2 className="text-xs tracking-wide text-white/60 uppercase">{t("overview.doctor")}</h2>
              <p className="mt-1 text-lg font-semibold">
                {doctor.data
                  ? doctor.data.fails + doctor.data.warns === 0
                    ? t("overview.doctorOk")
                    : t("overview.doctorSummary", { fails: doctor.data.fails, warns: doctor.data.warns })
                  : doctor.loading
                    ? t("common.loading")
                    : doctor.error}
              </p>
              <p className="mt-1 text-xs text-white/50">{t("overview.doctorHint")}</p>
            </div>
            <button
              type="button"
              onClick={doctor.reload}
              disabled={doctor.loading}
              className="rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm transition-all duration-200 enabled:hover:bg-white/10 disabled:opacity-50"
            >
              {t("overview.doctorOpen")}
            </button>
          </div>
          {doctorIssues.length > 0 ? (
            <ul className="mt-4 grid gap-2">
              {doctorIssues.slice(0, 12).map((f, i) => (
                <li key={`${f.area}-${f.text}-${i}`} className="rounded-2xl border border-white/10 bg-slate-950/40 px-3 py-2">
                  <div className="flex flex-wrap items-center gap-2">
                    <span className={`rounded-full px-2 py-0.5 text-xs ${doctorTone[f.level] ?? doctorTone.INFO}`}>
                      {f.level}
                    </span>
                    <span className="text-xs text-white/50">{f.area}</span>
                  </div>
                  <p className="mt-1 text-sm">{f.text}</p>
                </li>
              ))}
            </ul>
          ) : null}
        </article>
      ) : null}

      {info ? (
        <>
          <div className="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
            <MetricRing label={t("overview.cpu")} value={cpu} detail={`${Math.round(cpu)}%`} />
            <MetricRing
              label={t("overview.ram")}
              value={percent(info.memUsed, info.memTotal)}
              detail={`${formatBytes(info.memUsed)} / ${formatBytes(info.memTotal)}`}
            />
            <MetricRing
              label={t("overview.disk")}
              value={percent(info.diskUsed, info.diskTotal)}
              detail={`${formatBytes(info.diskUsed)} / ${formatBytes(info.diskTotal)}`}
            />
            <article className="rounded-3xl border border-white/10 bg-slate-900/60 p-5 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out hover:-translate-y-0.5">
              <div className="text-xs tracking-wide text-white/60 uppercase">{t("overview.peers")}</div>
              <div className="mt-3 text-3xl font-semibold tracking-tight">{livePeers}</div>
              <div className="mt-4 flex flex-wrap gap-2">
                <StatusPill tone={tunnelUp ? "ok" : "idle"}>
                  {tunnelUp ? t("overview.tunnelUp") : t("overview.tunnelDown")}
                </StatusPill>
                <StatusPill tone={blocked > 0 ? "bad" : "ok"}>
                  {blocked > 0 ? t("overview.blockedCount", { count: blocked }) : t("overview.noneBlocked")}
                </StatusPill>
              </div>
            </article>
          </div>

          <dl className="mt-4 max-w-3xl overflow-hidden rounded-3xl border border-white/10 bg-slate-900/60 text-white backdrop-blur-xl">
            <Row label={t("overview.hostname")} value={info.hostname} />
            <Row label={t("overview.os")} value={info.os} />
            <Row label={t("overview.kernel")} value={info.kernel} />
            <Row label={t("overview.source")} value="hostname · uname -r · /etc/os-release · /proc" mono />
          </dl>
          <a
            href="#/setup-summary"
            className="mt-4 inline-block rounded-2xl border border-white/10 bg-slate-900/60 px-4 py-2 text-sm text-white backdrop-blur-xl transition-all duration-200 hover:bg-white/10"
          >
            {t("summary.open")} →
          </a>
        </>
      ) : null}

      {info?.mocked ? (
        <p className="mt-4 max-w-3xl rounded-2xl border border-amber-300/20 bg-amber-950/30 px-4 py-3 text-sm text-amber-100">
          {t("overview.sample")}
          {info.detail ? ` ${info.detail}` : ""}
        </p>
      ) : null}
    </section>
  );
}

function percent(used: number, total: number) {
  if (!total) {
    return 0;
  }
  return (used / total) * 100;
}

function Row({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="grid gap-1 border-b border-white/10 px-4 py-3 transition-all duration-200 ease-in-out last:border-b-0 hover:bg-white/5 sm:grid-cols-[11rem_1fr] sm:gap-4">
      <dt className="text-xs tracking-wide text-white/50 uppercase">{label}</dt>
      <dd className={mono ? "font-mono text-sm" : "text-sm"}>{value}</dd>
    </div>
  );
}
