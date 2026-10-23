import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { useGet } from "@/lib/useAsync.ts";

type Certificate = {
  name: string;
  subject?: string;
  notBefore?: string;
  notAfter?: string;
  path: string;
};

type CertStatus = {
  path: string;
  command: string[];
  mocked: boolean;
  detail?: string;
  certificates: Certificate[];
};

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<CertStatus>("/api/plugins/certbot/certificates");
  const certificates = query.data?.certificates ?? [];

  return (
    <PanelFrame
      title={t("certbot.title")}
      description={t("certbot.description")}
      command={query.data?.command}
      mocked={query.data?.mocked}
      detail={query.data?.detail}
      loading={query.loading}
      error={query.error}
      onRefresh={query.reload}
    >
      {query.data ? (
        certificates.length === 0 ? (
          <p className="text-sm text-muted">{t("certbot.empty")}</p>
        ) : (
          <DataTable head={[t("certbot.name"), t("certbot.subject"), t("certbot.from"), t("certbot.until"), t("certbot.path")]}>
            {certificates.map((cert) => (
              <tr key={cert.path}>
                <td className="px-3 py-2 font-mono text-xs">{cert.name}</td>
                <td className="px-3 py-2">{cert.subject || "—"}</td>
                <td className="px-3 py-2 font-mono text-xs">{cert.notBefore || "—"}</td>
                <td className="px-3 py-2 font-mono text-xs">{cert.notAfter || "—"}</td>
                <td className="px-3 py-2 font-mono text-xs">{cert.path}</td>
              </tr>
            ))}
          </DataTable>
        )
      ) : null}
    </PanelFrame>
  );
}
