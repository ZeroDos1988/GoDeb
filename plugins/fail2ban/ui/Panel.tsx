import { useState } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import type { CommandEnvelope } from "@/lib/types.ts";
import { useGet } from "@/lib/useAsync.ts";

type Jail = {
  name: string;
  currentlyFailed: number;
  totalFailed: number;
  currentlyBanned: number;
  totalBanned: number;
  bannedIPs: string[];
};

type Fail2banStatus = CommandEnvelope & {
  jails: Jail[];
};

const actionButton =
  "rounded-2xl border border-white/10 bg-white/5 px-3 py-1 text-xs text-white transition-all duration-200 ease-in-out enabled:hover:bg-white/10 disabled:opacity-50";

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<Fail2banStatus>("/api/plugins/fail2ban/status");
  const [busy, setBusy] = useState("");
  const [actionError, setActionError] = useState("");
  const [notice, setNotice] = useState("");

  const jails = query.data?.jails ?? [];
  const banned = jails.flatMap((jail) => jail.bannedIPs.map((ip) => ({ ip, jail: jail.name })));

  async function unban(ip: string) {
    setBusy(ip);
    setActionError("");
    setNotice("");
    try {
      await api("/api/plugins/fail2ban/unban", {
        method: "POST",
        body: JSON.stringify({ ip }),
      });
      setNotice(t("fail2ban.unbanned", { ip }));
      query.reload();
    } catch (err: unknown) {
      setActionError(err instanceof Error ? err.message : t("fail2ban.unbanFailed"));
    } finally {
      setBusy("");
    }
  }

  return (
    <PanelFrame
      title={t("fail2ban.title")}
      description={t("fail2ban.description")}
      command={query.data?.command}
      mocked={query.data?.mocked}
      detail={query.data?.detail}
      loading={query.loading}
      error={query.error}
      onRefresh={query.reload}
    >
      {query.data ? (
        <>
          {jails.length === 0 ? (
            <p className="text-sm text-muted">{t("fail2ban.none")}</p>
          ) : (
            <DataTable
              head={[
                t("fail2ban.jail"),
                t("fail2ban.failedNow"),
                t("fail2ban.failedTotal"),
                t("fail2ban.bannedNow"),
                t("fail2ban.bannedTotal"),
              ]}
            >
              {jails.map((jail) => (
                <tr key={jail.name}>
                  <td className="px-3 py-2 font-mono text-xs">{jail.name}</td>
                  <td className="px-3 py-2">{jail.currentlyFailed}</td>
                  <td className="px-3 py-2">{jail.totalFailed}</td>
                  <td className={`px-3 py-2 ${jail.currentlyBanned > 0 ? "font-medium text-rose-300" : ""}`}>
                    {jail.currentlyBanned}
                  </td>
                  <td className="px-3 py-2">{jail.totalBanned}</td>
                </tr>
              ))}
            </DataTable>
          )}

          <h2 className="mt-8 mb-3 text-sm font-semibold">{t("fail2ban.blockedTitle")}</h2>
          {actionError ? <p className="mb-3 text-sm text-rose-300">{actionError}</p> : null}
          {notice ? <p className="mb-3 text-sm text-emerald-300">{notice}</p> : null}
          {banned.length === 0 ? (
            <p className="text-sm text-muted">{t("fail2ban.noneBlocked")}</p>
          ) : (
            <DataTable head={[t("fail2ban.ip"), t("fail2ban.jail"), ""]}>
              {banned.map((entry) => (
                <tr key={`${entry.jail}-${entry.ip}`}>
                  <td className="px-3 py-2 font-mono text-xs">{entry.ip}</td>
                  <td className="px-3 py-2 font-mono text-xs">{entry.jail}</td>
                  <td className="px-3 py-2 text-right">
                    <button type="button" disabled={busy !== ""} onClick={() => void unban(entry.ip)} className={actionButton}>
                      {busy === entry.ip ? t("fail2ban.unbanning") : t("fail2ban.unban")}
                    </button>
                  </td>
                </tr>
              ))}
            </DataTable>
          )}
        </>
      ) : null}
    </PanelFrame>
  );
}
