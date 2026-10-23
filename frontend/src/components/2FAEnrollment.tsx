import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { authApi, type Enrollment } from "@/lib/auth.ts";

type Props = {
  onComplete: (username: string) => void;
  onCancel: () => void;
};

const glassButton =
  "rounded-2xl border border-white/10 bg-white/5 px-3 py-1.5 text-xs text-white transition-all duration-200 ease-in-out hover:bg-white/10";

export function TwoFAEnrollment({ onComplete, onCancel }: Props) {
  const { t } = useTranslation();
  const [enrollment, setEnrollment] = useState<Enrollment | null>(null);
  const [code, setCode] = useState("");
  const [saved, setSaved] = useState(false);
  const [copied, setCopied] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function start() {
    setBusy(true);
    setError("");
    try {
      setEnrollment(await authApi.enroll());
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("enroll.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function confirm(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const session = await authApi.verify(code.trim());
      onComplete(session.username);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("enroll.badCode"));
      setCode("");
    } finally {
      setBusy(false);
    }
  }

  async function copySecret() {
    if (!enrollment) {
      return;
    }
    await copyText(enrollment.secret);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 2000);
  }

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center overflow-y-auto bg-slate-950/50 p-4 backdrop-blur-sm md:items-center">
      <div
        role="dialog"
        aria-modal="true"
        aria-labelledby="enroll-title"
        className="w-full max-w-xl rounded-3xl border border-white/10 bg-slate-900/60 text-white shadow-xl backdrop-blur-xl"
      >
        <div className="flex items-center justify-between border-b border-white/10 px-5 py-3">
          <h2 id="enroll-title" className="text-sm font-semibold">
            {t("enroll.title")}
          </h2>
          <button
            type="button"
            onClick={onCancel}
            className="text-sm text-white/70 transition-all duration-200 ease-in-out hover:text-white"
          >
            {t("common.cancel")}
          </button>
        </div>

        <div className="px-5 py-5">
          {!enrollment ? (
            <>
              <p className="text-sm text-white/70">{t("enroll.intro")}</p>
              <button
                type="button"
                onClick={() => void start()}
                disabled={busy}
                className="mt-4 rounded-2xl bg-debian px-4 py-2 text-sm text-white transition-all duration-200 ease-in-out enabled:hover:opacity-90 disabled:opacity-50"
              >
                {busy ? t("enroll.starting") : t("enroll.start")}
              </button>
            </>
          ) : (
            <>
              <ol className="space-y-6 text-sm">
                <li>
                  <p className="font-medium">{t("enroll.scan")}</p>
                  <div className="mt-3 flex flex-col gap-4 sm:flex-row sm:items-start">
                    <img
                      src={enrollment.qr_code}
                      alt={t("enroll.qrAlt")}
                      width={176}
                      height={176}
                      className="h-44 w-44 shrink-0 rounded-2xl border border-white/10 bg-white p-2"
                    />
                    <div className="min-w-0 flex-1">
                      <p className="text-xs text-white/60">{t("enroll.manual")}</p>
                      <div className="mt-2 flex items-stretch">
                        <code className="min-w-0 flex-1 overflow-x-auto rounded-s-2xl bg-black/40 px-3 py-2 font-mono text-xs break-all text-white">
                          {enrollment.secret}
                        </code>
                        <button type="button" onClick={() => void copySecret()} className={`${glassButton} rounded-s-none`}>
                          {copied ? t("common.copied") : t("common.copy")}
                        </button>
                      </div>
                    </div>
                  </div>
                </li>

                <li>
                  <p className="font-medium">{t("enroll.backup")}</p>
                  <p className="mt-2 rounded-2xl border border-amber-300/30 bg-amber-950/40 px-3 py-2 text-amber-100">
                    {t("enroll.warning")}
                  </p>
                  <ul className="mt-3 grid grid-cols-2 gap-2 font-mono text-sm">
                    {enrollment.backup_codes.map((backup) => (
                      <li key={backup} className="rounded-2xl border border-white/10 bg-white/5 px-3 py-1.5 text-center">
                        {backup}
                      </li>
                    ))}
                  </ul>
                  <div className="mt-3 flex flex-wrap items-center gap-4">
                    <button type="button" onClick={() => void copyText(enrollment.backup_codes.join("\n"))} className={glassButton}>
                      {t("enroll.copyCodes")}
                    </button>
                    <button type="button" onClick={() => window.print()} className={glassButton}>
                      {t("common.print")}
                    </button>
                    <label className="flex items-center gap-2 text-xs">
                      <input type="checkbox" checked={saved} onChange={(event) => setSaved(event.target.checked)} />
                      {t("enroll.saved")}
                    </label>
                  </div>
                </li>

                <li>
                  <p className="font-medium">{t("enroll.confirm")}</p>
                  <form onSubmit={(event) => void confirm(event)} className="mt-3 flex flex-wrap items-end gap-2">
                    <input
                      value={code}
                      onChange={(event) => setCode(event.target.value.replace(/\D/g, "").slice(0, 6))}
                      inputMode="numeric"
                      autoComplete="one-time-code"
                      placeholder="123456"
                      aria-label={t("enroll.codeLabel")}
                      className="w-36 rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-center font-mono text-lg tracking-[0.3em] text-white transition-all duration-200 ease-in-out outline-none"
                    />
                    <button
                      type="submit"
                      disabled={busy || code.length !== 6 || !saved}
                      className="rounded-2xl bg-debian px-4 py-2 text-sm text-white transition-all duration-200 ease-in-out enabled:hover:opacity-90 disabled:opacity-50"
                    >
                      {t("enroll.activate")}
                    </button>
                  </form>
                </li>
              </ol>
            </>
          )}
          {error ? <p className="mt-4 text-sm text-rose-300">{error}</p> : null}
        </div>
      </div>
    </div>
  );
}

async function copyText(text: string) {
  if (navigator.clipboard && window.isSecureContext) {
    await navigator.clipboard.writeText(text);
    return;
  }
  const area = document.createElement("textarea");
  area.value = text;
  area.setAttribute("readonly", "");
  area.style.position = "fixed";
  area.style.opacity = "0";
  document.body.appendChild(area);
  area.select();
  document.execCommand("copy");
  area.remove();
}
