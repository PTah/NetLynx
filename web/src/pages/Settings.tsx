import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiDelete, apiDeleteJson, apiGet, apiPatch, apiPost, apiUpload } from "../api";
import { EventTypeChecklistDropdown } from "../components/EventTypeChecklistDropdown";
import { DeviceCategoryIcon } from "../components/DeviceCategoryIcon";
import { useAuthRole } from "../hooks/useAuthRole";
import { useDeviceCategories, invalidateDeviceCategoriesCache } from "../hooks/useDeviceCategories";
import {
  suggestCategoryIdFromLabel,
  type DeviceCategoryDef,
} from "../deviceCategories";
import JournalPanel from "./JournalPanel";
import TrapLogsPanel from "./TrapLogsPanel";

type SettingsTab =
  | "inventory"
  | "mac"
  | "access"
  | "notifications"
  | "users"
  | "backup"
  | "journal"
  | "traps"
  | "about";

const SETTINGS_TABS: { id: SettingsTab; label: string }[] = [
  { id: "inventory", label: "Инвентарь" },
  { id: "mac", label: "MAC" },
  { id: "access", label: "Доступ" },
  { id: "notifications", label: "Уведомления" },
  { id: "users", label: "Пользователи" },
  { id: "backup", label: "Резервные копии" },
  { id: "journal", label: "Журнал" },
  { id: "traps", label: "Trap logs" },
  { id: "about", label: "О программе" },
];

function parseSettingsTab(raw: string | null): SettingsTab {
  if (
    raw === "inventory" ||
    raw === "mac" ||
    raw === "access" ||
    raw === "notifications" ||
    raw === "users" ||
    raw === "backup" ||
    raw === "journal" ||
    raw === "traps" ||
    raw === "about"
  ) {
    return raw;
  }
  return "inventory";
}

type UispSettings = {
  enabled: boolean;
  base_url?: string | null;
  has_api_token: boolean;
  import_community: string;
};

type MacInvestigationSettings = {
  track_wifi_clients: boolean;
  wifi_client_ip_prefix: string;
};

type NotifSettings = {
  webhook_url?: string | null;
  webhook_enabled: boolean;
  webhook_event_types?: string | null;
  webhook_severities?: string | null;
  email_enabled: boolean;
  email_from?: string | null;
  email_to?: string | null;
  email_event_types?: string | null;
  email_severities?: string | null;
  smtp_host?: string | null;
  smtp_port: number;
  smtp_username?: string | null;
  smtp_password?: string | null;
  smtp_tls_skip_verify?: boolean;
  has_smtp_password?: boolean;
  has_telegram_bot_token?: boolean;
  telegram_bot_token?: string | null;
  telegram_chat_id?: string | null;
  telegram_enabled: boolean;
  telegram_event_types?: string | null;
  telegram_severities?: string | null;
  notify_max_retries: number;
  notify_retry_backoff_ms: number;
  incident_action_enabled?: boolean;
  incident_action_event_types?: string | null;
  incident_action_dry_run?: boolean;
  incident_action_cooldown_seconds?: number;
};

type BackupSettings = {
  schedule_enabled: boolean;
  schedule_hour: number;
  schedule_minute: number;
  local_enabled: boolean;
  local_dir: string;
  local_retain_days: number;
  email_enabled: boolean;
  email_to?: string | null;
  share_enabled: boolean;
  share_kind: string;
  share_url?: string | null;
  share_username?: string | null;
  share_domain?: string | null;
  share_retain_days: number;
  has_share_password?: boolean;
  switch_cfg_enabled: boolean;
  ssh_user?: string | null;
  ssh_port: number;
  ssh_timeout_seconds: number;
  has_ssh_password?: boolean;
  has_ssh_enable_password?: boolean;
  last_run_at?: string | null;
  last_status?: string | null;
  last_error?: string | null;
  last_log?: string | null;
  live_device_count?: number;
  job_running?: boolean;
  process_started_at?: string;
  app_version?: string;
};

type BackupArchive = { name: string; size: number; mod_time: string };

type AuthUserRow = { id: number; username: string; role: string; is_active: boolean };
type AuditRow = {
  id: number;
  username?: string | null;
  action: string;
  entity_type?: string | null;
  created_at: string;
};

function backupStatusLabel(s: string): string {
  switch (s) {
    case "running":
      return "выполняется";
    case "ok":
      return "готово";
    case "fail":
      return "ошибка";
    case "partial":
      return "частично";
    default:
      return s;
  }
}

