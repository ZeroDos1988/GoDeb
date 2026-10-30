import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import type { CommandEnvelope } from "@/lib/types.ts";
import { useGet } from "@/lib/useAsync.ts";
import * as ui from "@/components/ui.ts";
import { SectionHeader, StatGrid } from "@/components/Section.tsx";

type Rule = {
  name: string;
  proto: "udp" | "tcp";
  from: number;
  to: number;
  target: string;
};

type Target = {
  address: string;
  name: string;
  site: boolean;
};

type Status = CommandEnvelope & {
  rules: Rule[];
  publicIf: string;
  publicIp: string;
  hubAddress: string;
  network: string;
  targets: Target[];
  forwarding: boolean;
  tableLive: boolean;
  unit: string;
};

const statusPath = "/api/plugins/portforward/status";
const button = ui.button;
const primary = ui.primary;
const card = ui.card;
const input = ui.input;

// Templates for services that typically run on a site behind the hub.
const templates: { label: string; rule: Omit<Rule, "target"> }[] = [
  { label: "Enshrouded", rule: { name: "enshrouded", proto: "udp", from: 15636, to: 15637 } },
];

function ports(rule: Rule): string {
  return rule.from === rule.to ? String(rule.from) : `${rule.from}–${rule.to}`;
}

export default function Panel() {
  const { t } = useTranslation();
  const status = useGet<Status>(statusPath);
  const data = status.data;
  const [current, setCurrent] = useState<Status | null>(null);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");
  const [adding, setAdding] = useState(false);
  const view = current ?? data;

  useEffect(() => setCurrent(null), [data]);

  async function remove(rule: Rule) {
    setBusy(rule.name);
    setError("");
    try {
      setCurrent(await api<Status>(`/api/plugins/portforward/rules/${encodeURIComponent(rule.name)}/delete`, { method: "POST" }));
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("pf.failed"));
    } finally {
      setBusy("");
    }
  }

  const targetName = (address: string) => view?.targets.find((x) => x.address === address)?.name;

  return (
    <PanelFrame
      title={t("pf.title")}
      description={t("pf.description")}
      command={view?.command}
      mocked={view?.mocked}
      detail={view?.detail}
      loading={status.loading}
      error={status.error}
      onRefresh={status.reload}
    >
      {view ? (
        <div className="space-y-4">
          <StatGrid
            items={[
              { label: t("pf.public"), value: view.publicIp || "—", mono: true },
              { label: t("pf.hub"), value: view.hubAddress, mono: true },
              {
                label: t("pf.forwarding"),
                value: view.forwarding ? t("pf.on") : t("pf.off"),
                tone: view.forwarding ? "text-emerald-300" : "text-amber-200",
              },
              {
                label: t("pf.table"),
                value: view.tableLive ? t("pf.active") : t("pf.inactive"),
                tone: view.tableLive ? "text-emerald-300" : "text-amber-200",
              },
            ]}
          />

          <SectionHeader title={t("pf.rules")} hint={t("pf.snatHint", { hub: view.hubAddress })}>
            <button type="button" className={adding ? ui.button : ui.primary} onClick={() => setAdding((v) => !v)}>
              {adding ? t("common.cancel") : `+ ${t("pf.new")}`}
            </button>
          </SectionHeader>
          {adding ? (
            <AddForm
              status={view}
              disabled={view.mocked}
              onSaved={(next) => {
                setCurrent(next);
                setAdding(false);
              }}
            />
          ) : null}

          {view.rules.length === 0 ? (
            <p className="text-sm text-muted">{t("pf.none")}</p>
          ) : (
            <DataTable head={[t("pf.name"), t("pf.proto"), t("pf.publicPorts"), t("pf.target"), ""]}>
              {view.rules.map((rule) => (
                <tr key={rule.name}>
                  <td className="px-3 py-2">{rule.name}</td>
                  <td className="px-3 py-2 font-mono text-xs uppercase">{rule.proto}</td>
                  <td className="px-3 py-2 font-mono text-xs whitespace-nowrap">
                    {view.publicIp || view.publicIf}:{ports(rule)}
                  </td>
                  <td className="px-3 py-2 font-mono text-xs whitespace-nowrap">
                    {rule.target}
                    {targetName(rule.target) ? <span className="ms-2 font-sans text-white/60">{targetName(rule.target)}</span> : null}
                  </td>
                  <td className="px-3 py-2 text-right">
                    <button type="button" className={button} disabled={busy !== "" || view.mocked} onClick={() => void remove(rule)}>
                      {busy === rule.name ? t("pf.removing") : t("pf.remove")}
                    </button>
                  </td>
                </tr>
              ))}
            </DataTable>
          )}
          {error ? <p className="text-sm text-debian">{error}</p> : null}

        </div>
      ) : null}
    </PanelFrame>
  );
}

