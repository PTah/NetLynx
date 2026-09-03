import type { CSSProperties } from "react";
import { FormEvent, Fragment, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { apiDelete, apiGet, apiPatch, apiPost, apiPut, asArray } from "../api";
import { DeviceSearchSelect } from "../components/DeviceSearchSelect";
import { LocationCombobox } from "../components/LocationCombobox";
import MetricChart from "../components/MetricChart";
import { PortOverviewGrid, isLikelyFiberPort, showPoEIndicator } from "../components/PortOverviewGrid";
import { PortSettingsModal, type PortLiveSettings, type PortSettingsTarget } from "../components/PortSettingsModal";
import { formatBitRate, TrafficSparkline } from "../components/TrafficSparkline";
import { formatEventSummary } from "../eventFormat";
import { usePersistedColumnWidths } from "../hooks/usePersistedColumnWidths";
import { useAuthRole } from "../hooks/useAuthRole";
import { formatPortSpeedFromRow, linkMbps } from "../linkSpeedFormat";
import { DEVICE_BACK_DEFAULT, type DeviceBackRef, deviceLinkState } from "../navigation";
import {
  type DeviceCategory,
  normalizeDeviceCategory,
} from "../deviceCategories";
import {
  shouldSkipDeviceCliSyncSession,
  writeDeviceCliSyncSession,
} from "../deviceCliSync";
import { DeviceCategoryIcon } from "../components/DeviceCategoryIcon";
import { DeviceConfigHistory } from "../components/DeviceConfigHistory";
import { DeviceVlanTab } from "../components/DeviceVlanTab";
import { useDeviceCategories } from "../hooks/useDeviceCategories";
import {
  deviceReachabilityLabel,
  onlineOverrideMode,
  type OnlineOverrideMode,
} from "../deviceOnline";
import { formatSysUptime } from "../uptimeFormat";
import { formatMacDisplay, macVendorLabel } from "../macUtil";
import type { Device, EventRow } from "../types";
import type { ManualTopologyLink } from "../topologyTypes";
import { requestTopologyRefresh } from "../topologyRefresh";
import { PromoteDiscoveredForm, type PromoteFormValues, type PromotePreview } from "../components/PromoteDiscoveredForm";

type IfRow = {
  if_index: number;
  if_descr?: string | null;
  if_name?: string | null;
  if_type?: number | null;
  admin_status?: number | null;
  oper_status?: number | null;
  if_speed?: number | null;
  if_high_speed?: number | null;
  port_role?: string | null;
  util_in_pct?: number | null;
  util_out_pct?: number | null;
  util_max_pct?: number | null;
  poe_active?: boolean | null;
  poe_power_w?: number | null;
  util_high_pct?: number | null;
  util_ok_pct?: number | null;
  event_ignored?: boolean;
  ignore_mode?: PortIgnoreMode;
  descr_override?: string | null;
  cli_description?: string | null;
  vlan_id?: number | null;
  cli_port_mode?: string | null;
  cli_access_vlan?: number | null;
};

function portDisplayDescr(p: {
  descr_override?: string | null;
  cli_description?: string | null;
  if_descr?: string | null;
}): string {
  const ov = (p.descr_override ?? "").trim();
  if (ov) return ov;
  const cli = (p.cli_description ?? "").trim();
  if (cli) return cli;
  return (p.if_descr ?? "").trim();
}

/** Запись настроек порта по SSH: EdgeSwitch Fastpath / Eltex / SNR (не XP). RouterOS-роутеры — только просмотр. */
function devicePortSettingsWritable(d: {
  device_category?: string;
  name?: string;
  sys_descr?: string | null;
  sys_name?: string | null;
  ssh_vendor?: string | null;
}): boolean {
  if (isMikrotikRouter(d)) return false;
  const blob = `${d.sys_descr ?? ""} ${d.sys_name ?? ""} ${d.name ?? ""}`.toLowerCase();
  if (blob.includes("5xp") || blob.includes("8xp") || blob.includes("edgeswitch xp")) return false;
  if (blob.includes("edgeswitch") || blob.includes("ubiquiti") || blob.includes("ubnt")) return true;
  if (blob.includes("eltex") || blob.includes(" mes")) return true;
  if (blob.includes("snr") || blob.includes("nag llc")) return true;
  return false;
}

/** RouterOS-роутер MikroTik: без периодического SSH sync; конфиг — только в бэкапе. */
function isMikrotikRouter(d: {
  device_category?: string;
  name?: string;
  sys_descr?: string | null;
  sys_name?: string | null;
  ssh_vendor?: string | null;
}): boolean {
  if (normalizeDeviceCategory(d.device_category) !== "router") return false;
  const vendor = (d.ssh_vendor ?? "auto").trim().toLowerCase();
  if (vendor === "mikrotik") return true;
  const blob = `${d.sys_descr ?? ""} ${d.sys_name ?? ""} ${d.name ?? ""}`.toLowerCase();
  return (
    blob.includes("mikrotik") ||
    blob.includes("routeros") ||
    blob.includes("routerboard")
  );
}

function devicePoE24VSupported(d: {
  name?: string;
  sys_descr?: string | null;
  sys_name?: string | null;
}): boolean {
  const blob = `${d.sys_descr ?? ""} ${d.sys_name ?? ""} ${d.name ?? ""}`.toLowerCase();
  if (blob.includes("5xp") || blob.includes("8xp") || blob.includes("edgeswitch xp")) return false;
  return blob.includes("edgeswitch") || blob.includes("ubiquiti") || blob.includes("ubnt");
}

type PortIgnoreMode = "off" | "soft" | "all";

function portIgnoreMode(p: IfRow): PortIgnoreMode {
  if (p.ignore_mode === "soft" || p.ignore_mode === "all") return p.ignore_mode;
  if (p.event_ignored) return "soft";
  return "off";
}

function nextPortIgnoreMode(cur: PortIgnoreMode): PortIgnoreMode {
  if (cur === "off") return "soft";
  if (cur === "soft") return "all";
  return "off";
}

function portIgnoreButtonLabel(mode: PortIgnoreMode): string {
  switch (mode) {
    case "soft":
      return "Тихий";
    case "all":
      return "Выкл";
    default:
      return "Монит.";
  }
}

function portIgnoreButtonClass(mode: PortIgnoreMode): string {
  return `port-ignore-btn port-ignore-btn--${mode}`;
}

function portIgnoreRowStyle(mode: PortIgnoreMode): CSSProperties | undefined {
  if (mode === "soft") return { boxShadow: "inset 3px 0 0 #e6c84a" };
  if (mode === "all") return { boxShadow: "inset 3px 0 0 #c44" };
  return undefined;
}

function portIgnoreTitle(mode: PortIgnoreMode): string {
  switch (mode) {
    case "soft":
      return "Тихий: без Telegram/действий по link/util; MAC и интрузия остаются. События пишутся в журнал. Клик → полностью выключить мониторинг.";
    case "all":
      return "Мониторинг выключен: события порта не пишутся в журнал, Telegram и действия отключены. Клик → снова включить.";
    default:
      return "Мониторинг включён. Клик: Тихий (без оповещений link/util) → Выкл (без шума в логах, напр. Wi‑Fi AP) → снова вкл.";
  }
}

type Neighbor = {
  if_index: number;
  rem_index?: number;
  protocol?: string;
  remote_sys_name?: string | null;
  remote_port_id?: string | null;
  remote_chassis_id?: string | null;
  remote_device_id?: number | null;
  stale?: boolean;
};

type PortClient = {
  mac: string;
  vlan_id?: number | null;
  ip?: string | null;
  ips?: string[] | null;
  last_seen_at: string;
  mac_vendor?: string | null;
  existing_device_id?: number | null;
  existing_device_name?: string | null;
};

type PortPromoteTarget = {
  ifIndex: number;
  mac: string;
};

const emptyPortPromote: PromoteFormValues = {
  host: "",
  name: "",
  location: "",
  category: "other",
  community: "public",
};

const PORT_TABLE_COLS = 14;

function portVlanDisplay(p: Pick<IfRow, "port_role" | "vlan_id" | "cli_access_vlan">): string {
  const role = (p.port_role ?? "").toLowerCase();
  if (role === "trunk") return "";
  if (p.cli_access_vlan != null && p.cli_access_vlan > 0) return String(p.cli_access_vlan);
  if (p.vlan_id != null && p.vlan_id > 0) return String(p.vlan_id);
  return "1";
}

type Detail = {
  device: Device & { sys_descr?: string | null; util_high_pct?: number | null; util_ok_pct?: number | null; fdb_poll_interval_seconds?: number | null };
  interfaces: IfRow[];
  recent_events: EventRow[];
  neighbors?: Neighbor[];
  /** MAC из ARP L3-узлов по IP, если chassis_mac пуст */
  arp_observed_macs?: string[];
};

function ifNaturalSortKey(p: IfRow): number {
  const n = (p.if_name ?? "").trim();
  const m3 = n.match(/(\d+)\/(\d+)\/(\d+)$/);
  if (m3) return +m3[1] * 1_000_000 + +m3[2] * 1000 + +m3[3];
  const m2 = n.match(/(\d+)\/(\d+)$/);
  if (m2) return +m2[1] * 1_000_000 + +m2[2] * 1000;
  return 9_000_000_000 + p.if_index;
}

function isPhysicalLikePort(p: IfRow): boolean {
  // ifType по IANAifType: ethernetCsmacd(6), fastEther(62), gigabitEthernet(117), ieee8023adLag(161)
  if (p.if_type === 6 || p.if_type === 62 || p.if_type === 117 || p.if_type === 161) return true;
  const ifName = `${p.if_name ?? ""}`.trim().toLowerCase();
  // «VLAN» в ifDescr (ROOM-VLAN162-…) — обычная подпись порта; исключаем только VLAN/loopback по имени if.
  if (ifName.startsWith("vlan") || ifName.includes("loopback")) return false;
  const joined = `${p.if_name ?? ""} ${p.if_descr ?? ""}`.toLowerCase();
  // Для вендоров с нестандартным ifType оставляем явные физические имена.
  return /(^|\s)(gi|ge|te|xe|eth|ethernet)\d/.test(joined);
}

function adm(v: number | null | undefined): string {
  if (v == null) return "—";
  switch (v) {
    case 1:
      return "up";
    case 2:
      return "down";
    default:
      return String(v);
  }
}

/** Цвета как у сетки портов: 10G светлый текст, 1G зелёный, 100M жёлтый. */
/** Светло-серый: admin up, линка нет — все ячейки, кроме колонки «Линк» (там operStyle). */
const portRowLightDim: CSSProperties = { color: "#aeb6c8" };

/** Тёмно-серый: порт выключен и линка нет — вся строка одним тоном. */
const portRowDarkDim: CSSProperties = { color: "#5c6478" };

type PortRowTone = "normal" | "light-dim" | "dark-dim";

function portRowTone(p: IfRow): PortRowTone {
  const adminUp = p.admin_status === 1;
  const linkUp = p.oper_status === 1;
  if (adminUp && !linkUp) return "light-dim";
  if (!adminUp && !linkUp) return "dark-dim";
  if (!adminUp && linkUp) return "dark-dim";
  return "normal";
}

/** Стиль ячейки таблицы портов: для oper в режиме light-dim — цвета скорости/линка как сейчас. */
function portTableCellStyle(
  p: IfRow,
  column: "data" | "admin" | "oper",
  base?: CSSProperties,
): CSSProperties {
  const tone = portRowTone(p);
  const merged: CSSProperties = { ...(base ?? {}) };
  if (tone === "normal") return merged;
  if (tone === "light-dim") {
    if (column === "oper") {
      return { ...merged, ...operStyle(p.oper_status, p.if_high_speed, p.if_speed) };
    }
    return { ...merged, ...portRowLightDim };
  }
  return { ...merged, ...portRowDarkDim };
}

function operStyle(v: number | null | undefined, ifHighSpeed?: number | null, ifSpeed?: number | null): CSSProperties {
  if (v === 1) {
    const mbps = linkMbps(ifHighSpeed, ifSpeed);
    if (mbps >= 10_000) return { color: "#c8d4e8" };
    if (mbps >= 1_000) return { color: "#6d6" };
    if (mbps >= 100) return { color: "#e6c84a" };
    return { color: "#9aa3b5" };
  }
  if (v === 2) return { color: "#f88" };
  return { color: "#9aa3b5" };
}

type SnmpTestResult = { ok: boolean; sys_name?: string; sys_descr?: string; error?: string };
type TracerouteHop = {
  hop: number;
  address?: string;
  hostname?: string;
  rtt_ms?: number[];
  timeout?: boolean;
};
type TracerouteResult = {
  ok: boolean;
  target: string;
  via?: string;
  hops?: TracerouteHop[];
  error?: string;
};
type TCPProbeResult = {
  target: string;
  port: number;
  open: boolean;
  rtt_ms?: number;
  banner?: string;
  error?: string;
};
type SNMPVersion = "v1" | "v2c" | "v3";

type DeviceDetailTab = "info" | "snmp" | "state" | "vlan" | "ports";

const DEVICE_DETAIL_TABS: { id: DeviceDetailTab; label: string }[] = [
  { id: "info", label: "Информация об узле" },
  { id: "snmp", label: "SNMP/SSH" },
  { id: "state", label: "Состояние" },
  { id: "vlan", label: "VLAN" },
  { id: "ports", label: "Порты" },
];

function parseDeviceDetailTab(raw: string | null): DeviceDetailTab {
  if (raw === "info" || raw === "snmp" || raw === "state" || raw === "vlan" || raw === "ports") {
    return raw;
  }
  return "info";
}

export default function DeviceDetail() {
  const { id } = useParams();
  const navigate = useNavigate();
  const location = useLocation();
  const [searchParams, setSearchParams] = useSearchParams();
  const { canWrite } = useAuthRole();
  const { categories } = useDeviceCategories();
  const back: DeviceBackRef =
    (location.state as { deviceBack?: DeviceBackRef } | null)?.deviceBack ?? DEVICE_BACK_DEFAULT;
  const detailTab = parseDeviceDetailTab(searchParams.get("tab"));
  const setDetailTab = (next: DeviceDetailTab) => {
    setSearchParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        p.set("tab", next);
        return p;
      },
      { replace: true },
    );
  };
  const {
    colgroup: portsColgroup,
    ResizeHandle: PortsResizeHandle,
  } = usePersistedColumnWidths("device-detail-ports-v3", [40, 76, 180, 72, 80, 48, 56, 72, 88, 100, 100, 100, 140, 72]);
  const {
    colgroup: eventsColgroup,
    ResizeHandle: EventsResizeHandle,
  } = usePersistedColumnWidths("device-detail-events", [150, 90, 70, 100, 400]);
  const [data, setData] = useState<Detail | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [snmpDlg, setSnmpDlg] = useState<string | null>(null);
  const [traceBusy, setTraceBusy] = useState(false);
  const [traceResult, setTraceResult] = useState<TracerouteResult | null>(null);
  const [traceTarget, setTraceTarget] = useState("");
  const [tcpProbeBusy, setTcpProbeBusy] = useState(false);
  const [tcpProbeResult, setTcpProbeResult] = useState<TCPProbeResult | null>(null);
  const [tcpProbePort, setTcpProbePort] = useState("22");
  const [tcpProbeTarget, setTcpProbeTarget] = useState("");
  const [nameEdit, setNameEdit] = useState("");
  const [nameMsg, setNameMsg] = useState<string | null>(null);
  const [hostEdit, setHostEdit] = useState("");
  const [hostMsg, setHostMsg] = useState<string | null>(null);
  const [macEdit, setMacEdit] = useState("");
  const [macMsg, setMacMsg] = useState<string | null>(null);
  const [locEdit, setLocEdit] = useState("");
  const [locMsg, setLocMsg] = useState<string | null>(null);
  const [categoryEdit, setCategoryEdit] = useState<DeviceCategory>("switch");
  const [categoryMsg, setCategoryMsg] = useState<string | null>(null);
  const [onlineOverrideEdit, setOnlineOverrideEdit] = useState<OnlineOverrideMode>("auto");
  const [onlineOverrideMsg, setOnlineOverrideMsg] = useState<string | null>(null);
  const [trustLinkMsg, setTrustLinkMsg] = useState<string | null>(null);
  const [pollIntervalEdit, setPollIntervalEdit] = useState("");
  const [pollIntervalMsg, setPollIntervalMsg] = useState<string | null>(null);
  const [snmpVersionEdit, setSnmpVersionEdit] = useState<SNMPVersion>("v2c");
  const [communityEdit, setCommunityEdit] = useState("public");
  const [v3UserEdit, setV3UserEdit] = useState("");
  const [v3AuthProtoEdit, setV3AuthProtoEdit] = useState<"SHA" | "MD5" | "SHA256" | "SHA512">("SHA");
  const [v3AuthPassEdit, setV3AuthPassEdit] = useState("");
  const [v3PrivProtoEdit, setV3PrivProtoEdit] = useState<"AES" | "AES256" | "DES" | "NONE">("AES");
  const [v3PrivPassEdit, setV3PrivPassEdit] = useState("");
  const [snmpCfgMsg, setSnmpCfgMsg] = useState<string | null>(null);
  const [sshUserEdit, setSshUserEdit] = useState("");
  const [sshPassEdit, setSshPassEdit] = useState("");
  const [sshPortEdit, setSshPortEdit] = useState("");
  const [sshEnableEdit, setSshEnableEdit] = useState("");
  const [sshVendorEdit, setSshVendorEdit] = useState("auto");
  const [sshMsg, setSshMsg] = useState<string | null>(null);
  const [hasSshPass, setHasSshPass] = useState(false);
  const [hasSshEnable, setHasSshEnable] = useState(false);
  const formSyncedId = useRef<number | null>(null);
  const [showAllInterfaces, setShowAllInterfaces] = useState(false);
  const [hoveredPortIfIndex, setHoveredPortIfIndex] = useState<number | null>(null);
  const [cpuSamples, setCpuSamples] = useState<{ value: number; sampled_at: string }[]>([]);
  const [utilSamples, setUtilSamples] = useState<{ value: number; sampled_at: string }[]>([]);
  const [devUtilHigh, setDevUtilHigh] = useState("");
  const [devUtilOk, setDevUtilOk] = useState("");
  const [devFdbPoll, setDevFdbPoll] = useState("");
  const [monMsg, setMonMsg] = useState<string | null>(null);
  const [ignoreMsg, setIgnoreMsg] = useState<string | null>(null);
  const [expandedIfIndex, setExpandedIfIndex] = useState<number | null>(null);
  const [portClientsCache, setPortClientsCache] = useState<Record<number, PortClient[]>>({});
  const [clientsLoadingIf, setClientsLoadingIf] = useState<number | null>(null);
  const [clientsErr, setClientsErr] = useState<string | null>(null);
  const [portPromote, setPortPromote] = useState<PortPromoteTarget | null>(null);
  const [portPromoteForm, setPortPromoteForm] = useState<PromoteFormValues>(emptyPortPromote);
  const [portPromotePreview, setPortPromotePreview] = useState<PromotePreview | null>(null);
  const [portPromoteBusy, setPortPromoteBusy] = useState(false);
  const [portPromoteMsg, setPortPromoteMsg] = useState<string | null>(null);
  const [descrEdits, setDescrEdits] = useState<Record<number, string>>({});
  const descrFocusIf = useRef<number | null>(null);
  const portStatusMsgRef = useRef<HTMLParagraphElement | null>(null);
  const portRowRefs = useRef<Map<number, HTMLTableRowElement>>(new Map());
  const [cliSyncMsg, setCliSyncMsg] = useState<string | null>(null);
  const [cliSyncBusy, setCliSyncBusy] = useState(false);
  const [descrMsg, setDescrMsg] = useState<string | null>(null);
  const [portSettings, setPortSettings] = useState<PortSettingsTarget | null>(null);
  const [trafficByIf, setTrafficByIf] = useState<
    Record<number, { rx: { t: string; v: number }[]; tx: { t: string; v: number }[] }>
  >({});
  const [manualLinks, setManualLinks] = useState<ManualTopologyLink[]>([]);
  const [allDevices, setAllDevices] = useState<Device[]>([]);
  const [mlMsg, setMlMsg] = useState<string | null>(null);
  const [mlLocalIf, setMlLocalIf] = useState("");
  const [mlPeerId, setMlPeerId] = useState("");
  const [mlPeerIf, setMlPeerIf] = useState("");
  const [mlPeerIfaces, setMlPeerIfaces] = useState<IfRow[]>([]);
  const [mlBusy, setMlBusy] = useState(false);
  const load = (signal?: AbortSignal) => {
    if (!id) return;
    setErr(null);
    apiGet<Detail>(`/api/v1/devices/${id}/detail`, signal ? { signal } : undefined)
      .then((d) =>
        setData({
          ...d,
          interfaces: asArray(d.interfaces),
          recent_events: asArray(d.recent_events),
          neighbors: asArray(d.neighbors),
        }),
      )
      .catch((e: Error) => {
        if (e.name !== "AbortError") setErr(e.message);
      });
    apiGet<ManualTopologyLink[]>(`/api/v1/manual-links?device_id=${id}&status=all`, signal ? { signal } : undefined)
      .then((list) => setManualLinks(Array.isArray(list) ? list : []))
      .catch(() => setManualLinks([]));
  };

  useEffect(() => {
    if (!id) return;
    load();
  }, [id]);

  useEffect(() => {
    if (location.hash.replace(/^#/, "") !== "device-ports") return;
    setSearchParams(
      (prev) => {
        const p = new URLSearchParams(prev);
        if (p.get("tab") === "ports") return prev;
        p.set("tab", "ports");
        return p;
      },
      { replace: true },
    );
  }, [location.hash, id, setSearchParams]);

  const runCliPortSync = (force: boolean) => {
    if (!id) return;
    setCliSyncBusy(true);
    setCliSyncMsg("Читаем конфиг свитча…");
    const q = force ? "?force=1" : "";
    apiPost<{
      ok?: boolean;
      updated?: number;
      roles?: number;
      descriptions?: number;
      skipped?: boolean;
    }>(`/api/v1/devices/${id}/sync-port-roles-from-config${q}`, {})
      .then((r) => {
        writeDeviceCliSyncSession(Number(id));
        if (r.skipped) {
          setCliSyncMsg(null);
          return;
        }
        const roles = r.roles ?? 0;
        const descrs = r.descriptions ?? 0;
        if (roles > 0 || descrs > 0) {
          const parts: string[] = [];
          if (roles > 0) parts.push(`роли ${roles}`);
          if (descrs > 0) parts.push(`описания ${descrs}`);
          setCliSyncMsg(`Из конфига: ${parts.join(", ")}`);
          load();
        } else {
          setCliSyncMsg(null);
        }
      })
      .catch((e: unknown) => {
        const msg = e instanceof Error ? e.message : "Не удалось прочитать конфиг";
        setCliSyncMsg(
          msg.includes("SSH") || msg.includes("ssh")
            ? `${msg} (карточка узла или Настройки → бэкап)`
            : msg,
        );
      })
      .finally(() => setCliSyncBusy(false));
  };

  useEffect(() => {
    if (!id || !data?.device) return;
    if (isMikrotikRouter(data.device)) return;
    if (shouldSkipDeviceCliSyncSession(Number(id))) return;
    runCliPortSync(false);
  }, [id, data?.device?.device_category, data?.device?.sys_descr, data?.device?.ssh_vendor]);

  useEffect(() => {
    if (!id) return;
    let request: AbortController | null = null;
    const refresh = () => {
      request?.abort();
      request = new AbortController();
      load(request.signal);
    };
    const t = setInterval(refresh, 10000);
    return () => {
      clearInterval(t);
      request?.abort();
    };
  }, [id]);

  useEffect(() => {
    const ac = new AbortController();
    apiGet<Device[]>("/api/v1/devices", { signal: ac.signal })
      .then((list) => setAllDevices(Array.isArray(list) ? list : []))
      .catch((e: Error) => {
        if (e.name !== "AbortError") setAllDevices([]);
      });
    return () => ac.abort();
  }, []);

  useEffect(() => {
    const peer = Number(mlPeerId);
    if (!peer || peer <= 0) {
      setMlPeerIfaces([]);
      return;
    }
    const ac = new AbortController();
    apiGet<IfRow[]>(`/api/v1/devices/${peer}/interfaces`, { signal: ac.signal })
      .then((rows) => setMlPeerIfaces(Array.isArray(rows) ? rows.filter((r) => r.if_index > 0) : []))
      .catch((e: Error) => {
        if (e.name !== "AbortError") setMlPeerIfaces([]);
      });
    return () => ac.abort();
  }, [mlPeerId]);

  useEffect(() => {
    if (!id) return;
    const ac = new AbortController();
    const from = new Date(Date.now() - 24 * 3600 * 1000).toISOString();
    const to = new Date().toISOString();
    const q = `from=${encodeURIComponent(from)}&to=${encodeURIComponent(to)}`;
    apiGet<{ samples: { value: number; sampled_at: string }[] }>(`/api/v1/devices/${id}/metrics?metric_type=cpu_pct&${q}`, { signal: ac.signal })
      .then((r) => setCpuSamples(r.samples ?? []))
      .catch((e: Error) => {
        if (e.name !== "AbortError") setCpuSamples([]);
      });
    apiGet<{ samples: { value: number; sampled_at: string }[] }>(
      `/api/v1/devices/${id}/metrics?metric_type=util_max_pct&if_index=1&${q}`,
      { signal: ac.signal },
    )
      .then((r) => setUtilSamples(r.samples ?? []))
      .catch((e: Error) => {
        if (e.name !== "AbortError") setUtilSamples([]);
      });
    apiGet<{
      ports?: Record<string, { rx?: { t: string; v: number }[]; tx?: { t: string; v: number }[] }>;
    }>(`/api/v1/devices/${id}/traffic-series?minutes=60`, { signal: ac.signal })
      .then((r) => {
        const next: Record<number, { rx: { t: string; v: number }[]; tx: { t: string; v: number }[] }> = {};
        for (const [k, v] of Object.entries(r.ports ?? {})) {
          const idx = Number(k);
          if (!Number.isFinite(idx) || idx <= 0) continue;
          next[idx] = { rx: v.rx ?? [], tx: v.tx ?? [] };
        }
        setTrafficByIf(next);
      })
      .catch((e: Error) => {
        if (e.name !== "AbortError") setTrafficByIf({});
      });
    return () => ac.abort();
  }, [id]);

  useEffect(() => {
    formSyncedId.current = null;
  }, [id]);

  useEffect(() => {
    if (!data?.device) return;
    if (formSyncedId.current === data.device.id) return;
    formSyncedId.current = data.device.id;
    setNameEdit(data.device.name ?? "");
    setHostEdit(data.device.host ?? "");
    setMacEdit(data.device.chassis_mac?.trim() ?? "");
    setLocEdit(data.device.location ?? "");
    setCategoryEdit(normalizeDeviceCategory(data.device.device_category));
    setOnlineOverrideEdit(onlineOverrideMode(data.device));
    const poll = data.device.poll_interval_seconds;
    setPollIntervalEdit(String(poll >= 10 && poll <= 86400 ? poll : 60));
    const verRaw = (data.device.snmp_version ?? "v2c").toLowerCase();
    const ver: SNMPVersion = verRaw === "v1" || verRaw === "v2c" || verRaw === "v3" ? (verRaw as SNMPVersion) : "v2c";
    setSnmpVersionEdit(ver);
    // community не приходит с API — placeholder; пустое при save = не менять
    setCommunityEdit("");
    setV3UserEdit((data.device.v3_user ?? "").trim());
    const v3Auth = (data.device.v3_auth_protocol ?? "SHA").toUpperCase();
    setV3AuthProtoEdit(
      v3Auth === "MD5" || v3Auth === "SHA256" || v3Auth === "SHA512" ? v3Auth : "SHA",
    );
    const v3Priv = (data.device.v3_priv_protocol ?? "AES").toUpperCase();
    setV3PrivProtoEdit(v3Priv === "AES256" || v3Priv === "DES" || v3Priv === "NONE" ? v3Priv : "AES");
    setV3AuthPassEdit("");
    setV3PrivPassEdit("");
    setSnmpCfgMsg(null);
    setSshUserEdit((data.device.ssh_user ?? "").trim());
    setSshPassEdit("");
    setSshPortEdit(data.device.ssh_port != null ? String(data.device.ssh_port) : "");
    setSshEnableEdit("");
    setSshVendorEdit((data.device.ssh_vendor ?? "auto").trim() || "auto");
    setHasSshPass(Boolean(data.device.has_ssh_password));
    setHasSshEnable(Boolean(data.device.has_ssh_enable_password));
    setSshMsg(null);
    setDevUtilHigh(data.device.util_high_pct != null ? String(data.device.util_high_pct) : "");
    setDevUtilOk(data.device.util_ok_pct != null ? String(data.device.util_ok_pct) : "");
    setDevFdbPoll(
      data.device.fdb_poll_interval_seconds != null ? String(data.device.fdb_poll_interval_seconds) : "",
    );
  }, [data]);

  useEffect(() => {
    if (!data?.interfaces) return;
    setDescrEdits((prev) => {
      const next = { ...prev };
      for (const p of data.interfaces) {
        if (descrFocusIf.current === p.if_index) continue;
        next[p.if_index] = portDisplayDescr(p);
      }
      return next;
    });
  }, [data]);

  useEffect(() => {
    if (!descrMsg && !ignoreMsg) return;
    portStatusMsgRef.current?.scrollIntoView({ behavior: "smooth", block: "nearest" });
  }, [descrMsg, ignoreMsg]);

  const neighborsByIf = useMemo(() => {
    const m = new Map<number, Neighbor[]>();
    for (const n of data?.neighbors ?? []) {
      const list = m.get(n.if_index) ?? [];
      const idKey = [
        n.protocol || "lldp",
        n.remote_sys_name || "",
        n.remote_port_id || "",
        n.remote_chassis_id || "",
      ].join("\0");
      const prevIdx = list.findIndex(
        (x) =>
          [x.protocol || "lldp", x.remote_sys_name || "", x.remote_port_id || "", x.remote_chassis_id || ""].join(
            "\0",
          ) === idKey,
      );
      if (prevIdx < 0) {
        list.push(n);
      } else {
        const prev = list[prevIdx];
        const preferNew =
          (!n.stale && prev.stale) ||
          (n.stale === prev.stale && (n.rem_index ?? 0) >= (prev.rem_index ?? 0));
        if (preferNew) list[prevIdx] = n;
      }
      m.set(n.if_index, list);
    }
    return m;
  }, [data?.neighbors]);

  const existingLocations = useMemo(() => {
    const set = new Set<string>();
    for (const d of allDevices) {
      const loc = d.location?.trim();
      if (loc) set.add(loc);
    }
    return [...set].sort((a, b) => a.localeCompare(b, "ru", { sensitivity: "base" }));
  }, [allDevices]);

  async function saveMonitoring(e: FormEvent) {
    e.preventDefault();
    if (!id) return;
    setMonMsg(null);
    const body: Record<string, unknown> = {};
    if (devUtilHigh.trim()) body.util_high_pct = Number(devUtilHigh);
    else body.util_high_pct = null;
    if (devUtilOk.trim()) body.util_ok_pct = Number(devUtilOk);
    else body.util_ok_pct = null;
    if (devFdbPoll.trim()) body.fdb_poll_interval_seconds = Number(devFdbPoll);
    else body.fdb_poll_interval_seconds = null;
    try {
      await apiPatch(`/api/v1/devices/${id}/monitoring`, body);
      setMonMsg("Сохранено");
      load();
    } catch (err) {
      setMonMsg(err instanceof Error ? err.message : "Ошибка");
    }
  }

  function scrollPortRowIntoView(ifIndex: number) {
    requestAnimationFrame(() => {
      portRowRefs.current.get(ifIndex)?.scrollIntoView({ block: "center", behavior: "smooth" });
    });
  }

  function togglePortExpand(ifIndex: number) {
    if (expandedIfIndex === ifIndex) {
      setExpandedIfIndex(null);
      setPortPromote(null);
      setPortPromotePreview(null);
      setPortPromoteMsg(null);
      return;
    }
    setExpandedIfIndex(ifIndex);
    scrollPortRowIntoView(ifIndex);
    setPortPromote(null);
    setPortPromotePreview(null);
    setPortPromoteMsg(null);
    setClientsErr(null);
    if (portClientsCache[ifIndex] != null) return;
    if (!id) return;
    setClientsLoadingIf(ifIndex);
    apiGet<{ clients: PortClient[] }>(`/api/v1/devices/${id}/interfaces/${ifIndex}/clients`)
      .then((r) => {
        setClientsErr(null);
        setPortClientsCache((prev) => ({ ...prev, [ifIndex]: Array.isArray(r.clients) ? r.clients : [] }));
      })
      .catch((e: unknown) => {
        setClientsErr(e instanceof Error ? e.message : String(e));
        // Не кэшируем [] при ошибке — иначе показывается ложное «нет записей FDB».
      })
      .finally(() => setClientsLoadingIf(null));
  }

  function refreshPortClients(ifIndex: number) {
    if (!id) return;
    apiGet<{ clients: PortClient[] }>(`/api/v1/devices/${id}/interfaces/${ifIndex}/clients`)
      .then((r) => {
        setPortClientsCache((prev) => ({ ...prev, [ifIndex]: Array.isArray(r.clients) ? r.clients : [] }));
      })
      .catch(() => {
        /* оставляем кэш */
      });
  }

  function clientIPs(c: PortClient): string[] {
    if (Array.isArray(c.ips) && c.ips.length > 0) return c.ips;
    if (c.ip) return [c.ip];
    return [];
  }

  function openPortPromote(ifIndex: number, c: PortClient) {
    const ips = clientIPs(c);
    const host = ips[0] ?? "";
    setPortPromote({ ifIndex, mac: c.mac });
    setPortPromoteForm({
      host,
      name: host || formatMacDisplay(c.mac),
      location: (data?.device.location ?? "").trim(),
      category: "other",
      community: "public",
    });
    setPortPromotePreview(null);
    setPortPromoteMsg(null);
  }

  async function previewPortClient() {
    if (!id || !portPromote) return;
    if (!portPromoteForm.host.trim()) {
      setPortPromotePreview({ ok: false, error: "Укажите IP для проверки SNMP (с сервера NetLynx)" });
      return;
    }
    setPortPromoteBusy(true);
    setPortPromotePreview(null);
    try {
      const res = await apiPost<PromotePreview>(
        `/api/v1/devices/${id}/interfaces/${portPromote.ifIndex}/clients/preview`,
        {
          mac: portPromote.mac,
          host: portPromoteForm.host,
          name: portPromoteForm.name,
          snmp_version: "v2c",
          community: portPromoteForm.community,
        },
      );
      setPortPromotePreview(res);
    } catch (e) {
      setPortPromotePreview({ ok: false, error: e instanceof Error ? e.message : String(e) });
    } finally {
      setPortPromoteBusy(false);
    }
  }

  async function submitPortClient(e: FormEvent) {
    e.preventDefault();
    if (!id || !portPromote) return;
    if (!portPromoteForm.name.trim()) {
      setPortPromotePreview({ ok: false, error: "Укажите имя узла." });
      return;
    }
    setPortPromoteBusy(true);
    setPortPromoteMsg(null);
    try {
      const res = await apiPost<{ id: number; already?: boolean; linked?: boolean }>(
        `/api/v1/devices/${id}/interfaces/${portPromote.ifIndex}/clients/promote`,
        {
          mac: portPromote.mac,
          host: portPromoteForm.host,
          name: portPromoteForm.name,
          location: portPromoteForm.location,
          device_category: portPromoteForm.category,
          snmp_version: "v2c",
          community: portPromoteForm.community,
        },
      );
      setPortPromoteMsg(
        res.already
          ? `Узел уже в списке (id=${res.id}). Связь на топологии записана.`
          : `Узел создан: id=${res.id}. На топологии появится линк с этого порта.`,
      );
      requestTopologyRefresh();
      setPortPromote(null);
      setPortPromoteForm(emptyPortPromote);
      setPortPromotePreview(null);
      refreshPortClients(portPromote.ifIndex);
      load();
    } catch (err) {
      setPortPromotePreview({ ok: false, error: err instanceof Error ? err.message : String(err) });
    } finally {
      setPortPromoteBusy(false);
    }
  }

  async function linkExistingPortClient(ifIndex: number, c: PortClient) {
    if (!id) return;
    const ips = clientIPs(c);
    setPortPromoteBusy(true);
    setPortPromoteMsg(null);
    try {
      const res = await apiPost<{ id: number }>(`/api/v1/devices/${id}/interfaces/${ifIndex}/clients/promote`, {
        mac: c.mac,
        host: ips[0] ?? "",
        name: c.existing_device_name || formatMacDisplay(c.mac),
        snmp_version: "v2c",
        community: "public",
      });
      setPortPromoteMsg(`Связь на топологии записана (узел id=${res.id}).`);
      requestTopologyRefresh();
      refreshPortClients(ifIndex);
      load();
    } catch (err) {
      setPortPromoteMsg(err instanceof Error ? err.message : String(err));
    } finally {
      setPortPromoteBusy(false);
    }
  }

  async function cyclePortIgnore(p: IfRow) {
    if (!id || !canWrite) return;
    const cur = portIgnoreMode(p);
    const next = nextPortIgnoreMode(cur);
    const portLabel = p.if_name?.trim() || String(p.if_index);
    try {
      await apiPut(`/api/v1/devices/${id}/interfaces/${p.if_index}/ignore`, { mode: next });
      const labels: Record<PortIgnoreMode, string> = {
        off: "мониторинг включён",
        soft: "тихий режим (без оповещений link/util, MAC остаётся)",
        all: "мониторинг выключен (нет записей в журнале и Telegram)",
      };
      setIgnoreMsg(`Порт ${portLabel}: ${labels[next]}`);
      load();
    } catch (err) {
      setErr(err instanceof Error ? err.message : "Ошибка настройки мониторинга порта");
    }
  }

  async function savePortDescr(p: IfRow) {
    if (!id || !canWrite) return;
    const next = (descrEdits[p.if_index] ?? "").trim();
    const cur = portDisplayDescr(p).trim();
    if (next === cur) return;
    const want = next;
    const portLabel = p.if_name?.trim() || String(p.if_index);
    try {
      const res = await apiPatch<{ ok: boolean; via?: string; descr?: string }>(
        `/api/v1/devices/${id}/interfaces/${p.if_index}/descr`,
        { descr: want },
      );
      const via = res.via ? ` (${res.via})` : "";
      setDescrMsg(
        want
          ? res.via === "local"
            ? `Порт ${portLabel}: описание сохранено в NetLynx (на ES XP запись на свитч недоступна)`
            : `Порт ${portLabel}: описание записано на свитч${via}`
          : res.via === "local"
            ? `Порт ${portLabel}: локальная подпись снята`
            : `Порт ${portLabel}: описание снято на свитче${via}`,
      );
      setDescrEdits((prev) => {
        const n = { ...prev };
        delete n[p.if_index];
        return n;
      });
      load();
    } catch (err) {
      setErr(err instanceof Error ? err.message : "Ошибка описания порта");
    }
  }

  async function setPortAdmin(p: IfRow, up: boolean, opts?: { skipConfirm?: boolean }) {
    if (!id || !canWrite) return;
    const portLabel = p.if_name?.trim() || String(p.if_index);
    const action = up ? "включить (no shutdown)" : "выключить (shutdown)";
    if (!opts?.skipConfirm && !window.confirm(`Порт ${portLabel}: ${action} на коммутаторе?`)) return;
    try {
      const res = await apiPatch<{ ok: boolean; via?: string }>(
        `/api/v1/devices/${id}/interfaces/${p.if_index}/admin`,
        { admin_up: up },
      );
      const via = res.via ? ` (${res.via})` : "";
      setDescrMsg(`Порт ${portLabel}: ${up ? "включён" : "выключен"}${via}`);
      load();
    } catch (err) {
      setErr(err instanceof Error ? err.message : "Ошибка admin status порта");
      throw err;
    }
  }

  const removeDevice = () => {
    if (!id || !canWrite) return;
    const name = data?.device.name ?? id;
    if (!window.confirm(`Удалить узел «${name}»? Данные по портам и событиям этого узла будут удалены из базы.`)) {
      return;
    }
    apiDelete(`/api/v1/devices/${id}`)
      .then(() => navigate("/devices"))
      .catch((e: Error) => setErr(e.message));
  };

  const saveName = () => {
    if (!id || !canWrite) return;
    setErr(null);
    setNameMsg(null);
    const trimmed = nameEdit.trim();
    if (!trimmed) {
      setErr("Имя устройства не может быть пустым.");
      return;
    }
    apiPatch<{ ok: boolean; name?: string }>(`/api/v1/devices/${id}/name`, { name: trimmed })
      .then(() => {
        setNameMsg("Имя сохранено.");
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const saveHost = () => {
    if (!id || !canWrite) return;
    setErr(null);
    setHostMsg(null);
    const trimmed = hostEdit.trim();
    apiPatch<{ ok: boolean; host?: string }>(`/api/v1/devices/${id}/host`, { host: trimmed })
      .then(() => {
        setHostMsg(trimmed ? "Адрес сохранён." : "Адрес очищен (узел без IP).");
        setHostEdit(trimmed);
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const saveChassisMac = () => {
    if (!id || !canWrite) return;
    setErr(null);
    setMacMsg(null);
    const trimmed = macEdit.trim();
    apiPatch<{ ok: boolean; chassis_mac?: string }>(`/api/v1/devices/${id}/chassis-mac`, {
      chassis_mac: trimmed,
    })
      .then((r) => {
        const saved = (r.chassis_mac ?? trimmed).trim();
        setMacMsg(saved ? "MAC сохранён." : "MAC очищен.");
        setMacEdit(saved);
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const clearChassisMac = () => {
    if (!id || !canWrite) return;
    setErr(null);
    setMacMsg(null);
    setMacEdit("");
    apiPatch<{ ok: boolean }>(`/api/v1/devices/${id}/chassis-mac`, { chassis_mac: "" })
      .then(() => {
        setMacMsg("MAC очищен.");
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const saveLocation = () => {
    if (!id) return;
    setErr(null);
    setLocMsg(null);
    const trimmed = locEdit.trim();
    apiPatch<{ ok: boolean }>(`/api/v1/devices/${id}/location`, { location: trimmed || "" })
      .then(() => {
        setLocMsg("Расположение сохранено.");
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const saveCategory = () => {
    if (!id) return;
    setErr(null);
    setCategoryMsg(null);
    apiPatch<{ ok: boolean; device_category?: string }>(`/api/v1/devices/${id}/category`, {
      device_category: categoryEdit,
    })
      .then(() => {
        setCategoryMsg("Тип устройства сохранён.");
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  /** Ширина как у SVG MetricChart (CPU). */
  const fieldWidthLikeCpu = 480;

  const savePollInterval = () => {
    if (!id) return;
    setErr(null);
    setPollIntervalMsg(null);
    const n = parseInt(pollIntervalEdit.trim(), 10);
    if (Number.isNaN(n) || n < 10 || n > 86400) {
      setErr("Интервал опроса: введите целое число секунд от 10 до 86400.");
      return;
    }
    apiPatch<{ ok: boolean }>(`/api/v1/devices/${id}/poll-interval`, { poll_interval_seconds: n })
      .then(() => {
        setPollIntervalMsg("Интервал опроса сохранён.");
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const saveSnmpSettings = () => {
    if (!id || !data?.device) return;
    setErr(null);
    setSnmpCfgMsg(null);

    const baseBody: Record<string, unknown> = {
      name: data.device.name,
      host: data.device.host,
      snmp_version: snmpVersionEdit,
      poll_interval_seconds: data.device.poll_interval_seconds,
    };

    if (snmpVersionEdit === "v1" || snmpVersionEdit === "v2c") {
      const c = communityEdit.trim();
      if (c) baseBody.community = c;
      // пустое community — сервер сохранит прежнее (has_community)
    } else {
      const user = v3UserEdit.trim();
      if (!user) {
        setErr("Для SNMP v3 укажите user.");
        return;
      }
      if (v3AuthPassEdit.trim().length < 8) {
        setErr("Для SNMP v3 auth password должен быть не короче 8 символов.");
        return;
      }
      if (v3PrivProtoEdit !== "NONE" && v3PrivPassEdit.trim().length < 8) {
        setErr("Для SNMP v3 priv password должен быть не короче 8 символов (или выберите NONE).");
        return;
      }
      baseBody.v3_user = user;
      baseBody.v3_auth_protocol = v3AuthProtoEdit;
      baseBody.v3_auth_pass = v3AuthPassEdit.trim();
      baseBody.v3_priv_protocol = v3PrivProtoEdit;
      if (v3PrivProtoEdit !== "NONE") baseBody.v3_priv_pass = v3PrivPassEdit.trim();
    }

    apiPatch<{ ok: boolean }>(`/api/v1/devices/${id}`, baseBody)
      .then(() => {
        setSnmpCfgMsg("Параметры SNMP сохранены.");
        setV3AuthPassEdit("");
        setV3PrivPassEdit("");
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const saveSSH = () => {
    if (!id || !canWrite) return;
    setErr(null);
    setSshMsg(null);
    const body: Record<string, unknown> = {
      ssh_user: sshUserEdit.trim(),
      ssh_vendor: sshVendorEdit,
    };
    const port = sshPortEdit.trim();
    if (port === "") {
      body.ssh_port = 0;
    } else {
      const n = Number(port);
      if (!Number.isFinite(n) || n < 1 || n > 65535) {
        setErr("SSH порт: 1–65535 или пусто");
        return;
      }
      body.ssh_port = n;
    }
    if (sshPassEdit.trim()) body.ssh_password = sshPassEdit;
    if (sshEnableEdit.trim()) body.ssh_enable_password = sshEnableEdit;
    apiPatch(`/api/v1/devices/${id}/ssh`, body)
      .then(() => {
        setSshMsg("SSH сохранён.");
        setSshPassEdit("");
        setSshEnableEdit("");
        load();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const runSnmpTest = () => {
    if (!id) return;
    apiPost<SnmpTestResult>(`/api/v1/devices/${id}/snmp-test`, {})
      .then((r) => {
        if (r.ok) setSnmpDlg(`SNMP OK\nsysName: ${r.sys_name ?? ""}\n\nsysDescr:\n${r.sys_descr ?? ""}`);
        else setSnmpDlg(`Ошибка: ${r.error ?? ""}`);
      })
      .catch((e: Error) => setErr(e.message));
  };

  const runTraceroute = () => {
    if (!id || !canWrite) return;
    setTraceBusy(true);
    setTraceResult(null);
    setErr(null);
    const body: { target?: string } = {};
    const t = traceTarget.trim();
    if (t) body.target = t;
    apiPost<TracerouteResult>(`/api/v1/devices/${id}/traceroute`, body)
      .then((r) => setTraceResult(r))
      .catch((e: Error) => setErr(e.message))
      .finally(() => setTraceBusy(false));
  };

  const runTCPProbe = () => {
    if (!id || !canWrite) return;
    const port = Number(tcpProbePort.trim());
    if (!Number.isFinite(port) || port < 1 || port > 65535) {
      setErr("TCP порт: 1–65535");
      return;
    }
    setTcpProbeBusy(true);
    setTcpProbeResult(null);
    setErr(null);
    const body: { target?: string; port: number } = { port };
    const t = tcpProbeTarget.trim();
    if (t) body.target = t;
    apiPost<TCPProbeResult>(`/api/v1/devices/${id}/tcp-probe`, body)
      .then((r) => setTcpProbeResult(r))
      .catch((e: Error) => setErr(e.message))
      .finally(() => setTcpProbeBusy(false));
  };

  if (!id) return <p>Неверный адрес.</p>;

  const showTabs =
    data != null && normalizeDeviceCategory(data.device.device_category) === "switch";
  const showPanel = (t: DeviceDetailTab) => !showTabs || detailTab === t;

  const shownInterfaces = data
    ? (showAllInterfaces ? data.interfaces : data.interfaces.filter(isPhysicalLikePort))
        .slice()
        .sort((a, b) => {
          const d = ifNaturalSortKey(a) - ifNaturalSortKey(b);
          if (d !== 0) return d;
          return a.if_index - b.if_index;
        })
    : [];

  return (
    <div className="device-detail-page">
      <div className="device-detail-sticky">
        <p className="device-detail-back">
          <Link to={back.path}>← {back.label}</Link>
        </p>
        {data ? (
          <div className="device-detail-sticky-name">
            <h1>{data.device.name}</h1>
            <Link to={`/topology?focus=${data.device.id}`} className="device-detail-topo-link">
              Показать на топологии
            </Link>
            {cliSyncBusy && (
              <p style={{ color: "#9aa3b5", fontSize: "0.9rem", margin: "0.35rem 0 0" }}>
                Идёт опрос конфига по SSH, подождите пожалуйста…
              </p>
            )}
          </div>
        ) : (
          <div className="device-detail-sticky-name">
            <p style={{ color: "#9aa3b5", margin: 0 }}>
              {cliSyncBusy
                ? "Идёт опрос конфига по SSH, подождите пожалуйста…"
                : "Загрузка карточки узла…"}
            </p>
          </div>
        )}
      </div>
      <div className="device-detail-body">
      {err && <p style={{ color: "#f88" }}>{err}</p>}
      {!data && !err && <p>Загрузка…</p>}
      {data && (
        <>
          {showTabs && (
            <div className="device-detail-tabs" role="tablist" aria-label="Разделы карточки коммутатора">
              {DEVICE_DETAIL_TABS.map((t) => (
                <button
                  key={t.id}
                  type="button"
                  role="tab"
                  aria-selected={detailTab === t.id}
                  className={[
                    "device-detail-tab",
                    detailTab === t.id ? "device-detail-tab--active" : "",
                  ]
                    .filter(Boolean)
                    .join(" ")}
                  onClick={() => setDetailTab(t.id)}
                >
                  {t.label}
                </button>
              ))}
            </div>
          )}

          {showPanel("info") && (
          <div role={showTabs ? "tabpanel" : undefined} aria-label={showTabs ? "Информация об узле" : undefined}>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>Имя</strong> (отображаемое в списке и на топологии; sysName от SNMP не меняется)
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", marginTop: "0.35rem" }}>
              <input
                value={nameEdit}
                readOnly={!canWrite}
                onChange={(e) => {
                  setNameEdit(e.target.value);
                  setNameMsg(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    saveName();
                  }
                }}
                placeholder="Имя устройства"
                style={{ width: fieldWidthLikeCpu, maxWidth: "100%", boxSizing: "border-box" }}
              />
              <button type="button" onClick={saveName} disabled={!canWrite}>
                Сохранить имя
              </button>
            </div>
            {nameMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{nameMsg}</span>}
          </div>
          <div style={{ marginBottom: "0.5rem" }}>
            <strong>MAC (chassis)</strong>
            {data.device.chassis_mac?.trim() ? (
              <p style={{ marginTop: "0.35rem", marginBottom: "0.35rem" }}>
                <code style={{ fontSize: "0.9rem" }}>{formatMacDisplay(data.device.chassis_mac)}</code>
              </p>
            ) : (
              <p style={{ marginTop: "0.35rem", marginBottom: "0.35rem", color: "#9aa3b5" }}>— не задан</p>
            )}
            {canWrite ? (
              <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", marginTop: "0.35rem" }}>
                <input
                  value={macEdit}
                  onChange={(e) => {
                    setMacEdit(e.target.value);
                    setMacMsg(null);
                  }}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") {
                      e.preventDefault();
                      saveChassisMac();
                    }
                  }}
                  placeholder="aa:bb:cc:dd:ee:ff"
                  style={{ width: fieldWidthLikeCpu, maxWidth: "100%", boxSizing: "border-box" }}
                />
                <button type="button" onClick={saveChassisMac} disabled={!canWrite}>
                  Сохранить MAC
                </button>
                <button type="button" onClick={clearChassisMac} disabled={!canWrite || (!data.device.chassis_mac?.trim() && !macEdit.trim())}>
                  Очистить
                </button>
              </div>
            ) : null}
            {macMsg ? <span style={{ color: "#8d8", fontSize: "0.9rem", display: "block", marginTop: "0.35rem" }}>{macMsg}</span> : null}
            {!data.device.chassis_mac?.trim() && (data.arp_observed_macs?.length ?? 0) > 0 ? (
              <div style={{ marginTop: "0.5rem" }}>
                <span style={{ fontSize: "0.9rem", color: "#9aa3b5" }}>
                  MAC из ARP по IP узла (шлюзы/L3, последний опрос):
                </span>
                <ul style={{ margin: "0.35rem 0 0", paddingLeft: "1.25rem" }}>
                  {data.arp_observed_macs!.map((mac) => (
                    <li key={mac} style={{ marginBottom: "0.2rem" }}>
                      <Link to={`/investigate/mac?mac=${encodeURIComponent(mac)}`} title="Расследовать MAC">
                        <code style={{ fontSize: "0.9rem" }}>{formatMacDisplay(mac)}</code>
                      </Link>
                    </li>
                  ))}
                </ul>
                <p style={{ margin: "0.35rem 0 0", fontSize: "0.85rem", color: "#9aa3b5" }}>
                  При bond несколько MAC — для inventory можно вручную задать один (bond / primary NIC).
                </p>
              </div>
            ) : null}
          </div>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>Адрес (IP / host)</strong> — можно сменить или очистить (склад / смена VLAN)
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", marginTop: "0.35rem" }}>
              <input
                value={hostEdit}
                readOnly={!canWrite}
                onChange={(e) => {
                  setHostEdit(e.target.value);
                  setHostMsg(null);
                }}
                onKeyDown={(e) => {
                  if (e.key === "Enter") {
                    e.preventDefault();
                    saveHost();
                  }
                }}
                placeholder="IP или hostname (пусто = без адреса)"
                style={{ width: fieldWidthLikeCpu, maxWidth: "100%", boxSizing: "border-box" }}
              />
              <button type="button" onClick={saveHost} disabled={!canWrite}>
                Сохранить адрес
              </button>
              <button
                type="button"
                onClick={() => {
                  setHostEdit("");
                  setHostMsg(null);
                  if (!id) return;
                  setErr(null);
                  apiPatch<{ ok: boolean }>(`/api/v1/devices/${id}/host`, { host: "" })
                    .then(() => {
                      setHostMsg("Адрес очищен (узел без IP).");
                      load();
                    })
                    .catch((e: Error) => setErr(e.message));
                }}
                disabled={!hostEdit.trim() && !(data.device.host ?? "").trim()}
                title="Убрать IP — узел остаётся в списке, опрос SNMP/ping не выполняется"
              >
                Очистить IP
              </button>
            </div>
            {hostMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{hostMsg}</span>}
            <div style={{ marginTop: "0.35rem", color: "#9aa3b5", fontSize: "0.9rem" }}>
              <strong>SNMP:</strong> {data.device.snmp_version}
              {data.device.uisp_device_id && (
                <>
                  {" "}
                  · <strong>UISP id:</strong> <code style={{ fontSize: "0.85rem" }}>{data.device.uisp_device_id}</code>
                  {" "}
                  · <strong>UISP NMS:</strong> {data.device.uisp_overview_status?.trim() || "—"}
                </>
              )}
            </div>
          </div>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>Расположение</strong> (как site в UISP или произвольная подпись)
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", marginTop: "0.35rem" }}>
              <LocationCombobox
                value={locEdit}
                options={existingLocations}
                onChange={setLocEdit}
                onSubmit={saveLocation}
                placeholder="начните вводить, например: Дирекция"
                width={fieldWidthLikeCpu}
              />
              <button type="button" onClick={saveLocation}>
                Сохранить расположение
              </button>
            </div>
            {locMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{locMsg}</span>}
          </div>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>Тип устройства</strong>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", marginTop: "0.35rem" }}>
              <DeviceCategoryIcon category={categoryEdit} height={28} />
              <select
                value={categoryEdit}
                onChange={(e) => setCategoryEdit(e.target.value)}
                style={{ width: fieldWidthLikeCpu, maxWidth: "100%", boxSizing: "border-box" }}
              >
                {categories.map((o) => (
                  <option key={o.id} value={o.id}>
                    {o.label}
                  </option>
                ))}
              </select>
              <button type="button" onClick={saveCategory}>
                Сохранить тип
              </button>
            </div>
            {categoryMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{categoryMsg}</span>}
          </div>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>Достижимость (онлайн / оффлайн)</strong>
            <div style={{ marginTop: "0.35rem", fontSize: "0.9rem" }}>
              Сейчас:{" "}
              <span style={{ color: deviceReachabilityLabel(data.device).color }}>
                {deviceReachabilityLabel(data.device).text}
              </span>
            </div>
            <p style={{ margin: "0.35rem 0 0", color: "#9aa3b5", fontSize: "0.85rem" }}>
              Для ПК с жёстким файрволом (нет ping и SNMP) отметьте «Онлайн вручную» — дашборд, список Узлы и
              топология будут считать узел онлайн. «Авто» снова смотрит на ping и SNMP.
            </p>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", marginTop: "0.35rem" }}>
              <select
                value={onlineOverrideEdit}
                onChange={(e) => setOnlineOverrideEdit(e.target.value as OnlineOverrideMode)}
                style={{ width: fieldWidthLikeCpu, maxWidth: "100%", boxSizing: "border-box" }}
              >
                <option value="auto">Авто (ping / SNMP)</option>
                <option value="online">Онлайн вручную</option>
                <option value="offline">Оффлайн вручную</option>
              </select>
              <button
                type="button"
                onClick={() => {
                  if (!id) return;
                  setErr(null);
                  setOnlineOverrideMsg(null);
                  apiPatch<{ ok: boolean; mode: string }>(`/api/v1/devices/${id}/online-override`, {
                    mode: onlineOverrideEdit,
                  })
                    .then(() => {
                      setOnlineOverrideMsg(
                        onlineOverrideEdit === "auto"
                          ? "Снова автоопределение."
                          : onlineOverrideEdit === "online"
                            ? "Отмечен как онлайн вручную."
                            : "Отмечен как оффлайн вручную.",
                      );
                      load();
                    })
                    .catch((e: Error) => setErr(e.message));
                }}
              >
                Сохранить
              </button>
            </div>
            {onlineOverrideMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{onlineOverrideMsg}</span>}
          </div>
          </div>
          )}

          {showPanel("snmp") && (
          <div role={showTabs ? "tabpanel" : undefined} aria-label={showTabs ? "SNMP/SSH" : undefined}>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>SNMP trap → мгновенные link-события</strong>
            <p style={{ margin: "0.35rem 0 0", color: "#9aa3b5", fontSize: "0.85rem" }}>
              Имеет смысл, если в Настройки → Уведомления → Принимать traps режим «Только с флагом на
              устройстве». Тогда <code>linkUp</code>/<code>linkDown</code> сразу попадут в «События» без
              ожидания опроса.
            </p>
            <label
              style={{
                display: "inline-flex",
                alignItems: "center",
                gap: "0.5rem",
                marginTop: "0.35rem",
                cursor: "pointer",
              }}
            >
              <input
                type="checkbox"
                checked={!!data.device.trust_link_traps}
                onChange={(e) => {
                  if (!id) return;
                  setErr(null);
                  setTrustLinkMsg(null);
                  const trust = e.target.checked;
                  apiPatch<{ ok: boolean; trust_link_traps: boolean }>(
                    `/api/v1/devices/${id}/trust-link-traps`,
                    { trust_link_traps: trust },
                  )
                    .then(() => {
                      setTrustLinkMsg(trust ? "Флаг включён." : "Флаг выключен.");
                      load();
                    })
                    .catch((err: Error) => setErr(err.message));
                }}
              />
              Доверять SNMP trap для link-событий
            </label>
            {trustLinkMsg && <span style={{ color: "#8d8", fontSize: "0.9rem", marginLeft: 8 }}>{trustLinkMsg}</span>}
          </div>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>Интервал SNMP-опроса этого узла</strong> — как часто сервер опрашивает коммутатор (секунды,{" "}
            <strong>10–86400</strong>). Планировщик проверяет очередь чаще (переменная{" "}
            <code style={{ fontSize: "0.85rem" }}>POLL_SCHEDULER_SECONDS</code> в окружении); здесь задаётся именно
            пауза между полными опросами <em>данного</em> узла.
            <form
              onSubmit={(e) => {
                e.preventDefault();
                savePollInterval();
              }}
              style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", marginTop: "0.35rem" }}
            >
              <label style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                Опрос каждые
                <input
                  type="number"
                  min={10}
                  max={86400}
                  step={1}
                  value={pollIntervalEdit}
                  onChange={(e) => setPollIntervalEdit(e.target.value)}
                  style={{ width: 88 }}
                />
                с
              </label>
              <button type="submit">Сохранить интервал</button>
            </form>
            {pollIntervalMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{pollIntervalMsg}</span>}
          </div>
          <form onSubmit={saveMonitoring} style={{ marginBottom: "0.75rem", display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "flex-end" }}>
            <strong style={{ width: "100%" }}>Мониторинг узла</strong>
            <label>
              Util high %
              <br />
              <input value={devUtilHigh} onChange={(e) => setDevUtilHigh(e.target.value)} placeholder="90" style={{ width: 64 }} />
            </label>
            <label>
              Util OK %
              <br />
              <input value={devUtilOk} onChange={(e) => setDevUtilOk(e.target.value)} placeholder="85" style={{ width: 64 }} />
            </label>
            <label>
              FDB poll (с)
              <br />
              <input value={devFdbPoll} onChange={(e) => setDevFdbPoll(e.target.value)} placeholder="900" style={{ width: 88 }} />
            </label>
            <button type="submit">Сохранить</button>
            {monMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{monMsg}</span>}
          </form>
          <div style={{ marginBottom: "0.75rem" }}>
            <strong>Параметры SNMP этого узла</strong>
            <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "flex-end", marginTop: "0.35rem" }}>
              <label>
                Версия
                <br />
                <select value={snmpVersionEdit} onChange={(e) => setSnmpVersionEdit(e.target.value as SNMPVersion)}>
                  <option value="v1">v1</option>
                  <option value="v2c">v2c</option>
                  <option value="v3">v3</option>
                </select>
              </label>
              {(snmpVersionEdit === "v1" || snmpVersionEdit === "v2c") && (
                <label>
                  Community{data.device.has_community ? " (оставьте пустым, чтобы не менять)" : ""}
                  <br />
                  <input
                    value={communityEdit}
                    onChange={(e) => setCommunityEdit(e.target.value)}
                    placeholder={data.device.has_community ? "••••••••" : "public"}
                    autoComplete="off"
                  />
                </label>
              )}
              {snmpVersionEdit === "v3" && (
                <>
                  <label>
                    User
                    <br />
                    <input value={v3UserEdit} onChange={(e) => setV3UserEdit(e.target.value)} />
                  </label>
                  <label>
                    Auth protocol
                    <br />
                    <select
                      value={v3AuthProtoEdit}
                      onChange={(e) => setV3AuthProtoEdit(e.target.value as "SHA" | "MD5" | "SHA256" | "SHA512")}
                    >
                      <option value="SHA">SHA</option>
                      <option value="SHA256">SHA256</option>
                      <option value="SHA512">SHA512</option>
                      <option value="MD5">MD5</option>
                    </select>
                  </label>
                  <label>
                    Auth password
                    <br />
                    <input
                      type="password"
                      value={v3AuthPassEdit}
                      onChange={(e) => setV3AuthPassEdit(e.target.value)}
                      minLength={8}
                      placeholder="Введите заново"
                    />
                  </label>
                  <label>
                    Privacy protocol
                    <br />
                    <select
                      value={v3PrivProtoEdit}
                      onChange={(e) => setV3PrivProtoEdit(e.target.value as "AES" | "AES256" | "DES" | "NONE")}
                    >
                      <option value="AES">AES</option>
                      <option value="AES256">AES256</option>
                      <option value="DES">DES</option>
                      <option value="NONE">NONE</option>
                    </select>
                  </label>
                  {v3PrivProtoEdit !== "NONE" && (
                    <label>
                      Priv password
                      <br />
                      <input
                        type="password"
                        value={v3PrivPassEdit}
                        onChange={(e) => setV3PrivPassEdit(e.target.value)}
                        minLength={8}
                        placeholder="Введите заново"
                      />
                    </label>
                  )}
                </>
              )}
              <button type="button" onClick={saveSnmpSettings}>
                Сохранить SNMP
              </button>
            </div>
            {snmpCfgMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{snmpCfgMsg}</span>}
          </div>
          {(normalizeDeviceCategory(data.device.device_category) === "switch" ||
            normalizeDeviceCategory(data.device.device_category) === "router" ||
            normalizeDeviceCategory(data.device.device_category) === "ap") && (
            <div style={{ marginBottom: "0.75rem" }}>
              <strong>SSH для бэкапа конфига</strong>
              <p style={{ margin: "0.35rem 0", color: "#9aa3b5", fontSize: "0.85rem" }}>
                Если поля пустые, берётся общий логин из Настройки → Резервные копии. При первом подключении
                неизвестный ключ хоста принимается автоматически.
              </p>
              <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "flex-end", marginTop: "0.35rem" }}>
                <label>
                  Пользователь
                  <br />
                  <input
                    value={sshUserEdit}
                    readOnly={!canWrite}
                    onChange={(e) => setSshUserEdit(e.target.value)}
                    placeholder="ubnt / admin"
                  />
                </label>
                <label>
                  Пароль{hasSshPass ? " (оставьте пустым, чтобы не менять)" : ""}
                  <br />
                  <input
                    type="password"
                    value={sshPassEdit}
                    readOnly={!canWrite}
                    onChange={(e) => setSshPassEdit(e.target.value)}
                    placeholder={hasSshPass ? "••••••••" : ""}
                    autoComplete="new-password"
                  />
                </label>
                <label>
                  Порт
                  <br />
                  <input
                    value={sshPortEdit}
                    readOnly={!canWrite}
                    onChange={(e) => setSshPortEdit(e.target.value)}
                    placeholder="22"
                    style={{ width: 72 }}
                  />
                </label>
                <label>
                  Enable{hasSshEnable ? " (пусто = не менять)" : ""}
                  <br />
                  <input
                    type="password"
                    value={sshEnableEdit}
                    readOnly={!canWrite}
                    onChange={(e) => setSshEnableEdit(e.target.value)}
                    placeholder={hasSshEnable ? "••••••••" : "как login, если пусто"}
                    autoComplete="new-password"
                  />
                </label>
                <label>
                  Вендор
                  <br />
                  <select
                    value={sshVendorEdit}
                    disabled={!canWrite}
                    onChange={(e) => setSshVendorEdit(e.target.value)}
                  >
                    <option value="auto">Авто</option>
                    <option value="ubiquiti">Ubiquiti</option>
                    <option value="eltex">Eltex</option>
                    <option value="snr">SNR</option>
                    <option value="mikrotik">MikroTik</option>
                    <option value="cisco">Cisco</option>
                    <option value="aruba">Aruba</option>
                    <option value="zyxel">Zyxel</option>
                    <option value="huawei">Huawei</option>
                    <option value="hp">HP ProCurve</option>
                    <option value="tplink">TP-Link</option>
                    <option value="dlink">D-Link</option>
                    <option value="dahua">Dahua (switch)</option>
                    <option value="hikvision">Hikvision (switch)</option>
                    <option value="hiwatch">HiWatch (switch)</option>
                    <option value="trassir">Trassir (switch)</option>
                  </select>
                </label>
                <button type="button" onClick={saveSSH} disabled={!canWrite}>
                  Сохранить SSH
                </button>
              </div>
              {sshMsg && <span style={{ color: "#8d8", fontSize: "0.9rem" }}>{sshMsg}</span>}
              <DeviceConfigHistory deviceId={Number(id)} canWrite={canWrite} />
            </div>
          )}
          </div>
          )}

          {showPanel("state") && (
          <div role={showTabs ? "tabpanel" : undefined} aria-label={showTabs ? "Состояние" : undefined}>
          <p>
            <strong>SNMP-состояние:</strong>{" "}
            {data.device.last_snmp_ok === true ? (
              <span style={{ color: "#6d6" }}>ответ есть</span>
            ) : data.device.last_snmp_ok === false ? (
              <span style={{ color: "#f88" }}>ошибка: {data.device.last_snmp_error ?? "—"}</span>
            ) : (
              <span>ещё не опрошен</span>
            )}
            {data.device.sys_name && (
              <>
                {" "}
                · <strong>sysName:</strong> {data.device.sys_name}
              </>
            )}
            {" "}
            · <strong>CPU:</strong>{" "}
            {data.device.last_cpu_pct == null
              ? "N/A"
              : `${data.device.last_cpu_pct.toFixed(1)}%${data.device.cpu_profile ? ` (${data.device.cpu_profile})` : ""}`}
            {data.device.last_sys_uptime_cs != null && data.device.last_poll_at && (
              <>
                {" "}
                · <strong>Uptime:</strong>{" "}
                {formatSysUptime(data.device.last_sys_uptime_cs, data.device.last_poll_at)}
              </>
            )}
            {" "}
            · <strong>MAC/FDB:</strong>{" "}
            {data.device.fdb_monitoring_status === "ok"
              ? "активен"
              : data.device.fdb_monitoring_status === "learning"
                ? "обучение"
                : data.device.fdb_monitoring_status === "unavailable"
                  ? "недоступен"
                  : "—"}
          </p>
          <MetricChart title="CPU (24 ч)" samples={cpuSamples} />
          {utilSamples.length > 0 && <MetricChart title="Макс. утилизация порта ifIndex=1 (24 ч)" samples={utilSamples} />}
          <p style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center" }}>
            <button type="button" onClick={runSnmpTest} disabled={!canWrite}>
              Проверить SNMP сейчас
            </button>
            <button type="button" onClick={runTraceroute} disabled={!canWrite || traceBusy}>
              {traceBusy ? "Traceroute…" : "Traceroute"}
            </button>
            <input
              type="text"
              value={traceTarget}
              onChange={(e) => setTraceTarget(e.target.value)}
              placeholder={data.device.host ? `цель (по умолчанию ${data.device.host})` : "IP или hostname"}
              disabled={!canWrite || traceBusy}
              style={{ minWidth: 220, padding: "4px 8px", background: "#12161f", border: "1px solid #2e3648", borderRadius: 6, color: "#c8d0e0" }}
            />
            <button type="button" onClick={runTCPProbe} disabled={!canWrite || tcpProbeBusy}>
              {tcpProbeBusy ? "TCP…" : "TCP probe"}
            </button>
            <input
              type="number"
              min={1}
              max={65535}
              value={tcpProbePort}
              onChange={(e) => setTcpProbePort(e.target.value)}
              title="TCP порт"
              disabled={!canWrite || tcpProbeBusy}
              style={{ width: 72, padding: "4px 8px", background: "#12161f", border: "1px solid #2e3648", borderRadius: 6, color: "#c8d0e0" }}
            />
            <input
              type="text"
              value={tcpProbeTarget}
              onChange={(e) => setTcpProbeTarget(e.target.value)}
              placeholder="TCP host (по умолчанию IP узла)"
              disabled={!canWrite || tcpProbeBusy}
              style={{ minWidth: 180, padding: "4px 8px", background: "#12161f", border: "1px solid #2e3648", borderRadius: 6, color: "#c8d0e0" }}
            />
            <button type="button" onClick={removeDevice} disabled={!canWrite}>
              Удалить узел
            </button>
          </p>
          {traceResult && (
            <div style={{ marginBottom: "1rem" }}>
              <p style={{ fontSize: "0.9rem", color: "#9aa3b5" }}>
                Traceroute → <strong>{traceResult.target}</strong>
                {traceResult.via ? ` (${traceResult.via})` : ""}
                {!traceResult.ok && traceResult.error ? (
                  <span style={{ color: "#f88" }}> — {traceResult.error}</span>
                ) : null}
              </p>
              {traceResult.hops && traceResult.hops.length > 0 && (
                <table style={{ width: "100%", maxWidth: 520, borderCollapse: "collapse", fontSize: "0.88rem" }}>
                  <thead>
                    <tr style={{ borderBottom: "1px solid #2e3648", color: "#9aa3b5", textAlign: "left" }}>
                      <th style={{ padding: "4px 8px" }}>#</th>
                      <th style={{ padding: "4px 8px" }}>Адрес</th>
                      <th style={{ padding: "4px 8px" }}>RTT</th>
                    </tr>
                  </thead>
                  <tbody>
                    {traceResult.hops.map((h) => (
                      <tr key={h.hop} style={{ borderBottom: "1px solid #1e2430" }}>
                        <td style={{ padding: "4px 8px" }}>{h.hop}</td>
                        <td style={{ padding: "4px 8px" }}>{h.timeout ? "—" : h.address ?? h.hostname ?? "—"}</td>
                        <td style={{ padding: "4px 8px" }}>{h.rtt_ms?.length ? h.rtt_ms.map((x) => `${x} ms`).join(", ") : h.timeout ? "timeout" : "—"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
              <button type="button" onClick={() => setTraceResult(null)} style={{ marginTop: 8 }}>
                Закрыть
              </button>
            </div>
          )}
          {tcpProbeResult && (
            <div style={{ marginBottom: "1rem" }}>
              <p style={{ fontSize: "0.9rem", color: "#9aa3b5" }}>
                TCP {tcpProbeResult.target}:{tcpProbeResult.port} —{" "}
                <strong style={{ color: tcpProbeResult.open ? "#6d6" : "#f88" }}>
                  {tcpProbeResult.open ? "open" : "closed/filtered"}
                </strong>
                {tcpProbeResult.rtt_ms != null ? ` · ${tcpProbeResult.rtt_ms} ms` : ""}
                {tcpProbeResult.error ? <span style={{ color: "#f88" }}> — {tcpProbeResult.error}</span> : null}
              </p>
              {tcpProbeResult.banner ? (
                <pre
                  style={{
                    margin: "0.35rem 0 0",
                    fontSize: "0.82rem",
                    color: "#9aa3b5",
                    background: "#1a1f2a",
                    padding: "0.5rem 0.65rem",
                    borderRadius: 6,
                    whiteSpace: "pre-wrap",
                  }}
                >
                  {tcpProbeResult.banner}
                </pre>
              ) : null}
              <button type="button" onClick={() => setTcpProbeResult(null)} style={{ marginTop: 8 }}>
                Закрыть
              </button>
            </div>
          )}
          {snmpDlg && (
            <div style={{ marginBottom: "1rem" }}>
              <pre
                style={{
                  background: "#1a1f2a",
                  padding: "0.75rem",
                  borderRadius: 8,
                  whiteSpace: "pre-wrap",
                  maxHeight: 220,
                  overflow: "auto",
                }}
              >
                {snmpDlg}
              </pre>
              <button type="button" onClick={() => setSnmpDlg(null)}>
                Закрыть
              </button>
            </div>
          )}
          {data.device.sys_descr && (
            <details style={{ marginBottom: "1rem" }}>
              <summary>sysDescr (полное)</summary>
              <pre style={{ fontSize: "0.8rem", maxHeight: 200, overflow: "auto" }}>{data.device.sys_descr}</pre>
            </details>
          )}
          </div>
          )}

          {showTabs && showPanel("vlan") && (
          <div role="tabpanel" aria-label="VLAN">
          <DeviceVlanTab
            deviceId={Number(id)}
            canWrite={canWrite}
            settingsWritable={devicePortSettingsWritable(data.device)}
            ports={data.interfaces}
            reloadToken={cliSyncMsg}
            onApplied={() => load()}
          />
          </div>
          )}

          {showPanel("ports") && (
          <div role={showTabs ? "tabpanel" : undefined} aria-label={showTabs ? "Порты" : undefined}>
          <h2>Ручные связи</h2>
          <p style={{ marginTop: 0, color: "#9aa3b5", fontSize: "0.9rem" }}>
            Если LLDP/CDP врёт или молчит (VLAN на одном SFP и т.п.), задайте порт↔порт вручную. Пока связь{" "}
            <code>active</code>, она закреплена: автообнаружение на этих портах не затирает и не прячет ручной линк.
          </p>
          {mlMsg && <p style={{ color: "#9dd" }}>{mlMsg}</p>}
          {manualLinks.length === 0 ? (
            <p style={{ color: "#9aa3b5" }}>Ручных связей для этого узла нет.</p>
          ) : (
            <table style={{ width: "100%", marginBottom: "0.75rem" }}>
              <thead>
                <tr>
                  <th>Локальный порт</th>
                  <th>Удалённый узел / порт</th>
                  <th>Статус</th>
                  <th>Заметка</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {manualLinks.map((ml) => {
                  const localIsA = ml.a_device_id === Number(id);
                  const localIf = localIsA ? ml.a_if_index : ml.b_if_index;
                  const peerId = localIsA ? ml.b_device_id : ml.a_device_id;
                  const peerIf = localIsA ? ml.b_if_index : ml.a_if_index;
                  const peerName = localIsA ? ml.b_device_name : ml.a_device_name;
                  return (
                    <tr key={ml.id}>
                      <td>if{localIf}</td>
                      <td>
                        <Link to={`/devices/${peerId}`}>{peerName || `#${peerId}`}</Link> / if{peerIf}
                      </td>
                      <td>
                        {ml.status}
                        {ml.superseded_by ? ` (${ml.superseded_by})` : ""}
                      </td>
                      <td style={{ fontSize: "0.85rem" }}>{ml.note || "—"}</td>
                      <td style={{ whiteSpace: "nowrap" }}>
                        <button
                          type="button"
                          disabled={mlBusy}
                          onClick={() => {
                            const next = window.prompt(`Заметка для связи #${ml.id}`, ml.note ?? "");
                            if (next === null) return;
                            setMlBusy(true);
                            apiPatch(`/api/v1/manual-links/${ml.id}`, { note: next })
                              .then(() => {
                                setMlMsg(`Заметка #${ml.id} обновлена`);
                                load();
                              })
                              .catch((e: Error) => setMlMsg(e.message))
                              .finally(() => setMlBusy(false));
                          }}
                        >
                          Изменить
                        </button>{" "}
                        {ml.status === "superseded" && (
                          <button
                            type="button"
                            disabled={mlBusy}
                            onClick={() => {
                              setMlBusy(true);
                              apiPatch(`/api/v1/manual-links/${ml.id}`, { status: "active" })
                                .then(() => {
                                  setMlMsg(`Связь #${ml.id} восстановлена`);
                                  requestTopologyRefresh();
                                  load();
                                })
                                .catch((e: Error) => setMlMsg(e.message))
                                .finally(() => setMlBusy(false));
                            }}
                          >
                            Восстановить
                          </button>
                        )}{" "}
                        <button
                          type="button"
                          disabled={mlBusy}
                          onClick={() => {
                            if (!window.confirm(`Удалить ручную связь #${ml.id}?`)) return;
                            setMlBusy(true);
                            apiDelete(`/api/v1/manual-links/${ml.id}`)
                              .then(() => {
                                setMlMsg(`Связь #${ml.id} удалена`);
                                requestTopologyRefresh();
                                load();
                              })
                              .catch((e: Error) => setMlMsg(e.message))
                              .finally(() => setMlBusy(false));
                          }}
                        >
                          Удалить
                        </button>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
          <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "end", marginBottom: "1.25rem" }}>
            <label>
              Локальный if
              <br />
              <select value={mlLocalIf} onChange={(e) => setMlLocalIf(e.target.value)}>
                <option value="">—</option>
                {(data.interfaces || []).map((p) => (
                  <option key={p.if_index} value={p.if_index}>
                    {p.if_index}
                    {p.if_name ? ` · ${p.if_name}` : ""}
                  </option>
                ))}
              </select>
            </label>
            <label>
              Удалённый узел
              <br />
              <DeviceSearchSelect
                devices={allDevices}
                value={mlPeerId}
                excludeId={Number(id) || undefined}
                disabled={mlBusy}
                ariaLabel="Удалённый узел"
                onChange={(next) => {
                  setMlPeerId(next);
                  setMlPeerIf("");
                }}
              />
            </label>
            <label>
              Удалённый if
              <br />
              <select value={mlPeerIf} onChange={(e) => setMlPeerIf(e.target.value)}>
                <option value="">—</option>
                {mlPeerIfaces.map((p) => (
                  <option key={p.if_index} value={p.if_index}>
                    {p.if_index}
                    {p.if_name ? ` · ${p.if_name}` : ""}
                  </option>
                ))}
              </select>
            </label>
            <button
              type="button"
              disabled={mlBusy || !mlLocalIf || !mlPeerId || !mlPeerIf}
              onClick={() => {
                setMlBusy(true);
                setMlMsg(null);
                apiPost("/api/v1/manual-links", {
                  a_device_id: Number(id),
                  a_if_index: Number(mlLocalIf),
                  b_device_id: Number(mlPeerId),
                  b_if_index: Number(mlPeerIf),
                })
                  .then(() => {
                    setMlMsg("Ручная связь создана");
                    requestTopologyRefresh();
                    setMlLocalIf("");
                    setMlPeerId("");
                    setMlPeerIf("");
                    load();
                  })
                  .catch((e: Error) => setMlMsg(e.message))
                  .finally(() => setMlBusy(false));
              }}
            >
              Добавить связь
            </button>
          </div>

          <h2 id="device-ports">Порты</h2>
          <p style={{ marginTop: 0, marginBottom: "0.5rem", display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center" }}>
            {cliSyncMsg ? (
              <span style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>{cliSyncMsg}</span>
            ) : null}
            {!isMikrotikRouter(data.device) ? (
              <button
                type="button"
                disabled={cliSyncBusy || !canWrite}
                onClick={() => runCliPortSync(true)}
                title="Принудительно перечитать show running-config по SSH"
              >
                {cliSyncBusy ? "Читаем конфиг…" : "Перечитать конфиг (SSH)"}
              </button>
            ) : (
              <span style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>
                RouterOS: конфиг снимается при резервном копировании; порты только для просмотра.
              </span>
            )}
          </p>
          <p style={{ marginTop: 0, marginBottom: "0.5rem", fontSize: "0.85rem", color: "#9aa3b5" }}>
            Колонка «Монит.»: цикл по клику — <strong>Монит.</strong> (обычный) → жёлтый <strong>Тихий</strong>{" "}
            (без Telegram по link/util, MAC остаётся) → красный <strong>Выкл</strong> (порт не пишет в журнал и не
            шлёт оповещения — для шумных портов вроде Wi‑Fi AP без клиентов).
          </p>
          <p style={{ marginTop: 0, marginBottom: "0.5rem" }}>
            <label style={{ display: "inline-flex", alignItems: "center", gap: 8, cursor: "pointer" }}>
              <input
                type="checkbox"
                checked={showAllInterfaces}
                onChange={(e) => setShowAllInterfaces(e.target.checked)}
              />
              Показывать все интерфейсы (включая loopback/VLAN)
            </label>
          </p>
          <p style={{ marginTop: 0, marginBottom: "0.75rem", fontSize: "0.85rem", color: "#9aa3b5" }}>
            Клик по порту в сетке или ▶ в таблице — список MAC/IP на порту (данные FDB/ARP последнего опроса).
            Колонка «Комментарий»: {canWrite
              ? "Enter/уход с поля — запись на свитч (SNMP ifAlias, иначе SSH). Пустое поле — снять description на устройстве."
              : "подпись порта (только просмотр)."}
          </p>
          {data.interfaces.length === 0 && <p>Интерфейсы появятся после первого успешного опроса.</p>}
          {data.interfaces.length > 0 && shownInterfaces.length === 0 && (
            <p>По текущему фильтру физические порты не найдены. Включите показ всех интерфейсов.</p>
          )}
          {shownInterfaces.length > 0 && (
            <>
              <PortOverviewGrid
                interfaces={shownInterfaces}
                sysDescr={[data.device.sys_descr, data.device.sys_name].filter(Boolean).join("\n") || null}
                onPortHoverChange={setHoveredPortIfIndex}
                onPortClick={togglePortExpand}
                onPortEditSettings={(ifIndex) => {
                  const p = shownInterfaces.find((x) => x.if_index === ifIndex);
                  if (!p) return;
                  setPortSettings({
                    if_index: p.if_index,
                    if_name: p.if_name,
                    admin_status: p.admin_status,
                    label: portDisplayDescr(p) || undefined,
                    cli_access_vlan: p.cli_access_vlan,
                    cli_port_mode: p.cli_port_mode,
                  });
                }}
                expandedIfIndex={expandedIfIndex}
              />
              <PortSettingsModal
                open={portSettings != null}
                port={portSettings}
                canWrite={canWrite}
                settingsWritable={devicePortSettingsWritable(data.device)}
                poe24vSupported={devicePoE24VSupported(data.device)}
                loadLiveSettings={async () => {
                  if (!portSettings) throw new Error("Порт не выбран");
                  return apiGet<PortLiveSettings>(
                    `/api/v1/devices/${id}/interfaces/${portSettings.if_index}/settings`,
                  );
                }}
                loadKnownVlans={async () => {
                  const r = await apiGet<{ vlans?: { vlan_id: number; in_database?: boolean }[] }>(
                    `/api/v1/devices/${id}/vlans`,
                  );
                  return (r.vlans ?? [])
                    .filter((v) => v.in_database && v.vlan_id >= 1 && v.vlan_id <= 4094)
                    .map((v) => v.vlan_id);
                }}
                onClose={() => setPortSettings(null)}
                onSave={async ({
                  adminUp,
                  poeMode,
                  isolate,
                  dhcpTrusted,
                  flowControl,
                  stpEnabled,
                  edgePort,
                  portPriority,
                  pathCost,
                  enableDirty,
                  poeDirty,
                  isolateDirty,
                  dhcpTrustedDirty,
                  flowControlDirty,
                  stpDirty,
                  vlanDirty,
                  accessVlan,
                }) => {
                  const p = shownInterfaces.find((x) => x.if_index === portSettings?.if_index);
                  if (!p) throw new Error("Порт не найден");
                  const portLabel = p.if_name?.trim() || String(p.if_index);
                  const notes: string[] = [];
                  if (enableDirty) {
                    await setPortAdmin(p, adminUp, { skipConfirm: true });
                  }
                  if (isolateDirty) {
                    const res = await apiPatch<{ ok: boolean; via?: string }>(
                      `/api/v1/devices/${id}/interfaces/${p.if_index}/isolate`,
                      { isolate },
                    );
                    notes.push(`Isolate → ${isolate ? "on" : "off"}${res.via ? ` (${res.via})` : ""}`);
                  }
                  if (dhcpTrustedDirty) {
                    const res = await apiPatch<{ ok: boolean; via?: string }>(
                      `/api/v1/devices/${id}/interfaces/${p.if_index}/dhcp-snooping`,
                      { trusted: dhcpTrusted },
                    );
                    notes.push(`DHCP Trusted → ${dhcpTrusted ? "on" : "off"}${res.via ? ` (${res.via})` : ""}`);
                  }
                  if (flowControlDirty) {
                    const res = await apiPatch<{ ok: boolean; via?: string }>(
                      `/api/v1/devices/${id}/interfaces/${p.if_index}/flow-control`,
                      { flow_control: flowControl },
                    );
                    notes.push(`Flow Control → ${flowControl ? "on" : "off"}${res.via ? ` (${res.via})` : ""}`);
                  }
                  if (stpDirty) {
                    const res = await apiPatch<{ ok: boolean; via?: string }>(
                      `/api/v1/devices/${id}/interfaces/${p.if_index}/stp`,
                      {
                        enabled: stpEnabled,
                        edge_port: edgePort,
                        port_priority: portPriority,
                        path_cost: pathCost,
                      },
                    );
                    notes.push(`STP → ${stpEnabled ? "on" : "off"}${res.via ? ` (${res.via})` : ""}`);
                  }
                  if (vlanDirty) {
                    const res = await apiPatch<{ ok: boolean; via?: string; vlan_id?: number }>(
                      `/api/v1/devices/${id}/interfaces/${p.if_index}/vlan`,
                      { op: "set_access", vlan_id: accessVlan },
                    );
                    notes.push(`VLAN access → ${res.vlan_id ?? accessVlan}${res.via ? ` (${res.via})` : ""}`);
                  }
                  if (poeDirty) {
                    const res = await apiPatch<{ ok: boolean; via?: string; poe_mode?: string }>(
                      `/api/v1/devices/${id}/interfaces/${p.if_index}/poe`,
                      { poe_mode: poeMode },
                    );
                    notes.push(`PoE Mode → ${res.poe_mode ?? poeMode}${res.via ? ` (${res.via})` : ""}`);
                  }
                  if (notes.length) {
                    setDescrMsg(`Порт ${portLabel}: ${notes.join("; ")}`);
                    load();
                  }
                }}
              />
              {(ignoreMsg || descrMsg) && (
                <p
                  ref={portStatusMsgRef}
                  style={{
                    margin: "0.75rem 0 0.5rem",
                    padding: "0.45rem 0.65rem",
                    borderRadius: 6,
                    background: "rgba(80, 160, 140, 0.18)",
                    border: "1px solid rgba(120, 200, 180, 0.35)",
                    color: "#b8f0e0",
                    fontSize: "0.9rem",
                  }}
                  role="status"
                >
                  {descrMsg ?? ignoreMsg}
                </p>
              )}
              <div className="ports-table-scroll">
                <table style={{ tableLayout: "fixed", width: "100%" }}>
                  {portsColgroup}
                  <thead>
                    <tr>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        if
                        <PortsResizeHandle colIndex={0} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        Порт №
                        <PortsResizeHandle colIndex={1} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }} title="Подпись порта (ifAlias / description)">
                        Комментарий
                        <PortsResizeHandle colIndex={2} />
                      </th>
                      <th
                        style={{ position: "relative", userSelect: "none" }}
                        title="Команда порта: no shutdown / shutdown (ifAdminStatus)"
                      >
                        Admin
                        <PortsResizeHandle colIndex={3} />
                      </th>
                      <th
                        style={{ position: "relative", userSelect: "none" }}
                        title="Кабель / сигнал на порту (ifOperStatus): воткнут — up, нет линка — down"
                      >
                        Линк
                        <PortsResizeHandle colIndex={4} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        VLAN
                        <PortsResizeHandle colIndex={5} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none", textAlign: "center" }}>
                        PoE
                        <PortsResizeHandle colIndex={6} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        Роль
                        <PortsResizeHandle colIndex={7} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        Скорость
                        <PortsResizeHandle colIndex={8} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        Утилизация in/out/max %
                        <PortsResizeHandle colIndex={9} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        Rx
                        <PortsResizeHandle colIndex={10} />
                      </th>
                      <th style={{ position: "relative", userSelect: "none" }}>
                        Tx
                        <PortsResizeHandle colIndex={11} />
                      </th>
                      <th>Сосед LLDP</th>
                      <th title="Мониторинг порта: Монит. → Тихий → Выкл">Монит.</th>
                    </tr>
                  </thead>
                  <tbody>
                    {shownInterfaces.map((p) => {
                      const igMode = portIgnoreMode(p);
                      const expanded = expandedIfIndex === p.if_index;
                      const rowHighlight =
                        expanded
                          ? { background: "rgba(88, 164, 255, 0.12)" }
                          : hoveredPortIfIndex === p.if_index
                            ? {
                                background: "rgba(88, 164, 255, 0.18)",
                                boxShadow: "inset 0 0 0 1px rgba(120, 188, 255, 0.55)",
                              }
                            : portIgnoreRowStyle(igMode);
                      const clients = portClientsCache[p.if_index];
                      const loadingClients = clientsLoadingIf === p.if_index;
                      return (
                      <Fragment key={p.if_index}>
                      <tr
                        style={rowHighlight}
                        ref={(el) => {
                          if (el) portRowRefs.current.set(p.if_index, el);
                          else portRowRefs.current.delete(p.if_index);
                        }}
                      >
                        <td style={portTableCellStyle(p, "data")}>{p.if_index}</td>
                        <td
                          style={portTableCellStyle(p, "data", {
                            overflow: "hidden",
                            textOverflow: "ellipsis",
                          })}
                        >
                          <button
                            type="button"
                            className="port-expand-btn"
                            onClick={() => togglePortExpand(p.if_index)}
                            title="Список MAC на порту"
                          >
                            <span className="port-expand-chevron">{expanded ? "▼" : "▶"}</span>
                            {p.if_name ?? "—"}
                          </button>
                        </td>
                        <td
                          style={portTableCellStyle(p, "data", {
                            overflow: "hidden",
                          })}
                        >
                          {canWrite ? (
                          <input
                            className="port-descr-input"
                            value={descrEdits[p.if_index] ?? portDisplayDescr(p)}
                            placeholder={(p.if_descr ?? "").trim() || "описание"}
                            maxLength={200}
                            title={
                              "Описание пишется на коммутатор (SNMP ifAlias, иначе SSH). Очистите поле, чтобы снять description на свитче."
                            }
                            onFocus={() => {
                              descrFocusIf.current = p.if_index;
                            }}
                            onChange={(e) => {
                              const v = e.target.value;
                              setDescrEdits((prev) => ({ ...prev, [p.if_index]: v }));
                            }}
                            onBlur={() => {
                              descrFocusIf.current = null;
                              void savePortDescr(p);
                            }}
                            onKeyDown={(e) => {
                              if (e.key === "Enter") {
                                e.preventDefault();
                                (e.target as HTMLInputElement).blur();
                              }
                            }}
                            onClick={(e) => e.stopPropagation()}
                          />
                          ) : (
                            <span className="port-descr-text" title="Только просмотр">
                              {portDisplayDescr(p) || "—"}
                            </span>
                          )}
                        </td>
                        <td style={portTableCellStyle(p, "admin")}>{adm(p.admin_status)}</td>
                        <td style={portTableCellStyle(p, "oper")}>{adm(p.oper_status)}</td>
                        <td style={portTableCellStyle(p, "data", { textAlign: "center" })}>
                          {portVlanDisplay(p)}
                        </td>
                        <td
                          style={portTableCellStyle(p, "data", { textAlign: "center" })}
                          title={
                            showPoEIndicator(p) && !isLikelyFiberPort(p, data.device.sys_descr)
                              ? p.poe_power_w != null && p.poe_power_w > 0
                                ? `PoE активен · ${p.poe_power_w.toFixed(1)} W`
                                : "PoE активен (SNMP/SSH или LLDP-PD)"
                              : "PoE нет"
                          }
                        >
                          {showPoEIndicator(p) && !isLikelyFiberPort(p, data.device.sys_descr) ? (
                            <span style={{ color: "#00e676", fontWeight: 700 }} aria-label="PoE">
                              ✓
                            </span>
                          ) : (
                            "—"
                          )}
                        </td>
                        <td style={portTableCellStyle(p, "data")}>
                          {cliSyncBusy && !p.cli_port_mode ? "…" : (p.port_role ?? "auto")}
                        </td>
                        <td style={portTableCellStyle(p, "data", { whiteSpace: "nowrap" })}>
                          {formatPortSpeedFromRow(p.if_high_speed, p.if_speed)}
                        </td>
                        <td style={portTableCellStyle(p, "data")}>
                          {p.util_in_pct != null ? p.util_in_pct.toFixed(1) : "—"} /{" "}
                          {p.util_out_pct != null ? p.util_out_pct.toFixed(1) : "—"} /{" "}
                          {p.util_max_pct != null ? p.util_max_pct.toFixed(1) : "—"}
                        </td>
                        <td style={portTableCellStyle(p, "data")}>
                          {(() => {
                            const ser = trafficByIf[p.if_index];
                            const vals = (ser?.rx ?? []).map((x) => x.v);
                            const last = vals.length ? vals[vals.length - 1] : null;
                            return (
                              <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
                                <span style={{ fontSize: "0.8rem", whiteSpace: "nowrap" }}>{formatBitRate(last)}</span>
                                <TrafficSparkline values={vals} color="#5b8def" title="Rx за ~60 мин" />
                              </div>
                            );
                          })()}
                        </td>
                        <td style={portTableCellStyle(p, "data")}>
                          {(() => {
                            const ser = trafficByIf[p.if_index];
                            const vals = (ser?.tx ?? []).map((x) => x.v);
                            const last = vals.length ? vals[vals.length - 1] : null;
                            return (
                              <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
                                <span style={{ fontSize: "0.8rem", whiteSpace: "nowrap" }}>{formatBitRate(last)}</span>
                                <TrafficSparkline values={vals} color="#8b6cf0" title="Tx за ~60 мин" />
                              </div>
                            );
                          })()}
                        </td>
                        <td
                          className="port-neighbors-cell"
                          style={portTableCellStyle(p, "data", { fontSize: "0.85rem" })}
                        >
                          {(() => {
                            const list = neighborsByIf.get(p.if_index) ?? [];
                            if (list.length === 0) return "—";
                            return list.map((n, i) => {
                              const proto = (n.protocol || "lldp").toUpperCase();
                              const name = n.remote_sys_name || "—";
                              const port = n.remote_port_id ? ` / ${n.remote_port_id}` : "";
                              const stale = n.stale ? " (stale)" : "";
                              const rid = n.remote_device_id;
                              return (
                                <span key={`${proto}-${name}-${port}-${n.rem_index ?? i}`}>
                                  {i > 0 ? "; " : null}
                                  {proto}:{" "}
                                  {rid != null && rid > 0 ? (
                                    <>
                                      <Link to={`/devices/${rid}`}>{name}</Link>
                                      {port}
                                      {" "}
                                      <Link
                                        to={`/topology?focus=${rid}`}
                                        title="На топологии"
                                        style={{ fontSize: "0.8rem", opacity: 0.85 }}
                                      >
                                        ↗
                                      </Link>
                                    </>
                                  ) : (
                                    <>
                                      {name}
                                      {port}
                                    </>
                                  )}
                                  {stale}
                                </span>
                              );
                            });
                          })()}
                        </td>
                        <td style={portTableCellStyle(p, "data")}>
                          <button
                            type="button"
                            className={portIgnoreButtonClass(igMode)}
                            disabled={!canWrite}
                            onClick={(e) => {
                              e.stopPropagation();
                              cyclePortIgnore(p);
                            }}
                            title={canWrite ? portIgnoreTitle(igMode) : "Только просмотр"}
                          >
                            {portIgnoreButtonLabel(igMode)}
                          </button>
                        </td>
                      </tr>
                      {expanded && (
                        <tr>
                          <td colSpan={PORT_TABLE_COLS} style={{ padding: 0, verticalAlign: "top" }}>
                            <div className="port-clients-panel">
                              {loadingClients && <p style={{ margin: "0.5rem 0.75rem", color: "#9aa3b5" }}>Загрузка…</p>}
                              {!loadingClients && clientsErr && (
                                <p style={{ margin: "0.5rem 0.75rem", color: "#f88" }}>{clientsErr}</p>
                              )}
                              {!loadingClients && !clientsErr && clients != null && clients.length === 0 && (
                                <p style={{ margin: "0.5rem 0.75rem", color: "#9aa3b5", fontSize: "0.9rem" }}>
                                  На порту нет записей FDB. Проверьте опрос MAC/FDB или это uplink/trunk.
                                </p>
                              )}
                              {!loadingClients && !clientsErr && clients != null && clients.length > 0 && (
                                <>
                                <p style={{ margin: "0.35rem 0.75rem 0", color: "#9aa3b5", fontSize: "0.8rem" }}>
                                  IP берётся из ARP любого узла (обычно шлюз/L3). Если «—» — добавьте в Узлы маршрутизатор с SNMP и дождитесь опроса; рядом с MAC тогда показывается производитель по IEEE OUI (камеры, СКУД и т.п. часто не светятся в ARP).
                                  «Добавить устройство» создаёт узел и сразу рисует линк на топологии с этого порта (FDB, не LLDP).
                                </p>
                                {portPromoteMsg && expandedIfIndex === p.if_index && (
                                  <p style={{ margin: "0.35rem 0.75rem 0", color: "#9bd08b", fontSize: "0.85rem" }} role="status">
                                    {portPromoteMsg}
                                  </p>
                                )}
                                <table className="port-clients-table">
                                  <thead>
                                    <tr>
                                      <th>VLAN</th>
                                      <th>MAC</th>
                                      <th>IP</th>
                                      <th />
                                    </tr>
                                  </thead>
                                  <tbody>
                                    {clients.map((c) => {
                                      const ips = clientIPs(c);
                                      const vendor = macVendorLabel(c.mac_vendor, ips.length > 0);
                                      const promoting =
                                        portPromote != null &&
                                        portPromote.ifIndex === p.if_index &&
                                        portPromote.mac === c.mac;
                                      return (
                                      <tr key={`${c.mac}-${c.vlan_id ?? ""}`}>
                                        <td>{c.vlan_id != null ? c.vlan_id : "—"}</td>
                                        <td>
                                          <Link to={`/investigate/mac?mac=${encodeURIComponent(c.mac)}`} title="Расследовать MAC">
                                            {formatMacDisplay(c.mac)}
                                          </Link>
                                          {vendor ? (
                                            <span className="mac-vendor" title={vendor}>
                                              {" "}
                                              ({vendor})
                                            </span>
                                          ) : null}
                                        </td>
                                        <td>
                                          {ips.length > 0 ? (
                                            ips.map((ip, i) => (
                                              <span key={ip}>
                                                {i > 0 ? ", " : ""}
                                                <a href={`http://${ip}`} target="_blank" rel="noreferrer noopener">
                                                  {ip}
                                                </a>
                                              </span>
                                            ))
                                          ) : (
                                            "—"
                                          )}
                                        </td>
                                        <td style={{ whiteSpace: "nowrap" }}>
                                          {c.existing_device_id != null ? (
                                            <>
                                              <Link
                                                to={`/devices/${c.existing_device_id}`}
                                                state={deviceLinkState({
                                                  path: `/devices/${id}`,
                                                  label: data.device.name || "Узел",
                                                })}
                                              >
                                                {c.existing_device_name || `Узел #${c.existing_device_id}`}
                                              </Link>{" "}
                                              <button
                                                type="button"
                                                disabled={portPromoteBusy || !canWrite}
                                                title="Записать связь этого порта с уже известным узлом на топологии"
                                                onClick={() => void linkExistingPortClient(p.if_index, c)}
                                              >
                                                Связать
                                              </button>
                                            </>
                                          ) : (
                                            <button
                                              type="button"
                                              disabled={portPromoteBusy || !canWrite}
                                              onClick={() => {
                                                if (promoting) {
                                                  setPortPromote(null);
                                                  setPortPromotePreview(null);
                                                  return;
                                                }
                                                openPortPromote(p.if_index, c);
                                              }}
                                            >
                                              {promoting ? "Скрыть форму" : "Добавить устройство"}
                                            </button>
                                          )}
                                        </td>
                                      </tr>
                                      );
                                    })}
                                  </tbody>
                                </table>
                                {portPromote != null && portPromote.ifIndex === p.if_index && (
                                  <div style={{ margin: "0.5rem 0.75rem 0.75rem", maxWidth: 560 }}>
                                    <PromoteDiscoveredForm
                                      title={`Добавление в список Узлы — ${formatMacDisplay(portPromote.mac)}`}
                                      values={portPromoteForm}
                                      locations={existingLocations}
                                      preview={portPromotePreview}
                                      busy={portPromoteBusy}
                                      onChange={(patch) => setPortPromoteForm((v) => ({ ...v, ...patch }))}
                                      onPreview={() => void previewPortClient()}
                                      onSubmit={(e) => void submitPortClient(e)}
                                      onCancel={() => {
                                        setPortPromote(null);
                                        setPortPromoteForm(emptyPortPromote);
                                        setPortPromotePreview(null);
                                      }}
                                    />
                                  </div>
                                )}
                                </>
                              )}
                            </div>
                          </td>
                        </tr>
                      )}
                      </Fragment>
                    );
                    })}
                  </tbody>
                </table>
              </div>
            </>
          )}
          </div>
          )}

          <h2>Последние события по этому узлу</h2>
          {data.recent_events.length === 0 && <p>Событий пока нет.</p>}
          {data.recent_events.length > 0 && (
            <table style={{ tableLayout: "fixed", width: "100%" }}>
              {eventsColgroup}
              <thead>
                <tr>
                  <th style={{ position: "relative", userSelect: "none" }}>
                    Время
                    <EventsResizeHandle colIndex={0} />
                  </th>
                  <th style={{ position: "relative", userSelect: "none" }}>
                    Тип
                    <EventsResizeHandle colIndex={1} />
                  </th>
                  <th style={{ position: "relative", userSelect: "none" }}>
                    if
                    <EventsResizeHandle colIndex={2} />
                  </th>
                  <th style={{ position: "relative", userSelect: "none" }}>
                    Серьёзность
                    <EventsResizeHandle colIndex={3} />
                  </th>
                  <th style={{ position: "relative", userSelect: "none" }}>
                    Сводка
                    <EventsResizeHandle colIndex={4} />
                  </th>
                </tr>
              </thead>
              <tbody>
                {data.recent_events.map((ev) => (
                  <tr key={ev.id}>
                    <td style={{ whiteSpace: "nowrap" }}>{new Date(ev.created_at).toLocaleString()}</td>
                    <td>{ev.event_type}</td>
                    <td>{ev.if_index ?? "—"}</td>
                    <td>{ev.severity}</td>
                    <td style={{ fontSize: "0.9rem", overflow: "hidden", textOverflow: "ellipsis" }}>{formatEventSummary(ev)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
      </div>
    </div>
  );
}
