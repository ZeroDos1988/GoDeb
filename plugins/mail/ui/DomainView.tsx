import { useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";
import {
  Badge,
  base,
  button,
  card,
  CopyButton,
  danger,
  type Account,
  type DnsRecord,
  type Domain,
  errorText,
  Field,
  input,
  primary,
  randomPassword,
  size,
  type Status,
} from "./shared.tsx";

type DomainData = {
  domain: Domain;
  accounts: Account[];
  records: DnsRecord[];
  client: {
    host: string;
    imapPort: number;
    smtpPort: number;
    submissionPort: number;
  };
};

type Tab = "accounts" | "dns" | "settings" | "client";

export function DomainView({
  name,
  status,
  onBack,
  onChanged,
}: {
  name: string;
  status: Status;
  onBack: () => void;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const query = useGet<DomainData>(
    `${base}/domains/${encodeURIComponent(name)}`,
  );
  const [tab, setTab] = useState<Tab>("accounts");
  const data = query.data;
  const reload = () => {
    query.reload();
    onChanged();
  };

  return (
    <section>
      <button
        type="button"
        onClick={onBack}
        className="mb-2 text-sm text-muted hover:text-debian"
      >
        ← {t("mail.title")}
      </button>
      <div className="mb-4 flex flex-wrap items-center gap-3">
        <h1 className="font-mono text-2xl font-semibold tracking-tight">
          {name}
        </h1>
        {data?.domain.suspended ? (
          <Badge tone="warn">{t("mail.suspended")}</Badge>
        ) : null}
      </div>
      <div className="mb-4 inline-flex max-w-full overflow-x-auto rounded-2xl border border-white/10 bg-slate-900/60 p-1 text-sm text-white backdrop-blur-xl">
        {(["accounts", "dns", "settings", "client"] as const).map((id) => (
          <button
            key={id}
            type="button"
            aria-pressed={tab === id}
            onClick={() => setTab(id)}
            className={`shrink-0 rounded-xl px-3 py-1.5 transition-all duration-200 ${tab === id ? "bg-white/15" : "text-white/70 hover:text-white"}`}
          >
            {t(`mail.tab.${id}`)}
          </button>
        ))}
      </div>
      {query.error ? (
        <p className="text-sm text-debian">{query.error}</p>
      ) : null}
      {data ? (
        <>
          {tab === "accounts" ? (
            <Accounts data={data} onChanged={reload} />
          ) : null}
          {tab === "dns" ? <Dns name={name} initial={data.records} /> : null}
          {tab === "settings" ? (
            <DomainSettings
              data={data}
              onChanged={reload}
              onDeleted={() => {
                onChanged();
                onBack();
              }}
            />
          ) : null}
          {tab === "client" ? (
            <ClientSettings data={data} letsEncrypt={status.letsEncrypt} />
          ) : null}
        </>
      ) : null}
    </section>
  );
}

function Accounts({
  data,
  onChanged,
}: {
  data: DomainData;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const [edit, setEdit] = useState<Account | "new" | null>(null);
  const [created, setCreated] = useState<{
    address: string;
    password: string;
  } | null>(null);

  return (
    <div className="space-y-4">
      {created ? (
        <Credentials
          {...created}
          data={data}
          onClose={() => setCreated(null)}
        />
      ) : null}
      {edit ? (
        <AccountForm
          domain={data.domain.name}
          account={edit === "new" ? null : edit}
          onCancel={() => setEdit(null)}
          onSaved={(addr, pw) => {
            setEdit(null);
            if (pw) setCreated({ address: addr, password: pw });
            onChanged();
          }}
        />
      ) : (
        <button
          type="button"
          className={primary}
          onClick={() => setEdit("new")}
        >
          {t("mail.addAccount")}
        </button>
      )}
      {data.accounts.length === 0 ? (
        <p className="text-sm text-muted">{t("mail.noAccounts")}</p>
      ) : (
        <DataTable
          head={[
            t("mail.address"),
            t("mail.usage"),
            t("mail.aliases"),
            t("mail.forward"),
            t("mail.status"),
            "",
          ]}
        >
          {data.accounts.map((a) => (
            <tr key={a.address}>
              <td className="px-3 py-2 font-mono text-sm">{a.address}</td>
              <td className="px-3 py-2">
                {a.forwardOnly ? (
                  <span className="text-xs text-white/50">
                    {t("mail.forwardOnlyShort")}
                  </span>
                ) : (
                  <Usage usedKB={a.usedKB} quotaMB={a.quotaMB} />
                )}
              </td>
              <td className="px-3 py-2 font-mono text-xs">
                {a.aliases.length
                  ? a.aliases.map((x) => `${x}@`).join(", ")
                  : "–"}
              </td>
              <td className="max-w-[16rem] px-3 py-2 font-mono text-xs break-all">
                {a.forward.length ? a.forward.join(", ") : "–"}
              </td>
              <td className="px-3 py-2">
                {a.suspended ? (
                  <Badge tone="warn">{t("mail.suspended")}</Badge>
                ) : (
                  <Badge tone="ok">{t("mail.active")}</Badge>
                )}
              </td>
              <td className="px-3 py-2 text-right whitespace-nowrap">
                <button
                  type="button"
                  className="text-xs text-white/80 hover:underline"
                  onClick={() => setEdit(a)}
                >
                  {t("mail.edit")}
                </button>
              </td>
            </tr>
          ))}
        </DataTable>
      )}
    </div>
  );
}

function Usage({ usedKB, quotaMB }: { usedKB: number; quotaMB: number }) {
  const { t } = useTranslation();
  const limitKB = quotaMB * 1024;
  const pct =
    limitKB > 0 && usedKB >= 0
      ? Math.min(100, Math.round((usedKB / limitKB) * 100))
      : 0;
  return (
    <div className="min-w-28 text-xs">
      <div>
        {size(usedKB)} / {quotaMB > 0 ? size(limitKB) : t("mail.unlimited")}
      </div>
      {quotaMB > 0 ? (
        <div className="mt-1 h-1.5 rounded-full bg-white/10">
          <div
            className={`h-1.5 rounded-full ${pct > 90 ? "bg-rose-400" : pct > 70 ? "bg-amber-300" : "bg-emerald-400"}`}
            style={{ width: `${pct}%` }}
          />
        </div>
      ) : null}
    </div>
  );
}

function lines(v: string): string[] {
  return v
    .split(/[\s,;]+/)
    .map((x) => x.trim())
    .filter(Boolean);
}

function AccountForm({
  domain,
  account,
  onCancel,
  onSaved,
}: {
  domain: string;
  account: Account | null;
  onCancel: () => void;
  onSaved: (address: string, password: string) => void;
}) {
  const { t } = useTranslation();
  const isNew = account === null;
  const [local, setLocal] = useState(account?.local ?? "");
  const [password, setPassword] = useState(isNew ? randomPassword() : "");
  const [show, setShow] = useState(isNew);
  const [quota, setQuota] = useState(account?.quotaMB ?? 1024);
  const [aliases, setAliases] = useState((account?.aliases ?? []).join("\n"));
  const [forward, setForward] = useState((account?.forward ?? []).join("\n"));
  const [forwardOnly, setForwardOnly] = useState(account?.forwardOnly ?? false);
  const [suspended, setSuspended] = useState(account?.suspended ?? false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    const body = {
      local,
      password,
      quotaMB: quota,
      aliases: lines(aliases),
      forward: lines(forward),
      forwardOnly,
      suspended,
    };
    try {
      const url = isNew
        ? `${base}/domains/${domain}/accounts`
        : `${base}/domains/${domain}/accounts/${account.local}`;
      await api(url, { method: "POST", body: JSON.stringify(body) });
      onSaved(
        `${local.trim().toLowerCase()}@${domain}`,
        !forwardOnly ? password : "",
      );
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    if (
      !account ||
      !window.confirm(
        t("mail.confirmDeleteAccount", { address: account.address }),
      )
    )
      return;
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${domain}/accounts/${account.local}/delete`, {
        method: "POST",
      });
      onSaved(account.address, "");
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={(e) => void submit(e)} className={`${card} space-y-4`}>
      <h2 className="text-sm font-semibold">
        {isNew ? t("mail.addAccount") : account.address}
      </h2>
      <div className="grid gap-4 md:grid-cols-2">
        <Field label={t("mail.address")}>
          <div className="flex items-center gap-2">
            <input
              value={local}
              onChange={(e) => setLocal(e.target.value)}
              disabled={!isNew}
              required
              className={`${input} font-mono`}
              placeholder="info"
            />
            <span className="shrink-0 font-mono text-sm text-white/60">
              @{domain}
            </span>
          </div>
        </Field>
        {!forwardOnly ? (
          <Field
            label={isNew ? t("mail.password") : t("mail.newPassword")}
            hint={isNew ? t("mail.passwordHint") : t("mail.newPasswordHint")}
          >
            <div className="flex gap-2">
              <input
                type={show ? "text" : "password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required={isNew}
                minLength={10}
                maxLength={128}
                autoComplete="new-password"
                className={`${input} font-mono`}
              />
              <button
                type="button"
                className={button}
                onClick={() => setShow((v) => !v)}
                aria-label={t("mail.showPassword")}
              >
                {show ? t("mail.hide") : t("mail.show")}
              </button>
              <button
                type="button"
                className={button}
                onClick={() => {
                  setPassword(randomPassword());
                  setShow(true);
                }}
              >
                {t("mail.generate")}
              </button>
            </div>
          </Field>
        ) : null}
        {!forwardOnly ? (
          <Field label={t("mail.quota")} hint={t("mail.quotaHint")}>
            <input
              type="number"
              min={0}
              max={1048576}
              value={quota}
              onChange={(e) => setQuota(Number(e.target.value) || 0)}
              className={input}
            />
          </Field>
        ) : null}
        <Field
          label={t("mail.aliases")}
          hint={t("mail.aliasesHint", { domain })}
        >
          <textarea
            value={aliases}
            onChange={(e) => setAliases(e.target.value)}
            rows={3}
            className={`${input} font-mono text-xs`}
            placeholder={"kontakt\npostmaster"}
          />
        </Field>
        <Field label={t("mail.forward")} hint={t("mail.forwardHint")}>
          <textarea
            value={forward}
            onChange={(e) => setForward(e.target.value)}
            rows={3}
            className={`${input} font-mono text-xs`}
            placeholder="name@example.org"
          />
        </Field>
      </div>
      <div className="space-y-2 text-sm">
        <label className="flex items-start gap-2">
          <input
            type="checkbox"
            className="mt-1 h-4 w-4"
            checked={forwardOnly}
            onChange={(e) => setForwardOnly(e.target.checked)}
          />
          <span>
            {t("mail.forwardOnly")}
            <span className="block text-xs text-white/50">
              {t("mail.forwardOnlyHint")}
            </span>
          </span>
        </label>
        {!isNew ? (
          <label className="flex items-start gap-2">
            <input
              type="checkbox"
              className="mt-1 h-4 w-4"
              checked={suspended}
              onChange={(e) => setSuspended(e.target.checked)}
            />
            <span>
              {t("mail.suspend")}
              <span className="block text-xs text-white/50">
                {t("mail.suspendHint")}
              </span>
            </span>
          </label>
        ) : null}
      </div>
      {error ? <p className="text-sm text-rose-300">{error}</p> : null}
      <div className="flex flex-wrap gap-2">
        <button type="submit" className={primary} disabled={busy}>
          {isNew ? t("mail.create") : t("mail.save")}
        </button>
        <button
          type="button"
          className={button}
          onClick={onCancel}
          disabled={busy}
        >
          {t("common.cancel")}
        </button>
        {!isNew ? (
          <button
            type="button"
            className={`${danger} ms-auto`}
            onClick={() => void remove()}
            disabled={busy}
          >
            {t("mail.delete")}
          </button>
        ) : null}
      </div>
    </form>
  );
}

// Credentials is shown once after creating a mailbox or setting a new
// password: GoDeb keeps only the hash.
function Credentials({
  address,
  password,
  data,
  onClose,
}: {
  address: string;
  password: string;
  data: DomainData;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const text = t("mail.credentialsText", {
    address,
    password,
    host: data.client.host,
    imap: data.client.imapPort,
    smtp: data.client.smtpPort,
  });
  return (
    <section className={`${card} border-emerald-300/30`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold">{t("mail.credentials")}</h2>
        <div className="flex gap-2">
          <CopyButton text={text} label={t("mail.copyAll")} />
          <button
            type="button"
            className="text-sm text-white/60 hover:text-white"
            onClick={onClose}
          >
            ✕
          </button>
        </div>
      </div>
      <pre className="mt-3 overflow-x-auto rounded-2xl bg-slate-950/60 p-3 font-mono text-xs whitespace-pre-wrap">
        {text}
      </pre>
      <p className="mt-2 text-xs text-amber-200">{t("mail.credentialsOnce")}</p>
    </section>
  );
}

function Dns({ name, initial }: { name: string; initial: DnsRecord[] }) {
  const { t } = useTranslation();
  const [records, setRecords] = useState(initial);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function check() {
    setBusy(true);
    setError("");
    try {
      const res = await api<{ records: DnsRecord[] }>(
        `${base}/domains/${encodeURIComponent(name)}/dns`,
      );
      setRecords(res.records);
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy(false);
    }
  }

  const tone = {
    ok: "ok",
    missing: "bad",
    wrong: "warn",
    unknown: "idle",
  } as const;
  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-3">
        <button
          type="button"
          className={primary}
          onClick={() => void check()}
          disabled={busy}
        >
          {busy ? t("mail.checking") : t("mail.checkDns")}
        </button>
        <span className="text-xs text-muted">{t("mail.dnsHint")}</span>
        {error ? <span className="text-sm text-debian">{error}</span> : null}
      </div>
      <ul className="space-y-2">
        {records.map((r) => (
          <li key={r.kind + r.name} className={card}>
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-semibold">{t(`mail.dns.${r.kind}`)}</span>
              <Badge tone={tone[r.status]}>
                {t(`mail.dnsStatus.${r.status}`)}
              </Badge>
            </div>
            <p className="mt-1 text-xs text-white/60">
              {t(`mail.dnsHelp.${r.kind}`)}
            </p>
            <dl className="mt-2 grid gap-2 text-xs sm:grid-cols-[6rem_1fr]">
              <dt className="text-white/50">{t("mail.dnsName")}</dt>
              <dd className="flex min-w-0 items-start gap-2">
                <span className="min-w-0 font-mono break-all">{r.name}</span>
                <CopyButton text={r.name} />
              </dd>
              <dt className="text-white/50">{t("mail.dnsType")}</dt>
              <dd className="font-mono">{r.type}</dd>
              <dt className="text-white/50">{t("mail.dnsValue")}</dt>
              <dd className="flex min-w-0 items-start gap-2">
                <span className="min-w-0 font-mono break-all">{r.value}</span>
                <CopyButton text={r.value} />
              </dd>
              {r.found && r.status !== "ok" ? (
                <>
                  <dt className="text-white/50">{t("mail.dnsFound")}</dt>
                  <dd className="min-w-0 font-mono break-all text-amber-200">
                    {r.found}
                  </dd>
                </>
              ) : null}
            </dl>
          </li>
        ))}
      </ul>
    </div>
  );
}

function DomainSettings({
  data,
  onChanged,
  onDeleted,
}: {
  data: DomainData;
  onChanged: () => void;
  onDeleted: () => void;
}) {
  const { t } = useTranslation();
  const d = data.domain;
  const [catchAll, setCatchAll] = useState(d.catchAll);
  const [suspended, setSuspended] = useState(d.suspended);
  const [confirmName, setConfirmName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);

  async function save(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${d.name}`, {
        method: "POST",
        body: JSON.stringify({ catchAll, suspended }),
      });
      setSaved(true);
      window.setTimeout(() => setSaved(false), 2000);
      onChanged();
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy(false);
    }
  }

  async function remove() {
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains/${d.name}/delete`, { method: "POST" });
      onDeleted();
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
      setBusy(false);
    }
  }

  return (
    <div className="space-y-4">
      <form
        onSubmit={(e) => void save(e)}
        className={`${card} max-w-2xl space-y-4`}
      >
        <Field label={t("mail.catchAll")} hint={t("mail.catchAllHint")}>
          <input
            value={catchAll}
            onChange={(e) => setCatchAll(e.target.value)}
            list="mail-addresses"
            placeholder={`info@${d.name}`}
            className={`${input} font-mono`}
          />
          <datalist id="mail-addresses">
            {data.accounts.map((a) => (
              <option key={a.address} value={a.address} />
            ))}
          </datalist>
        </Field>
        <label className="flex items-start gap-2 text-sm">
          <input
            type="checkbox"
            className="mt-1 h-4 w-4"
            checked={suspended}
            onChange={(e) => setSuspended(e.target.checked)}
          />
          <span>
            {t("mail.suspendDomain")}
            <span className="block text-xs text-white/50">
              {t("mail.suspendDomainHint")}
            </span>
          </span>
        </label>
        <dl className="text-xs text-white/60">
          <dt className="inline">DKIM-Selector: </dt>
          <dd className="inline font-mono">{d.selector}</dd>
        </dl>
        {error ? <p className="text-sm text-rose-300">{error}</p> : null}
        <button type="submit" className={primary} disabled={busy}>
          {saved ? t("mail.saved") : t("mail.save")}
        </button>
      </form>
      <section className={`${card} max-w-2xl border-rose-300/30`}>
        <h2 className="text-sm font-semibold text-rose-200">
          {t("mail.deleteDomain")}
        </h2>
        <p className="mt-1 text-xs text-white/60">
          {t("mail.deleteDomainHint", {
            domain: d.name,
            count: data.accounts.length,
          })}
        </p>
        <div className="mt-3 flex flex-wrap gap-2">
          <input
            value={confirmName}
            onChange={(e) => setConfirmName(e.target.value)}
            placeholder={d.name}
            className={`${input} max-w-72 font-mono`}
          />
          <button
            type="button"
            className={danger}
            disabled={busy || confirmName.trim() !== d.name}
            onClick={() => void remove()}
          >
            {t("mail.delete")}
          </button>
        </div>
      </section>
    </div>
  );
}

function ClientSettings({
  data,
  letsEncrypt,
}: {
  data: DomainData;
  letsEncrypt: boolean;
}) {
  const { t } = useTranslation();
  const c = data.client;
  const rows: [string, string][] = [
    [
      t("mail.client.user"),
      t("mail.client.userValue", { domain: data.domain.name }),
    ],
    [
      t("mail.client.imap"),
      `${c.host} · ${t("mail.client.port")} ${c.imapPort} · SSL/TLS`,
    ],
    [
      t("mail.client.smtp"),
      `${c.host} · ${t("mail.client.port")} ${c.smtpPort} · SSL/TLS`,
    ],
    [
      t("mail.client.smtpAlt"),
      `${c.host} · ${t("mail.client.port")} ${c.submissionPort} · STARTTLS`,
    ],
    [t("mail.client.auth"), t("mail.client.authValue")],
  ];
  return (
    <section className={`${card} max-w-2xl`}>
      <p className="mb-3 text-sm text-white/70">{t("mail.client.text")}</p>
      <dl className="grid gap-x-4 gap-y-2 text-sm sm:grid-cols-[10rem_1fr]">
        {rows.map(([k, v]) => (
          <div key={k} className="contents">
            <dt className="text-white/50">{k}</dt>
            <dd className="font-mono break-all">{v}</dd>
          </div>
        ))}
      </dl>
      {!letsEncrypt ? (
        <p className="mt-3 text-xs text-amber-200">
          {t("mail.client.selfSigned")}
        </p>
      ) : null}
    </section>
  );
}
