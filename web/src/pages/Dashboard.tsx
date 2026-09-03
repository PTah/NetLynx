import type { ComponentType, ReactNode } from "react";
import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet } from "../api";
import {
  type CategoryFilterState,
  type DeviceCategory,
  normalizeDeviceCategory,
  readDashboardCategoryFilter,
  writeDashboardCategoryFilter,
  dashboardDevicesTableTitle,
} from "../deviceCategories";
import { useDeviceCategories } from "../hooks/useDeviceCategories";
import { DeviceCategoryIcon } from "../components/DeviceCategoryIcon";
import { useEventStream } from "../hooks/useEventStream";
import EventsTable from "../components/EventsTable";
import { formatDashboardLastEventTooltip } from "../eventFormat";
import { usePersistedColumnWidths } from "../hooks/usePersistedColumnWidths";
import { deviceLinkState } from "../navigation";
import { deviceReachabilityLabel, isDeviceOnline } from "../deviceOnline";
import type { Device, EventRow } from "../types";

type DeviceFilter = "all" | "active" | "inactive";

type SortKey = "name" | "host" | "location" | "status" | "cpu";
type SortDir = "asc" | "desc";

type SortState = { key: SortKey; dir: SortDir };

const SORT_STORAGE_KEY = "invetor_dashboard_device_sort";
const FILTER_STORAGE_KEY = "invetor_dashboard_device_filter";
const SEARCH_STORAGE_KEY = "invetor_dashboard_device_search";

function asNum(v: unknown): number | null {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string" && v.trim() !== "") {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return null;
}

function eventPortNum(ev: EventRow): string | null {
  if (ev.if_index != null && Number.isFinite(Number(ev.if_index)) && Number(ev.if_index) > 0) {
    return String(ev.if_index);
  }
  const p = ev.payload ?? {};
  const fromPayload = asNum((p as Record<string, unknown>).if_index);
  if (fromPayload != null && fromPayload > 0) return String(Math.trunc(fromPayload));
  const raw = String((p as Record<string, unknown>).if_name ?? (p as Record<string, unknown>).if_descr ?? "").trim();
  const slash = raw.match(/\/(\d+)\s*$/);
  if (slash) return slash[1];
  const word = raw.match(/Port:\s*(\d+)/i);
  if (word) return word[1];
  return null;
}

function renderLastEvent(ev?: EventRow): ReactNode {
  if (!ev) return "—";
  const portNum = eventPortNum(ev);
  const portSuffix = portNum ? ` Port ${portNum}` : "";
  const p = ev.payload ?? {};
  const tip = formatDashboardLastEventTooltip(ev);
  const wrap = (node: ReactNode) => (
    <span title={tip} style={{ cursor: "help" }}>
      {node}
    </span>
  );
  switch (ev.event_type) {
    case "PORT_UTILIZATION_HIGH": {
      const util = asNum((p as Record<string, unknown>).util_max_pct);
      const utilTxt = util == null ? "—" : `${util.toFixed(1)}%`;
      return wrap(
        <>
          <span style={{ color: "#f88", fontWeight: 700 }}>PUH ({utilTxt})</span>
          {portSuffix}
        </>,
      );
    }
    case "PORT_UTILIZATION_OK":
      return wrap(
        <>
          <span style={{ color: "#6d6", fontWeight: 700 }}>PUO</span>
          {portSuffix}
        </>,
      );
    case "PORT_SPEED_DOWN": {
      const oldM = asNum((p as Record<string, unknown>).old_mbps);
      const newM = asNum((p as Record<string, unknown>).new_mbps);
      const spd =
        oldM != null && newM != null ? `${Math.round(oldM)}→${Math.round(newM)}` : "—";
      return wrap(
        <>
          <span style={{ color: "#f88", fontWeight: 700 }}>PSD ({spd})</span>
          {portSuffix}
        </>,
      );
    }
    case "PORT_SPEED_OK":
      return wrap(
        <>
          <span style={{ color: "#6d6", fontWeight: 700 }}>PSO</span>
          {portSuffix}
        </>,
      );
    case "MAC_REMOVED":
      return wrap(
        <>
          <span style={{ color: "#fff" }}>MR</span>
          {portSuffix}
        </>,
      );
    case "UNKNOWN_MAC_ON_ACCESS_PORT":
      return wrap(
        <>
          <span style={{ color: "#f88", fontWeight: 700 }}>UMAC!</span>
          {portSuffix}
        </>,
      );
    default:
      return wrap(ev.event_type);
  }
}

