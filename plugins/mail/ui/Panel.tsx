import { useEffect, useState, type FormEvent } from "react";
import { useTranslation } from "react-i18next";
import { DataTable, PanelFrame } from "@/components/PanelFrame.tsx";
import { api } from "@/lib/api.ts";
import { useGet } from "@/lib/useAsync.ts";
import { DomainView } from "./DomainView.tsx";
import {
  Badge,
  base,
  button,
  card,
  danger,
  errorText,
  Field,
  input,
  JobCard,
  primary,
  size,
  type Status,
} from "./shared.tsx";

type QueueMessage = {
  id: string;
  sender: string;
  recipients: string[];
  arrival: string;
  status: string;
  reason?: string;
  size: number;
};

type QueueData = {
  messages: QueueMessage[];
  count: number;
  mocked?: boolean;
};

// The mail server follows HestiaCP: a list of mail domains, and per domain
// its mailboxes, DNS records, settings and the client configuration.
export default function Panel() {
  const { t } = useTranslation();
  const query = useGet<Status>(`${base}/status`);
  const { reload } = query;
  const data = query.data;
  const [open, setOpen] = useState("");

  // Follow a running installation.
  const running = data?.job?.running ?? false;
  useEffect(() => {
    if (!running) return;
    const id = window.setInterval(reload, 3000);
    return () => window.clearInterval(id);
  }, [running, reload]);

  function openDomain(name: string) {
    setOpen(name);
    window.scrollTo({ top: 0 });
  }

  if (data?.installed && open) {
    return (
      <DomainView
        name={open}
        status={data}
        onBack={() => openDomain("")}
        onChanged={reload}
      />
    );
  }

  return (
    <PanelFrame
      title={t("mail.title")}
      description={t("mail.description")}
      loading={query.loading && !data}
      error={running ? "" : query.error}
      onRefresh={reload}
    >
      {data ? (
        data.installed ? (
          <>
            <ServerCard status={data} onChanged={reload} />
            <QueueCard />
            <Domains status={data} onOpen={openDomain} onChanged={reload} />
          </>
        ) : (
          <Setup status={data} onStarted={reload} />
        )
      ) : null}
      {data?.job ? <JobCard job={data.job} /> : null}
    </PanelFrame>
  );
}

function Setup({
  status,
  onStarted,
}: {
  status: Status;
  onStarted: () => void;
}) {
  const { t } = useTranslation();
  const [hostname, setHostname] = useState(status.settings.hostname || "");
  const [clamav, setClamav] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const running = status.job?.running ?? false;

  async function submit(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(`${base}/install`, {
        method: "POST",
        body: JSON.stringify({ hostname, clamav }),
      });
      onStarted();
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy(false);
    }
  }

  return (
    <form
      onSubmit={(e) => void submit(e)}
      className={`${card} space-y-4`}
    >
      <div>
        <h2 className="text-base font-semibold">{t("mail.setup.title")}</h2>
        <p className="mt-1 text-sm text-white/60">{t("mail.setup.text")}</p>
      </div>
      {status.gatewayDomains > 0 ? (
        <p className="rounded-2xl border border-amber-300/30 bg-amber-400/10 px-3 py-2 text-sm text-amber-100">
          {t("mail.setup.gateway", { count: status.gatewayDomains })}
        </p>
      ) : null}
      <Field
        label={t("mail.setup.hostname")}
        hint={t("mail.setup.hostnameHint", { ip: status.publicIp || "–" })}
      >
        <input
          value={hostname}
          onChange={(e) => setHostname(e.target.value)}
          placeholder="mail.example.com"
          required
          className={`${input} font-mono`}
        />
      </Field>
      <label className="flex items-start gap-2 text-sm">
        <input
          type="checkbox"
          className="mt-1 h-4 w-4"
          checked={clamav}
          onChange={(e) => setClamav(e.target.checked)}
        />
        <span>
          {t("mail.clamav")}
          <span className="block text-xs text-white/50">
            {t("mail.clamavHint")}
          </span>
        </span>
      </label>
      <ul className="list-disc space-y-1 ps-5 text-xs text-white/60">
        <li>{t("mail.setup.what1")}</li>
        <li>{t("mail.setup.what2")}</li>
        <li>{t("mail.setup.what3")}</li>
      </ul>
      {error ? <p className="text-sm text-rose-300">{error}</p> : null}
      <button
        type="submit"
        className={primary}
        disabled={
          busy || running || hostname.trim() === "" || status.gatewayDomains > 0
        }
      >
        {running ? t("mail.running") : t("mail.setup.install")}
      </button>
    </form>
  );
}

