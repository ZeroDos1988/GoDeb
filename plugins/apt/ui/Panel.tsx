import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import type { CommandEnvelope } from "@/lib/types.ts";
import { useGet } from "@/lib/useAsync.ts";

type AptPackage = {
  name: string;
  suite: string;
  version: string;
  arch: string;
  from: string;
};

type AptStatus = CommandEnvelope & {
  packages: AptPackage[];
};

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<AptStatus>("/api/plugins/apt/upgradable");
  const packages = query.data?.packages ?? [];

  return (
    <PanelFrame
      title={t("apt.title")}
      description={t("apt.description")}
      command={query.data?.command}
      mocked={query.data?.mocked}
      detail={query.data?.detail}
      loading={query.loading}
      error={query.error}
      onRefresh={query.reload}
    >
      {query.data ? (
        packages.length === 0 ? (
          <p className="text-sm text-muted">{t("apt.empty")}</p>
        ) : (
          <DataTable head={[t("apt.package"), t("apt.suite"), t("apt.installed"), t("apt.available"), t("apt.arch")]}>
            {packages.map((pkg) => (
              <tr key={`${pkg.name}-${pkg.arch}-${pkg.version}`}>
                <td className="px-3 py-2 font-mono text-xs">{pkg.name}</td>
                <td className="px-3 py-2">{pkg.suite}</td>
                <td className="px-3 py-2 font-mono text-xs">{pkg.from}</td>
                <td className="px-3 py-2 font-mono text-xs">{pkg.version}</td>
                <td className="px-3 py-2">{pkg.arch}</td>
              </tr>
            ))}
          </DataTable>
        )
      ) : null}
    </PanelFrame>
  );
}