function loadDeviceFilter(): DeviceFilter {
  if (typeof window === "undefined") return "all";
  try {
    const raw = localStorage.getItem(FILTER_STORAGE_KEY);
    if (raw === "all" || raw === "active" || raw === "inactive") return raw;
  } catch {
    /* ignore */
  }
  return "all";
}

function saveDeviceFilter(f: DeviceFilter) {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(FILTER_STORAGE_KEY, f);
  } catch {
    /* ignore */
  }
}

function loadSearchQuery(): string {
  if (typeof window === "undefined") return "";
  try {
    return localStorage.getItem(SEARCH_STORAGE_KEY) ?? "";
  } catch {
    return "";
  }
}

function saveSearchQuery(q: string) {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(SEARCH_STORAGE_KEY, q);
  } catch {
    /* ignore */
  }
}

function loadSortState(): SortState {
  if (typeof window === "undefined") return { key: "name", dir: "asc" };
  try {
    const raw = localStorage.getItem(SORT_STORAGE_KEY);
    if (!raw) return { key: "name", dir: "asc" };
    const o = JSON.parse(raw) as Partial<SortState>;
    if (
      o.key === "name" ||
      o.key === "host" ||
      o.key === "location" ||
      o.key === "status" ||
      o.key === "cpu"
    ) {
      if (o.dir === "asc" || o.dir === "desc") return { key: o.key, dir: o.dir };
    }
  } catch {
    /* ignore */
  }
  return { key: "name", dir: "asc" };
}

function saveSortState(s: SortState) {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify(s));
  } catch {
    /* ignore */
  }
}

