import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";

type Domain = {
  domain: string;
  selector: string;
  dkimPublic: string;
  created: string;
};

type Status = {
  relayHost: string;
  domains: Domain[];
  tunnelUp: boolean;
  localTunnel: boolean;
  mode: "relay" | "buffer" | "idle";
  publicIP: string;
  command?: string[];
  mocked?: boolean;
  detail?: string;
};

type QueueMessage = {
  id: string;
  sender: string;
  recipients: string[];
  arrival: string;
  status: string;
  reason?: string;
  size: number;
};

type Queue = {
  messages: QueueMessage[];
  count: number;
  command?: string[];
  mocked?: boolean;
  detail?: string;
};

type Check = {
  id: "ptr" | "spf" | "dkim" | "dmarc";
  status: "ok" | "warn" | "bad";
  found: string;
  expected: string;
  record: string;
};

type DnsReport = {
  domain: string;
  publicIP: string;
  checks: Check[];
};

const glass =
  "rounded-3xl border border-white/10 bg-slate-900/60 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out";

export default function MailGatewayDashboard() {
  const { t } = useTranslation();
  const status = useGet<Status>("/api/plugins/mailgateway/status");
  const queue = useGet<Queue>("/api/plugins/mailgateway/queue");
  const [domain, setDomain] = useState("");
  const [relayHost, setRelayHost] = useState("10.8.0.2");
  const [selector, setSelector] = useState("godeb");
  const [busy, setBusy] = useState("");
  const [notice, setNotice] = useState("");
  const [actionError, setActionError] = useState("");
  const [copied, setCopied] = useState("");
  const [dns, setDns] = useState<DnsReport | null>(null);

  const activeDomain = domain || status.data?.domains?.[0]?.domain || "";

  useEffect(() => {
    const timer = window.setInterval(() => queue.reload(), 10000);
    return () => window.clearInterval(timer);
  }, [queue.reload]);

  useEffect(() => {
    const current = status.data;
    if (!current) {
      return;
    }
    const first = current.domains[0];
    if (first) {
      setDomain((value) => value || first.domain);
      setSelector((value) => (value === "godeb" && first.selector ? first.selector : value));
    }
    if (current.relayHost) {
      setRelayHost((value) => (value === "10.8.0.2" ? current.relayHost : value));
    }
  }, [status.data]);

  useEffect(() => {
    if (!activeDomain) {
      setDns(null);
      return;
    }
    let cancelled = false;
    api<DnsReport>(`/api/plugins/mailgateway/dns-check?domain=${encodeURIComponent(activeDomain)}`)
      .then((report) => {
        if (!cancelled) {
          setDns(report);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setDns(null);
        }
      });
    return () => {
      cancelled = true;
    };
  }, [activeDomain, notice]);

  async function setup(event: FormEvent) {
    event.preventDefault();
    setBusy("setup");
    setActionError("");
    setNotice("");
    try {
      await api("/api/plugins/mailgateway/setup", {
        method: "POST",
        body: JSON.stringify({ domain, relayHost, selector }),
      });
      setNotice(t("mailgw.applied", { domain }));
      status.reload();
      queue.reload();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : t("mailgw.setupFailed"));
    } finally {
      setBusy("");
    }
  }

  async function flush() {
    setBusy("flush");
    setActionError("");
    setNotice("");
    try {
      await api("/api/plugins/mailgateway/queue/flush", { method: "POST", body: "{}" });
      setNotice(t("mailgw.flushed"));
      queue.reload();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : t("mailgw.flushFailed"));
    } finally {
      setBusy("");
    }
  }

  async function copy(id: string, value: string) {
    await navigator.clipboard.writeText(value);
    setCopied(id);
    window.setTimeout(() => setCopied(""), 2000);
  }

  const mode = status.data?.mode ?? "idle";

  return (
    <PanelFrame
      title={t("mailgw.title")}
      description={t("mailgw.description")}
      command={queue.data?.command}
      mocked={queue.data?.mocked || status.data?.mocked}
      detail={queue.data?.detail || status.data?.detail}
      loading={status.loading && !status.data}
      error={status.error || queue.error}
      onRefresh={() => {
        status.reload();
        queue.reload();
      }}
    >
      <div className="grid gap-4 lg:grid-cols-[minmax(0,1.1fr)_minmax(0,0.9fr)]">
        <article className={`${glass} p-5`}>
          <div className="text-xs tracking-wide text-white/60 uppercase">{t("mailgw.tunnel")}</div>
          <div className="mt-3 flex flex-wrap items-center gap-2">
            <Light tone={status.data?.localTunnel ? "ok" : "bad"}>{status.data?.localTunnel ? t("mailgw.tunnelLocal") : t("mailgw.tunnelMissing")}</Light>
            <Light tone={mode === "relay" ? "ok" : mode === "buffer" ? "warn" : "idle"}>
              {t(`mailgw.mode.${mode}`)}
            </Light>
          </div>
          <p className="mt-3 text-sm text-white/70">
            {t("mailgw.relayHint", { host: status.data?.relayHost || relayHost })}
          </p>
          <form onSubmit={(event) => void setup(event)} className="mt-4 grid gap-3 sm:grid-cols-2">
            <Field label={t("mailgw.domain")} value={domain} onChange={setDomain} placeholder="example.org" />
            <Field label={t("mailgw.relay")} value={relayHost} onChange={setRelayHost} placeholder="10.8.0.2" />
            <Field label={t("mailgw.selector")} value={selector} onChange={setSelector} placeholder="godeb" />
            <div className="flex items-end">
              <button type="submit" disabled={busy !== "" || domain.trim() === ""} className="rounded-2xl bg-debian px-4 py-2 text-sm text-white transition-all duration-200 ease-in-out enabled:hover:opacity-90 disabled:opacity-50">
                {busy === "setup" ? t("mailgw.applying") : t("mailgw.apply")}
              </button>
            </div>
          </form>
        </article>

        <article className={`${glass} p-5`}>
          <div className="flex items-center justify-between gap-3">
            <div className="text-xs tracking-wide text-white/60 uppercase">{t("mailgw.health")}</div>
            <div className="font-mono text-xs text-white/50">{dns?.publicIP || status.data?.publicIP || "—"}</div>
          </div>
          <div className="mt-3 grid gap-2">
            {(dns?.checks ?? []).map((check) => (
              <div key={check.id} className="rounded-2xl border border-white/10 bg-white/5 px-3 py-2">
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2">
                    <Light tone={check.status}>{t(`mailgw.check.${check.id}`)}</Light>
                  </div>
                  <button type="button" onClick={() => void copy(check.id, check.record)} className="text-xs text-white/70 transition-all duration-200 ease-in-out hover:text-white">
                    {copied === check.id ? t("common.copied") : t("common.copy")}
                  </button>
                </div>
                <p className="mt-2 font-mono text-[11px] break-all text-white/80">{check.record}</p>
                <p className="mt-1 text-xs text-white/50">{check.found}</p>
              </div>
            ))}
            {!dns ? <p className="text-sm text-white/60">{t("mailgw.noDomain")}</p> : null}
          </div>
        </article>
      </div>

      <div className="mt-6 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-sm font-semibold">{t("mailgw.queue", { count: queue.data?.count ?? 0 })}</h2>
        <button type="button" onClick={() => void flush()} disabled={busy !== ""} className="rounded-2xl border border-white/10 bg-slate-900/60 px-3 py-1.5 text-sm text-white backdrop-blur-xl transition-all duration-200 ease-in-out enabled:hover:bg-white/10 disabled:opacity-50">
          {busy === "flush" ? t("mailgw.flushing") : t("mailgw.flush")}
        </button>
      </div>
      {notice ? <p className="mt-3 text-sm text-emerald-300">{notice}</p> : null}
      {actionError ? <p className="mt-3 text-sm text-rose-300">{actionError}</p> : null}
      {queue.data && queue.data.messages.length === 0 ? <p className="mt-3 text-sm text-muted">{t("mailgw.empty")}</p> : null}
      {queue.data && queue.data.messages.length > 0 ? (
        <div className="mt-3">
          <DataTable head={[t("mailgw.sender"), t("mailgw.recipient"), t("mailgw.time"), t("mailgw.status"), t("mailgw.reason")]}>
            {queue.data.messages.map((message) => (
              <tr key={message.id}>
                <td className="px-3 py-2 font-mono text-xs">{message.sender || "—"}</td>
                <td className="px-3 py-2 font-mono text-xs">{message.recipients.join(", ") || "—"}</td>
                <td className="px-3 py-2 text-xs whitespace-nowrap">{message.arrival || "—"}</td>
                <td className="px-3 py-2 text-xs">{message.status}</td>
                <td className="px-3 py-2 text-xs text-white/70">{message.reason || "—"}</td>
              </tr>
            ))}
          </DataTable>
        </div>
      ) : null}
    </PanelFrame>
  );
}

function Field({
  label,
  value,
  onChange,
  placeholder,
}: {
  label: string;
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
}) {
  return (
    <label className="text-sm">
      <span className="mb-1 block text-xs tracking-wide text-white/50 uppercase">{label}</span>
      <input
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        className="w-full rounded-2xl border border-white/10 bg-white/5 px-3 py-1.5 font-mono text-sm text-white transition-all duration-200 ease-in-out outline-none"
      />
    </label>
  );
}

function Light({ tone, children }: { tone: "ok" | "warn" | "bad" | "idle"; children: string }) {
  const toneClass = {
    ok: "bg-emerald-400 shadow-[0_0_12px_rgb(52_211_153/0.8)]",
    warn: "bg-amber-300 shadow-[0_0_12px_rgb(252_211_77/0.7)]",
    bad: "bg-rose-400 shadow-[0_0_12px_rgb(251_113_133/0.8)]",
    idle: "bg-white/40",
  }[tone];
  return (
    <span className="inline-flex items-center gap-2 rounded-full border border-white/10 bg-white/5 px-2.5 py-1 text-xs">
      <span className={`h-2.5 w-2.5 rounded-full ${tone === "idle" ? "" : "animate-pulse"} ${toneClass}`} />
      {children}
    </span>
  );
}
