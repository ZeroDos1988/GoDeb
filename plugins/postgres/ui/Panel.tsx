import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";
import * as ui from "@/components/ui.ts";

type Status = {
  active: string;
  databases: string[];
  roles: string[];
  managed: Record<string, boolean>;
  mocked?: boolean;
  detail?: string;
  listen: string;
};

const input = `${ui.input} font-mono`;
const primary = ui.primary;
const card = ui.card;

export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<Status>("/api/plugins/postgres/status");
  const [dbName, setDbName] = useState("");
  const [dbOwner, setDbOwner] = useState("");
  const [roleName, setRoleName] = useState("");
  const [rolePass, setRolePass] = useState("");
  const [browseDB, setBrowseDB] = useState("");
  const [tables, setTables] = useState<string[]>([]);
  const [table, setTable] = useState("");
  const [cols, setCols] = useState<string[]>([]);
  const [rows, setRows] = useState<string[][]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function createDB(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/api/plugins/postgres/databases", {
        method: "POST",
        body: JSON.stringify({ name: dbName, owner: dbOwner || roleName }),
      });
      setDbName("");
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("postgres.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function createRole(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api("/api/plugins/postgres/roles", {
        method: "POST",
        body: JSON.stringify({ name: roleName, password: rolePass }),
      });
      setRoleName("");
      setRolePass("");
      query.reload();
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("postgres.failed"));
    } finally {
      setBusy(false);
    }
  }

  async function loadTables(name: string) {
    setBrowseDB(name);
    setTable("");
    setCols([]);
    setRows([]);
    try {
      const res = await api<{ tables: string[] }>(
        `/api/plugins/postgres/databases/${encodeURIComponent(name)}/tables`,
      );
      setTables(res.tables);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("postgres.failed"));
    }
  }

  async function loadRows(tbl: string) {
    setTable(tbl);
    try {
      const res = await api<{ columns: string[]; rows: string[][] }>(
        `/api/plugins/postgres/databases/${encodeURIComponent(browseDB)}/tables/${encodeURIComponent(tbl)}`,
      );
      setCols(res.columns);
      setRows(res.rows);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : t("postgres.failed"));
    }
  }

  return (
    <PanelFrame
      title={t("postgres.title")}
      description={t("postgres.description")}
      mocked={query.data?.mocked}
      detail={query.data?.detail}
      loading={query.loading}
      error={query.error}
      onRefresh={query.reload}
    >
      <p className="mb-4 text-sm text-white/70">
        {t("postgres.status")}:{" "}
        <span className="font-mono">{query.data?.active ?? "–"}</span> · {query.data?.listen}
      </p>
      {error ? <p className="mb-4 text-sm text-debian">{error}</p> : null}

      <div className="mb-4 grid gap-4 md:grid-cols-2">
        <section className={card}>
          <h2 className="mb-2 text-sm font-semibold">{t("postgres.databases")}</h2>
          <ul className="mb-3 space-y-1 text-sm font-mono">
            {(query.data?.databases ?? []).map((d) => (
              <li key={d} className="flex justify-between gap-2">
                <button type="button" className="text-left hover:underline" onClick={() => void loadTables(d)}>
                  {d}
                </button>
                {d !== "postgres" ? (
                  <button
                    type="button"
                    className="text-xs text-rose-300"
                    disabled={busy}
                    onClick={() => {
                      if (window.confirm(t("postgres.confirmDropDB", { name: d }))) {
                        void api(`/api/plugins/postgres/databases/${encodeURIComponent(d)}/delete`, {
                          method: "POST",
                          body: "{}",
                        }).then(() => query.reload());
                      }
                    }}
                  >
                    {t("postgres.delete")}
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
          <form onSubmit={(e) => void createDB(e)} className="space-y-2">
            <input className={input} placeholder={t("postgres.dbName")} value={dbName} onChange={(e) => setDbName(e.target.value)} required />
            <input className={input} placeholder={t("postgres.owner")} value={dbOwner} onChange={(e) => setDbOwner(e.target.value)} />
            <button type="submit" className={primary} disabled={busy}>
              {t("postgres.createDB")}
            </button>
          </form>
        </section>

        <section className={card}>
          <h2 className="mb-2 text-sm font-semibold">{t("postgres.roles")}</h2>
          <ul className="mb-3 space-y-1 text-sm font-mono">
            {(query.data?.roles ?? []).map((role) => (
              <li key={role} className="flex justify-between gap-2">
                <span>
                  {role}
                  {query.data?.managed?.[role] ? (
                    <span className="ms-2 text-[10px] text-emerald-300 uppercase">GoDeb</span>
                  ) : null}
                </span>
                {role !== "postgres" && query.data?.managed?.[role] ? (
                  <button
                    type="button"
                    className="text-xs text-rose-300"
                    disabled={busy}
                    onClick={() => {
                      if (window.confirm(t("postgres.confirmDropRole", { name: role }))) {
                        void api(`/api/plugins/postgres/roles/${encodeURIComponent(role)}/delete`, {
                          method: "POST",
                          body: "{}",
                        }).then(() => query.reload());
                      }
                    }}
                  >
                    {t("postgres.delete")}
                  </button>
                ) : null}
              </li>
            ))}
          </ul>
          <form onSubmit={(e) => void createRole(e)} className="space-y-2">
            <input className={input} placeholder={t("postgres.roleName")} value={roleName} onChange={(e) => setRoleName(e.target.value)} required />
            <input className={input} type="password" placeholder={t("postgres.password")} value={rolePass} onChange={(e) => setRolePass(e.target.value)} required minLength={10} />
            <button type="submit" className={primary} disabled={busy}>
              {t("postgres.createRole")}
            </button>
          </form>
        </section>
      </div>

      {browseDB ? (
        <section className={`${card} space-y-3`}>
          <h2 className="text-sm font-semibold">
            {t("postgres.browse")}: <span className="font-mono">{browseDB}</span>
          </h2>
          <div className="flex flex-wrap gap-2">
            {tables.map((tbl) => (
              <button key={tbl} type="button" className="rounded-xl border border-white/10 px-3 py-1.5 font-mono text-xs" onClick={() => void loadRows(tbl)}>
                {tbl}
              </button>
            ))}
            {tables.length === 0 ? <p className="text-sm text-muted">{t("postgres.noTables")}</p> : null}
          </div>
          {table && cols.length ? (
            <DataTable mobileCards head={cols}>
              {rows.map((row, i) => (
                <tr key={i}>
                  {row.map((cell, j) => (
                    <td key={j} className="px-3 py-2 font-mono text-xs">
                      {cell}
                    </td>
                  ))}
                </tr>
              ))}
            </DataTable>
          ) : null}
        </section>
      ) : null}
    </PanelFrame>
  );
}
