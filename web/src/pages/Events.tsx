import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { apiGet, asArray } from "../api";
import EventsTable from "../components/EventsTable";
import { useEventStream } from "../hooks/useEventStream";
import { formatEventTypeLabel } from "../eventFormat";
import { deviceLinkState } from "../navigation";
import type { Device, EventRow } from "../types";

const EVENT_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: "", label: "Все" },
  { value: "LINK_UP", label: formatEventTypeLabel("LINK_UP") },
  { value: "LINK_DOWN", label: formatEventTypeLabel("LINK_DOWN") },
  { value: "DEVICE_OFFLINE", label: formatEventTypeLabel("DEVICE_OFFLINE") },
  { value: "DEVICE_ONLINE", label: formatEventTypeLabel("DEVICE_ONLINE") },
  { value: "PORT_UTILIZATION_HIGH", label: formatEventTypeLabel("PORT_UTILIZATION_HIGH") },
  { value: "PORT_UTILIZATION_OK", label: formatEventTypeLabel("PORT_UTILIZATION_OK") },
  { value: "UNKNOWN_MAC_ON_ACCESS_PORT", label: formatEventTypeLabel("UNKNOWN_MAC_ON_ACCESS_PORT") },
  { value: "MAC_MOVED", label: formatEventTypeLabel("MAC_MOVED") },
  { value: "MAC_FLAPPING", label: formatEventTypeLabel("MAC_FLAPPING") },
  { value: "MAC_MULTI_ACCESS", label: formatEventTypeLabel("MAC_MULTI_ACCESS") },
  { value: "MAC_REMOVED", label: formatEventTypeLabel("MAC_REMOVED") },
  { value: "ACCESS_PORT_MAC_SUBSTITUTED", label: formatEventTypeLabel("ACCESS_PORT_MAC_SUBSTITUTED") },
  { value: "ACCESS_PORT_LONG_IDLE_DEVICE", label: formatEventTypeLabel("ACCESS_PORT_LONG_IDLE_DEVICE") },
  { value: "MANUAL_LINK_SUPERSEDED", label: formatEventTypeLabel("MANUAL_LINK_SUPERSEDED") },
];
const SEVERITIES = ["", "info", "warning", "error"];

const EVENTS_POLL_MS = 8000;
const EVENT_LIMIT_DEFAULT = 200;
const EVENT_LIMIT_MIN = 1;
const EVENT_LIMIT_MAX = 500;

function clampEventLimit(raw: string): number {
  const n = parseInt(raw, 10);
  if (Number.isNaN(n)) return EVENT_LIMIT_DEFAULT;
  return Math.min(EVENT_LIMIT_MAX, Math.max(EVENT_LIMIT_MIN, n));
}

export default function Events() {
  const [rows, setRows] = useState<EventRow[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [err, setErr] = useState<string | null>(null);

  const [deviceId, setDeviceId] = useState("");
  const [eventType, setEventType] = useState("");
  const [severity, setSeverity] = useState("");
  const [eventLimit, setEventLimit] = useState(String(EVENT_LIMIT_DEFAULT));

  const load = useCallback((signal?: AbortSignal) => {
    setErr(null);
    const p = new URLSearchParams();
    p.set("limit", String(clampEventLimit(eventLimit)));
    const d = deviceId.trim();
    if (d) p.set("device_id", d);
    if (eventType) p.set("event_type", eventType);
    if (severity) p.set("severity", severity);
    apiGet<EventRow[]>(`/api/v1/events?${p.toString()}`, signal ? { signal } : undefined)
      .then((rows) => setRows(asArray(rows)))
      .catch((e: Error) => {
        if (e.name !== "AbortError") setErr(e.message);
      });
  }, [deviceId, eventType, severity, eventLimit]);

  useEventStream((ev) => {
    setRows((prev) => {
      if (prev.some((r) => r.id === ev.id)) return prev;
      return [ev, ...prev].slice(0, clampEventLimit(eventLimit));
    });
  });

  useEffect(() => {
    const ac = new AbortController();
    apiGet<Device[]>("/api/v1/devices", { signal: ac.signal })
      .then((rows) => setDevices(asArray(rows)))
      .catch((e: Error) => {
        if (e.name !== "AbortError") {
          /* список узлов для фильтра необязателен */
        }
      });
    return () => ac.abort();
  }, []);

  useEffect(() => {
    let request: AbortController | null = null;
    const refresh = () => {
      request?.abort();
      request = new AbortController();
      load(request.signal);
    };
    refresh();
    const t = window.setInterval(refresh, EVENTS_POLL_MS);
    return () => {
      window.clearInterval(t);
      request?.abort();
    };
  }, [load]);

  const deviceLabel = useMemo(() => {
    const m = new Map<number, string>();
    for (const d of devices) {
      m.set(d.id, d.name);
    }
    return (id: number) => m.get(id) ?? `#${id}`;
  }, [devices]);

  const applyFilters = (e: FormEvent) => {
    e.preventDefault();
    load();
  };

  return (
    <div>
      <h1 style={{ marginTop: 0 }}>События</h1>
      <form
        onSubmit={applyFilters}
        style={{ display: "flex", flexWrap: "wrap", gap: "0.75rem", alignItems: "flex-end", marginBottom: "1rem" }}
      >
        <label>
          Узел
          <br />
          <select value={deviceId} onChange={(e) => setDeviceId(e.target.value)} style={{ minWidth: 160 }}>
            <option value="">Все</option>
            {devices.map((d) => (
              <option key={d.id} value={String(d.id)}>
                {d.id} — {d.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          Тип
          <br />
          <select value={eventType} onChange={(e) => setEventType(e.target.value)}>
            {EVENT_TYPE_OPTIONS.map((o) => (
              <option key={o.value || "all"} value={o.value}>
                {o.label}
              </option>
            ))}
          </select>
        </label>
        <label>
          Серьёзность
          <br />
          <select value={severity} onChange={(e) => setSeverity(e.target.value)}>
            {SEVERITIES.map((t) => (
              <option key={t || "all"} value={t}>
                {t || "Все"}
              </option>
            ))}
          </select>
        </label>
        <label>
          Количество событий
          <br />
          <input
            type="number"
            min={EVENT_LIMIT_MIN}
            max={EVENT_LIMIT_MAX}
            value={eventLimit}
            onChange={(e) => setEventLimit(e.target.value)}
            style={{ width: 88 }}
            title={`От ${EVENT_LIMIT_MIN} до ${EVENT_LIMIT_MAX} (последние по времени)`}
          />
        </label>
        <button type="submit">Обновить</button>
      </form>
      {err && <p style={{ color: "#f88" }}>{err}</p>}
      <EventsTable
        rows={rows}
        deviceLabel={deviceLabel}
        deviceLinkState={deviceLinkState({ path: "/events", label: "События" })}
        widthStorageKey="events-page"
      />
    </div>
  );
}
