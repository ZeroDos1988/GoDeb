import { useState } from "react";
import { useTranslation } from "react-i18next";
import * as ui from "@/components/ui.ts";
import { copyText, downloadText } from "@/lib/clipboard.ts";
import { useGet } from "@/lib/useAsync.ts";

type DoctorText = {
  text: string;
  summary: string;
  version: string;
  redacted: boolean;
  fails: number;
  warns: number;
};

const ISSUE_URL = "https://github.com/ZeroDos1988/GoDeb/issues/new";
// Browsers and GitHub cut long URLs; the summary stays well below.
const MAX_SUMMARY = 5000;

// DiagnosisDialog shows the diagnosis report (godeb doctor --report) for
// copying or downloading, and opens a prefilled GitHub issue. Nothing is
// sent anywhere by GoDeb: the admin reviews and submits the issue in their
// own browser. The issue text is always redacted, the repository is public.
export function DiagnosisDialog({ onClose }: { onClose: () => void }) {
  const { t } = useTranslation();
  const [redact, setRedact] = useState(true);
  const [copied, setCopied] = useState(false);
  const report = useGet<DoctorText>(`/api/doctor/report?redact=${redact ? 1 : 0}`);
  const data = report.data;

  async function copy() {
    if (!data) return;
    await copyText(data.text);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  function download() {
    if (!data) return;
    const stamp = new Date().toISOString().slice(0, 16).replace(/[:T]/g, "-");
    downloadText(`godeb-diagnose-${stamp}.txt`, data.text);
  }

  function issueURL(d: DoctorText) {
    let summary = d.summary;
    if (summary.length > MAX_SUMMARY) summary = `${summary.slice(0, MAX_SUMMARY)}\n…`;
    const body = [
      t("diagnosis.issueIntro"),
      "",
      "",
      `### ${t("diagnosis.issueSummary")}`,
      "```",
      summary.trimEnd(),
      "```",
      "",
      t("diagnosis.issueAttach"),
    ].join("\n");
    const title = t("diagnosis.issueTitle", { version: d.version });
    return `${ISSUE_URL}?${new URLSearchParams({ title, body }).toString()}`;
  }

  return (
    <div className="fixed inset-0 z-30 grid place-items-center bg-slate-950/50 p-4 backdrop-blur-sm" role="presentation" onClick={onClose}>
      <div
        role="dialog"
        aria-labelledby="diagnosis-title"
        className="flex max-h-[90vh] w-full max-w-3xl flex-col rounded-3xl border border-white/10 bg-slate-900/60 p-5 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out"
        onClick={(event) => event.stopPropagation()}
      >
        <div className="mb-3 flex items-start justify-between gap-4">
          <div className="min-w-0">
            <h2 id="diagnosis-title" className="text-lg font-semibold">
              {t("diagnosis.title")}
            </h2>
            <p className="mt-1 text-sm text-white/70">{t("diagnosis.hint")}</p>
          </div>
          <button type="button" onClick={onClose} className="text-sm text-white/70 transition-all duration-200 ease-in-out hover:text-white">
            {t("common.close")}
          </button>
        </div>

        <label className="mb-3 flex min-h-11 items-center gap-3 text-sm md:min-h-10">
          <input type="checkbox" className="h-4 w-4 accent-debian" checked={redact} onChange={(e) => setRedact(e.target.checked)} />
          <span>{t("diagnosis.redact")}</span>
        </label>

        <div className="min-h-40 flex-1 overflow-auto rounded-xl border border-white/10 bg-slate-950/50 p-3">
          {report.error ? (
            <p className="text-sm text-rose-300">{report.error}</p>
          ) : !data || report.loading ? (
            <p className="text-sm text-white/60">{t("diagnosis.loading")}</p>
          ) : (
            <pre className="font-mono text-xs leading-relaxed whitespace-pre-wrap break-words text-white/85">{data.text}</pre>
          )}
        </div>

        <div className="mt-4 flex flex-wrap gap-2">
          <button type="button" className={ui.button} disabled={!data || report.loading} onClick={() => void copy()}>
            {copied ? t("diagnosis.copied") : t("diagnosis.copy")}
          </button>
          <button type="button" className={ui.button} disabled={!data || report.loading} onClick={download}>
            {t("diagnosis.download")}
          </button>
          <button type="button" className={ui.button} disabled={report.loading} onClick={report.reload}>
            {t("diagnosis.refresh")}
          </button>
          {data ? (
            <a className={`${ui.primary} sm:ms-auto`} href={issueURL(data)} target="_blank" rel="noopener noreferrer">
              {t("diagnosis.issue")}
            </a>
          ) : null}
        </div>
        <p className={ui.hint}>{t("diagnosis.issueHint")}</p>
      </div>
    </div>
  );
}