function formatBytes(n: number): string {
  if (n < 1024) return `${n} Б`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} КБ`;
  return `${(n / (1024 * 1024)).toFixed(1)} МБ`;
}

function formatArchiveTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function backupJobRunning(x: BackupSettings): boolean {
  if (typeof x.job_running === "boolean") return x.job_running;
  return x.last_status === "running";
}

export default function Settings() {
  const { role, canWrite } = useAuthRole();
  const isAdmin = role === "admin";
  const { categories, reload: reloadCategories } = useDeviceCategories();
  const [catErr, setCatErr] = useState<string | null>(null);
  const [catOk, setCatOk] = useState<string | null>(null);
  const [catBusy, setCatBusy] = useState(false);
  const [newCatLabel, setNewCatLabel] = useState("");
  const [newCatId, setNewCatId] = useState("");
  const [newCatColor, setNewCatColor] = useState("#4a90e2");
  const [newCatBlink, setNewCatBlink] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const tab = parseSettingsTab(searchParams.get("tab"));
  const setTab = (id: SettingsTab) => {
    setSearchParams({ tab: id }, { replace: true });
  };

  const [whURL, setWhURL] = useState("");
  const [whEn, setWhEn] = useState(false);
  const [tgTok, setTgTok] = useState("");
  const [tgChat, setTgChat] = useState("");
  const [tgEn, setTgEn] = useState(false);
  const [whTypes, setWhTypes] = useState("");
  const [whSev, setWhSev] = useState("");
  const [tgTypes, setTgTypes] = useState("");
  const [tgSev, setTgSev] = useState("");
  const [retryMax, setRetryMax] = useState("2");
  const [retryBackoff, setRetryBackoff] = useState("500");
  const [emEn, setEmEn] = useState(false);
  const [emFrom, setEmFrom] = useState("");
  const [emTo, setEmTo] = useState("");
  const [emTypes, setEmTypes] = useState("");
  const [emSev, setEmSev] = useState("");
  const [smtpHost, setSmtpHost] = useState("");
  const [smtpPort, setSmtpPort] = useState("587");
  const [smtpUser, setSmtpUser] = useState("");
  const [smtpPass, setSmtpPass] = useState("");
  const [hasSmtpPass, setHasSmtpPass] = useState(false);
  const [smtpTLSSkip, setSmtpTLSSkip] = useState(false);
  const [hasTgTok, setHasTgTok] = useState(false);
  const [notifErr, setNotifErr] = useState<string | null>(null);
  const [notifOk, setNotifOk] = useState<string | null>(null);
  const [emailTesting, setEmailTesting] = useState(false);
  const [aboutVersion, setAboutVersion] = useState("");
  const [aboutCommit, setAboutCommit] = useState("");
  const [aboutBuiltAt, setAboutBuiltAt] = useState("");

  const [uispEn, setUispEn] = useState(false);
  const [uispUrl, setUispUrl] = useState("");
  const [uispTok, setUispTok] = useState("");
  const [uispHasTok, setUispHasTok] = useState(false);
  const [uispComm, setUispComm] = useState("public");
  const [uispErr, setUispErr] = useState<string | null>(null);
  const [uispOk, setUispOk] = useState<string | null>(null);

  const [macTrackWiFi, setMacTrackWiFi] = useState(false);
  const [macWiFiPrefix, setMacWiFiPrefix] = useState("192.168.120.0/24");
  const [macInvErr, setMacInvErr] = useState<string | null>(null);
  const [macInvOk, setMacInvOk] = useState<string | null>(null);

  const [incidentEn, setIncidentEn] = useState(false);
  const [incidentTypes, setIncidentTypes] = useState("UNKNOWN_MAC_ON_ACCESS_PORT");
  const [incidentDry, setIncidentDry] = useState(false);
  const [incidentCooldown, setIncidentCooldown] = useState("300");

  const [trapListenEn, setTrapListenEn] = useState(false);
  const [trapListenPort, setTrapListenPort] = useState("9162");
  const [linkTrapMode, setLinkTrapMode] = useState("off");
  const [linkTrapEffects, setLinkTrapEffects] = useState("notify");
  const [trapListenBusy, setTrapListenBusy] = useState(false);
  const [trapListenErr, setTrapListenErr] = useState<string | null>(null);
  const [trapListenOk, setTrapListenOk] = useState<string | null>(null);
  const [trapReceiverOn, setTrapReceiverOn] = useState(false);

  const [users, setUsers] = useState<AuthUserRow[]>([]);
  const [newUserName, setNewUserName] = useState("");
  const [newUserPass, setNewUserPass] = useState("");
  const [newUserRole, setNewUserRole] = useState("viewer");
  const [usersErr, setUsersErr] = useState<string | null>(null);
  const [audit, setAudit] = useState<AuditRow[]>([]);
  const [devicesCount, setDevicesCount] = useState<number | null>(null);
  const [devicesClearMsg, setDevicesClearMsg] = useState<string | null>(null);
  const [devicesClearErr, setDevicesClearErr] = useState<string | null>(null);

  const [staleFdbDays, setStaleFdbDays] = useState("60");
  const [staleFdbPreview, setStaleFdbPreview] = useState<{
    count: number;
    devices: { id: number; name: string; host: string; entry_count: number; last_fdb_poll_at?: string }[];
  } | null>(null);
  const [staleFdbMsg, setStaleFdbMsg] = useState<string | null>(null);
  const [staleFdbErr, setStaleFdbErr] = useState<string | null>(null);
  const [staleFdbBusy, setStaleFdbBusy] = useState(false);

  const [offlineDays, setOfflineDays] = useState("60");
  const [offlinePreview, setOfflinePreview] = useState<{
    count: number;
    devices: { id: number; name: string; host: string; offline_since: string }[];
  } | null>(null);
  const [offlineMsg, setOfflineMsg] = useState<string | null>(null);
  const [offlineErr, setOfflineErr] = useState<string | null>(null);
  const [offlineBusy, setOfflineBusy] = useState(false);

  const [bkScheduleEn, setBkScheduleEn] = useState(false);
  const [bkTime, setBkTime] = useState("02:00");
  const [bkLocalEn, setBkLocalEn] = useState(true);
  const [bkLocalDir, setBkLocalDir] = useState("/var/backups/netlynx");
  const [bkLocalDays, setBkLocalDays] = useState("3");
  const [bkEmailEn, setBkEmailEn] = useState(false);
  const [bkEmailTo, setBkEmailTo] = useState("");
  const [bkShareEn, setBkShareEn] = useState(false);
  const [bkShareKind, setBkShareKind] = useState("smb");
  const [bkShareURL, setBkShareURL] = useState("");
  const [bkShareUser, setBkShareUser] = useState("");
  const [bkSharePass, setBkSharePass] = useState("");
  const [bkShareDomain, setBkShareDomain] = useState("");
  const [bkHasSharePass, setBkHasSharePass] = useState(false);
  const [bkShareDays, setBkShareDays] = useState("3");
  const [bkSwitchEn, setBkSwitchEn] = useState(false);
  const [bkSshUser, setBkSshUser] = useState("");
  const [bkSshPass, setBkSshPass] = useState("");
  const [bkSshPort, setBkSshPort] = useState("22");
  const [bkSshEnable, setBkSshEnable] = useState("");
  const [bkHasSshPass, setBkHasSshPass] = useState(false);
  const [bkHasSshEnable, setBkHasSshEnable] = useState(false);
  const [bkSshTimeout, setBkSshTimeout] = useState("30");
  const [bkLastAt, setBkLastAt] = useState<string | null>(null);
  const [bkLastStatus, setBkLastStatus] = useState<string | null>(null);
  const [bkLastError, setBkLastError] = useState<string | null>(null);
  const [bkLog, setBkLog] = useState("");
  const [bkErr, setBkErr] = useState<string | null>(null);
  const [bkOk, setBkOk] = useState<string | null>(null);
  const [bkRunning, setBkRunning] = useState(false);
  const [bkArchives, setBkArchives] = useState<BackupArchive[]>([]);
  const [bkArchiveSel, setBkArchiveSel] = useState("");
  const [bkZipFile, setBkZipFile] = useState<File | null>(null);
  const [bkLiveDevices, setBkLiveDevices] = useState<number | null>(null);
  const bkProcRef = useRef<string | null>(null);
  const loadUsers = useCallback(() => {
    setUsersErr(null);
    return apiGet<AuthUserRow[]>("/api/v1/users")
      .then((list) => setUsers(Array.isArray(list) ? list : []))
      .catch((e: Error) => setUsersErr(e.message));
  }, []);

  const applyBackup = (x: BackupSettings) => {
    setBkScheduleEn(Boolean(x.schedule_enabled));
    const hh = String(x.schedule_hour ?? 2).padStart(2, "0");
    const mm = String(x.schedule_minute ?? 0).padStart(2, "0");
    setBkTime(`${hh}:${mm}`);
    setBkLocalEn(x.local_enabled !== false);
    setBkLocalDir(x.local_dir?.trim() ? x.local_dir : "/var/backups/netlynx");
    setBkLocalDays(String(x.local_retain_days > 0 ? x.local_retain_days : 3));
    setBkEmailEn(Boolean(x.email_enabled));
    setBkEmailTo(x.email_to ?? "");
    setBkShareEn(Boolean(x.share_enabled));
    setBkShareKind(x.share_kind === "nfs" ? "nfs" : "smb");
    setBkShareURL(x.share_url ?? "");
    setBkShareUser(x.share_username ?? "");
    setBkSharePass("");
    setBkShareDomain(x.share_domain ?? "");
    setBkHasSharePass(Boolean(x.has_share_password));
    setBkShareDays(String(x.share_retain_days > 0 ? x.share_retain_days : 3));
    setBkSwitchEn(Boolean(x.switch_cfg_enabled));
    setBkSshUser(x.ssh_user ?? "");
    setBkSshPass("");
    setBkSshEnable("");
    setBkSshPort(String(x.ssh_port > 0 ? x.ssh_port : 22));
    setBkHasSshPass(Boolean(x.has_ssh_password));
    setBkHasSshEnable(Boolean(x.has_ssh_enable_password));
    setBkSshTimeout(String(x.ssh_timeout_seconds >= 5 ? x.ssh_timeout_seconds : 30));
    setBkLastAt(x.last_run_at ?? null);
    setBkLastStatus(x.last_status ?? null);
    setBkLastError(x.last_error ?? null);
    setBkLog((x.last_log ?? "").trim());
    setBkRunning(backupJobRunning(x));
    setBkLiveDevices(typeof x.live_device_count === "number" ? x.live_device_count : null);
    if ((x.process_started_at ?? "").trim()) {
      bkProcRef.current = x.process_started_at!.trim();
    }
  };

  const loadBackup = useCallback(() => {
    return apiGet<BackupSettings>("/api/v1/settings/backup")
      .then((x) => {
        applyBackup(x);
        setBkErr(null);
      })
      .catch((e: Error) => setBkErr(e.message));
  }, []);

  const loadArchives = useCallback(() => {
    return apiGet<BackupArchive[]>("/api/v1/backup/archives")
      .then((list) => {
        const arr = Array.isArray(list) ? list.slice() : [];
        arr.sort((a, b) => (a.mod_time < b.mod_time ? 1 : -1));
        setBkArchives(arr);
        setBkArchiveSel((cur) => {
          if (cur && arr.some((x) => x.name === cur)) return cur;
          return arr[0]?.name ?? "";
        });
      })
      .catch(() => setBkArchives([]));
  }, []);

  useEffect(() => {
    apiGet<{ id: number }[] | null>("/api/v1/devices")
      .then((rows) => setDevicesCount(Array.isArray(rows) ? rows.length : 0))
      .catch(() => setDevicesCount(null));
  }, [devicesClearMsg, offlineMsg]);

  const loadStaleFdbPreview = () => {
    const n = Number(staleFdbDays);
    if (!Number.isFinite(n) || n < 1) {
      setStaleFdbErr("Укажите число дней ≥ 1");
      return;
    }
    setStaleFdbBusy(true);
    setStaleFdbErr(null);
    setStaleFdbMsg(null);
    apiGet<{
      count: number;
      devices: { id: number; name: string; host: string; entry_count: number; last_fdb_poll_at?: string }[];
    }>(`/api/v1/settings/inventory/stale-fdb?older_than_days=${Math.floor(n)}`)
      .then((r) => setStaleFdbPreview({ count: r.count, devices: r.devices ?? [] }))
      .catch((e: Error) => setStaleFdbErr(e.message))
      .finally(() => setStaleFdbBusy(false));
  };

  const clearStaleFdb = () => {
    const n = Number(staleFdbDays);
    if (!Number.isFinite(n) || n < 1) {
      setStaleFdbErr("Укажите число дней ≥ 1");
      return;
    }
    const previewN = staleFdbPreview?.count ?? "?";
    if (
      !window.confirm(
        `Очистить live FDB у узлов без успешного опроса ≥ ${Math.floor(n)} дн. (сейчас в превью: ${previewN})? История снимков fdb_snapshots не трогается.`,
      )
    ) {
      return;
    }
    setStaleFdbBusy(true);
    setStaleFdbErr(null);
    setStaleFdbMsg(null);
    apiPost<{ devices_affected: number; entries_deleted: number }>(
      "/api/v1/settings/inventory/stale-fdb/clear",
      { older_than_days: Math.floor(n) },
      { headers: { "X-Confirm": "CLEAR-STALE-FDB" } },
    )
      .then((r) => {
        setStaleFdbMsg(`Очищено: узлов ${r.devices_affected}, записей FDB ${r.entries_deleted}.`);
        setStaleFdbPreview(null);
      })
      .catch((e: Error) => setStaleFdbErr(e.message))
      .finally(() => setStaleFdbBusy(false));
  };

  const loadOfflinePreview = () => {
    const n = Number(offlineDays);
    if (!Number.isFinite(n) || n < 1) {
      setOfflineErr("Укажите число дней ≥ 1");
      return;
    }
    setOfflineBusy(true);
    setOfflineErr(null);
    setOfflineMsg(null);
    apiGet<{
      count: number;
      devices: { id: number; name: string; host: string; offline_since: string }[];
    }>(`/api/v1/settings/inventory/offline-devices?older_than_days=${Math.floor(n)}`)
      .then((r) => setOfflinePreview({ count: r.count, devices: r.devices ?? [] }))
      .catch((e: Error) => setOfflineErr(e.message))
      .finally(() => setOfflineBusy(false));
  };

  const deleteOfflineDevices = () => {
    const n = Number(offlineDays);
    if (!Number.isFinite(n) || n < 1) {
      setOfflineErr("Укажите число дней ≥ 1");
      return;
    }
    const previewN = offlinePreview?.count ?? "?";
    if (
      !window.confirm(
        `Удалить из инвентаря узлы, оффлайн ≥ ${Math.floor(n)} дн. (сейчас в превью: ${previewN})? Действие необратимо.`,
      )
    ) {
      return;
    }
    setOfflineBusy(true);
    setOfflineErr(null);
    setOfflineMsg(null);
    apiPost<{ deleted: number }>(
      "/api/v1/settings/inventory/offline-devices/delete",
      { older_than_days: Math.floor(n) },
      { headers: { "X-Confirm": "DELETE-OFFLINE-DEVICES" } },
    )
      .then((r) => {
        setOfflineMsg(`Удалено узлов: ${r.deleted}.`);
        setOfflinePreview(null);
      })
      .catch((e: Error) => setOfflineErr(e.message))
      .finally(() => setOfflineBusy(false));
  };

  const applyNotifSettings = (x: NotifSettings) => {
    setWhURL(x.webhook_url ?? "");
    setWhEn(x.webhook_enabled);
    setTgTok("");
    setHasTgTok(Boolean(x.has_telegram_bot_token));
    setTgChat(x.telegram_chat_id ?? "");
    setTgEn(x.telegram_enabled);
    setWhTypes(x.webhook_event_types ?? "");
    setWhSev(x.webhook_severities ?? "");
    setEmEn(Boolean(x.email_enabled));
    setEmFrom(x.email_from ?? "");
    setEmTo(x.email_to ?? "");
    setEmTypes(x.email_event_types ?? "");
    setEmSev(x.email_severities ?? "");
    setSmtpHost(x.smtp_host ?? "");
    setSmtpPort(String(x.smtp_port ?? 587));
    setSmtpUser(x.smtp_username ?? "");
    setSmtpPass("");
    setHasSmtpPass(Boolean(x.has_smtp_password));
    setSmtpTLSSkip(Boolean(x.smtp_tls_skip_verify));
    setTgTypes(x.telegram_event_types ?? "");
    setTgSev(x.telegram_severities ?? "");
    setRetryMax(String(x.notify_max_retries ?? 2));
    setRetryBackoff(String(x.notify_retry_backoff_ms ?? 500));
    setIncidentEn(Boolean(x.incident_action_enabled));
    setIncidentTypes(x.incident_action_event_types ?? "UNKNOWN_MAC_ON_ACCESS_PORT");
    setIncidentDry(Boolean(x.incident_action_dry_run));
    setIncidentCooldown(String(x.incident_action_cooldown_seconds ?? 300));
  };

  useEffect(() => {
    apiGet<NotifSettings>("/api/v1/settings/notifications")
      .then(applyNotifSettings)
      .catch((e: Error) => setNotifErr(e.message));
  }, []);

  type TrapListenSettings = {
    listen_enabled: boolean;
    listen_port: number;
    receiver_enabled: boolean;
    link_trap_events_mode?: string;
    link_trap_effects?: string;
  };

  const loadTrapListen = useCallback(() => {
    return apiGet<TrapListenSettings>("/api/v1/settings/snmp-traps")
      .then((x) => {
        setTrapListenEn(Boolean(x.listen_enabled));
        setTrapListenPort(String(x.listen_port > 0 ? x.listen_port : 9162));
        setTrapReceiverOn(Boolean(x.receiver_enabled));
        setLinkTrapMode(x.link_trap_events_mode || "off");
        setLinkTrapEffects(x.link_trap_effects || "notify");
        setTrapListenErr(null);
      })
      .catch((e: Error) => setTrapListenErr(e.message));
  }, []);

  useEffect(() => {
    void loadTrapListen();
  }, [loadTrapListen]);

  useEffect(() => {
    if (tab === "traps" && !trapListenEn) {
      setTab("notifications");
    }
  }, [tab, trapListenEn]);

  async function saveTrapListen() {
    if (!canWrite) return;
    setTrapListenBusy(true);
    setTrapListenErr(null);
    setTrapListenOk(null);
    const port = Number(trapListenPort);
    if (!Number.isFinite(port) || port < 1 || port > 65535) {
      setTrapListenErr("Порт должен быть от 1 до 65535");
      setTrapListenBusy(false);
      return;
    }
    try {
      const x = (await apiPatch("/api/v1/settings/snmp-traps", {
        listen_enabled: trapListenEn,
        listen_port: Math.trunc(port),
        link_trap_events_mode: linkTrapMode,
        link_trap_effects: linkTrapEffects,
      })) as TrapListenSettings;
      setTrapListenEn(Boolean(x.listen_enabled));
      setTrapListenPort(String(x.listen_port > 0 ? x.listen_port : 9162));
      setTrapReceiverOn(Boolean(x.receiver_enabled));
      setLinkTrapMode(x.link_trap_events_mode || "off");
      setLinkTrapEffects(x.link_trap_effects || "notify");
      setTrapListenOk("Настройки traps сохранены");
    } catch (err) {
      setTrapListenErr(err instanceof Error ? err.message : String(err));
    } finally {
      setTrapListenBusy(false);
    }
  }

  useEffect(() => {
    if (!isAdmin) {
      setUsers([]);
      setAudit([]);
      return;
    }
    void loadUsers();
    apiGet<AuditRow[]>("/api/v1/audit?limit=50")
      .then((list) => setAudit(Array.isArray(list) ? list : []))
      .catch(() => setAudit([]));
    void loadBackup();
    void loadArchives();
  }, [isAdmin, loadUsers, loadBackup, loadArchives]);

  useEffect(() => {
    if (!isAdmin) return;
    if (bkLastStatus !== "running" && !bkRunning) return;
    const tick = () => {
      apiGet<BackupSettings>("/api/v1/settings/backup")
        .then((x) => {
          const proc = (x.process_started_at ?? "").trim();
          if (bkProcRef.current && proc && bkProcRef.current !== proc) {
            applyBackup(x);
            setBkOk("Служба перезапущена — журнал обновлён.");
          } else {
            setBkLastAt(x.last_run_at ?? null);
            setBkLastStatus(x.last_status ?? null);
            setBkLastError(x.last_error ?? null);
            setBkLog((x.last_log ?? "").trim());
            setBkRunning(backupJobRunning(x));
            setBkLiveDevices(typeof x.live_device_count === "number" ? x.live_device_count : null);
          }
          if (proc) bkProcRef.current = proc;
        })
        .catch(() => {
          fetch("/health", { credentials: "include" })
            .then((r) => (r.ok ? r.json() : null))
            .then((h: { started_at?: string } | null) => {
              const proc = (h?.started_at ?? "").trim();
              if (bkProcRef.current && proc && bkProcRef.current !== proc) {
                void loadBackup().then(() => setBkOk("Служба перезапущена — журнал обновлён."));
                bkProcRef.current = proc;
              }
            })
            .catch(() => {
              /* рестарт: следующий тик */
            });
        });
    };
    tick();
    const id = window.setInterval(tick, 1000);
    return () => window.clearInterval(id);
  }, [isAdmin, bkLastStatus, bkRunning, loadBackup]);

  useEffect(() => {
    if (!isAdmin || bkRunning) return;
    void loadArchives();
  }, [isAdmin, bkRunning, bkLastStatus, loadArchives]);

  useEffect(() => {
    if (tab !== "about") return;
    let cancelled = false;
    fetch("/health", { credentials: "include" })
      .then((r) => (r.ok ? r.json() : Promise.reject()))
      .then((h: { version?: string; commit?: string; built_at?: string }) => {
        if (cancelled) return;
        setAboutVersion((h.version ?? "").trim());
        setAboutCommit((h.commit ?? "").trim());
        setAboutBuiltAt((h.built_at ?? "").trim());
      })
      .catch(() => {
        if (!cancelled) {
          setAboutVersion("");
          setAboutCommit("");
          setAboutBuiltAt("");
        }
      });
    return () => {
      cancelled = true;
    };
  }, [tab]);

  useEffect(() => {
    apiGet<UispSettings>("/api/v1/settings/uisp")
      .then((x) => {
        setUispEn(Boolean(x.enabled));
        setUispUrl(x.base_url ?? "");
        setUispHasTok(Boolean(x.has_api_token));
        setUispTok("");
        setUispComm(x.import_community?.trim() ? x.import_community : "public");
      })
      .catch((e: Error) => setUispErr(e.message));
    apiGet<MacInvestigationSettings>("/api/v1/settings/mac-investigation")
      .then((x) => {
        setMacTrackWiFi(Boolean(x.track_wifi_clients));
        setMacWiFiPrefix(x.wifi_client_ip_prefix?.trim() || "192.168.120.0/24");
      })
      .catch((e: Error) => setMacInvErr(e.message));
  }, []);

  const saveUisp = (e: FormEvent) => {
    e.preventDefault();
    setUispErr(null);
    setUispOk(null);
    if (uispEn && !uispHasTok && !uispTok.trim()) {
      setUispErr("Укажите API token или сначала сохраните токен с выключенной галочкой.");
      return;
    }
    const body: Record<string, unknown> = {
      enabled: uispEn,
      base_url: uispUrl.trim(),
      import_community: uispComm.trim() || "public",
    };
    if (uispTok.trim()) {
      body.api_token = uispTok.trim();
    }
    apiPatch<UispSettings>("/api/v1/settings/uisp", body)
      .then((x) => {
        setUispEn(Boolean(x.enabled));
        setUispUrl(x.base_url ?? "");
        setUispHasTok(Boolean(x.has_api_token));
        setUispTok("");
        setUispComm(x.import_community?.trim() ? x.import_community : "public");
        setUispOk("Настройки UISP сохранены.");
      })
      .catch((e: Error) => setUispErr(e.message));
  };

  const saveMacInvestigation = (e: FormEvent) => {
    e.preventDefault();
    if (!canWrite) return;
    setMacInvErr(null);
    setMacInvOk(null);
    const prefix = macWiFiPrefix.trim();
    if (!prefix) {
      setMacInvErr("Укажите подсеть WiFi-клиентов (CIDR).");
      return;
    }
    apiPatch<MacInvestigationSettings>("/api/v1/settings/mac-investigation", {
      track_wifi_clients: macTrackWiFi,
      wifi_client_ip_prefix: prefix,
    })
      .then((x) => {
        setMacTrackWiFi(Boolean(x.track_wifi_clients));
        setMacWiFiPrefix(x.wifi_client_ip_prefix?.trim() || "192.168.120.0/24");
        setMacInvOk("Сохранено");
      })
      .catch((e: Error) => setMacInvErr(e.message));
  };

  const saveNotifications = (e: FormEvent) => {
    e.preventDefault();
    setNotifErr(null);
    setNotifOk(null);
    const maxRetriesNum = Number(retryMax);
    const backoffNum = Number(retryBackoff);
    const smtpPortNum = Number(smtpPort);
    if (!Number.isFinite(maxRetriesNum) || !Number.isFinite(backoffNum) || !Number.isFinite(smtpPortNum)) {
      setNotifErr("Поля retry/backoff должны быть числами.");
      return;
    }
    apiPatch<NotifSettings>("/api/v1/settings/notifications", {
      webhook_url: whURL.trim(),
      webhook_enabled: whEn,
      webhook_event_types: whTypes.trim(),
      webhook_severities: whSev.trim(),
      email_enabled: emEn,
      email_from: emFrom.trim(),
      email_to: emTo.trim(),
      email_event_types: emTypes.trim(),
      email_severities: emSev.trim(),
      smtp_host: smtpHost.trim(),
      smtp_port: smtpPortNum,
      smtp_username: smtpUser.trim(),
      smtp_password: smtpPass.trim(),
      smtp_tls_skip_verify: smtpTLSSkip,
      telegram_bot_token: tgTok.trim(),
      telegram_chat_id: tgChat.trim(),
      telegram_enabled: tgEn,
      telegram_event_types: tgTypes.trim(),
      telegram_severities: tgSev.trim(),
      notify_max_retries: maxRetriesNum,
      notify_retry_backoff_ms: backoffNum,
      incident_action_enabled: incidentEn,
      incident_action_event_types: incidentTypes.trim(),
      incident_action_dry_run: incidentDry,
      incident_action_cooldown_seconds: Number(incidentCooldown) || 0,
    })
      .then((x) => {
        applyNotifSettings(x);
        setNotifOk("Настройки уведомлений сохранены на сервере.");
      })
      .catch((e: Error) => setNotifErr(e.message));
  };

  const saveBackup = (e: FormEvent) => {
    e.preventDefault();
    setBkErr(null);
    setBkOk(null);
    const localDays = Number(bkLocalDays);
    const shareDays = Number(bkShareDays);
    const sshPort = Number(bkSshPort);
    const sshTimeout = Number(bkSshTimeout);
    const [hhRaw, mmRaw] = bkTime.split(":");
    const hour = Number(hhRaw);
    const minute = Number(mmRaw);
    if (!Number.isInteger(localDays) || localDays < 1 || localDays > 365) {
      setBkErr("Срок хранения локально: целое число 1–365 дней.");
      return;
    }
    if (!Number.isInteger(shareDays) || shareDays < 1 || shareDays > 365) {
      setBkErr("Срок хранения на шаре: целое число 1–365 дней.");
      return;
    }
    if (!Number.isInteger(hour) || hour < 0 || hour > 23 || !Number.isInteger(minute) || minute < 0 || minute > 59) {
      setBkErr("Укажите время расписания в формате ЧЧ:ММ.");
      return;
    }
    if (!Number.isInteger(sshPort) || sshPort < 1 || sshPort > 65535) {
      setBkErr("SSH-порт: 1–65535.");
      return;
    }
    if (!Number.isInteger(sshTimeout) || sshTimeout < 5 || sshTimeout > 300) {
      setBkErr("SSH timeout: 5–300 секунд.");
      return;
    }
    const body: Record<string, unknown> = {
      schedule_enabled: bkScheduleEn,
      schedule_hour: hour,
      schedule_minute: minute,
      local_enabled: bkLocalEn,
      local_dir: bkLocalDir.trim() || "/var/backups/netlynx",
      local_retain_days: localDays,
      email_enabled: bkEmailEn,
      email_to: bkEmailTo.trim(),
      share_enabled: bkShareEn,
      share_kind: bkShareKind,
      share_url: bkShareURL.trim(),
      share_username: bkShareUser.trim(),
      share_domain: bkShareDomain.trim(),
      share_retain_days: shareDays,
      switch_cfg_enabled: bkSwitchEn,
      ssh_user: bkSshUser.trim(),
      ssh_port: sshPort,
      ssh_timeout_seconds: sshTimeout,
    };
    if (bkSharePass.trim()) body.share_password = bkSharePass;
    if (bkSshPass.trim()) body.ssh_password = bkSshPass;
    if (bkSshEnable.trim()) body.ssh_enable_password = bkSshEnable;
    apiPatch<BackupSettings>("/api/v1/settings/backup", body)
      .then((x) => {
        applyBackup(x);
        setBkOk("Настройки резервных копий сохранены.");
      })
      .catch((e: Error) => setBkErr(e.message));
  };

  const runBackupNow = () => {
    setBkErr(null);
    setBkOk(null);
    setBkRunning(true);
    setBkLastStatus("running");
    setBkLog("запуск…");
    apiPost<{ ok: boolean }>("/api/v1/backup/run", {})
      .then(() => {
        setBkOk(null);
      })
      .catch((e: Error) => {
        setBkRunning(false);
        setBkErr(e.message);
      });
  };

  const startRestoreJob = (doImport: boolean) => {
    setBkErr(null);
    setBkOk(null);
    const live = bkLiveDevices ?? 0;
    if (doImport && live > 0) {
      setBkErr(
        `Импорт запрещён: в рабочей БД ${live} узлов. На работающем проде используйте только «Проверить дамп» — временная БД, invetor не меняется.`,
      );
      return;
    }
    if (doImport) {
      const typed = window.prompt(
        "Дамп заменит содержимое рабочей базы. Это только для чистой системы (0 узлов). Введите IMPORT.",
      );
      if ((typed ?? "").trim().toUpperCase() !== "IMPORT") {
        return;
      }
    }
    if (!bkZipFile && !bkArchiveSel) {
      setBkErr("Выберите ZIP из каталога бэкапов или загрузите файл.");
      return;
    }
    setBkRunning(true);
    setBkLastStatus("running");
    setBkLog(doImport ? "импорт…" : "проверка дампа…");
    const path = doImport ? "/api/v1/backup/import" : "/api/v1/backup/verify";
    const req = bkZipFile
      ? (() => {
          const fd = new FormData();
          fd.append("file", bkZipFile);
          if (doImport) fd.append("confirm", "IMPORT");
          return apiUpload<{ ok: boolean }>(path, fd);
        })()
      : apiPost<{ ok: boolean }>(path, {
          filename: bkArchiveSel,
          confirm: doImport ? "IMPORT" : "",
        });
    void req
      .then(() => {
        setBkOk(
          doImport
            ? "Импорт запущен. После «готово» перезапустите службу: sudo systemctl restart NetLynx.service"
            : "Проверка запущена. Рабочая база не затрагивается — смотрите журнал.",
        );
      })
      .catch((e: Error) => {
        setBkRunning(false);
        setBkErr(e.message);
      });
  };

  return (
    <div>
      <h1 style={{ marginTop: 0 }}>Настройки</h1>
      <p className="settings-lead">
        Разделы сгруппированы по задачам. Закладка запоминается в адресе страницы (
        <code>?tab=</code>
        ).
      </p>
      <div className="settings-tabs" role="tablist" aria-label="Разделы настроек">
        {SETTINGS_TABS.map((t) => {
          const trapsDisabled = t.id === "traps" && !trapListenEn;
          return (
            <button
              key={t.id}
              type="button"
              role="tab"
              aria-selected={tab === t.id}
              aria-disabled={trapsDisabled}
              disabled={trapsDisabled}
              title={
                trapsDisabled
                  ? "Сначала включите «Принимать traps» во вкладке Уведомления"
                  : undefined
              }
              className={[
                "settings-tab",
                tab === t.id ? "settings-tab--active" : "",
                trapsDisabled ? "settings-tab--disabled" : "",
              ]
                .filter(Boolean)
                .join(" ")}
              onClick={() => {
                if (!trapsDisabled) setTab(t.id);
              }}
            >
              {t.label}
            </button>
          );
        })}
      </div>

      {tab === "inventory" && (
        <div role="tabpanel">
          <p className="settings-lead">
            Как наполнить систему узлами: импорт из UISP, ручное добавление, соседи по LLDP/CDP. Сканирование подсети
            по SNMP пока не реализовано.
          </p>

          <section className="settings-card">
            <h2>Типы узлов</h2>
            <p>
              Список типов в фильтрах и формах. Цвет и мигание точки на топологии — у любого типа; свои типы —
              добавить ниже (роль operator/admin). Встроенные типы удалять и переименовывать нельзя.
            </p>
            {catErr && <p style={{ color: "#f88" }}>{catErr}</p>}
            {catOk && <p style={{ color: "#8d8" }}>{catOk}</p>}
            <table className="settings-cat-table" style={{ width: "100%", maxWidth: 720, borderCollapse: "collapse" }}>
              <thead>
                <tr style={{ textAlign: "left", color: "#9aa3b5", fontSize: "0.85rem" }}>
                  <th style={{ padding: "0.35rem 0.5rem" }}>Иконка</th>
                  <th style={{ padding: "0.35rem 0.5rem" }}>Цвет</th>
                  <th style={{ padding: "0.35rem 0.5rem" }}>Мигание</th>
                  <th style={{ padding: "0.35rem 0.5rem" }}>Название</th>
                  <th style={{ padding: "0.35rem 0.5rem" }}>id</th>
                  <th style={{ padding: "0.35rem 0.5rem" }} />
                </tr>
              </thead>
              <tbody>
                {categories.map((c) => (
                  <CategoryRow
                    key={c.id}
                    cat={c}
                    canWrite={canWrite}
                    busy={catBusy}
                    onSaved={() => {
                      setCatOk("Сохранено");
                      setCatErr(null);
                      invalidateDeviceCategoriesCache();
                      reloadCategories();
                    }}
                    onError={(m) => {
                      setCatErr(m);
                      setCatOk(null);
                    }}
                    onDeleted={() => {
                      setCatOk(`Тип «${c.label}» удалён (узлы → Иные)`);
                      setCatErr(null);
                      invalidateDeviceCategoriesCache();
                      reloadCategories();
                    }}
                    setBusy={setCatBusy}
                  />
                ))}
              </tbody>
            </table>
            {canWrite && (
              <form
                style={{
                  display: "flex",
                  flexWrap: "wrap",
                  gap: "0.5rem 0.75rem",
                  alignItems: "flex-end",
                  marginTop: "1rem",
                  maxWidth: 640,
                }}
                onSubmit={(e) => {
                  e.preventDefault();
                  setCatBusy(true);
                  setCatErr(null);
                  setCatOk(null);
                  const id = (newCatId.trim() || suggestCategoryIdFromLabel(newCatLabel)).toLowerCase();
                  apiPost<DeviceCategoryDef>("/api/v1/settings/device-categories", {
                    id,
                    label: newCatLabel.trim(),
                    color: newCatColor,
                    blink: newCatBlink,
                  })
                    .then(() => {
                      setNewCatLabel("");
                      setNewCatId("");
                      setNewCatColor("#4a90e2");
                      setNewCatBlink(false);
                      setCatOk("Тип добавлен");
                      invalidateDeviceCategoriesCache();
                      reloadCategories();
                    })
                    .catch((err: Error) => setCatErr(err.message))
                    .finally(() => setCatBusy(false));
                }}
              >
                <label>
                  Название
                  <br />
                  <input
                    value={newCatLabel}
                    onChange={(e) => {
                      setNewCatLabel(e.target.value);
                      if (!newCatId.trim()) setNewCatId(suggestCategoryIdFromLabel(e.target.value));
                    }}
                    placeholder="ИБП"
                    disabled={catBusy}
                    required
                    style={{ width: 160 }}
                  />
                </label>
                <label>
                  id (латиница)
                  <br />
                  <input
                    value={newCatId}
                    onChange={(e) => setNewCatId(e.target.value.toLowerCase().replace(/[^a-z0-9_]/g, ""))}
                    placeholder="ups"
                    disabled={catBusy}
                    pattern="[a-z][a-z0-9_]{0,31}"
                    title="a-z, затем a-z0-9_, до 32 символов"
                    style={{ width: 120 }}
                  />
                </label>
                <label>
                  Цвет
                  <br />
                  <input
                    type="color"
                    value={newCatColor}
                    onChange={(e) => setNewCatColor(e.target.value)}
                    disabled={catBusy}
                    style={{ width: 48, height: 32, padding: 0, border: "1px solid #40506a", background: "transparent" }}
                  />
                </label>
                <label style={{ display: "inline-flex", alignItems: "center", gap: 6, paddingBottom: 4 }}>
                  <input
                    type="checkbox"
                    checked={newCatBlink}
                    onChange={(e) => setNewCatBlink(e.target.checked)}
                    disabled={catBusy}
                  />
                  Мигание
                </label>
                <button type="submit" disabled={catBusy || !newCatLabel.trim()}>
                  Добавить тип
                </button>
              </form>
            )}
          </section>

          <section className="settings-card">
            <h2>Импорт из Ubiquiti UISP (разово)</h2>
            <p>
              Опционально: если в инфраструктуре ещё есть <strong>UISP</strong>, можно один раз подтянуть список
              коммутаторов (<code>role=switch</code>) на странице «Узлы». Укажите URL и API token. Для импорта
              используется SNMP <strong>v2c</strong> и community ниже. После импорта NetLynx ведёт учёт сам — по SNMP
              и ping; статус из UISP на дашборд не влияет.
            </p>
            {uispErr && <p style={{ color: "#f88" }}>{uispErr}</p>}
            {uispOk && <p style={{ color: "#8d8" }}>{uispOk}</p>}
            <form onSubmit={saveUisp} style={{ display: "flex", flexDirection: "column", gap: "0.75rem", maxWidth: 520 }}>
              <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                <input type="checkbox" checked={uispEn} onChange={(e) => setUispEn(e.target.checked)} />
                В сети используется UISP
              </label>
              <label>
                Базовый URL UISP (без слэша в конце)
                <br />
                <input
                  style={{ width: "100%" }}
                  value={uispUrl}
                  onChange={(e) => setUispUrl(e.target.value)}
                  placeholder="https://unms.example.com"
                  disabled={!uispEn}
                />
              </label>
              <label>
                API token (x-auth-token)
                <br />
                <input
                  style={{ width: "100%" }}
                  type="password"
                  value={uispTok}
                  onChange={(e) => setUispTok(e.target.value)}
                  placeholder={uispHasTok ? "оставьте пустым, чтобы не менять" : "вставьте токен"}
                  disabled={!uispEn}
                  autoComplete="off"
                />
              </label>
              <label>
                SNMP community для импортированных узлов (v2c)
                <br />
                <input
                  style={{ width: "100%" }}
                  value={uispComm}
                  onChange={(e) => setUispComm(e.target.value)}
                  disabled={!uispEn}
                />
              </label>
              <button type="submit">Сохранить настройки UISP</button>
            </form>
          </section>

          <section className="settings-card">
            <h2>Ручное добавление коммутаторов</h2>
            <p>
              Если известны management IP и SNMP, узел добавляется формой на странице «Узлы»: имя, адрес, расположение,
              SNMP v1/v2c/v3. Для нового офиса без UISP достаточно завести 1–2 опорных свитча (core/distribution) — дальше
              соседей подхватит автообнаружение.
            </p>
            <p>
              <Link to="/devices#add">Открыть форму на странице «Узлы»</Link>
            </p>
          </section>

          <section className="settings-card">
            <h2>Автообнаружение (LLDP / CDP)</h2>
            <p>
              После опроса известных свитчей poller видит соседей по LLDP и CDP. Кандидаты появляются на странице
              «Обнаружено» и как виртуальные узлы на топологии — их можно проверить по SNMP и добавить в инвентарь
              (promote). Это основной способ наполнить новую систему без UISP.
            </p>
            <p>
              <Link to="/discovered">Открыть «Обнаружено»</Link>
              {" · "}
              <Link to="/topology">Открыть топологию</Link>
            </p>
          </section>

          <section className="settings-card">
            <h2>
              Сканирование сети
              <span className="settings-soon">скоро</span>
            </h2>
            <p>
              Массовый обход подсети (SNMP ping/scan списка IP) в NetLynx пока не реализован. Пока используйте ручное
              добавление опорных свитчей и автообнаружение по LLDP/CDP либо импорт из UISP, если он есть.
            </p>
          </section>

          <section className="settings-card">
            <h2>Очистка старых FDB</h2>
            <p>
              Удаляет <strong>live</strong> таблицу MAC (<code>device_fdb_entries</code>) у узлов, у которых последний
              успешный FDB-опрос старше N дней (или опроса не было). Так из «Где сейчас (FDB)» пропадают давно
              отключённые свитчи. Ежедневные снимки истории (<code>fdb_snapshots</code>) не удаляются. Авто-очистка на
              сервере: <code>FDB_STALE_CLEAR_DAYS</code> (по умолчанию 60).
            </p>
            {staleFdbErr && <p style={{ color: "#f88" }}>{staleFdbErr}</p>}
            {staleFdbMsg && <p style={{ color: "#8d8" }}>{staleFdbMsg}</p>}
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.75rem", alignItems: "flex-end" }}>
              <label>
                Старше (дней)
                <br />
                <input
                  type="number"
                  min={1}
                  max={3650}
                  value={staleFdbDays}
                  onChange={(e) => setStaleFdbDays(e.target.value)}
                  disabled={staleFdbBusy || !canWrite}
                  style={{ width: 100 }}
                />
              </label>
              <button type="button" disabled={staleFdbBusy} onClick={loadStaleFdbPreview}>
                Показать кандидатов
              </button>
              {canWrite && (
                <button type="button" disabled={staleFdbBusy} style={{ borderColor: "#864" }} onClick={clearStaleFdb}>
                  Очистить FDB
                </button>
              )}
            </div>
            {staleFdbPreview && (
              <div style={{ marginTop: "0.75rem" }}>
                <p style={{ color: "#9aa3b5", fontSize: "0.9rem" }}>
                  Кандидатов: {staleFdbPreview.count}
                  {staleFdbPreview.count > 20 ? " (показаны первые 20)" : ""}
                </p>
                {staleFdbPreview.devices.length > 0 && (
                  <ul style={{ margin: "0.35rem 0 0", paddingLeft: "1.2rem", maxHeight: 220, overflow: "auto" }}>
                    {staleFdbPreview.devices.slice(0, 20).map((d) => (
                      <li key={d.id}>
                        {d.name} ({d.host || "—"}) — {d.entry_count} MAC
                        {d.last_fdb_poll_at
                          ? `, FDB: ${new Date(d.last_fdb_poll_at).toLocaleString()}`
                          : ", FDB: никогда"}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </section>

          <section className="settings-card settings-card--danger">
            <h2>Удаление устройств (оффлайн)</h2>
            <p>
              Удаляет из инвентаря узлы с <code>offline_since</code> старше N дней (без ручного статуса «онлайн»).
              Нужна роль admin. События и порты узла удаляются каскадом.
            </p>
            {offlineErr && <p style={{ color: "#f88" }}>{offlineErr}</p>}
            {offlineMsg && <p style={{ color: "#8d8" }}>{offlineMsg}</p>}
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.75rem", alignItems: "flex-end" }}>
              <label>
                Оффлайн более (дней)
                <br />
                <input
                  type="number"
                  min={1}
                  max={3650}
                  value={offlineDays}
                  onChange={(e) => setOfflineDays(e.target.value)}
                  disabled={offlineBusy || !isAdmin}
                  style={{ width: 100 }}
                />
              </label>
              <button type="button" disabled={offlineBusy} onClick={loadOfflinePreview}>
                Показать кандидатов
              </button>
              {isAdmin && (
                <button
                  type="button"
                  disabled={offlineBusy}
                  style={{ borderColor: "#844" }}
                  onClick={deleteOfflineDevices}
                >
                  Удалить устройства
                </button>
              )}
            </div>
            {offlinePreview && (
              <div style={{ marginTop: "0.75rem" }}>
                <p style={{ color: "#9aa3b5", fontSize: "0.9rem" }}>
                  Кандидатов: {offlinePreview.count}
                  {offlinePreview.count > 20 ? " (показаны первые 20)" : ""}
                </p>
                {offlinePreview.devices.length > 0 && (
                  <ul style={{ margin: "0.35rem 0 0", paddingLeft: "1.2rem", maxHeight: 220, overflow: "auto" }}>
                    {offlinePreview.devices.slice(0, 20).map((d) => (
                      <li key={d.id}>
                        {d.name} ({d.host || "—"}) — оффлайн с {new Date(d.offline_since).toLocaleString()}
                      </li>
                    ))}
                  </ul>
                )}
              </div>
            )}
          </section>

          <section className="settings-card settings-card--danger">
            <h2>Очистить инвентарь</h2>
            <p>
              Удаляет все коммутаторы из базы вместе с портами, событиями и правилами ignore. Нужна роль operator/admin.
              Действие необратимо.
            </p>
            {devicesClearErr && <p style={{ color: "#f88" }}>{devicesClearErr}</p>}
            {devicesClearMsg && <p style={{ color: "#8d8" }}>{devicesClearMsg}</p>}
            <button
              type="button"
              style={{ borderColor: "#844" }}
              onClick={() => {
                const n = devicesCount ?? 0;
                if (
                  !window.confirm(
                    `Удалить все узлы (${n} шт.)? События и данные портов будут удалены из базы. Действие необратимо.`,
                  )
                ) {
                  return;
                }
                setDevicesClearErr(null);
                setDevicesClearMsg(null);
                apiDeleteJson<{ deleted: number }>("/api/v1/devices", {
                  headers: { "X-Confirm": "DELETE-ALL-DEVICES" },
                })
                  .then((r) => setDevicesClearMsg(`Удалено узлов: ${r.deleted}.`))
                  .catch((e: Error) => setDevicesClearErr(e.message));
              }}
            >
              Очистить весь список узлов
            </button>
            {devicesCount != null && (
              <span style={{ marginLeft: "0.75rem", color: "#9aa3b5", fontSize: "0.9rem" }}>
                Сейчас в базе: {devicesCount}
              </span>
            )}
          </section>
        </div>
      )}

      {tab === "mac" && (
        <div role="tabpanel">
          <p className="settings-lead">
            Расследование MAC: переходы между портами, flap, Postmortem. WiFi-клиенты на точках доступа часто дают шум —
            их можно не отслеживать, пока не нужен инцидент.
          </p>
          <section className="settings-card">
            <h2>WiFi-клиенты на AP</h2>
            <p>
              WiFi-устройства определяются по ARP-IP из подсети ниже (телефоны, ноутбуки за точками доступа). Пока опция
              <strong> выключена</strong>, для таких MAC не пишутся переходы, события и записи в «Горячих MAC» / Postmortem.
              Live FDB на карточке порта остаётся — скрывается только расследование и шум в логах.
            </p>
            {macInvErr && <p style={{ color: "#f88" }}>{macInvErr}</p>}
            {macInvOk && <p style={{ color: "#8d8" }}>{macInvOk}</p>}
            <form
              onSubmit={saveMacInvestigation}
              style={{ display: "flex", flexDirection: "column", gap: "0.75rem", maxWidth: 520 }}
            >
              <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
                <input
                  type="checkbox"
                  checked={macTrackWiFi}
                  onChange={(e) => setMacTrackWiFi(e.target.checked)}
                  disabled={!canWrite}
                />
                Отслеживать WiFi устройства на AP
              </label>
              <label>
                Подсеть ARP WiFi-клиентов (CIDR)
                <br />
                <input
                  style={{ width: "100%" }}
                  value={macWiFiPrefix}
                  onChange={(e) => setMacWiFiPrefix(e.target.value)}
                  placeholder="192.168.120.0/24"
                  disabled={!canWrite}
                />
              </label>
              <p style={{ margin: 0, color: "#9aa3b5", fontSize: "0.85rem" }}>
                Нужен опрос ARP на L3-узле (роутер/MikroTik и т.п.), где видны IP клиентов WiFi. Для расследования
                инцидента с участием WiFi — включите галочку и сохраните.
              </p>
              {canWrite && <button type="submit">Сохранить</button>}
            </form>
          </section>
        </div>
      )}

      {tab === "access" && (
        <div role="tabpanel">
          <section className="settings-card">
            <h2>Доступ к API</h2>
            <p>
              Вход в интерфейс — страница «Вход в NetLynx» (JWT-сессия). Пароль в браузере не хранится.
            </p>
            <p>
              Аварийный HTTP Basic из <code>NETLYNX_ADMIN_PASSWORD</code> больше не подставляется из настроек.
              Для отладки API используйте вход в UI или вызов с сервера.
            </p>
          </section>
          <section className="settings-card">
            <h2>Параметры сервера</h2>
            <p>
              Интервал опроса коммутаторов, пороги загрузки порта и строка подключения к базе задаются{" "}
              <strong>на компьютере, где запущен сервер</strong>, в переменных окружения или в файле <code>.env</code>{" "}
              (см. <code>.env.example</code> в репозитории): <code>DATABASE_URL</code>,{" "}
              <code>POLL_SCHEDULER_SECONDS</code>, <code>ACCESS_PORT_LONG_IDLE_HOURS</code>,{" "}
              <code>PORT_UTIL_HIGH_PCT</code>, <code>PORT_UTIL_OK_PCT</code> (интервал SNMP по каждому узлу — поле{" "}
              <code>poll_interval_seconds</code> устройства в API/БД).
            </p>
          </section>
        </div>
      )}

      {tab === "notifications" && (
        <div role="tabpanel">
          <section className="settings-card">
            <h2>Каналы уведомлений</h2>
      <p>
        При каждом событии (линк, загрузка порта и т.д.) сервер может отправить <strong>POST с JSON</strong> на webhook
        и/или короткое сообщение в Telegram (Bot API). Включайте каналы только для доверенных адресов и ботов.
      </p>
      {notifErr && <p style={{ color: "#f88" }}>{notifErr}</p>}
      {notifOk && <p style={{ color: "#8d8" }}>{notifOk}</p>}
      <form onSubmit={saveNotifications} className="settings-notif-form">
        <div className="settings-channel">
          <strong>Принимать traps</strong>
          <p style={{ margin: "0.35rem 0 0.75rem", color: "#9aa3b5", fontSize: "0.9rem" }}>
            UDP-приёмник SNMP traps на сервере NetLynx. По умолчанию link up/down из trap — ранний сигнал, а{" "}
            <code>LINK_UP</code>/<code>LINK_DOWN</code> в «Событиях» — после опроса SNMP. Ниже можно включить
            мгновенные события из trap. Если порт приёма &gt; 1000 (по умолчанию 9162), на сервере лучше сделать
            перенаправление UDP <strong>162 → этот порт</strong>.
            {trapReceiverOn
              ? ` Сейчас слушаем :${trapListenPort}.`
              : trapListenEn
                ? " Включено в настройках, сокет ещё не поднят — сохраните ещё раз или проверьте логи."
                : " Приёмник выключен."}
          </p>
          {trapListenErr && (
            <p style={{ color: "#f88" }} role="alert">
              {trapListenErr}
            </p>
          )}
          {trapListenOk && <p style={{ color: "#8d8" }}>{trapListenOk}</p>}
          <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <input
              type="checkbox"
              checked={trapListenEn}
              disabled={!canWrite || trapListenBusy}
              onChange={(e) => setTrapListenEn(e.target.checked)}
            />
            Принимать SNMP traps
          </label>
          <label>
            Порт приёма (UDP)
            <br />
            <input
              style={{ width: "8rem" }}
              type="number"
              min={1}
              max={65535}
              value={trapListenPort}
              disabled={!canWrite || trapListenBusy || !trapListenEn}
              onChange={(e) => setTrapListenPort(e.target.value)}
            />
          </label>
          <div style={{ marginTop: "0.85rem" }}>
            <strong style={{ display: "block", marginBottom: "0.35rem" }}>Мгновенные link-события из trap</strong>
            <p style={{ margin: "0 0 0.5rem", color: "#9aa3b5", fontSize: "0.85rem" }}>
              При режиме не «Выкл» <code>linkUp</code>/<code>linkDown</code> сразу попадают в «События» (не ждут
              опрос). Для «Только с флагом» включите галочку на карточке устройства. Изменения применятся после
              «Сохранить».
            </p>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.75rem", alignItems: "center" }}>
              <label style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                Режим
                <select
                  value={linkTrapMode}
                  disabled={!canWrite || trapListenBusy || !trapListenEn}
                  onChange={(e) => setLinkTrapMode(e.target.value)}
                >
                  <option value="off">Выкл (только опрос)</option>
                  <option value="per_device">Только с флагом на устройстве</option>
                  <option value="all">Все устройства</option>
                </select>
              </label>
              <label style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                Эффекты
                <select
                  value={linkTrapEffects}
                  disabled={!canWrite || trapListenBusy || !trapListenEn || linkTrapMode === "off"}
                  onChange={(e) => setLinkTrapEffects(e.target.value)}
                >
                  <option value="notify">События + уведомления</option>
                  <option value="full">События+уведомления+port actions</option>
                </select>
              </label>
            </div>
          </div>
          <div style={{ marginTop: "0.75rem" }}>
            <button
              type="button"
              disabled={!canWrite || trapListenBusy}
              onClick={() => void saveTrapListen()}
            >
              Сохранить
            </button>
          </div>
        </div>

        <div className="settings-channel">
          <strong>Webhook</strong>
          <label>
            URL (https://…)
            <br />
            <input
              style={{ width: "100%" }}
              value={whURL}
              onChange={(e) => setWhURL(e.target.value)}
              placeholder="https://hooks.slack.com/services/…"
            />
          </label>
          <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <input type="checkbox" checked={whEn} onChange={(e) => setWhEn(e.target.checked)} />
            Включить webhook
          </label>
          <label>
            Типы событий для webhook (через запятую, пусто = все)
            <br />
            <input style={{ width: "100%" }} value={whTypes} onChange={(e) => setWhTypes(e.target.value)} placeholder="LINK_DOWN,MAC_MOVED" />
          </label>
          <label>
            Severity для webhook (через запятую, пусто = все)
            <br />
            <input style={{ width: "100%" }} value={whSev} onChange={(e) => setWhSev(e.target.value)} placeholder="warning,error" />
          </label>
        </div>

        <div className="settings-channel">
          <strong>Email (SMTP)</strong>
          <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <input type="checkbox" checked={emEn} onChange={(e) => setEmEn(e.target.checked)} />
            Включить Email
          </label>
          <label>
            Отправитель (From)
            <br />
            <input style={{ width: "100%" }} value={emFrom} onChange={(e) => setEmFrom(e.target.value)} placeholder="netlynx@home.local" />
          </label>
          <label>
            Получатели (через запятую)
            <br />
            <input style={{ width: "100%" }} value={emTo} onChange={(e) => setEmTo(e.target.value)} placeholder="admin@home.local,ops@home.local" />
          </label>
          <label>
            SMTP host
            <br />
            <input style={{ width: "100%" }} value={smtpHost} onChange={(e) => setSmtpHost(e.target.value)} placeholder="smtp.mail.local" />
          </label>
          <label>
            SMTP port
            <br />
            <input style={{ width: "100%" }} type="number" min={1} max={65535} value={smtpPort} onChange={(e) => setSmtpPort(e.target.value)} />
            <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
              Exchange/МФУ: 465 или 587 + STARTTLS (как на принтере). Auth LOGIN.
            </div>
          </label>
          <label>
            SMTP username (опционально)
            <br />
            <input style={{ width: "100%" }} value={smtpUser} onChange={(e) => setSmtpUser(e.target.value)} placeholder="user" />
          </label>
          <label>
            SMTP password (опционально)
            <br />
            <input
              style={{ width: "100%" }}
              type="password"
              value={smtpPass}
              onChange={(e) => setSmtpPass(e.target.value)}
              placeholder={hasSmtpPass ? "оставьте пустым, чтобы не менять" : "password"}
              autoComplete="new-password"
            />
          </label>
          <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <input type="checkbox" checked={smtpTLSSkip} onChange={(e) => setSmtpTLSSkip(e.target.checked)} />
            Не проверять TLS-сертификат (SMTP по IP / self-signed)
          </label>
          <label>
            Типы событий для Email
            <br />
            <EventTypeChecklistDropdown value={emTypes} onChange={setEmTypes} />
          </label>
          <label>
            Severity для Email (через запятую, пусто = все)
            <br />
            <input style={{ width: "100%" }} value={emSev} onChange={(e) => setEmSev(e.target.value)} placeholder="warning,error" />
          </label>
          <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center", marginTop: 4 }}>
            <button
              type="button"
              disabled={emailTesting}
              onClick={() => {
                setNotifErr(null);
                setNotifOk(null);
                setEmailTesting(true);
                const smtpPortNum = Number(smtpPort);
                apiPost<{ ok: boolean; to?: string }>("/api/v1/settings/notifications/email-test", {
                  email_from: emFrom.trim(),
                  email_to: emTo.trim(),
                  smtp_host: smtpHost.trim(),
                  smtp_port: Number.isFinite(smtpPortNum) ? smtpPortNum : 587,
                  smtp_username: smtpUser.trim(),
                  smtp_password: smtpPass.trim(),
                  smtp_tls_skip_verify: smtpTLSSkip,
                })
                  .then((r) => {
                    setNotifOk(`Тестовое письмо отправлено${r.to ? `: ${r.to}` : ""}.`);
                  })
                  .catch((e: Error) => setNotifErr(e.message))
                  .finally(() => setEmailTesting(false));
              }}
            >
              {emailTesting ? "Отправка…" : "Отправить тестовое письмо"}
            </button>
            <span style={{ fontSize: "0.8rem", color: "#888" }}>
              Проверка SMTP по полям выше (пароль можно не вводить, если уже сохранён).
            </span>
          </div>
        </div>

        <div className="settings-channel">
          <strong>Telegram</strong>
          <label>
            Токен бота (от @BotFather)
            <br />
            <input
              style={{ width: "100%" }}
              type="password"
              value={tgTok}
              onChange={(e) => setTgTok(e.target.value)}
              placeholder={hasTgTok ? "оставьте пустым, чтобы не менять" : "123456789:AA…"}
              autoComplete="off"
            />
          </label>
          <label>
            Chat id (число или @channelusername)
            <br />
            <input style={{ width: "100%" }} value={tgChat} onChange={(e) => setTgChat(e.target.value)} placeholder="-100…" />
          </label>
          <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <input type="checkbox" checked={tgEn} onChange={(e) => setTgEn(e.target.checked)} />
            Включить Telegram
          </label>
          <label>
            Типы событий для Telegram
            <br />
            <EventTypeChecklistDropdown value={tgTypes} onChange={setTgTypes} />
          </label>
          <label>
            Severity для Telegram (через запятую, пусто = все)
            <br />
            <input style={{ width: "100%" }} value={tgSev} onChange={(e) => setTgSev(e.target.value)} placeholder="warning,error" />
          </label>
        </div>

        <div className="settings-channel">
          <strong>Повторы доставки</strong>
          <label>
            Повторы при ошибке (0..10)
            <br />
            <input style={{ width: "100%" }} type="number" min={0} max={10} value={retryMax} onChange={(e) => setRetryMax(e.target.value)} />
          </label>
          <label>
            Базовый backoff, мс (100..60000)
            <br />
            <input
              style={{ width: "100%" }}
              type="number"
              min={100}
              max={60000}
              value={retryBackoff}
              onChange={(e) => setRetryBackoff(e.target.value)}
            />
          </label>
        </div>

        <div className="settings-channel">
          <strong>Действия при инцидентах</strong>
          <p style={{ margin: 0, fontSize: "0.9rem", color: "#aaa" }}>
            По умолчанию выключено. При включении — SNMP admin down порта для выбранных типов событий (нужны write-права
            community).
          </p>
          <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <input type="checkbox" checked={incidentEn} onChange={(e) => setIncidentEn(e.target.checked)} />
            Включить авто-блокировку порта
          </label>
          <label style={{ display: "flex", alignItems: "center", gap: "0.5rem" }}>
            <input type="checkbox" checked={incidentDry} onChange={(e) => setIncidentDry(e.target.checked)} />
            Dry-run (только событие, без SNMP SET)
          </label>
          <label>
            Типы событий (через запятую)
            <br />
            <input style={{ width: "100%" }} value={incidentTypes} onChange={(e) => setIncidentTypes(e.target.value)} />
          </label>
          <label>
            Cooldown (сек, не повторять на том же порту)
            <br />
            <input style={{ width: 120 }} value={incidentCooldown} onChange={(e) => setIncidentCooldown(e.target.value)} />
          </label>
        </div>

        <button type="submit">Сохранить уведомления на сервере</button>
      </form>
          </section>
        </div>
      )}

      {tab === "users" && (
        <div role="tabpanel">
          {!isAdmin ? (
            <p className="settings-lead">Управление пользователями доступно только администратору.</p>
          ) : (
            <>
              <section className="settings-card">
                <h2>Учётные записи</h2>
                <p>
                  Роли: <strong>viewer</strong> — только просмотр; <strong>operator</strong> — узлы, топология, уведомления;
                  <strong>admin</strong> — пользователи и резервные копии. Подробно: файл <code>docs/Roles.md</code> в
                  репозитории.
                </p>
                {usersErr && <p style={{ color: "#f88" }}>{usersErr}</p>}
      <ul className="settings-users-list">
        {users.map((u) => (
          <li key={u.id} className="settings-user-row">
            <span className="settings-user-row-info">
              {u.username} · {u.role} · {u.is_active ? "активен" : "выкл."}
            </span>
            <button
              type="button"
              className="settings-user-delete-btn"
              onClick={() => {
                if (!window.confirm(`Удалить пользователя «${u.username}»?`)) return;
                setUsersErr(null);
                apiDelete(`/api/v1/users/${u.id}`)
                  .then(() => loadUsers())
                  .catch((err: Error) => setUsersErr(err.message));
              }}
            >
              удалить
            </button>
          </li>
        ))}
      </ul>
      <form
        onSubmit={(e) => {
          e.preventDefault();
          setUsersErr(null);
          apiPost("/api/v1/users", { username: newUserName.trim(), password: newUserPass, role: newUserRole })
            .then(() => {
              setNewUserName("");
              setNewUserPass("");
              return loadUsers();
            })
            .catch((err: Error) => setUsersErr(err.message));
        }}
        style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap", maxWidth: 520 }}
      >
        <input placeholder="логин" value={newUserName} onChange={(e) => setNewUserName(e.target.value)} required />
        <input placeholder="пароль" type="password" value={newUserPass} onChange={(e) => setNewUserPass(e.target.value)} required />
        <select value={newUserRole} onChange={(e) => setNewUserRole(e.target.value)}>
          <option value="viewer">viewer</option>
          <option value="operator">operator</option>
          <option value="admin">admin</option>
        </select>
        <button type="submit">Добавить</button>
      </form>

              </section>
              <section className="settings-card">
                <h2>Журнал аудита</h2>
                <ul style={{ fontSize: "0.9rem", maxHeight: 200, overflow: "auto" }}>
                  {audit.map((a) => (
                    <li key={a.id}>
                      {new Date(a.created_at).toLocaleString()} · {a.username ?? "—"} · {a.action}
                      {a.entity_type ? ` (${a.entity_type})` : ""}
                    </li>
                  ))}
                </ul>
              </section>
            </>
          )}
        </div>
      )}

      {tab === "backup" && (
        <div role="tabpanel">
          {!isAdmin ? (
            <p className="settings-lead">Резервные копии доступны только администратору.</p>
          ) : (
            <>
              <section className="settings-card">
                <h2>Резервные копии</h2>
          <p style={{ color: "#9aa3b5", fontSize: "0.9rem", maxWidth: 720 }}>
            Архив: дамп БД, файл окружения сервера и (опционально) running-config коммутаторов по SSH.
            Локально и на шаре старые ZIP удаляются по сроку в днях. Почта — без ограничения срока: письмо
            уходит и дальше живёт в ящике получателя.
          </p>
          {bkErr && <p style={{ color: "#f88" }}>{bkErr}</p>}
          {bkOk && <p style={{ color: "#8d8" }}>{bkOk}</p>}
          <p style={{ fontSize: "0.9rem" }}>
            Последний запуск:{" "}
            {bkLastAt ? new Date(bkLastAt).toLocaleString() : "ещё не было"}
            {bkLastStatus ? ` · ${backupStatusLabel(bkLastStatus)}` : ""}
            {bkLastError ? ` · ${bkLastError}` : ""}
          </p>
          <pre
            style={{
              maxWidth: 720,
              maxHeight: 280,
              overflow: "auto",
              background: "#11161e",
              border: "1px solid #2a3344",
              borderRadius: 6,
              padding: "0.65rem 0.75rem",
              fontSize: "0.8rem",
              lineHeight: 1.45,
              whiteSpace: "pre-wrap",
              color: bkLastStatus === "fail" ? "#f8b4b4" : "#c5cdd8",
            }}
          >
            {bkLog || "Журнал появится после бэкапа, проверки дампа или импорта."}
          </pre>
          <form onSubmit={saveBackup} style={{ display: "flex", flexDirection: "column", gap: "0.65rem", maxWidth: 640 }}>
            <label>
              <input type="checkbox" checked={bkScheduleEn} onChange={(e) => setBkScheduleEn(e.target.checked)} />{" "}
              Расписание (время сервера)
            </label>
            <label>
              Время
              <br />
              <input type="time" value={bkTime} onChange={(e) => setBkTime(e.target.value || "02:00")} />
            </label>

            <h3 style={{ margin: "0.5rem 0 0" }}>Диск сервера NetLynx</h3>
            <label>
              <input type="checkbox" checked={bkLocalEn} onChange={(e) => setBkLocalEn(e.target.checked)} /> Сохранять
              локально
            </label>
            <label>
              Каталог
              <br />
              <input value={bkLocalDir} onChange={(e) => setBkLocalDir(e.target.value)} style={{ width: "100%" }} />
            </label>
            <label>
              Хранить, дней
              <br />
              <input
                type="number"
                min={1}
                max={365}
                value={bkLocalDays}
                onChange={(e) => setBkLocalDays(e.target.value)}
                style={{ width: 80 }}
              />
            </label>

            <h3 style={{ margin: "0.5rem 0 0" }}>Почта</h3>
            <label>
              <input type="checkbox" checked={bkEmailEn} onChange={(e) => setBkEmailEn(e.target.checked)} /> Отправлять
              ZIP вложением (SMTP из блока «Уведомления»). Срок хранения не ограничивается.
            </label>
            <label>
              Кому (пусто = адрес из уведомлений)
              <br />
              <input value={bkEmailTo} onChange={(e) => setBkEmailTo(e.target.value)} style={{ width: "100%" }} />
            </label>

            <h3 style={{ margin: "0.5rem 0 0" }}>Шара SMB / NFS</h3>
            <label>
              <input type="checkbox" checked={bkShareEn} onChange={(e) => setBkShareEn(e.target.checked)} /> Копировать
              на шару
            </label>
            <label>
              Тип
              <br />
              <select value={bkShareKind} onChange={(e) => setBkShareKind(e.target.value)}>
                <option value="smb">SMB</option>
                <option value="nfs">NFS (уже смонтированный каталог)</option>
              </select>
            </label>
            <label>
              Путь
              <br />
              <input
                value={bkShareURL}
                onChange={(e) => setBkShareURL(e.target.value)}
                placeholder={bkShareKind === "nfs" ? "/mnt/nas/netlynx" : "//nas/backups/netlynx"}
                style={{ width: "100%" }}
              />
            </label>
            {bkShareKind === "smb" && (
              <>
                <label>
                  Пользователь SMB
                  <br />
                  <input value={bkShareUser} onChange={(e) => setBkShareUser(e.target.value)} />
                </label>
                <label>
                  Пароль{bkHasSharePass ? " (пусто = не менять)" : ""}
                  <br />
                  <input
                    type="password"
                    value={bkSharePass}
                    onChange={(e) => setBkSharePass(e.target.value)}
                    placeholder={bkHasSharePass ? "••••••••" : ""}
                    autoComplete="new-password"
                  />
                </label>
                <label>
                  Домен (если нужен)
                  <br />
                  <input value={bkShareDomain} onChange={(e) => setBkShareDomain(e.target.value)} />
                </label>
              </>
            )}
            <label>
              Хранить на шаре, дней
              <br />
              <input
                type="number"
                min={1}
                max={365}
                value={bkShareDays}
                onChange={(e) => setBkShareDays(e.target.value)}
                style={{ width: 80 }}
              />
            </label>

            <h3 style={{ margin: "0.5rem 0 0" }}>Конфиги коммутаторов (SSH)</h3>
            <label>
              <input type="checkbox" checked={bkSwitchEn} onChange={(e) => setBkSwitchEn(e.target.checked)} /> Снимать
              running-config только у коммутаторов (категория switch)
            </label>
            <p style={{ margin: 0, color: "#9aa3b5", fontSize: "0.85rem" }}>
              Сначала учётка из карточки узла, иначе эти поля. Неизвестный SSH-ключ хоста при первом подключении
              принимается; при смене ключа — ошибка.
            </p>
            <label>
              Пользователь
              <br />
              <input value={bkSshUser} onChange={(e) => setBkSshUser(e.target.value)} placeholder="ubnt / admin" />
            </label>
            <label>
              Пароль{bkHasSshPass ? " (пусто = не менять)" : ""}
              <br />
              <input
                type="password"
                value={bkSshPass}
                onChange={(e) => setBkSshPass(e.target.value)}
                placeholder={bkHasSshPass ? "••••••••" : ""}
                autoComplete="new-password"
              />
            </label>
            <label>
              Enable{bkHasSshEnable ? " (пусто = не менять)" : ""}
              <br />
              <input
                type="password"
                value={bkSshEnable}
                onChange={(e) => setBkSshEnable(e.target.value)}
                placeholder={bkHasSshEnable ? "••••••••" : ""}
                autoComplete="new-password"
              />
            </label>
            <label>
              Порт
              <br />
              <input value={bkSshPort} onChange={(e) => setBkSshPort(e.target.value)} style={{ width: 80 }} />
            </label>
            <label>
              Таймаут, сек
              <br />
              <input value={bkSshTimeout} onChange={(e) => setBkSshTimeout(e.target.value)} style={{ width: 80 }} />
            </label>

            <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap" }}>
              <button type="submit">Сохранить настройки</button>
              <button type="button" onClick={runBackupNow} disabled={bkRunning}>
                {bkRunning ? "Выполняется…" : "Сделать бэкап сейчас"}
              </button>
            </div>
          </form>

          <h3 style={{ margin: "1.5rem 0 0.35rem" }}>Восстановление</h3>
          <p style={{ color: "#9aa3b5", fontSize: "0.9rem", maxWidth: 720, marginTop: 0 }}>
            <strong>Проверить дамп</strong> заливает ZIP во временную БД <code>invetor_rv_*</code> на том же PostgreSQL,
            считает таблицы и сразу удаляет её. Рабочая база <code>invetor</code> не меняется — так можно проверять
            архивы на проде. <strong>Импорт</strong> в рабочую БД разрешён только если узлов 0 (чистая система);
            после него перезапустите службу.
          </p>
          <p style={{ fontSize: "0.9rem" }}>
            Узлов в рабочей БД: {bkLiveDevices == null ? "…" : bkLiveDevices}
            {bkLiveDevices != null && bkLiveDevices > 0 ? " — импорт заблокирован" : ""}
          </p>
          <label>
            ZIP из каталога архивов
            <br />
            <select
              value={bkArchiveSel}
              onChange={(e) => {
                setBkArchiveSel(e.target.value);
                setBkZipFile(null);
              }}
              disabled={bkRunning}
              style={{ maxWidth: 640, width: "100%" }}
            >
              {bkArchives.length === 0 ? (
                <option value="">нет файлов netlynx-*.zip</option>
              ) : (
                bkArchives.map((a) => {
                  const tm = formatArchiveTime(a.mod_time);
                  return (
                    <option key={a.name} value={a.name}>
                      {a.name}
                      {tm ? ` · ${tm}` : ""} ({formatBytes(a.size)})
                    </option>
                  );
                })
              )}
            </select>
          </label>
          <label style={{ display: "block", marginTop: "0.5rem" }}>
            Или загрузить ZIP
            <br />
            <input
              type="file"
              accept=".zip,application/zip"
              disabled={bkRunning}
              onChange={(e) => setBkZipFile(e.target.files?.[0] ?? null)}
            />
            {bkZipFile ? (
              <span style={{ marginLeft: "0.5rem", color: "#9aa3b5", fontSize: "0.85rem" }}>
                будет использован файл: {bkZipFile.name}
              </span>
            ) : null}
          </label>
          <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap", marginTop: "0.65rem" }}>
            <button type="button" onClick={() => startRestoreJob(false)} disabled={bkRunning}>
              Проверить дамп
            </button>
            <button
              type="button"
              onClick={() => startRestoreJob(true)}
              disabled={bkRunning || (bkLiveDevices ?? 0) > 0}
              style={{ borderColor: "#844" }}
            >
              Импортировать в пустую БД
            </button>
          </div>
              </section>
            </>
          )}
        </div>
      )}

      {tab === "journal" && (
        <div role="tabpanel">
          {!isAdmin ? (
            <p className="settings-lead">Журнал службы доступен только администратору.</p>
          ) : (
            <JournalPanel />
          )}
        </div>
      )}

      {tab === "traps" && (
        <div role="tabpanel">
          <TrapLogsPanel canWrite={canWrite} />
        </div>
      )}

      {tab === "about" && (
        <div role="tabpanel" className="settings-about">
          <div className="settings-about-card">
            <div className="settings-about-brand">
              <img src="/logo.png" alt="" width={72} height={72} className="settings-about-logo" />
              <h2 className="settings-about-title">NetLynx</h2>
            </div>
            <dl className="settings-about-meta">
              <div>
                <dt>Версия</dt>
                <dd>
                  {aboutVersion || "—"}
                  {aboutCommit ? (
                    <span className="settings-about-muted"> · {aboutCommit.slice(0, 7)}</span>
                  ) : null}
                </dd>
              </div>
              {aboutBuiltAt ? (
                <div>
                  <dt>Сборка</dt>
                  <dd>{aboutBuiltAt.replace("T", " ").replace(/Z$/, " UTC")}</dd>
                </div>
              ) : null}
              <div>
                <dt>Автор</dt>
                <dd>
                  Андрей &laquo;PapaTramp&raquo; Луценко
                </dd>
              </div>
              <div>
                <dt>Лицензия</dt>
                <dd>
                  Freeware: бесплатно для личного и некоммерческого использования.
                  Коммерция и модификации — по письменному разрешению автора (
                  <a href="mailto:papatramp@gmail.com">papatramp@gmail.com</a>
                  ). Подробности — файл <code>LICENSE</code> в репозитории.
                </dd>
              </div>
              <div>
                <dt>Обратная связь</dt>
                <dd>
                  <a href="mailto:papatramp@gmail.com">papatramp@gmail.com</a>
                  <span className="settings-about-muted"> — глюки и предложения</span>
                </dd>
              </div>
            </dl>
            <hr className="settings-about-sep" />
            <p className="settings-about-lead">
              SNMP-мониторинг коммутаторов, веб-UI. Опрашивает оборудование по SNMP, показывает
              состояние портов, события и карту связей в браузере.
            </p>
            <ul className="settings-about-list">
              <li>Инвентарь узлов: коммутаторы, роутеры, точки доступа, ПК, телефоны, МФУ</li>
              <li>Опрос SNMP (линк, скорость, утилизация, PoE, FDB/ARP) и события по портам</li>
              <li>Топология по LLDP/CDP, FDB и ручным связям; обнаружение соседей</li>
              <li>Уведомления: Email, Telegram, webhook; журнал службы</li>
              <li>Резервные копии БД и конфигов свитчей; роли доступа viewer / operator / admin</li>
              <li>Импорт из UISP и ручное добавление устройств</li>
            </ul>
          </div>
        </div>
      )}
    </div>
  );
}

function CategoryRow({
  cat,
  canWrite,
  busy,
  onSaved,
  onError,
  onDeleted,
  setBusy,
}: {
  cat: DeviceCategoryDef;
  canWrite: boolean;
  busy: boolean;
  onSaved: () => void;
  onError: (m: string) => void;
  onDeleted: () => void;
  setBusy: (v: boolean) => void;
}) {
  const [color, setColor] = useState(cat.color);
  const [label, setLabel] = useState(cat.label);
  const [blink, setBlink] = useState(!!cat.blink);

  useEffect(() => {
    setColor(cat.color);
    setLabel(cat.label);
    setBlink(!!cat.blink);
  }, [cat.color, cat.label, cat.blink, cat.id]);

  function saveColor(next: string) {
    setColor(next);
    if (!canWrite) return;
    setBusy(true);
    apiPatch<DeviceCategoryDef>(`/api/v1/settings/device-categories/${cat.id}`, { color: next })
      .then(() => onSaved())
      .catch((e: Error) => onError(e.message))
      .finally(() => setBusy(false));
  }

  function saveBlink(next: boolean) {
    setBlink(next);
    if (!canWrite) return;
    setBusy(true);
    apiPatch<DeviceCategoryDef>(`/api/v1/settings/device-categories/${cat.id}`, { blink: next })
      .then(() => onSaved())
      .catch((e: Error) => {
        setBlink(!!cat.blink);
        onError(e.message);
      })
      .finally(() => setBusy(false));
  }

  function saveLabel() {
    const t = label.trim();
    if (!canWrite || cat.builtin || t === cat.label || !t) {
      setLabel(cat.label);
      return;
    }
    setBusy(true);
    apiPatch<DeviceCategoryDef>(`/api/v1/settings/device-categories/${cat.id}`, { label: t })
      .then(() => onSaved())
      .catch((e: Error) => {
        setLabel(cat.label);
        onError(e.message);
      })
      .finally(() => setBusy(false));
  }

  return (
    <tr>
      <td style={{ padding: "0.35rem 0.5rem", verticalAlign: "middle" }}>
        <DeviceCategoryIcon category={cat.id} height={24} title={cat.label} />
      </td>
      <td style={{ padding: "0.35rem 0.5rem", verticalAlign: "middle" }}>
        <input
          type="color"
          value={/^#[0-9a-fA-F]{6}$/.test(color) ? color : "#c5c9d0"}
          disabled={!canWrite || busy}
          onChange={(e) => saveColor(e.target.value)}
          title={canWrite ? "Цвет типа" : "Только просмотр"}
          style={{ width: 36, height: 28, padding: 0, border: "1px solid #40506a", background: "transparent", cursor: canWrite ? "pointer" : "default" }}
        />
      </td>
      <td style={{ padding: "0.35rem 0.5rem", verticalAlign: "middle" }}>
        <label style={{ display: "inline-flex", alignItems: "center", gap: 6, cursor: canWrite ? "pointer" : "default" }}>
          <input
            type="checkbox"
            checked={blink}
            disabled={!canWrite || busy}
            onChange={(e) => saveBlink(e.target.checked)}
            title="Мигание точки на топологии"
          />
          <span style={{ fontSize: "0.85rem", color: blink ? "#ff8c00" : "#6a7388" }}>{blink ? "да" : "нет"}</span>
        </label>
      </td>
      <td style={{ padding: "0.35rem 0.5rem", verticalAlign: "middle" }}>
        {cat.builtin || !canWrite ? (
          <span>{cat.label}</span>
        ) : (
          <input
            value={label}
            disabled={busy}
            onChange={(e) => setLabel(e.target.value)}
            onBlur={saveLabel}
            onKeyDown={(e) => {
              if (e.key === "Enter") {
                e.preventDefault();
                (e.target as HTMLInputElement).blur();
              }
            }}
            style={{ width: "100%", maxWidth: 220 }}
          />
        )}
        {cat.builtin ? (
          <span style={{ marginLeft: 8, color: "#6a7388", fontSize: "0.75rem" }}>встроенный</span>
        ) : null}
      </td>
      <td style={{ padding: "0.35rem 0.5rem", verticalAlign: "middle", fontFamily: "ui-monospace, monospace", fontSize: "0.85rem", color: "#9aa3b5" }}>
        {cat.id}
      </td>
      <td style={{ padding: "0.35rem 0.5rem", verticalAlign: "middle" }}>
        {!cat.builtin && canWrite ? (
          <button
            type="button"
            disabled={busy}
            style={{ fontSize: "0.8rem", borderColor: "#844" }}
            onClick={() => {
              if (!window.confirm(`Удалить тип «${cat.label}»? Узлы этого типа станут «Иные устройства».`)) return;
              setBusy(true);
              apiDelete(`/api/v1/settings/device-categories/${cat.id}`)
                .then(() => onDeleted())
                .catch((e: Error) => onError(e.message))
                .finally(() => setBusy(false));
            }}
          >
            Удалить
          </button>
        ) : null}
      </td>
    </tr>
  );
}

