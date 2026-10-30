import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { copyText } from "@/lib/clipboard.ts";
import * as ui from "@/components/ui.ts";

export type Settings = {
  hostname: string;
  clamav: boolean;
  certFile: string;
  keyFile: string;
};

export type Domain = {
  name: string;
  selector: string;
  dkimPublic: string;
  catchAll: string;
  suspended: boolean;
  created: string;
};

export type Account = {
  domain: string;
  local: string;
  address: string;
  quotaMB: number;
  aliases: string[];
  forward: string[];
  forwardOnly: boolean;
  suspended: boolean;
  vacation: boolean;
  vacationSubject: string;
  vacationBody: string;
  vacationDays: number;
  created: string;
  usedKB: number;
};

export type DnsRecord = {
  kind:
    | "mx"
    | "spf"
    | "dkim"
    | "dmarc"
    | "mtasts"
    | "tlsrpt"
    | "a"
    | "mtasts-host"
    | "autoconfig"
    | "autodiscover"
    | "ptr";
  name: string;
  type: string;
  value: string;
  zone: string;
  status: "ok" | "missing" | "wrong" | "unknown";
  found?: string;
};

export type Job = {
  action: string;
  running: boolean;
  ok: boolean;
  error?: string;
  started: string;
  finished?: string;
  log: string[];
};

export type Status = {
  installed: boolean;
  settings: Settings;
  letsEncrypt: boolean;
  publicIp: string;
  hostResolves: boolean;
  gatewayDomains: number;
  services: { unit: string; active: string }[];
  domains: (Domain & { accounts: number })[];
  job?: Job;
};

export const base = "/api/plugins/mail";

export const card = ui.card;
export const input = ui.input;
export const button = ui.button;
export const primary = ui.primary;
export const danger = ui.danger;

export function Field({
  label,
  hint,
  children,
}: {
  label: string;
  hint?: string;
  children: ReactNode;
}) {
  return (
    <label className="block min-w-0 text-sm">
      <span className="mb-1 block text-[11px] tracking-wide text-white/50 uppercase">
        {label}
      </span>
      {children}
      {hint ? (
        <span className="mt-1 block text-xs text-white/50">{hint}</span>
      ) : null}
    </label>
  );
}

export function Badge({
  tone,
  children,
}: {
  tone: "ok" | "warn" | "bad" | "idle";
  children: ReactNode;
}) {
  const cls = {
    ok: "border-emerald-300/30 bg-emerald-400/10 text-emerald-200",
    warn: "border-amber-300/30 bg-amber-400/10 text-amber-200",
    bad: "border-rose-300/30 bg-rose-400/10 text-rose-200",
    idle: "border-white/15 bg-white/5 text-white/60",
  }[tone];
  return (
    <span
      className={`inline-flex items-center rounded-full border px-2 py-0.5 text-[11px] whitespace-nowrap ${cls}`}
    >
      {children}
    </span>
  );
}

export function CopyButton({ text, label }: { text: string; label?: string }) {
  const { t } = useTranslation();
  const [done, setDone] = useState(false);
  return (
    <button
      type="button"
      className="shrink-0 rounded-xl border border-white/10 bg-white/5 px-2 py-0.5 text-xs text-white/80 transition-all duration-200 hover:bg-white/15"
      onClick={() => {
        void copyText(text).then(() => {
          setDone(true);
          window.setTimeout(() => setDone(false), 1500);
        });
      }}
    >
      {done ? t("common.copied") : (label ?? t("common.copy"))}
    </button>
  );
}

export function JobCard({ job }: { job: Job }) {
  const { t } = useTranslation();
  const tone = job.running
    ? "text-amber-200"
    : job.ok
      ? "text-emerald-300"
      : "text-rose-300";
  return (
    <section className={`${card} mt-4`}>
      <div className="flex flex-wrap items-baseline gap-x-3">
        <h2 className="text-sm font-semibold">
          {t(`mail.jobs.${job.action}`, { defaultValue: job.action })}
        </h2>
        <span className={`text-sm ${tone}`}>
          {job.running
            ? t("mail.running")
            : job.ok
              ? t("mail.done")
              : (job.error ?? t("mail.failed"))}
        </span>
      </div>
      <pre className="mt-3 max-h-80 overflow-auto rounded-2xl bg-slate-950/60 p-3 font-mono text-[11px] whitespace-pre-wrap text-white/75">
        {job.log.join("\n")}
      </pre>
    </section>
  );
}

export function errorText(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback;
}

export function size(kb: number): string {
  if (kb < 0) return "–";
  if (kb < 1024) return `${kb} KB`;
  if (kb < 1024 * 1024) return `${(kb / 1024).toFixed(kb < 10240 ? 1 : 0)} MB`;
  return `${(kb / 1024 / 1024).toFixed(1)} GB`;
}

// randomPassword uses the browser's CSPRNG: 20 characters without
// look-alikes, rejection sampling so every character is equally likely.
export function randomPassword(): string {
  const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  const limit = 256 - (256 % alphabet.length);
  let out = "";
  while (out.length < 20) {
    const bytes = new Uint8Array(32);
    crypto.getRandomValues(bytes);
    for (const b of bytes) {
      if (b < limit && out.length < 20) {
        out += alphabet[b % alphabet.length];
      }
    }
  }
  return out;
}
