import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api.ts";
import { copyText, downloadText } from "@/lib/clipboard.ts";

// ClientResult is a client created on the hub during the setup
// (firstrun.ClientResult). Config, QR and code all hold its private key.
export type ClientResult = {
  name: string;
  site: boolean;
  address: string;
  config: string;
  qr?: string;
  code?: string;
  detail?: string;
};

const glass =
  "rounded-2xl border border-white/10 bg-white/10 px-3 py-1.5 text-sm text-white transition-all duration-200 ease-in-out enabled:hover:bg-white/15 disabled:opacity-50";
const input =
  "w-full rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white transition-all duration-200 ease-in-out outline-none focus:border-white/30";

// TunnelClients lists the clients created in the wizard and adds more with
// "+": phones and laptops get a QR code and the file, GoDeb hosts at home
// (sites) get a join code to paste into "Mit Hub verbinden" there.
export function TunnelClients({ endpoint, clients, onAdded }: { endpoint: string; clients: ClientResult[]; onAdded: (c: ClientResult) => void }) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  return (
    <section className="mt-5 space-y-3">
      <div>
        <h2 className="text-sm font-semibold">{t("clients.title")}</h2>
        <p className="mt-1 text-xs text-white/60">{t("clients.text")}</p>
      </div>
      {clients.map((c, i) => (
        <ClientCard key={c.name} client={c} open={i === clients.length - 1} />
      ))}
      {adding ? (
        <AddClient
          endpoint={endpoint}
          taken={clients.map((c) => c.name)}
          hasSite={clients.some((c) => c.site)}
          onCancel={() => setAdding(false)}
          onAdded={(c) => {
            setAdding(false);
            onAdded(c);
          }}
        />
      ) : (
        <button type="button" className={`${glass} w-full border-dashed py-2.5`} onClick={() => setAdding(true)}>
          {t("clients.add")}
        </button>
      )}
    </section>
  );
}

function ClientCard({ client, open }: { client: ClientResult; open: boolean }) {
  const { t } = useTranslation();
  return (
    <details open={open} className="group rounded-2xl border border-white/10 bg-white/5 px-3 py-2">
      <summary className="flex cursor-pointer list-none flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <span className="text-white/40 transition-transform duration-200 group-open:rotate-90">›</span>
        <span className="font-medium">{client.name}</span>
        <span className="font-mono text-xs text-white/60">{client.address}</span>
        <span className="rounded-full border border-white/15 px-2 py-0.5 text-[11px] text-white/70">
          {client.site ? t("clients.kindSite") : t("clients.kindDevice")}
        </span>
      </summary>
      <div className="mt-3 pb-1">
        {client.site ? (
          <JoinCodeBox code={client.code ?? ""} filename={`${client.name}.conf`} config={client.config} />
        ) : (
          <div className="flex flex-col gap-4 sm:flex-row sm:items-start">
            {client.qr ? (
              <img
                src={client.qr}
                alt={t("setup.qrAlt", { name: client.name })}
                className="mx-auto w-56 max-w-full shrink-0 rounded-2xl bg-white p-2 [image-rendering:pixelated] sm:mx-0"
              />
            ) : null}
            <div className="min-w-0 space-y-3 text-xs text-white/70">
              <p>{t("clients.deviceHint")}</p>
              <button type="button" className={glass} onClick={() => downloadText(`${client.name}.conf`, client.config)}>
                {t("clients.download")}
              </button>
              <p className="text-amber-200/90">{t("clients.secret")}</p>
            </div>
          </div>
        )}
        {client.detail ? <p className="mt-2 text-xs text-amber-200">{client.detail}</p> : null}
      </div>
    </details>
  );
}

// JoinCodeBox shows the join code of a site with copy and download. Also
// used by the WireGuard view for sites created there.
export function JoinCodeBox({ code, filename, config }: { code: string; filename: string; config?: string }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  async function copy() {
    await copyText(code);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }
  return (
    <div className="space-y-3 text-xs text-white/70">
      <p>{t("clients.siteHint")}</p>
      <label className="block">
        <span className="mb-1 block text-[11px] tracking-wide text-white/50 uppercase">{t("clients.code")}</span>
        <textarea
          readOnly
          value={code}
          rows={4}
          spellCheck={false}
          onFocus={(e) => e.currentTarget.select()}
          className={`${input} resize-none font-mono text-[11px] break-all`}
        />
      </label>
      <div className="flex flex-wrap gap-2">
        <button type="button" className={glass} onClick={() => void copy()} disabled={!code}>
          {copied ? t("common.copied") : t("clients.copyCode")}
        </button>
        {config ? (
          <button type="button" className={glass} onClick={() => downloadText(filename, config)}>
            {t("clients.download")}
          </button>
        ) : null}
      </div>
      <p>
        {t("clients.siteTerminal")} <code className="rounded bg-black/30 px-1.5 py-0.5 font-mono text-white">sudo godeb setup --join -</code>
      </p>
      <p className="text-amber-200/90">{t("clients.secret")}</p>
    </div>
  );
}

