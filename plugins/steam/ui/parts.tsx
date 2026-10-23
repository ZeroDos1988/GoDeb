import type { ReactNode } from "react";

export const button =
  "rounded-2xl border border-white/10 bg-slate-900/60 px-3 py-2 text-sm text-white backdrop-blur-xl transition-all duration-200 ease-in-out enabled:hover:bg-white/10 disabled:opacity-50";
export const primary =
  "rounded-2xl bg-debian px-4 py-2 text-sm font-medium text-white transition-all duration-200 ease-in-out enabled:hover:opacity-90 disabled:opacity-50";
export const card = "rounded-3xl border border-white/10 bg-slate-900/60 p-4 text-white backdrop-blur-xl";
export const input =
  "w-full rounded-2xl border border-white/10 bg-slate-950/40 px-3 py-2 text-sm text-white outline-none transition-all duration-200 ease-in-out focus:border-white/30";

export function Stat({ label, value, ok, hint }: { label: string; value: string; ok?: boolean; hint?: string }) {
  return (
    <div className={card}>
      <div className="text-[11px] tracking-wide text-white/50 uppercase">{label}</div>
      <div className={`mt-1 text-sm font-medium ${ok === undefined ? "" : ok ? "text-emerald-400" : "text-white/80"}`}>{value}</div>
      {hint ? <div className="mt-1 font-mono text-xs break-all text-white/50">{hint}</div> : null}
    </div>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block min-w-0 text-sm">
      <span className="mb-1 block text-xs tracking-wide text-white/60 uppercase">{label}</span>
      {children}
    </label>
  );
}

export function Toggle({ label, checked, onChange }: { label: string; checked: boolean; onChange: (value: boolean) => void }) {
  return (
    <label className="inline-flex cursor-pointer items-center gap-2">
      <input type="checkbox" className="h-4 w-4 accent-[var(--color-primary)]" checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span>{label}</span>
    </label>
  );
}

export function randomPassword() {
  const alphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789";
  // Rejection sampling keeps every symbol equally likely.
  const limit = 256 - (256 % alphabet.length);
  let out = "";
  while (out.length < 16) {
    for (const b of crypto.getRandomValues(new Uint8Array(32))) {
      if (b < limit && out.length < 16) {
        out += alphabet[b % alphabet.length];
      }
    }
  }
  return out;
}
