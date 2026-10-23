import { useTranslation } from "react-i18next";
import { button, Field, input, randomPassword, Toggle } from "../parts.tsx";
import type { ConfigFormProps } from "./types.ts";

type Group = {
  name: string;
  password?: string;
  canKickBan?: boolean;
  canAccessInventories?: boolean;
  canEditBase?: boolean;
  canExtendBase?: boolean;
  reservedSlots?: number;
  [key: string]: unknown;
};

type ServerConfig = {
  name?: string;
  slotCount?: number;
  queryPort?: number;
  gamePort?: number;
  gameSettingsPreset?: string;
  enableVoiceChat?: boolean;
  enableTextChat?: boolean;
  voiceChatMode?: string;
  userGroups?: Group[];
  [key: string]: unknown;
};

const presets = ["Default", "Relaxed", "Hard", "Survival", "Custom"];
const permissions = ["canKickBan", "canAccessInventories", "canEditBase", "canExtendBase"] as const;

// EnshroudedForm edits enshrouded_server.json: name, slots, preset, ports,
// chat and the user groups with their passwords.
export function EnshroudedForm({ cfg: raw, update }: ConfigFormProps) {
  const { t } = useTranslation();
  const cfg = raw as ServerConfig;
  const groups = cfg.userGroups ?? [];
  const setGroup = (index: number, patch: Partial<Group>) =>
    update({ userGroups: groups.map((group, i) => (i === index ? { ...group, ...patch } : group)) });

  return (
    <div className="mt-4 space-y-4">
      <div className="grid gap-3 md:grid-cols-2">
        <Field label={t("ens.name")}>
          <input className={input} value={cfg.name ?? ""} maxLength={64} onChange={(e) => update({ name: e.target.value })} />
        </Field>
        <Field label={t("ens.slots")}>
          <input className={input} type="number" min={1} max={16} value={cfg.slotCount ?? 16} onChange={(e) => update({ slotCount: Number(e.target.value) })} />
        </Field>
        <Field label={t("ens.preset")}>
          <select className={input} value={cfg.gameSettingsPreset ?? "Default"} onChange={(e) => update({ gameSettingsPreset: e.target.value })}>
            {presets.map((preset) => (
              <option key={preset} value={preset}>
                {t(`ens.preset${preset}`)}
              </option>
            ))}
          </select>
        </Field>
        <Field label={t("ens.queryPort")}>
          <input className={input} type="number" min={1024} max={65535} value={cfg.queryPort ?? 15637} onChange={(e) => update({ queryPort: Number(e.target.value) })} />
        </Field>
        {cfg.gamePort !== undefined ? (
          <Field label={t("ens.gamePort")}>
            <input className={input} type="number" min={1024} max={65535} value={cfg.gamePort} onChange={(e) => update({ gamePort: Number(e.target.value) })} />
          </Field>
        ) : null}
      </div>
      {cfg.gameSettingsPreset === "Custom" ? <p className="text-xs text-white/60">{t("ens.customHint")}</p> : null}

      <div className="flex flex-wrap gap-x-6 gap-y-2 text-sm">
        <Toggle label={t("ens.voiceChat")} checked={cfg.enableVoiceChat ?? false} onChange={(v) => update({ enableVoiceChat: v })} />
        <Toggle label={t("ens.textChat")} checked={cfg.enableTextChat ?? false} onChange={(v) => update({ enableTextChat: v })} />
      </div>

      <div>
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-xs tracking-wide text-white/60 uppercase">{t("ens.groups")}</h3>
          <button
            type="button"
            className="text-xs text-white/70 hover:text-white"
            onClick={() => update({ userGroups: [...groups, { name: `Gruppe${groups.length + 1}`, password: randomPassword(), canEditBase: true, reservedSlots: 0 }] })}
          >
            + {t("ens.addGroup")}
          </button>
        </div>
        <div className="space-y-3">
          {groups.map((group, index) => (
            <div key={index} className="rounded-2xl border border-white/10 p-3">
              <div className="grid gap-3 sm:grid-cols-[1fr_1fr_6rem]">
                <Field label={t("ens.groupName")}>
                  <input className={input} value={group.name} maxLength={32} onChange={(e) => setGroup(index, { name: e.target.value })} />
                </Field>
                <Field label={t("ens.password")}>
                  <div className="flex gap-2">
                    <input className={`${input} font-mono`} value={group.password ?? ""} maxLength={64} onChange={(e) => setGroup(index, { password: e.target.value })} />
                    <button type="button" className={button} title={t("ens.generate")} onClick={() => setGroup(index, { password: randomPassword() })}>
                      ↻
                    </button>
                  </div>
                </Field>
                <Field label={t("ens.reserved")}>
                  <input className={input} type="number" min={0} max={16} value={group.reservedSlots ?? 0} onChange={(e) => setGroup(index, { reservedSlots: Number(e.target.value) })} />
                </Field>
              </div>
              <div className="mt-3 flex flex-wrap items-center gap-x-5 gap-y-2 text-sm">
                {permissions.map((perm) => (
                  <Toggle key={perm} label={t(`ens.${perm}`)} checked={group[perm] ?? false} onChange={(v) => setGroup(index, { [perm]: v })} />
                ))}
                <button type="button" className="ml-auto text-xs text-rose-300 hover:underline" onClick={() => update({ userGroups: groups.filter((_, i) => i !== index) })}>
                  {t("ens.removeGroup")}
                </button>
              </div>
            </div>
          ))}
          {groups.length === 0 ? <p className="text-xs text-white/60">{t("ens.noGroups")}</p> : null}
        </div>
      </div>
    </div>
  );
}