function formatOfflineSince(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return d.toLocaleString("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function formatOfflineDuration(iso: string, nowMs: number): string {
  const start = new Date(iso).getTime();
  if (Number.isNaN(start)) return "";
  const sec = Math.max(0, Math.floor((nowMs - start) / 1000));
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  const mins = Math.floor((sec % 3600) / 60);
  const secs = sec % 60;
  if (days > 0) return `${days} д ${hours} ч`;
  if (hours > 0) return `${hours} ч ${mins} мин`;
  if (mins > 0) return `${mins} мин ${secs} с`;
  return `${secs} с`;
}

/** Длительность оффлайна в мс; онлайн / без offline_since → 0. */
function offlineDurationMs(d: Device, nowMs: number): number {
  if (isDeviceOnline(d)) return 0;
  const since = d.offline_since?.trim();
  if (!since) return 0;
  const start = new Date(since).getTime();
  if (Number.isNaN(start)) return 0;
  return Math.max(0, nowMs - start);
}

function sortDevices(list: Device[], key: SortKey, dir: SortDir, nowMs: number): Device[] {
  const m = dir === "asc" ? 1 : -1;
  const out = [...list];
  out.sort((a, b) => {
    if (key === "name") {
      return m * a.name.localeCompare(b.name, "ru", { numeric: true, sensitivity: "base" });
    }
    if (key === "host") {
      return m * a.host.localeCompare(b.host, "ru", { numeric: true, sensitivity: "base" });
    }
    if (key === "location") {
      const la = (a.location ?? "").trim();
      const lb = (b.location ?? "").trim();
      if (!la && !lb) return 0;
      if (!la) return m;
      if (!lb) return -m;
      return m * la.localeCompare(lb, "ru", { numeric: true, sensitivity: "base" });
    }
    if (key === "status") {
      const da = offlineDurationMs(a, nowMs);
      const db = offlineDurationMs(b, nowMs);
      if (da !== db) return m * (da - db);
      // одинаковая длительность / оба онлайн — по имени
      return a.name.localeCompare(b.name, "ru", { numeric: true, sensitivity: "base" });
    }
    const na = a.last_cpu_pct;
    const nb = b.last_cpu_pct;
    const aOk = na != null && Number.isFinite(na);
    const bOk = nb != null && Number.isFinite(nb);
    if (!aOk && !bOk) return 0;
    if (!aOk) return 1;
    if (!bOk) return -1;
    return m * (na - nb);
  });
  return out;
}

function SortTh({
  label,
  sortKey,
  sort,
  onSort,
  colIndex,
  ResizeHandle,
}: {
  label: string;
  sortKey: SortKey;
  sort: SortState;
  onSort: (k: SortKey) => void;
  colIndex: number;
  ResizeHandle: ComponentType<{ colIndex: number }>;
}) {
  const active = sort.key === sortKey;
  const arrow = active ? (sort.dir === "asc" ? " ▲" : " ▼") : "";
  const title =
    sortKey === "name"
      ? "Сортировать по имени (повторный клик — обратный порядок)"
      : sortKey === "host"
        ? "Сортировать по IP-адресу"
        : sortKey === "location"
          ? "Сортировать по расположению"
          : sortKey === "status"
            ? "Сортировать по времени оффлайна (▲ короче / онлайн сверху, ▼ дольше оффлайн сверху)"
            : "Сортировать по загрузке CPU";
  return (
    <th style={{ whiteSpace: "nowrap", position: "relative", userSelect: "none" }}>
      <button
        type="button"
        title={title}
        onClick={() => onSort(sortKey)}
        style={{
          background: "transparent",
          border: "none",
          color: "inherit",
          font: "inherit",
          fontWeight: active ? 700 : 500,
          cursor: "pointer",
          padding: 0,
          textAlign: "left",
        }}
      >
        {label}
        {arrow}
      </button>
      <ResizeHandle colIndex={colIndex} />
    </th>
  );
}

export default function Dashboard() {
  const { categories } = useDeviceCategories();
  const { colgroup: devicesColgroup, ResizeHandle: DevicesResizeHandle } = usePersistedColumnWidths(
    "dashboard-devices",
    [140, 130, 200, 76, 200, 96, 240],
  );
  const [devices, setDevices] = useState<Device[] | null>(null);
  const [events, setEvents] = useState<EventRow[] | null>(null);
  const [lastEventByDevice, setLastEventByDevice] = useState<Map<number, EventRow>>(new Map());
  const [err, setErr] = useState<string | null>(null);
  const [deviceFilter, setDeviceFilter] = useState<DeviceFilter>(() => loadDeviceFilter());
  const [searchQuery, setSearchQuery] = useState(() => loadSearchQuery());
  const [categoryFilter, setCategoryFilter] = useState<CategoryFilterState>(() => readDashboardCategoryFilter());
  const [typeMenuOpen, setTypeMenuOpen] = useState(false);
  const typeMenuRef = useRef<HTMLDivElement>(null);
  const [sort, setSort] = useState<SortState>(() => loadSortState());
  const [nowMs, setNowMs] = useState(() => Date.now());

  useEffect(() => {
    setCategoryFilter((prev) => {
      const next = readDashboardCategoryFilter(categories);
      for (const c of categories) {
        if (typeof prev[c.id] === "boolean") next[c.id] = prev[c.id]!;
      }
      return next;
    });
  }, [categories]);

  const load = (signal?: AbortSignal) => {
    setErr(null);
    Promise.all([
      apiGet<Device[] | null>("/api/v1/devices", signal ? { signal } : undefined),
      apiGet<EventRow[] | null>("/api/v1/events?limit=40", signal ? { signal } : undefined),
    ])
      .then(([d, e]) => {
        setDevices(Array.isArray(d) ? d : []);
        setEvents(Array.isArray(e) ? e : []);
      })
      .catch((e: Error) => {
        if (e.name !== "AbortError") setErr(e.message);
      });
  };

  useEventStream((ev) => {
    setEvents((prev) => {
      const list = prev ?? [];
      if (list.some((r) => r.id === ev.id)) return list;
      return [ev, ...list].slice(0, 40);
    });
  });

  useEffect(() => {
    const t = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(t);
  }, []);

  useEffect(() => {
    let request: AbortController | null = null;
    const refresh = () => {
      request?.abort();
      request = new AbortController();
      load(request.signal);
    };
    refresh();
    const t = setInterval(refresh, 8000);
    return () => {
      clearInterval(t);
      request?.abort();
    };
  }, []);

  useEffect(() => {
    if (!typeMenuOpen) return;
    const onDocClick = (ev: MouseEvent) => {
      if (typeMenuRef.current && !typeMenuRef.current.contains(ev.target as Node)) {
        setTypeMenuOpen(false);
      }
    };
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [typeMenuOpen]);

  useEffect(() => {
    if (!events || events.length === 0) return;
    setLastEventByDevice((prev) => {
      const next = new Map(prev);
      for (const ev of events) {
        const prevEv = next.get(ev.device_id);
        if (!prevEv || new Date(ev.created_at).getTime() > new Date(prevEv.created_at).getTime()) {
          next.set(ev.device_id, ev);
        }
      }
      return next;
    });
  }, [events]);

  const { activeCount, inactiveCount, filteredDevices, deviceName, selectedTypeCount, devicesTableTitle } =
    useMemo(() => {
    const list = devices ?? [];
    const byCategory = list.filter((d) => {
      const id = normalizeDeviceCategory(d.device_category);
      return categoryFilter[id] !== false;
    });
    const active = byCategory.filter(isDeviceOnline).length;
    const inactive = byCategory.length - active;
    let filtered =
      deviceFilter === "active"
        ? byCategory.filter(isDeviceOnline)
        : deviceFilter === "inactive"
          ? byCategory.filter((d) => !isDeviceOnline(d))
          : byCategory;
    const q = searchQuery.trim().toLowerCase();
    if (q) {
      filtered = filtered.filter(
        (d) => d.name.toLowerCase().includes(q) || d.host.toLowerCase().includes(q),
      );
    }
    const nameById = new Map(list.map((d) => [d.id, d.name] as const));
    const selectedTypeCount = categories.filter((o) => categoryFilter[o.id] !== false).length;
    return {
      activeCount: active,
      inactiveCount: inactive,
      filteredDevices: filtered,
      deviceName: (id: number) => nameById.get(id) ?? `#${id}`,
      selectedTypeCount,
      devicesTableTitle: dashboardDevicesTableTitle(categoryFilter, categories),
    };
  }, [devices, deviceFilter, categoryFilter, searchQuery, categories]);

  const sortedDevices = useMemo(
    () => sortDevices(filteredDevices, sort.key, sort.dir, nowMs),
    [filteredDevices, sort, nowMs],
  );
  const onSortColumn = (key: SortKey) => {
    setSort((prev) => {
      // По статусу удобнее сразу видеть «дольше оффлайн» сверху.
      const defaultDir: SortDir = key === "status" ? "desc" : "asc";
      const next: SortState =
        prev.key === key ? { key, dir: prev.dir === "asc" ? "desc" : "asc" } : { key, dir: defaultDir };
      saveSortState(next);
      return next;
    });
  };

  const toggleCategoryFilter = (id: DeviceCategory) => {
    setCategoryFilter((prev) => {
      const next = { ...prev, [id]: !(prev[id] !== false) };
      writeDashboardCategoryFilter(next);
      return next;
    });
  };

  const typeMenuLabel =
    selectedTypeCount === categories.length
      ? "Тип устройства"
      : `Тип устройства (${selectedTypeCount}/${categories.length})`;

  return (
    <div>
      <h1 style={{ marginTop: 0 }}>Дашборд</h1>
      {err && <p style={{ color: "#f88" }}>{err}</p>}

      <h2>
        {devicesTableTitle}
        {devices != null && (
          <>
            {" — "}
            <span
              title="Онлайн: успешный SNMP-опрос (для ПК/серверов — также ping)"
              style={{
                color: "#00e676",
                fontWeight: 700,
                padding: "0.12rem 0.45rem",
                borderRadius: 6,
                background:
                  deviceFilter === "all" || deviceFilter === "active" ? "#252b38" : "transparent",
                boxShadow:
                  deviceFilter === "all" || deviceFilter === "active"
                    ? "inset 0 0 0 1px #333a4a"
                    : "none",
                opacity: deviceFilter === "inactive" ? 0.4 : 1,
                transition: "background 0.12s ease, opacity 0.12s ease",
              }}
            >
              {activeCount} онлайн
            </span>
            {" · "}
            <span
              title="Оффлайн: нет успешного SNMP (для ПК/серверов — нет ping)"
              style={{
                color: "#ff1744",
                fontWeight: 700,
                padding: "0.12rem 0.45rem",
                borderRadius: 6,
                background:
                  deviceFilter === "all" || deviceFilter === "inactive" ? "#252b38" : "transparent",
                boxShadow:
                  deviceFilter === "all" || deviceFilter === "inactive"
                    ? "inset 0 0 0 1px #333a4a"
                    : "none",
                opacity: deviceFilter === "active" ? 0.4 : 1,
                transition: "background 0.12s ease, opacity 0.12s ease",
              }}
            >
              {inactiveCount} оффлайн
            </span>
          </>
        )}
      </h2>
      {devices && devices.length > 0 && (
        <div
          style={{
            display: "flex",
            flexWrap: "wrap",
            gap: "0.75rem",
            alignItems: "center",
            justifyContent: "space-between",
            marginBottom: "0.75rem",
          }}
        >
          <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center", flex: "1 1 420px" }}>
            <span style={{ color: "#9aa3b5", fontSize: "0.9rem" }}>Показать:</span>
            {(
              [
                ["all", "все"],
                ["active", "только онлайн"],
                ["inactive", "только оффлайн"],
              ] as const
            ).map(([key, label]) => (
              <button
                key={key}
                type="button"
                onClick={() => {
                  setDeviceFilter(key);
                  saveDeviceFilter(key);
                }}
                style={{
                  fontWeight: deviceFilter === key ? 700 : 400,
                  background: deviceFilter === key ? "#252b38" : "transparent",
                  border: "1px solid #333a4a",
                  borderRadius: 6,
                  padding: "0.25rem 0.6rem",
                  cursor: "pointer",
                  color: "#c5cedd",
                }}
              >
                {label}
              </button>
            ))}
            <label style={{ display: "inline-flex", alignItems: "center", gap: "0.35rem", color: "#9aa3b5", fontSize: "0.9rem" }}>
              Найти:
              <input
                type="search"
                value={searchQuery}
                onChange={(e) => {
                  const next = e.target.value;
                  setSearchQuery(next);
                  saveSearchQuery(next);
                }}
                placeholder="имя или IP"
                style={{ minWidth: 160 }}
              />
            </label>
          </div>
          <div ref={typeMenuRef} style={{ position: "relative", flex: "0 0 auto" }}>
            <button
              type="button"
              aria-expanded={typeMenuOpen}
              onClick={() => setTypeMenuOpen((open) => !open)}
              style={{
                background: typeMenuOpen ? "#252b38" : "transparent",
                border: "1px solid #333a4a",
                borderRadius: 6,
                padding: "0.35rem 0.75rem",
                cursor: "pointer",
                color: "#c5cedd",
                whiteSpace: "nowrap",
              }}
            >
              {typeMenuLabel} {typeMenuOpen ? "▴" : "▾"}
            </button>
            {typeMenuOpen && (
              <div
                style={{
                  position: "absolute",
                  right: 0,
                  top: "calc(100% + 0.35rem)",
                  minWidth: 220,
                  padding: "0.65rem 0.75rem",
                  background: "#1a1f2a",
                  border: "1px solid #333a4a",
                  borderRadius: 8,
                  boxShadow: "0 8px 24px rgba(0,0,0,0.35)",
                  zIndex: 20,
                }}
              >
                {categories.map((o) => (
                  <label
                    key={o.id}
                    style={{
                      display: "flex",
                      alignItems: "center",
                      gap: 8,
                      padding: "0.25rem 0",
                      cursor: "pointer",
                      fontSize: "0.92rem",
                    }}
                  >
                    <input
                      type="checkbox"
                      checked={categoryFilter[o.id] !== false}
                      onChange={() => toggleCategoryFilter(o.id)}
                    />
                    <DeviceCategoryIcon category={o.id} height={18} title={o.label} />
                    {o.label}
                  </label>
                ))}
              </div>
            )}
          </div>
        </div>
      )}
      {!devices && <p>Загрузка…</p>}
      {devices && devices.length === 0 && (
        <p>
          Узлов пока нет. Добавьте на странице <Link to="/devices">Узлы</Link>.
        </p>
      )}
      {devices && devices.length > 0 && (
        <table style={{ tableLayout: "fixed", width: "100%" }}>
          {devicesColgroup}
          <thead>
            <tr>
              <SortTh
                label="Имя"
                sortKey="name"
                sort={sort}
                onSort={onSortColumn}
                colIndex={0}
                ResizeHandle={DevicesResizeHandle}
              />
              <SortTh
                label="Адрес"
                sortKey="host"
                sort={sort}
                onSort={onSortColumn}
                colIndex={1}
                ResizeHandle={DevicesResizeHandle}
              />
              <SortTh
                label="Расположение"
                sortKey="location"
                sort={sort}
                onSort={onSortColumn}
                colIndex={2}
                ResizeHandle={DevicesResizeHandle}
              />
              <th style={{ position: "relative", userSelect: "none" }}>
                SNMP
                <DevicesResizeHandle colIndex={3} />
              </th>
              <SortTh
                label="Статус"
                sortKey="status"
                sort={sort}
                onSort={onSortColumn}
                colIndex={4}
                ResizeHandle={DevicesResizeHandle}
              />
              <SortTh
                label="CPU"
                sortKey="cpu"
                sort={sort}
                onSort={onSortColumn}
                colIndex={5}
                ResizeHandle={DevicesResizeHandle}
              />
              <th style={{ position: "relative", userSelect: "none" }}>
                Последнее событие
                <DevicesResizeHandle colIndex={6} />
              </th>
            </tr>
          </thead>
          <tbody>
            {sortedDevices.map((d) => (
              <tr key={d.id}>
                <td>
                  <Link to={`/devices/${d.id}`} state={deviceLinkState({ path: "/", label: "Дашборд" })}>
                    {d.name}
                  </Link>
                </td>
                <td>{d.host}</td>
                <td style={{ fontSize: "0.85rem", overflow: "hidden", textOverflow: "ellipsis" }}>{d.location?.trim() ? d.location : "—"}</td>
                <td>{d.snmp_version}</td>
                <td>
                  {(() => {
                    const s = deviceReachabilityLabel(d);
                    const offline = !isDeviceOnline(d);
                    const since = offline ? d.offline_since?.trim() : "";
                    const sinceTxt = since ? formatOfflineSince(since) : "";
                    const durTxt = since ? formatOfflineDuration(since, nowMs) : "";
                    return (
                      <span style={{ color: s.color }}>
                        {s.text}
                        {sinceTxt && durTxt ? (
                          <span
                            style={{ display: "block", color: "#9aa3b5", fontSize: "0.82rem", fontWeight: 400 }}
                            title={`Ушёл в оффлайн: ${sinceTxt}`}
                          >
                            {sinceTxt} · {durTxt}
                          </span>
                        ) : null}
                      </span>
                    );
                  })()}
                </td>
                <td style={{ whiteSpace: "nowrap" }}>
                  {d.last_cpu_pct == null ? (
                    "N/A"
                  ) : (
                    <>
                      {d.last_cpu_pct.toFixed(1)}%
                      {d.cpu_profile ? ` (${d.cpu_profile})` : ""}
                    </>
                  )}
                </td>
                <td style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {renderLastEvent(lastEventByDevice.get(d.id))}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}

      <h2 style={{ marginTop: "2rem" }}>События</h2>
      {!events && <p>Загрузка…</p>}
      {events && events.length === 0 && <p>Событий пока нет (после опроса появятся линк/утилизация).</p>}
      {events && events.length > 0 && (
        <EventsTable
          rows={events}
          deviceLabel={deviceName}
          deviceLinkState={deviceLinkState({ path: "/", label: "Дашборд" })}
          widthStorageKey="dashboard-events"
        />
      )}
    </div>
  );
}
