import { useEffect, useRef, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { setStepUpPrompt, type StepUpRequest } from "@/lib/api.ts";

type Pending = StepUpRequest & { resolve: (code: string | null) => void };

// StepUpDialog asks for a 2FA code whenever the daemon refuses an admin
// action without one. It is mounted once for the signed-in dashboard.
export function StepUpDialog() {
  const { t } = useTranslation();
  const [pending, setPending] = useState<Pending | null>(null);
  const [code, setCode] = useState("");
  const input = useRef<HTMLInputElement>(null);

  useEffect(
    () =>
      setStepUpPrompt(
        (request) =>
          new Promise<string | null>((resolve) => {
            setCode("");
            setPending({ ...request, resolve });
          }),
      ),
    [],
  );

  useEffect(() => {
    if (!pending) {
      return;
    }
    input.current?.focus();
    function onKey(event: KeyboardEvent) {
      if (event.key === "Escape") {
        cancel();
      }
    }
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  if (!pending) {
    return null;
  }

  function finish(value: string | null) {
    pending?.resolve(value);
    setPending(null);
    setCode("");
  }

  function cancel() {
    finish(null);
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    const value = code.replace(/\s+/g, "");
    if (value) {
      finish(value);
    }
  }

  return (
    <div className="fixed inset-0 z-[60] grid place-items-center bg-slate-950/60 p-4 backdrop-blur-sm" role="presentation" onClick={cancel}>
      <form
        role="dialog"
        aria-modal="true"
        aria-labelledby="stepup-title"
        onSubmit={submit}
        onClick={(event) => event.stopPropagation()}
        className="w-full max-w-sm rounded-3xl border border-white/10 bg-slate-900/80 p-5 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out"
      >
        <h2 id="stepup-title" className="text-lg font-semibold">
          {t("stepUp.title")}
        </h2>
        <p className="mt-1 text-sm text-white/70">{pending.critical ? t("stepUp.critical") : t("stepUp.window")}</p>
        <label className="mt-4 block text-xs tracking-wide text-white/50 uppercase" htmlFor="stepup-code">
          {t("stepUp.code")}
        </label>
        <input
          id="stepup-code"
          ref={input}
          value={code}
          onChange={(event) => setCode(event.target.value)}
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={16}
          className="mt-1 w-full rounded-2xl border border-white/15 bg-slate-950/40 px-3 py-3 text-center font-mono text-lg tracking-[0.3em] text-white outline-none focus:border-white/40"
        />
        <p className="mt-2 text-xs text-white/50">{t("stepUp.backupHint")}</p>
        {pending.error ? (
          <p className="mt-3 rounded-2xl bg-rose-500/15 px-3 py-2 text-sm text-rose-200" role="alert">
            {pending.error}
            {pending.remaining ? ` ${t("stepUp.remaining", { count: pending.remaining })}` : ""}
          </p>
        ) : null}
        <div className="mt-4 flex flex-wrap justify-end gap-2">
          <button
            type="button"
            onClick={cancel}
            className="rounded-2xl px-4 py-2 text-sm text-white/70 transition-all duration-200 ease-in-out hover:bg-white/10 hover:text-white"
          >
            {t("common.cancel")}
          </button>
          <button
            type="submit"
            disabled={!code.trim()}
            className="rounded-2xl bg-debian px-4 py-2 text-sm font-semibold text-white transition-all duration-200 ease-in-out hover:opacity-90 disabled:opacity-50"
          >
            {t("stepUp.confirm")}
          </button>
        </div>
      </form>
    </div>
  );
}
