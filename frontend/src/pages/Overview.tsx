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

  const monitorIssues = (health.data?.findings ?? []).filter((f) => f.status !== "info");
  const allGood = Boolean(health.data || doctor.data) && monitorIssues.length === 0 && doctorIssues.length === 0;
  // The worse of monitor and doctor decides the colour of the dot.
  const rank: Health["status"][] = ["ok", "info", "warning", "critical"];
  const doctorStatus: Health["status"] = doctor.data?.fails ? "critical" : doctorIssues.length ? "warning" : "ok";
  const monitorStatus = health.data?.status ?? "ok";
  const status = rank[Math.max(rank.indexOf(monitorStatus), rank.indexOf(doctorStatus))];

  return (
    <section>
      <h1 className="text-2xl font-semibold tracking-tight">{t("overview.title")}</h1>
      {info ? (
        <p className="mt-1 text-sm text-muted">
          {info.os} · {t("overview.kernel")} {info.kernel}
        </p>
      ) : null}

      {query.loading && !info ? <p className="mt-6 text-sm text-muted">{t("overview.loading")}</p> : null}
      {query.error ? (
        <p className="mt-6 max-w-xl rounded-2xl border border-white/10 bg-rose-950/40 px-4 py-3 text-sm text-rose-100 backdrop-blur-xl">
          {t("overview.unreachable", { error: query.error })}
        </p>
      ) : null}

      {/* One card for the state of the server: green and quiet while it
          runs on its own, a short list as soon as something needs a look. */}
      {health.data || doctor.data || doctor.loading ? (
        <article className="mt-6 rounded-3xl border border-white/10 bg-slate-900/60 p-5 text-white backdrop-blur-xl">
          <div className="flex flex-wrap items-center gap-4">
            <span className={`h-3 w-3 shrink-0 rounded-full ${healthDot[status]}`} aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <p className="text-lg font-semibold">
                {allGood
                  ? t("overview.allGood")
                  : doctor.loading && !doctor.data && !health.data
                    ? t("common.loading")
                    : t("overview.attention", { count: monitorIssues.length + doctorIssues.length })}
              </p>
              <p className="mt-0.5 hidden text-xs text-white/50 sm:block">{t("overview.doctorHint")}</p>
            </div>
            <button
              type="button"
              onClick={() => {
                doctor.reload();
                reloadHealth();
              }}
              disabled={doctor.loading}
              className="min-h-11 rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm transition-all duration-200 enabled:hover:bg-white/10 disabled:opacity-50"
            >
              {t("overview.doctorOpen")}
            </button>
          </div>
          {monitorIssues.length + doctorIssues.length > 0 ? (
            <ul className="mt-4 grid gap-2">
              {monitorIssues.map((f) => (
                <li key={f.id}>
                  <a
                    href="#/monitor"
                    className="flex flex-wrap items-center gap-2 rounded-2xl border border-white/10 bg-slate-950/40 px-3 py-2 text-sm transition-all duration-200 hover:bg-white/5"
                  >
                    <span className={`rounded-full px-2 py-0.5 text-xs ${doctorTone[f.status === "critical" ? "FAIL" : "WARN"]}`}>
                      {t("monitor.tile")}
                    </span>
                    <span className="min-w-0 flex-1">{f.title}</span>
                  </a>
                </li>
              ))}
              {doctorIssues.slice(0, 12).map((f, i) => (
                <li key={`${f.area}-${f.text}-${i}`} className="flex flex-wrap items-center gap-2 rounded-2xl border border-white/10 bg-slate-950/40 px-3 py-2 text-sm">
                  <span className={`rounded-full px-2 py-0.5 text-xs ${doctorTone[f.level] ?? doctorTone.INFO}`}>{f.area}</span>
                  <span className="min-w-0 flex-1">{f.text}</span>
                </li>
              ))}
            </ul>
          ) : null}
        </article>
      ) : null}

      {info ? (
        <div className="mt-4 grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
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
          <article className="rounded-3xl border border-white/10 bg-slate-900/60 p-5 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out hover:-translate-y-0.5"
          >
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
      ) : null}

      {info?.mocked ? (
        <p className="mt-4 rounded-2xl border border-amber-300/20 bg-amber-950/30 px-4 py-3 text-sm text-amber-100">
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
