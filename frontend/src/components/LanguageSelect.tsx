import { applyLanguage, languages, persistLanguage } from "@/i18n.ts";
import { useTranslation } from "react-i18next";

export function LanguageSelect({ persist }: { persist: boolean }) {
  const { i18n, t } = useTranslation();

  return (
    <label className="flex items-center gap-2 text-xs text-white/70">
      <span className="sr-only">{t("common.language")}</span>
      <select
        aria-label={t("common.language")}
        value={i18n.language.split("-")[0]}
        onChange={(event) => {
          const next = event.target.value;
          if (!persist) {
            applyLanguage(next);
            return;
          }
          void persistLanguage(next).catch(() => undefined);
        }}
        className="rounded-2xl border border-white/10 bg-slate-900/60 px-3 py-1.5 text-sm text-white backdrop-blur-xl transition-all duration-200 ease-in-out outline-none hover:border-white/30"
      >
        {languages.map((language) => (
          <option key={language.id} value={language.id}>
            {language.label}
          </option>
        ))}
      </select>
    </label>
  );
}
