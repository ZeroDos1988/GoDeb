import { useState } from "react";
import { useTranslation } from "react-i18next";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";

type ServiceStatus = {
  unit: string;
  enabled: string;
  active: string;
};

type ManagedPlugin = {
  id: string;
  name: string;
  aptDependencies: string[];
  systemdServices: string[];
  dependenciesReady: boolean;
  installed: boolean;
  enabled: boolean;
  services: ServiceStatus[];
};

type PluginList = {
  plugins: ManagedPlugin[];
};

export function PluginManager({ onChanged }: { onChanged?: () => void }) {
  const { t } = useTranslation();
  const query = useGet<PluginList>("/api/plugin-manager");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const plugins = query.data?.plugins ?? [];

  async function run(id: string, action: "install" | "enable" | "disable") {
    setBusy(`${id}:${action}`);
    setError("");
    try {
      await api(`/api/plugin-manager/${id}/${action}`, { method: "POST" });
      query.reload();
      // Switching a plugin on or off changes the sidebar.
      onChanged?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Unbekannter Fehler");
    } finally {
      setBusy("");
    }
  }

  return (
    <section>
      <h1 className="text-2xl font-semibold tracking-tight">{t("pluginManager.title")}</h1>
      <p className="mt-1 max-w-2xl text-sm text-muted">{t("pluginManager.lead")}</p>
      {query.loading && !query.data ? <p className="mt-6 text-sm text-muted">{t("pluginManager.loading")}</p> : null}
      {query.error || error ? (
        <p className="mt-6 max-w-xl rounded-2xl border border-white/10 bg-rose-950/40 px-4 py-3 text-sm text-rose-100 backdrop-blur-xl">
          {error || query.error}
        </p>
      ) : null}
      {!query.loading && plugins.length === 0 ? <p className="mt-6 text-sm text-muted">{t("pluginManager.empty")}</p> : null}
      <div className="mt-6 grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
        {plugins.map((plugin) => {
          const working = busy.startsWith(`${plugin.id}:`);
          return (
            <article
              key={plugin.id}
              className="rounded-3xl border border-white/10 bg-slate-900/60 p-5 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out"
            >
              <div className="flex items-start justify-between gap-3">
                <div>
                  <h2 className="text-lg font-semibold tracking-tight">{plugin.name}</h2>
                  <p className="mt-1 font-mono text-xs text-white/50">{plugin.id}</p>
                </div>
                <Toggle
                  checked={plugin.enabled}
                  disabled={working || (!plugin.enabled && !plugin.dependenciesReady)}
                  label={plugin.name}
                  onChange={(next) => void run(plugin.id, next ? "enable" : "disable")}
                />
              </div>
              <div className="mt-4 flex flex-wrap gap-2">
                <Badge tone={plugin.installed ? "ok" : "bad"}>
                  {plugin.installed ? t("pluginManager.installed") : t("pluginManager.missing")}
                </Badge>
                <Badge tone={plugin.enabled ? "ok" : "idle"}>
                  {plugin.enabled ? t("pluginManager.enabled") : t("pluginManager.disabled")}
                </Badge>
                {plugin.services.map((service) => (
                  <Badge key={service.unit} tone={service.active === "active" ? "ok" : "idle"}>
                    {`${service.unit} · ${service.active}`}
                  </Badge>
                ))}
              </div>
              {plugin.aptDependencies.length > 0 ? (
                <p className="mt-4 text-xs text-white/60">
                  {t("pluginManager.deps")}: {plugin.aptDependencies.join(", ")}
                </p>
              ) : null}
              {!plugin.dependenciesReady ? (
                <button
                  type="button"
                  disabled={working}
                  onClick={() => void run(plugin.id, "install")}
                  className="mt-4 rounded-2xl bg-debian px-4 py-2 text-sm font-medium text-white transition-all duration-200 ease-in-out hover:brightness-110 disabled:opacity-50"
                >
                  {working ? t("pluginManager.working") : t("pluginManager.install")}
                </button>
              ) : null}
            </article>
          );
        })}
      </div>
    </section>
  );
}

function Badge({ tone, children }: { tone: "ok" | "bad" | "idle"; children: string }) {
  const toneClass =
    tone === "ok" ? "bg-emerald-400/15 text-emerald-200" : tone === "bad" ? "bg-rose-400/15 text-rose-200" : "bg-white/10 text-white/70";
  return <span className={`rounded-full px-2.5 py-1 text-[11px] tracking-wide uppercase ${toneClass}`}>{children}</span>;
}

function Toggle({
  checked,
  disabled,
  label,
  onChange,
}: {
  checked: boolean;
  disabled: boolean;
  label: string;
  onChange: (next: boolean) => void;
}) {
  return (
    <button
      type="button"
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      onClick={() => onChange(!checked)}
      className={`relative h-7 w-12 shrink-0 rounded-full transition-all duration-200 ease-in-out disabled:opacity-40 ${
        checked ? "bg-emerald-500" : "bg-white/20"
      }`}
    >
      <span
        className={`absolute top-0.5 left-0.5 h-6 w-6 rounded-full bg-white transition-all duration-200 ease-in-out ${
          checked ? "translate-x-5" : ""
        }`}
      />
    </button>
  );
}
