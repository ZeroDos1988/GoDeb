import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import type { CommandEnvelope } from "@/lib/types.ts";
import { useGet } from "@/lib/useAsync.ts";

type Service = {
  unit: string;
  load: string;
  active: string;
  sub: string;
  description: string;
};

type ServiceList = CommandEnvelope & {
  services: Service[];
};

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<ServiceList>("/api/plugins/systemd/services");
  const [filter, setFilter] = useState("");
  const [busy, setBusy] = useState("");
  const [actionError, setActionError] = useState("");

  const services = useMemo(() => {
    const needle = filter.trim().toLowerCase();
    const all = query.data?.services ?? [];
    if (!needle) {
      return all;
    }
    return all.filter(
      (service) =>
        service.unit.toLowerCase().includes(needle) ||
        service.description.toLowerCase().includes(needle),
    );
  }, [filter, query.data]);

  async function act(unit: string, action: "start" | "stop") {
    setBusy(unit);
    setActionError("");
    try {
      await api("/api/plugins/systemd/action", {
        method: "POST",
        body: JSON.stringify({ unit, action }),
      });
      query.reload();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : t("systemd.failed"));
    } finally {
      setBusy("");
    }
  }

  return (
    <PanelFrame
      title={t("systemd.title")}
      description={t("systemd.description")}
      command={query.data?.command}
      mocked={query.data?.mocked}
      detail={query.data?.detail}
      loading={query.loading}
      error={query.error}
      onRefresh={query.reload}
    >
      <label className="mb-4 block max-w-sm text-sm">
        <span className="mb-1 block text-xs tracking-wide text-muted uppercase">{t("systemd.filter")}</span>
        <input
          value={filter}
          onChange={(event) => setFilter(event.target.value)}
          placeholder="ssh"
          className="w-full rounded-2xl border border-white/10 bg-slate-900/60 px-3 py-1.5 font-mono text-sm text-white backdrop-blur-xl transition-all duration-200 ease-in-out outline-none"
        />
      </label>
      {actionError ? <p className="mb-4 text-sm text-debian">{actionError}</p> : null}
      {query.data ? (
        <DataTable head={[t("systemd.unit"), t("systemd.active"), t("systemd.sub"), t("systemd.descriptionCol"), ""]}>
          {services.map((service) => (
            <tr key={service.unit}>
              <td className="px-3 py-2 font-mono text-xs">{service.unit}</td>
              <td className={`px-3 py-2 ${service.active === "active" ? "text-emerald-300" : "text-white/50"}`}>
                {service.active}
              </td>
              <td className="px-3 py-2">{service.sub}</td>
              <td className="px-3 py-2">{service.description}</td>
              <td className="px-3 py-2 text-right whitespace-nowrap">
                <button
                  type="button"
                  disabled={busy !== ""}
                  onClick={() => void act(service.unit, "start")}
                  className="mr-3 text-xs text-emerald-300 transition-all duration-200 ease-in-out enabled:hover:underline disabled:opacity-50"
                >
                  {t("systemd.start")}
                </button>
                <button
                  type="button"
                  disabled={busy !== ""}
                  onClick={() => void act(service.unit, "stop")}
                  className="text-xs text-rose-300 transition-all duration-200 ease-in-out enabled:hover:underline disabled:opacity-50"
                >
                  {t("systemd.stop")}
                </button>
              </td>
            </tr>
          ))}
        </DataTable>
      ) : null}
    </PanelFrame>
  );
}