function AddForm({ status, disabled, onSaved }: { status: Status; disabled: boolean; onSaved: (s: Status) => void }) {
  const { t } = useTranslation();
  const firstTarget = status.targets[0]?.address ?? "";
  const [rule, setRule] = useState<Rule>({ name: "", proto: "udp", from: 0, to: 0, target: firstTarget });
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!rule.target && firstTarget) {
      setRule((r) => ({ ...r, target: firstTarget }));
    }
  }, [firstTarget, rule.target]);

  async function submit() {
    setBusy(true);
    setError("");
    try {
      const next = await api<Status>("/api/plugins/portforward/rules", {
        method: "POST",
        body: JSON.stringify({ ...rule, to: rule.to || rule.from }),
      });
      setRule({ name: "", proto: "udp", from: 0, to: 0, target: firstTarget });
      onSaved(next);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("pf.failed"));
    } finally {
      setBusy(false);
    }
  }

  const number = (value: string) => (value === "" ? 0 : Math.max(0, Math.min(65535, Number(value) || 0)));

  return (
    <form
      className={card}
      onSubmit={(event) => {
        event.preventDefault();
        void submit();
      }}
    >
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex flex-wrap gap-2">
          {templates.map((tpl) => (
            <button key={tpl.label} type="button" className={button} onClick={() => setRule((r) => ({ ...r, ...tpl.rule }))}>
              {t("pf.template", { name: tpl.label })}
            </button>
          ))}
        </div>
      </div>
      <div className="mt-3 grid gap-3 md:grid-cols-5">
        <Field label={t("pf.name")}>
          <input className={input} value={rule.name} required placeholder="enshrouded" onChange={(e) => setRule({ ...rule, name: e.target.value })} />
        </Field>
        <Field label={t("pf.proto")}>
          <select className={input} value={rule.proto} onChange={(e) => setRule({ ...rule, proto: e.target.value as Rule["proto"] })}>
            <option value="udp">UDP</option>
            <option value="tcp">TCP</option>
          </select>
        </Field>
        <Field label={t("pf.from")}>
          <input className={input} inputMode="numeric" required value={rule.from || ""} onChange={(e) => setRule({ ...rule, from: number(e.target.value) })} />
        </Field>
        <Field label={t("pf.to")}>
          <input className={input} inputMode="numeric" placeholder={rule.from ? String(rule.from) : ""} value={rule.to || ""} onChange={(e) => setRule({ ...rule, to: number(e.target.value) })} />
        </Field>
        <Field label={t("pf.target")}>
          <select className={input} value={rule.target} required onChange={(e) => setRule({ ...rule, target: e.target.value })}>
            {status.targets.length === 0 ? <option value="">{t("pf.noTargets")}</option> : null}
            {status.targets.map((x) => (
              <option key={x.address} value={x.address}>
                {x.address}
                {x.name ? ` · ${x.name}` : ""}
                {x.site ? ` (${t("pf.site")})` : ""}
              </option>
            ))}
          </select>
        </Field>
      </div>
      <p className="mt-2 text-xs text-white/50">{t("pf.rulesHint")}</p>
      <div className="mt-3 flex flex-wrap items-center gap-3">
        <button type="submit" className={primary} disabled={busy || disabled || !rule.name || !rule.from || !rule.target}>
          {busy ? t("pf.saving") : t("pf.add")}
        </button>
        {error ? <span className="text-sm text-debian">{error}</span> : null}
      </div>
    </form>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block text-[11px] tracking-wide text-white/50 uppercase">{label}</span>
      {children}
    </label>
  );
}

