import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";
import * as ui from "@/components/ui.ts";
import { More, SectionHeader } from "@/components/Section.tsx";

type Settings = {
  tokenSet: boolean;
  serverId?: number;
  tokenHint?: string;
};

type Domain = {
  name: string;
  mode: "manual" | "hetzner";
  created: string;
};

type Record = {
  zone: string;
  name: string;
  type: string;
  value: string;
  kind: string;
  source: string;
  zoneLine: string;
  status: string;
  found?: string;
  managed: boolean;
};

type Status = {
  settings: Settings;
  domains: Domain[];
  publicIp: string;
};

const base = "/api/plugins/domains";
const card = `${ui.card} mb-6`;
const button = ui.button;
const primary = ui.primary;
const input = ui.input;

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<Status>(`${base}/status`);
  const data = query.data;
  const [selected, setSelected] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);

  if (selected) {
    return (
      <DomainView
        name={selected}
        mode={
          data?.domains.find((d) => d.name === selected)?.mode ?? "manual"
        }
        tokenSet={data?.settings.tokenSet ?? false}
        onBack={() => setSelected(null)}
        onChanged={query.reload}
      />
    );
  }

  async function addDomain(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains`, {
        method: "POST",
        body: JSON.stringify({
          name: String(fd.get("name") ?? ""),
          mode: String(fd.get("mode") ?? "manual"),
        }),
      });
      e.currentTarget.reset();
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function saveToken(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    const fd = new FormData(e.currentTarget);
    setBusy(true);
    setError("");
    try {
      await api(`${base}/token`, {
        method: "POST",
        body: JSON.stringify({ token: String(fd.get("token") ?? "") }),
      });
      e.currentTarget.reset();
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function clearToken() {
    setBusy(true);
    setError("");
    try {
      await api(`${base}/token`, {
        method: "POST",
        body: JSON.stringify({ clear: true }),
      });
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  const tokenCard = (
  <section className={ui.card}>
    <h2 className="mb-2 text-base font-semibold">{t("domains.hetzner")}</h2>
    <p className="mb-3 text-sm text-white/70">{t("domains.hetznerHint")}</p>
    {data?.settings.tokenSet ? (
      <div className="mb-3 flex flex-wrap items-center gap-3 text-sm">
        <span>
          {t("domains.tokenSet", { hint: data.settings.tokenHint ?? "····" })}
        </span>
        <button type="button" className={button} disabled={busy} onClick={clearToken}>
          {t("domains.tokenClear")}
        </button>
      </div>
    ) : null}
    <form onSubmit={saveToken} className="grid gap-3 md:grid-cols-[1fr_auto]">
      <input
        name="token"
        type="password"
        autoComplete="off"
        required={!data?.settings.tokenSet}
        placeholder={t("domains.tokenPlaceholder")}
        className={input}
      />
      <button type="submit" className={primary} disabled={busy}>
        {t("domains.tokenSave")}
      </button>
    </form>
  </section>
  );

  return (
    <PanelFrame
      title={t("domains.title")}
      description={t("domains.description")}
      loading={query.loading && !data}
      error={error || query.error}
      onRefresh={query.reload}
    >
      <SectionHeader title={t("domains.list")} />
      <section>
        <DataTable head={[t("domains.name"), t("domains.mode"), ""]}>
          {(data?.domains ?? []).map((d) => (
            <tr key={d.name}>
              <td className="px-3 py-2 font-mono">{d.name}</td>
              <td className="px-3 py-2">{t(`domains.modes.${d.mode}`)}</td>
              <td className="px-3 py-2 text-right">
                <button type="button" className={button} onClick={() => setSelected(d.name)}>
                  {t("domains.open")}
                </button>
              </td>
            </tr>
          ))}
          {(data?.domains ?? []).length === 0 ? (
            <tr>
              <td colSpan={3} className="px-3 py-3 text-white/50">
                {t("domains.empty")}
              </td>
            </tr>
          ) : null}
        </DataTable>
        <form onSubmit={addDomain} className="mt-4 grid gap-3 md:grid-cols-[1fr_auto_auto]">
          <input
            name="name"
            required
            placeholder="example.com"
            className={input}
            autoComplete="off"
          />
          <select name="mode" className={input} defaultValue="manual">
            <option value="manual">{t("domains.modes.manual")}</option>
            <option value="hetzner">{t("domains.modes.hetzner")}</option>
          </select>
          <button type="submit" className={primary} disabled={busy}>
            {t("domains.add")}
          </button>
        </form>
      </section>
      {data?.settings.tokenSet ? (
        <More title={t("domains.hetzner")}>
          {tokenCard}
        </More>
      ) : (
        <div className="mt-6">
          {tokenCard}
        </div>
      )}
    </PanelFrame>
  );
}

function DomainView({
  name,
  mode,
  tokenSet,
  onBack,
  onChanged,
}: {
  name: string;
  mode: string;
  tokenSet: boolean;
  onBack: () => void;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const query = useGet<{
    domain: Domain;
    records: Record[];
    hetznerError?: string;
    zoneMissing?: boolean;
  }>(
    `${base}/domains/${encodeURIComponent(name)}`,
  );
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const records = query.data?.records ?? [];

  async function setMode(next: string) {
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${encodeURIComponent(name)}`, {
        method: "POST",
        body: JSON.stringify({ mode: next }),
      });
      query.reload();
      onChanged();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function apply() {
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${encodeURIComponent(name)}/apply`, {
        method: "POST",
        body: "{}",
      });
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (!window.confirm(t("domains.deleteConfirm", { name }))) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${encodeURIComponent(name)}/delete`, {
        method: "POST",
        body: "{}",
      });
      onChanged();
      onBack();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("domains.failed"));
    } finally {
      setBusy(false);
    }
  }

  return (
    <PanelFrame
      title={name}
      description={t("domains.domainLead")}
      loading={query.loading && !query.data}
      error={error || query.error}
      onRefresh={query.reload}
    >
      <button type="button" className="mb-3 text-sm text-muted hover:text-debian" onClick={onBack}>
        ← {t("domains.title")}
      </button>

      <section className={card}>
        <div className="flex flex-wrap items-center gap-3">
          <label className="text-sm text-white/70">{t("domains.mode")}</label>
          <select
            className={input + " max-w-xs"}
            value={query.data?.domain.mode ?? mode}
            disabled={busy}
            onChange={(e) => setMode(e.target.value)}
          >
            <option value="manual">{t("domains.modes.manual")}</option>
            <option value="hetzner">{t("domains.modes.hetzner")}</option>
          </select>
          {(query.data?.domain.mode ?? mode) === "hetzner" ? (
            <button type="button" className={primary} disabled={busy || !tokenSet} onClick={apply}>
              {t("domains.apply")}
            </button>
          ) : null}
          <button type="button" className={button} disabled={busy} onClick={() => query.reload()}>
            {t("common.refresh")}
          </button>
          <button type="button" className={button} disabled={busy} onClick={remove}>
            {t("domains.delete")}
          </button>
        </div>
        {!tokenSet && (query.data?.domain.mode ?? mode) === "hetzner" ? (
          <p className="mt-3 text-sm text-amber-200">{t("domains.needToken")}</p>
        ) : null}
        {query.data?.zoneMissing ? (
          <p className="mt-3 rounded-2xl border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-100">
            {t("domains.zoneMissing", { name })}
          </p>
        ) : query.data?.hetznerError ? (
          <p className="mt-3 rounded-2xl border border-amber-400/30 bg-amber-500/10 p-3 text-sm text-amber-100">
            {t("domains.hetznerUnreachable", { error: query.data.hetznerError })}
          </p>
        ) : null}
        <p className="mt-3 text-sm text-white/60">{t("domains.recordsHint")}</p>
      </section>

      <section className="grid gap-3">
        {records.length === 0 ? (
          <p className="text-sm text-white/50">{t("domains.noRecords")}</p>
        ) : (
          records.map((r) => (
            <article key={`${r.kind}-${r.name}-${r.type}`} className={card + " mb-0"}>
              <div className="mb-2 flex flex-wrap items-center gap-2">
                <span className="font-semibold">{t(`domains.kinds.${r.kind}`, r.kind)}</span>
                <StatusBadge status={r.status} />
                {r.managed ? (
                  <span className="rounded-full bg-emerald-500/20 px-2 py-0.5 text-xs text-emerald-200">
                    {t("domains.managed")}
                  </span>
                ) : null}
                <span className="text-xs text-white/40">{r.source}</span>
              </div>
              <dl className="grid gap-1 text-sm md:grid-cols-[6rem_1fr]">
                <dt className="text-white/50">{t("domains.dnsName")}</dt>
                <dd className="break-all font-mono">{r.name}</dd>
                <dt className="text-white/50">{t("domains.dnsType")}</dt>
                <dd className="font-mono">{r.type}</dd>
                <dt className="text-white/50">{t("domains.dnsValue")}</dt>
                <dd className="break-all font-mono">{r.value}</dd>
                {r.found ? (
                  <>
                    <dt className="text-white/50">{t("domains.dnsFound")}</dt>
                    <dd className="break-all font-mono text-white/70">{r.found}</dd>
                  </>
                ) : null}
              </dl>
            </article>
          ))
        )}
      </section>
    </PanelFrame>
  );
}

function StatusBadge({ status }: { status: string }) {
  const { t } = useTranslation();
  const tone =
    status === "ok"
      ? "bg-emerald-500/20 text-emerald-200"
      : status === "missing"
        ? "bg-amber-500/20 text-amber-100"
        : status === "conflict"
          ? "bg-rose-500/20 text-rose-200"
          : "bg-white/10 text-white/70";
  return (
    <span className={`rounded-full px-2 py-0.5 text-xs ${tone}`}>
      {t(`domains.status.${status}`, status)}
    </span>
  );
}
