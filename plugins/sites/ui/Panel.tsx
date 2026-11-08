import { useState } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { More, SectionHeader, StatGrid } from "@/components/Section.tsx";
import * as ui from "@/components/ui.ts";
import { api } from "@/lib/api.ts";
import { copyText, downloadText } from "@/lib/clipboard.ts";
import { useGet } from "@/lib/useAsync.ts";

type Profile = { kind?: string; mailHost?: string; mailDomains?: string[] };
type Cert = { notAfter?: string; lastRun?: string; lastError?: string; running: boolean };
type Site = { name: string; address: string; profile: Profile; created?: string; cert?: Cert; webUI: string };
type List = { hub: boolean; sites: Site[]; mailDomains: string[]; hetznerToken: boolean; endpoint?: string; detail?: string };
type Added = { site: Site; joinCode: string; script: string; scriptName: string; notes: string[] };

const base = "/api/plugins/sites";

export default function Panel() {
  const { t } = useTranslation();
  const list = useGet<List>(base);
  const [adding, setAdding] = useState<"" | "home" | "play">("");
  const [added, setAdded] = useState<Added | null>(null);
  const [error, setError] = useState("");
  const [renewing, setRenewing] = useState("");
  const data = list.data;

  async function renew(name: string) {
    setRenewing(name);
    try {
      await api(`${base}/${encodeURIComponent(name)}/certificate`, { method: "POST" });
      list.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("sites.failed"));
    } finally {
      setRenewing("");
    }
  }

  return (
    <PanelFrame title={t("sites.title")} description={t("sites.description")} loading={list.loading} error={list.error} onRefresh={list.reload}>
      {data && !data.hub ? <p className={ui.card}>{t("sites.notHub")}</p> : null}
      {data?.hub ? (
        <div className="space-y-4">
          <StatGrid
            items={[
              { label: t("sites.count"), value: String(data.sites.length) },
              { label: t("sites.mailDomains"), value: data.mailDomains.length ? data.mailDomains.join(", ") : "—" },
              { label: t("sites.token"), value: data.hetznerToken ? t("sites.yes") : t("sites.no"), tone: data.hetznerToken ? "text-emerald-300" : "text-amber-200" },
            ]}
          />
          {error ? <p className="text-sm text-debian break-words">{error}</p> : null}
          {added ? <AddedCard added={added} onClose={() => setAdded(null)} /> : null}

          <SectionHeader title={t("sites.list")} hint={t("sites.listHint")}>
            <div className="flex flex-wrap gap-2">
              <button type="button" className={adding === "home" ? ui.button : ui.primary} onClick={() => setAdding((v) => (v === "home" ? "" : "home"))}>
                {adding === "home" ? t("common.cancel") : `+ ${t("sites.addHome")}`}
              </button>
              <button type="button" className={ui.button} onClick={() => setAdding((v) => (v === "play" ? "" : "play"))}>
                {adding === "play" ? t("common.cancel") : `+ ${t("sites.addPlay")}`}
              </button>
            </div>
          </SectionHeader>
          {adding ? (
            <AddForm
              kind={adding}
              list={data}
              onDone={(res) => {
                setAdded(res);
                setAdding("");
                setError("");
                list.reload();
              }}
              onError={setError}
            />
          ) : null}

          {data.sites.length ? (
            <DataTable mobileCards head={[t("sites.name"), t("sites.address"), t("sites.profile"), t("sites.mailHost"), t("sites.cert"), ""]}>
              {data.sites.map((s) => (
                <tr key={s.name}>
                  <td className="px-3 py-2 font-medium">{s.name}</td>
                  <td className="px-3 py-2 font-mono text-xs">
                    <a className="underline-offset-2 hover:underline" href={s.webUI}>
                      {s.address}
                    </a>
                  </td>
                  <td className="px-3 py-2">{profileName(t, s.profile.kind)}</td>
                  <td className="px-3 py-2 font-mono text-xs break-words">{s.profile.mailHost || "—"}</td>
                  <td className="px-3 py-2 text-xs" title={s.cert?.lastError ?? ""}>
                    {certText(t, s)}
                  </td>
                  <td className="px-3 py-2 text-right">
                    {s.profile.mailHost ? (
                      <button type="button" className={ui.link} disabled={renewing !== "" || s.cert?.running} onClick={() => void renew(s.name)}>
                        {t("sites.renew")}
                      </button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </DataTable>
          ) : (
            <p className="text-sm text-white/60">{t("sites.empty")}</p>
          )}

          <More title={t("sites.details")}>
            <ul className="list-disc space-y-1 ps-5 text-sm text-white/70">
              <li>{t("sites.detailSplit")}</li>
              <li>{t("sites.detailMail")}</li>
              <li>{t("sites.detailCert")}</li>
              <li>{t("sites.detailScript")}</li>
            </ul>
          </More>
        </div>
      ) : null}
    </PanelFrame>
  );
}

function profileName(t: (k: string) => string, kind?: string) {
  return kind === "home" ? t("sites.kindHome") : kind === "play" ? t("sites.kindPlay") : t("sites.kindPlain");
}

function certText(t: (k: string, o?: Record<string, string>) => string, s: Site) {
  if (!s.profile.mailHost) return "—";
  if (s.cert?.running) return t("sites.certRunning");
  if (s.cert?.notAfter) return t("sites.certUntil", { date: new Date(s.cert.notAfter).toLocaleDateString() });
  if (s.cert?.lastError) return t("sites.certError");
  return t("sites.certNone");
}

function AddForm({ kind, list, onDone, onError }: { kind: "home" | "play"; list: List; onDone: (r: Added) => void; onError: (e: string) => void }) {
  const { t } = useTranslation();
  const [name, setName] = useState(kind === "home" ? "heim" : "spiele");
  const [endpoint, setEndpoint] = useState(list.endpoint ?? "");
  const [busy, setBusy] = useState(false);
  const mailHost = list.mailDomains.length ? `postfach.${list.mailDomains[0]}` : "";

  async function submit() {
    setBusy(true);
    try {
      onDone(await api<Added>(base, { method: "POST", body: JSON.stringify({ name: name.trim(), profile: kind, endpoint: endpoint.trim() }) }));
    } catch (err: unknown) {
      onError(err instanceof Error ? err.message : t("sites.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      className={ui.card}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <h3 className={ui.cardTitle}>{kind === "home" ? t("sites.addHome") : t("sites.addPlay")}</h3>
      <p className={ui.hint}>{kind === "home" ? t("sites.homeHint") : t("sites.playHint")}</p>
      <div className="mt-3 grid gap-3 md:grid-cols-2">
        <label className="block text-sm">
          <span className={ui.label}>{t("sites.name")}</span>
          <input className={ui.input} value={name} required maxLength={32} pattern="[A-Za-z0-9][A-Za-z0-9_\-]{0,31}" onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="block text-sm">
          <span className={ui.label}>{t("sites.endpoint")}</span>
          <input className={ui.input} value={endpoint} placeholder="vpn.example.com:51820" onChange={(e) => setEndpoint(e.target.value)} />
        </label>
      </div>
      {kind === "home" ? (
        <ul className="mt-3 list-disc space-y-1 ps-5 text-sm text-white/70">
          {mailHost ? (
            <>
              <li>{t("sites.willMail", { domains: list.mailDomains.join(", ") })}</li>
              <li>{t("sites.willHost", { host: mailHost })}</li>
              <li>{list.hetznerToken ? t("sites.willCert") : t("sites.noToken")}</li>
            </>
          ) : (
            <li>{t("sites.noMail")}</li>
          )}
        </ul>
      ) : null}
      <div className="mt-4">
        <button type="submit" className={ui.primary} disabled={busy}>
          {busy ? t("sites.working") : t("sites.create")}
        </button>
      </div>
    </form>
  );
}

function AddedCard({ added, onClose }: { added: Added; onClose: () => void }) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState("");
  return (
    <div className={`${ui.card} border-amber-300/30`}>
      <h3 className={ui.cardTitle}>{t("sites.addedTitle", { name: added.site.name, address: added.site.address })}</h3>
      <p className="mt-2 text-sm text-amber-100">{t("sites.secretWarning")}</p>
      {added.notes.length ? (
        <ul className="mt-3 list-disc space-y-1 ps-5 text-sm text-white/70">
          {added.notes.map((n) => (
            <li key={n}>{n}</li>
          ))}
        </ul>
      ) : null}
      <p className="mt-3 text-sm text-white/80">{t("sites.runHint")}</p>
      <pre className="mt-2 overflow-x-auto rounded-xl bg-slate-950/60 p-3 font-mono text-xs">
        {t("sites.runCommand", { file: added.scriptName })}
      </pre>
      <div className="mt-4 flex flex-wrap gap-2">
        <button type="button" className={ui.primary} onClick={() => downloadText(added.scriptName, added.script)}>
          {t("sites.download")}
        </button>
        <button
          type="button"
          className={ui.button}
          onClick={() => void copyText(added.joinCode).then(() => setCopied("code"))}
        >
          {copied === "code" ? t("sites.copied") : t("sites.copyCode")}
        </button>
        <button type="button" className={ui.button} onClick={onClose}>
          {t("sites.done")}
        </button>
      </div>
      <p className={ui.hint}>{t("sites.codeHint")}</p>
    </div>
  );
}
