import { useTranslation } from "react-i18next";
import { SetupSummary, type SetupSummaryData } from "@/components/SetupSummary.tsx";
import { useGet } from "@/lib/useAsync.ts";

export function SetupSummaryPage() {
  const { t } = useTranslation();
  const query = useGet<SetupSummaryData>("/api/setup/summary");
  return (
    <section>
      {query.loading && !query.data ? <p className="text-sm text-muted">{t("common.loading")}</p> : null}
      {query.error ? <p className="text-sm text-debian">{query.error}</p> : null}
      {query.data ? (
        <SetupSummary
          data={query.data}
          actions={
            <button
              type="button"
              onClick={query.reload}
              className="rounded-2xl border border-white/10 bg-slate-900/60 px-3 py-1.5 text-sm text-white transition-all duration-200 hover:bg-white/10"
            >
              {t("common.refresh")}
            </button>
          }
        />
      ) : null}
    </section>
  );
}