function ServerCard({
  status,
  onChanged,
}: {
  status: Status;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const [email, setEmail] = useState("");
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");
  const running = status.job?.running ?? false;
  const s = status.settings;

  async function post(path: string, body: unknown) {
    setBusy(path);
    setError("");
    try {
      await api(`${base}/${path}`, {
        method: "POST",
        body: JSON.stringify(body),
      });
      onChanged();
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy("");
    }
  }

  return (
    <section className={`${card} mb-4 space-y-3`}>
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <h2 className="text-sm font-semibold">{t("mail.server")}</h2>
        <span className="font-mono text-sm">{s.hostname}</span>
        {status.services.map((sv) => (
          <Badge key={sv.unit} tone={sv.active === "active" ? "ok" : "bad"}>
            {sv.unit} · {sv.active}
          </Badge>
        ))}
      </div>
      {!status.hostResolves ? (
        <p className="text-xs text-amber-200">
          {t("mail.hostNotResolved", {
            host: s.hostname,
            ip: status.publicIp || "–",
          })}
        </p>
      ) : null}
      <div className="grid gap-3 md:grid-cols-2">
        <div className="space-y-2 text-sm">
          <div className="flex flex-wrap items-center gap-2">
            <span className="text-white/60">{t("mail.tls")}</span>
            {status.letsEncrypt ? (
              <Badge tone="ok">Let's Encrypt</Badge>
            ) : (
              <Badge tone="warn">{t("mail.selfSigned")}</Badge>
            )}
          </div>
          {!status.letsEncrypt ? (
            <div className="flex flex-wrap gap-2">
              <input
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                placeholder={t("mail.certEmail")}
                className={`${input} max-w-60 flex-1`}
                type="email"
              />
              <button
                type="button"
                className={button}
                disabled={running || busy !== "" || !status.hostResolves}
                onClick={() => void post("certificate", { email })}
              >
                {t("mail.getCert")}
              </button>
            </div>
          ) : null}
          {!status.letsEncrypt ? (
            <p className="text-xs text-white/50">{t("mail.certHint")}</p>
          ) : null}
        </div>
        <label className="flex items-start gap-2 text-sm">
          <input
            type="checkbox"
            className="mt-1 h-4 w-4"
            checked={s.clamav}
            disabled={running || busy !== ""}
            onChange={(e) => void post("clamav", { enabled: e.target.checked })}
          />
          <span>
            {t("mail.clamav")}
            <span className="block text-xs text-white/50">
              {t("mail.clamavHint")}
            </span>
          </span>
        </label>
      </div>
      {error ? <p className="text-sm text-rose-300">{error}</p> : null}
    </section>
  );
}

function Domains({
  status,
  onOpen,
  onChanged,
}: {
  status: Status;
  onOpen: (name: string) => void;
  onChanged: () => void;
}) {
  const { t } = useTranslation();
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function add(e: FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      await api(`${base}/domains`, {
        method: "POST",
        body: JSON.stringify({ name }),
      });
      setAdding(false);
      onChanged();
      onOpen(name.trim().toLowerCase());
      setName("");
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy(false);
    }
  }

  return (
    <section>
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-lg font-semibold tracking-tight">
          {t("mail.domains")}
        </h2>
        {!adding ? (
          <button
            type="button"
            className={primary}
            onClick={() => setAdding(true)}
          >
            {t("mail.addDomain")}
          </button>
        ) : null}
      </div>
      {adding ? (
        <form
          onSubmit={(e) => void add(e)}
          className={`${card} mb-4 flex flex-wrap items-end gap-3`}
        >
          <div className="min-w-0 flex-1 basis-60">
            <Field label={t("mail.domain")} hint={t("mail.addDomainHint")}>
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="example.com"
                required
                autoFocus
                className={`${input} font-mono`}
              />
            </Field>
          </div>
          <div className="flex gap-2 pb-0.5">
            <button type="submit" className={primary} disabled={busy}>
              {t("mail.create")}
            </button>
            <button
              type="button"
              className={button}
              onClick={() => setAdding(false)}
            >
              {t("common.cancel")}
            </button>
          </div>
          {error ? (
            <p className="basis-full text-sm text-rose-300">{error}</p>
          ) : null}
        </form>
      ) : null}
      {status.domains.length === 0 ? (
        <p className="text-sm text-muted">{t("mail.noDomains")}</p>
      ) : (
        <DataTable
          head={[
            t("mail.domain"),
            t("mail.mailboxes"),
            "DKIM",
            t("mail.catchAll"),
            t("mail.status"),
            "",
          ]}
        >
          {status.domains.map((d) => (
            <tr
              key={d.name}
              className="cursor-pointer"
              onClick={() => onOpen(d.name)}
            >
              <td className="px-3 py-2 font-mono text-sm">{d.name}</td>
              <td className="px-3 py-2">{d.accounts}</td>
              <td className="px-3 py-2">
                {d.dkimPublic ? (
                  <Badge tone="ok">✓</Badge>
                ) : (
                  <Badge tone="idle">–</Badge>
                )}
              </td>
              <td className="px-3 py-2 font-mono text-xs">
                {d.catchAll || "–"}
              </td>
              <td className="px-3 py-2">
                {d.suspended ? (
                  <Badge tone="warn">{t("mail.suspended")}</Badge>
                ) : (
                  <Badge tone="ok">{t("mail.active")}</Badge>
                )}
              </td>
              <td className="px-3 py-2 text-right">
                <button
                  type="button"
                  className="text-xs text-white/80 hover:underline"
                  onClick={() => onOpen(d.name)}
                >
                  {t("mail.open")} →
                </button>
              </td>
            </tr>
          ))}
        </DataTable>
      )}
    </section>
  );
}

