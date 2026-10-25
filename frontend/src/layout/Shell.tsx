import { useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useGet } from "@/lib/useAsync.ts";
import { PluginIcon } from "@/components/Icons.tsx";
import { SessionsDialog } from "@/components/SessionsDialog.tsx";
import { ThemeSettings } from "@/components/ThemeSettings.tsx";
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
  return (
    <div className="min-h-screen bg-paper text-ink">
      <div className="mx-auto grid min-h-screen max-w-[1600px] grid-cols-[minmax(0,1fr)] content-start gap-4 p-3 md:grid-cols-[17rem_minmax(0,1fr)] md:content-stretch md:p-4">
        <aside className={`${glass} relative z-30 flex min-w-0 flex-col md:sticky md:top-4 md:h-[calc(100vh-2rem)]`}>
          <div className="flex items-center gap-3 px-5 py-5">
            <span className="grid h-10 w-10 place-items-center rounded-2xl bg-debian text-sm font-semibold text-white">
              gd
            </span>
            <div className="min-w-0 flex-1">
              <div className="text-base leading-none font-semibold tracking-tight">GoDeb</div>
              <div className="mt-1 text-[11px] tracking-wide text-white/60 uppercase">{t("nav.subtitle")}</div>
            </div>
            {/* On phones the sidebar is the top bar, so the menu sits here. */}
            <div className="md:hidden">{menu}</div>
          </div>

          <nav className="flex gap-1 overflow-x-auto px-3 pb-4 md:block md:flex-1">
            <NavLink href="#/" active={route === "/"} icon="server" label={t("nav.overview")} />
            <NavLink href="#/plugin-manager" active={route === "/plugin-manager"} icon="cogs" label={t("nav.plugins")} />
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

          <p className="hidden border-t border-white/10 px-5 py-4 text-[11px] text-white/50 md:block">
            {t("nav.footer")}
          </p>
          {themeOpen ? <ThemeSettings onClose={() => setThemeOpen(false)} /> : null}
          {sessionsOpen ? <SessionsDialog onClose={() => setSessionsOpen(false)} onSignedOut={onLogout} /> : null}
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
      className={`mb-1 flex shrink-0 items-center gap-2 rounded-2xl px-3 py-2 text-sm transition-all duration-200 ease-in-out ${
        active ? "bg-white/10 text-white" : "text-white/75 hover:bg-white/5"
      }`}
    >
      <PluginIcon name={icon} />
      <span>{label}</span>
    </a>
  );
}
