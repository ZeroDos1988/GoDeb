import { useEffect, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useGet } from "@/lib/useAsync.ts";
import { PluginIcon } from "@/components/Icons.tsx";
import { SessionsDialog } from "@/components/SessionsDialog.tsx";
import { ThemeSettings } from "@/components/ThemeSettings.tsx";
import { Shield, Wordmark } from "@/components/Brand.tsx";
import { UserMenu } from "@/components/UserMenu.tsx";
import type { PluginMeta } from "@/lib/types.ts";

type ShellProps = {
  plugins: PluginMeta[];
  pluginsError: string;
  route: string;
  username: string;
  onLogout: () => void;
  children: ReactNode;
};

const glass =
  "rounded-3xl border border-white/10 bg-slate-900/60 text-white shadow-xl backdrop-blur-xl transition-all duration-200 ease-in-out";

export function Shell({ plugins, pluginsError, route, username, onLogout, children }: ShellProps) {
  const { t } = useTranslation();
  const [themeOpen, setThemeOpen] = useState(false);
  const [sessionsOpen, setSessionsOpen] = useState(false);
  const [navOpen, setNavOpen] = useState(false);
  const health = useGet<{ version?: string }>("/api/health");
  const menu = (
    <UserMenu
      username={username}
      version={health.data?.version}
      onAppearance={() => setThemeOpen(true)}
      onSessions={() => setSessionsOpen(true)}
      onLogout={onLogout}
    />
  );

  // Close the drawer after a route change (hash navigation).
  useEffect(() => {
    setNavOpen(false);
  }, [route]);

  useEffect(() => {
    if (!navOpen) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setNavOpen(false);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [navOpen]);

  const nav = (
    <nav className="flex flex-col gap-1 px-3 pb-4 md:flex-1">
      <NavLink href="#/" active={route === "/"} icon="server" label={t("nav.overview")} />
      <NavLink
        href="#/plugin-manager"
        active={route === "/plugin-manager"}
        icon="cogs"
        label={t("nav.plugins")}
      />
      {plugins.map((plugin) => (
        <NavLink
          key={plugin.id}
          href={`#/${plugin.id}`}
          active={route === `/${plugin.id}`}
          icon={plugin.icon}
          label={plugin.name}
        />
      ))}
      {pluginsError ? <p className="px-3 py-2 text-xs text-rose-300">{pluginsError}</p> : null}
      {!pluginsError && plugins.length === 0 ? (
        <p className="px-3 py-2 text-xs text-white/60">{t("nav.noPlugins")}</p>
      ) : null}
    </nav>
  );

  return (
    <div className="min-h-screen bg-paper text-ink">
      <div className="mx-auto grid min-h-screen max-w-[1600px] grid-cols-[minmax(0,1fr)] content-start gap-4 p-3 md:grid-cols-[17rem_minmax(0,1fr)] md:content-stretch md:p-4">
        {/* Mobile top bar */}
        <header className={`${glass} relative z-40 flex items-center gap-2 px-3 py-2 md:hidden`}>
          <button
            type="button"
            className="grid h-11 w-11 shrink-0 place-items-center rounded-2xl text-white/90 hover:bg-white/10"
            aria-expanded={navOpen}
            aria-controls="godeb-nav"
            aria-label={navOpen ? t("nav.closeMenu") : t("nav.openMenu")}
            onClick={() => setNavOpen((v) => !v)}
          >
            <MenuIcon open={navOpen} />
          </button>
          <div className="flex min-w-0 flex-1 items-center gap-2">
            <Shield className="h-9 w-auto" />
            <div className="min-w-0">
              <Wordmark className="h-4 w-auto" />
              <div className="truncate text-[10px] tracking-wide text-white/60 uppercase">
                {t("nav.subtitle")}
              </div>
            </div>
          </div>
          {menu}
        </header>

        {navOpen ? (
          <button
            type="button"
            className="fixed inset-0 z-40 bg-black/50 md:hidden"
            aria-label={t("nav.closeMenu")}
            onClick={() => setNavOpen(false)}
          />
        ) : null}

        <aside
          id="godeb-nav"
          className={`${glass} relative z-50 flex min-w-0 flex-col max-md:fixed max-md:inset-y-3 max-md:start-3 max-md:w-[min(18rem,calc(100vw-1.5rem))] max-md:transition-transform max-md:duration-200 ${
            navOpen ? "max-md:translate-x-0" : "max-md:-translate-x-[120%]"
          } md:sticky md:top-4 md:h-[calc(100vh-2rem)] md:translate-x-0`}
        >
          <div className="hidden items-center gap-3 px-5 py-5 md:flex">
            <Shield className="h-11 w-auto" />
            <div className="min-w-0 flex-1">
              <Wordmark className="h-5 w-auto" />
              <div className="mt-1 text-[11px] tracking-wide text-white/60 uppercase">
                {t("nav.subtitle")}
              </div>
            </div>
          </div>
          <div className="flex items-center justify-between px-4 py-3 md:hidden">
            <span className="text-sm font-semibold">{t("nav.menu")}</span>
            <button
              type="button"
              className="grid h-11 w-11 place-items-center rounded-2xl hover:bg-white/10"
              aria-label={t("nav.closeMenu")}
              onClick={() => setNavOpen(false)}
            >
              <MenuIcon open />
            </button>
          </div>
          {nav}
          <p className="hidden border-t border-white/10 px-5 py-4 text-[11px] text-white/50 md:block">
            {t("nav.footer")}
          </p>
          {themeOpen ? <ThemeSettings onClose={() => setThemeOpen(false)} /> : null}
          {sessionsOpen ? (
            <SessionsDialog onClose={() => setSessionsOpen(false)} onSignedOut={onLogout} />
          ) : null}
        </aside>

        <div className="min-w-0">
          <header className="relative z-20 mb-4 hidden items-center justify-between gap-3 rounded-2xl border border-white/10 bg-slate-900/60 px-4 py-2 text-white backdrop-blur-xl transition-all duration-200 ease-in-out md:flex">
            <div className="text-xs tracking-wide text-white/70 uppercase">{t("nav.serverAdmin")}</div>
            {menu}
          </header>
          <main>{children}</main>
        </div>
      </div>
    </div>
  );
}

function NavLink({
  href,
  active,
  icon,
  label,
}: {
  href: string;
  active: boolean;
  icon: string;
  label: string;
}) {
  return (
    <a
      href={href}
      className={`mb-1 flex min-h-11 shrink-0 items-center gap-2 rounded-2xl px-3 py-2.5 text-sm transition-all duration-200 ease-in-out ${
        active ? "bg-white/10 text-white" : "text-white/75 hover:bg-white/5"
      }`}
    >
      <PluginIcon name={icon} />
      <span>{label}</span>
    </a>
  );
}

function MenuIcon({ open }: { open?: boolean }) {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" aria-hidden="true">
      {open ? (
        <path
          d="M6 6l12 12M18 6L6 18"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
        />
      ) : (
        <path
          d="M4 7h16M4 12h16M4 17h16"
          stroke="currentColor"
          strokeWidth="2"
          strokeLinecap="round"
        />
      )}
    </svg>
  );
}