function AddClient({
  endpoint,
  taken,
  hasSite,
  onCancel,
  onAdded,
}: {
  endpoint: string;
  taken: string[];
  hasSite: boolean;
  onCancel: () => void;
  onAdded: (c: ClientResult) => void;
}) {
  const { t } = useTranslation();
  const [site, setSite] = useState(!hasSite);
  const [name, setName] = useState(() => suggest(!hasSite, taken));
  const [address, setAddress] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  function choose(next: boolean) {
    setSite(next);
    setError("");
    // Replace the suggestion, keep what the admin typed.
    if (name === suggest(site, taken)) {
      setName(suggest(next, taken));
    }
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      onAdded(
        await api<ClientResult>("/api/setup/client", {
          method: "POST",
          body: JSON.stringify({ name: name.trim(), site, address: site ? address.trim() : "", endpoint }),
        }),
      );
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("setup.failed"));
    } finally {
      setBusy(false);
    }
  }

  const kinds = [
    { site: false, title: t("clients.kindDevice"), text: t("clients.kindDeviceText") },
    { site: true, title: t("clients.kindSite"), text: t("clients.kindSiteText") },
  ];
  return (
    <form onSubmit={(e) => void submit(e)} className="space-y-3 rounded-2xl border border-white/20 bg-white/5 p-3">
      <div className="grid gap-2 sm:grid-cols-2">
        {kinds.map((k) => (
          <button
            key={String(k.site)}
            type="button"
            aria-pressed={site === k.site}
            onClick={() => choose(k.site)}
            className={`rounded-2xl border px-3 py-2.5 text-left text-sm transition-all duration-200 ease-in-out ${site === k.site ? "border-white/40 bg-white/15" : "border-white/10 bg-white/5 hover:bg-white/10"}`}
          >
            <span className="block font-medium">{k.title}</span>
            <span className="mt-0.5 block text-xs text-white/60">{k.text}</span>
          </button>
        ))}
      </div>
      <div className="grid gap-3 md:grid-cols-2">
        <label className="block min-w-0 text-sm">
          <span className="mb-1 block text-[11px] tracking-wide text-white/50 uppercase">{t("clients.name")}</span>
          <input value={name} onChange={(e) => setName(e.target.value)} required maxLength={32} pattern="[A-Za-z0-9_\-]+" className={input} />
        </label>
        {site ? (
          <label className="block min-w-0 text-sm">
            <span className="mb-1 block text-[11px] tracking-wide text-white/50 uppercase">{t("clients.address")}</span>
            <input value={address} onChange={(e) => setAddress(e.target.value)} placeholder={t("clients.addressAuto")} className={`${input} font-mono`} />
          </label>
        ) : null}
      </div>
      {error ? <p className="text-sm text-debian">{error}</p> : null}
      <div className="flex flex-wrap gap-2">
        <button type="submit" disabled={busy || name.trim() === ""} className="rounded-2xl bg-debian px-4 py-2 text-sm text-white transition-all duration-200 ease-in-out enabled:hover:opacity-90 disabled:opacity-50">
          {busy ? t("setup.working") : t("clients.create")}
        </button>
        <button type="button" className={glass} onClick={onCancel} disabled={busy}>
          {t("common.cancel")}
        </button>
      </div>
    </form>
  );
}

// suggest proposes a free name: heim-vm for the first site, laptop for a
// device, with a number if it is taken.
function suggest(site: boolean, taken: string[]): string {
  const base = site ? "heim-vm" : "laptop";
  const used = new Set(taken.map((n) => n.toLowerCase()));
  if (!used.has(base)) {
    return base;
  }
  for (let i = 2; ; i++) {
    if (!used.has(`${base}-${i}`)) {
      return `${base}-${i}`;
    }
  }
}
