import { useEffect, useState, type FormEvent, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { TwoFAEnrollment } from "@/components/2FAEnrollment.tsx";
import { ThemeSettings } from "@/components/ThemeSettings.tsx";
import { api, ApiError } from "@/lib/api.ts";
import { authApi } from "@/lib/auth.ts";

type OIDCProvider = {
  id: string;
  name: string;
  enabled: boolean;
};

type Step = "password" | "code" | "enroll";

export function Login2FA({ onAuthenticated }: { onAuthenticated: (username: string) => void }) {
  const { t } = useTranslation();
  const [step, setStep] = useState<Step>("password");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [themeOpen, setThemeOpen] = useState(false);
  const [providers, setProviders] = useState<OIDCProvider[]>([]);

  useEffect(() => {
    api<{ providers: OIDCProvider[] }>("/api/auth/oidc/providers")
      .then((res) => setProviders(res.providers.filter((provider) => provider.enabled)))
      .catch(() => undefined);
    api<{ username: string; enrollment_required: boolean }>("/api/auth/oidc/resume")
      .then((res) => {
        setUsername(res.username);
        setStep(res.enrollment_required ? "enroll" : "code");
      })
      .catch(() => undefined);
  }, []);

  async function submitPassword(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await authApi.login(username.trim(), password);
      setPassword("");
      setStep(res.enrollment_required ? "enroll" : "code");
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("login.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function submitCode(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError("");
    try {
      const session = await authApi.authenticate(code.trim());
      onAuthenticated(session.username);
    } catch (err: unknown) {
      setCode("");
      if (err instanceof ApiError && err.status === 429) {
        // The daemon dropped the pending login after too many wrong codes.
        setStep("password");
      }
      setError(err instanceof Error ? err.message : t("login.badCode"));
    } finally {
      setBusy(false);
    }
  }

  function restart() {
    setStep("password");
    setCode("");
    setError("");
  }

  return (
    <div className="grid min-h-screen place-items-center bg-paper px-4 text-ink">
      <div className="w-full max-w-sm">
        <div className="mb-6 flex items-center gap-3">
          <span className="grid h-10 w-10 place-items-center bg-debian text-sm font-semibold text-white">
            gd
          </span>
          <div>
            <div className="text-lg leading-none font-semibold tracking-tight">GoDeb</div>
            <div className="mt-1 text-[11px] tracking-wide text-muted uppercase">{t("login.title")}</div>
          </div>
        </div>

        <div className="rounded-3xl border border-white/10 bg-slate-900/60 px-5 py-5 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out">
          <p className="mb-4 text-[11px] tracking-wide text-white/50 uppercase">
            {t("login.step", { step: step === "password" ? "1" : "2" })}
          </p>

          {step === "password" || step === "enroll" ? (
            <form onSubmit={(event) => void submitPassword(event)} className="space-y-4">
              <Field label={t("login.username")}>
                <input
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  autoComplete="username"
                  autoFocus
                  required
                  className="w-full rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white transition-all duration-200 ease-in-out outline-none"
                />
              </Field>
              <Field label={t("login.password")}>
                <input
                  type="password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  autoComplete="current-password"
                  required
                  className="w-full rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white transition-all duration-200 ease-in-out outline-none"
                />
              </Field>
              <button
                type="submit"
                disabled={busy}
                className="w-full rounded-2xl bg-debian px-4 py-2 text-sm text-white transition-all duration-200 ease-in-out enabled:hover:opacity-90 disabled:opacity-50"
              >
                {busy ? t("login.checking") : t("login.next")}
              </button>
            </form>
          ) : (
            <form onSubmit={(event) => void submitCode(event)} className="space-y-4">
              <p className="text-sm text-white/70">{t("login.codeHint", { user: username })}</p>
              <Field label={t("login.code")}>
                <input
                  value={code}
                  onChange={(event) => setCode(event.target.value.slice(0, 11))}
                  autoComplete="one-time-code"
                  autoFocus
                  required
                  placeholder={t("login.codePlaceholder")}
                  className="w-full rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-center font-mono text-lg tracking-widest text-white transition-all duration-200 ease-in-out outline-none"
                />
              </Field>
              <button
                type="submit"
                disabled={busy || code.trim().length < 6}
                className="w-full rounded-2xl bg-debian px-4 py-2 text-sm text-white transition-all duration-200 ease-in-out enabled:hover:opacity-90 disabled:opacity-50"
              >
                {busy ? t("login.checking") : t("login.signIn")}
              </button>
              <button type="button" onClick={restart} className="w-full text-xs text-white/60 transition-all duration-200 ease-in-out hover:text-white">
                {t("login.otherUser")}
              </button>
            </form>
          )}

          {error ? <p className="mt-4 text-sm text-debian">{error}</p> : null}
          {providers.length > 0 && step === "password" ? (
            <div className="mt-4 space-y-2 border-t border-white/10 pt-4">
              {providers.map((provider) => (
                <a
                  key={provider.id}
                  href={`/api/auth/oidc/${provider.id}/start`}
                  className="block rounded-2xl border border-white/10 px-4 py-2 text-center text-sm transition-all duration-200 ease-in-out hover:bg-white/10"
                >
                  {t("login.withProvider", { name: provider.name })}
                </a>
              ))}
            </div>
          ) : null}
        </div>
        <div className="mt-4 flex items-center justify-between gap-3 text-xs text-muted">
          <button type="button" onClick={() => setThemeOpen(true)} className="transition-all duration-200 ease-in-out hover:text-debian">
            {t("nav.appearance")}
          </button>
          <span>
            {t("login.lostAccess")} <code className="font-mono">sudo godeb 2fa reset</code>
          </span>
        </div>
        {themeOpen ? <ThemeSettings onClose={() => setThemeOpen(false)} /> : null}
      </div>

      {step === "enroll" ? <TwoFAEnrollment onComplete={onAuthenticated} onCancel={restart} /> : null}
    </div>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block text-sm">
      <span className="mb-1 block text-xs tracking-wide text-white/50 uppercase">{label}</span>
      {children}
    </label>
  );
}