function QueueCard() {
  const { t } = useTranslation();
  const query = useGet<QueueData>(`${base}/queue`);
  const [busy, setBusy] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    const id = window.setInterval(query.reload, 15000);
    return () => window.clearInterval(id);
  }, [query.reload]);

  async function act(path: string, id = "") {
    setBusy(id || path);
    setError("");
    try {
      await api(`${base}/queue/${path}`, { method: "POST", body: "{}" });
      query.reload();
    } catch (err) {
      setError(errorText(err, t("mail.failed")));
    } finally {
      setBusy("");
    }
  }

  const messages = query.data?.messages ?? [];
  return (
    <section className={`${card} mb-4 space-y-3`}>
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h2 className="text-sm font-semibold">
          {t("mail.queue.title", { count: query.data?.count ?? 0 })}
        </h2>
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            className={button}
            disabled={busy !== ""}
            onClick={() => query.reload()}
          >
            {t("common.refresh")}
          </button>
          <button
            type="button"
            className={button}
            disabled={busy !== "" || messages.length === 0}
            onClick={() => void act("flush")}
          >
            {t("mail.queue.flush")}
          </button>
        </div>
      </div>
      <p className="text-xs text-white/50">{t("mail.queue.hint")}</p>
      {query.data?.mocked ? (
        <p className="text-xs text-amber-200">{t("mail.queue.mocked")}</p>
      ) : null}
      {error ? <p className="text-sm text-rose-300">{error}</p> : null}
      {query.loading && !query.data ? (
        <p className="text-sm text-muted">{t("common.loading")}</p>
      ) : null}
      {!query.loading && messages.length === 0 ? (
        <p className="text-sm text-muted">{t("mail.queue.empty")}</p>
      ) : null}
      {messages.length > 0 ? (
        <DataTable
          head={[
            t("mail.queue.id"),
            t("mail.queue.sender"),
            t("mail.queue.recipients"),
            t("mail.queue.status"),
            "",
          ]}
        >
          {messages.map((m) => (
            <tr key={m.id}>
              <td className="px-3 py-2 font-mono text-xs">
                <div>{m.id}</div>
                <div className="text-white/40">
                  {m.arrival}
                  {m.size > 0 ? ` · ${size(Math.round(m.size / 1024))}` : ""}
                </div>
              </td>
              <td className="px-3 py-2 font-mono text-xs break-all">
                {m.sender || "–"}
              </td>
              <td className="max-w-[14rem] px-3 py-2 font-mono text-xs break-all">
                {m.recipients.length ? m.recipients.join(", ") : "–"}
                {m.reason ? (
                  <div className="mt-1 text-amber-200/90">{m.reason}</div>
                ) : null}
              </td>
              <td className="px-3 py-2">
                <Badge tone={m.status === "active" ? "ok" : "warn"}>
                  {m.status}
                </Badge>
              </td>
              <td className="px-3 py-2 text-right whitespace-nowrap">
                <button
                  type="button"
                  className="me-2 text-xs text-white/80 hover:underline"
                  disabled={busy !== ""}
                  onClick={() => void act(`${m.id}/retry`, m.id)}
                >
                  {t("mail.queue.retry")}
                </button>
                <button
                  type="button"
                  className={`${danger} !px-2 !py-1 text-xs`}
                  disabled={busy !== ""}
                  onClick={() => {
                    if (
                      window.confirm(t("mail.queue.confirmDelete", { id: m.id }))
                    ) {
                      void act(`${m.id}/delete`, m.id);
                    }
                  }}
                >
                  {t("mail.queue.delete")}
                </button>
              </td>
            </tr>
          ))}
        </DataTable>
      ) : null}
    </section>
  );
}
