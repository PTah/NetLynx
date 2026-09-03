import type { CSSProperties, FormEvent } from "react";
import { useCallback, useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiGet } from "../api";
import { DeviceSearchSelect } from "../components/DeviceSearchSelect";
import { formatEventTypeLabel } from "../eventFormat";
import { deviceLinkState } from "../navigation";
import type { Device } from "../types";

type ScopeDevice = {
  id: number;
  name: string;
  host: string;
  hop: number;
};

type TimelineItem = {
  at: string;
  kind: string;
  device_id: number;
  device_name?: string;
  summary: string;
  severity?: string;
  if_index?: number;
  detail?: Record<string, unknown>;
};

type Report = {
  device_id: number;
  device_name: string;
  device_host: string;
  center: string;
  window: string;
  from: string;
  to: string;
  hops: number;
  scope_devices: ScopeDevice[];
  timeline: TimelineItem[];
};
function kindLabel(k: string): string {
  switch (k) {
    case "event":
      return "Событие";
    case "trap":
      return "SNMP trap";
    case "mac_move":
      return "MAC move";
    case "config_snapshot":
      return "Config";
    default:
      return k;
  }
}

function formatLocalInput(d: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function toRFC3339Local(localValue: string): string {
  const d = new Date(localValue);
  if (Number.isNaN(d.getTime())) return new Date().toISOString();
  return d.toISOString();
}

export default function Postmortem() {
  const [params, setParams] = useSearchParams();
  const [devices, setDevices] = useState<Device[]>([]);
  const [deviceId, setDeviceId] = useState(params.get("device_id") ?? "");
  const [aroundLocal, setAroundLocal] = useState(formatLocalInput(new Date()));
  const [windowMin, setWindowMin] = useState("5");
  const [hops, setHops] = useState("1");
  const [report, setReport] = useState<Report | null>(null);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    apiGet<Device[]>("/api/v1/devices")
      .then((d) => setDevices(Array.isArray(d) ? d : []))
      .catch(() => setDevices([]));
  }, []);

  useEffect(() => {
    const id = params.get("device_id");
    if (id) setDeviceId(id);
    const around = params.get("around");
    if (around) {
      const d = new Date(around);
      if (!Number.isNaN(d.getTime())) setAroundLocal(formatLocalInput(d));
    }
  }, [params]);

  const load = useCallback(async () => {
    if (!deviceId.trim()) {
      setErr("Выберите узел");
      return;
    }
    setLoading(true);
    setErr(null);
    try {
      const w = Math.max(1, Math.min(120, Number(windowMin) || 5));
      const q = new URLSearchParams({
        device_id: deviceId.trim(),
        around: toRFC3339Local(aroundLocal),
        window: `${w}m`,
        hops: hops.trim() || "1",
      });
      const rep = await apiGet<Report>(`/api/v1/postmortem?${q}`);
      setReport(rep);
      setParams(q, { replace: true });
    } catch (ex) {
      setReport(null);
      setErr(ex instanceof Error ? ex.message : "Ошибка загрузки");
    } finally {
      setLoading(false);
    }
  }, [deviceId, aroundLocal, windowMin, hops, setParams]);

  useEffect(() => {
    if (params.get("device_id")) {
      void load();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- только при первом открытии с query
  }, []);

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    void load();
  }

  return (
    <div style={{ maxWidth: 960, margin: "0 auto" }}>
      <h1 style={{ marginTop: 0 }}>Postmortem</h1>
      <p style={{ color: "#9aa3b5", marginBottom: "1.25rem" }}>
        Общий таймлайн вокруг момента на узле: события, SNMP traps, перемещения MAC, снимки конфига — плюс соседи по LLDP.
      </p>

      <form onSubmit={onSubmit} style={{ display: "grid", gap: 12, marginBottom: "1.5rem" }}>
        <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <span style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>Узел</span>
          <DeviceSearchSelect
            devices={devices}
            value={deviceId}
            onChange={setDeviceId}
            disabled={loading}
            ariaLabel="Узел для postmortem"
            inputStyle={inputStyle}
          />
        </label>
        <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr 1fr", gap: 12 }}>
          <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            <span style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>Момент (локальное время)</span>
            <input type="datetime-local" value={aroundLocal} onChange={(e) => setAroundLocal(e.target.value)} style={inputStyle} />
          </label>
          <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            <span style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>Окно (мин)</span>
            <input type="number" min={1} max={120} value={windowMin} onChange={(e) => setWindowMin(e.target.value)} style={inputStyle} />
          </label>
          <label style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            <span style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>LLDP-хопов</span>
            <select value={hops} onChange={(e) => setHops(e.target.value)} style={inputStyle}>
              <option value="0">0 — только узел</option>
              <option value="1">1 — прямые соседи</option>
              <option value="2">2</option>
              <option value="3">3</option>
            </select>
          </label>
        </div>
        <button type="submit" disabled={loading} style={btnPrimary}>
          {loading ? "Сбор…" : "Собрать таймлайн"}
        </button>
      </form>

      {err && (
        <p style={{ color: "#f88" }} role="alert">
          {err}
        </p>
      )}

      {report && (
        <>
          <p style={{ color: "#9aa3b5", fontSize: "0.9rem" }}>
            <Link to={`/devices/${report.device_id}`} state={deviceLinkState({ path: "/postmortem", label: "Postmortem" })}>
              {report.device_name}
            </Link>
            {" · "}
            {report.from.slice(0, 19)} — {report.to.slice(0, 19)} · окно {report.window} · узлов в области: {report.scope_devices.length}
          </p>
          {report.scope_devices.length > 1 && (
            <p style={{ fontSize: "0.8rem", color: "#7a8499", marginBottom: "1rem" }}>
              Область:{" "}
              {report.scope_devices.map((d) => (
                <span key={d.id} style={{ marginRight: 8 }}>
                  {d.hop > 0 ? `+${d.hop} ` : ""}
                  {d.name}
                </span>
              ))}
            </p>
          )}
          {report.timeline.length === 0 ? (
            <p style={{ color: "#9aa3b5" }}>В окне нет записей (события, traps, MAC moves, config).</p>
          ) : (
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.88rem" }}>
              <thead>
                <tr style={{ borderBottom: "1px solid #2e3648", color: "#9aa3b5", textAlign: "left" }}>
                  <th style={{ padding: "6px 8px" }}>Время</th>
                  <th style={{ padding: "6px 8px" }}>Тип</th>
                  <th style={{ padding: "6px 8px" }}>Узел</th>
                  <th style={{ padding: "6px 8px" }}>Суть</th>
                </tr>
              </thead>
              <tbody>
                {report.timeline.map((row, i) => (
                  <tr key={`${row.kind}-${row.at}-${i}`} style={{ borderBottom: "1px solid #1e2430" }}>
                    <td style={{ padding: "6px 8px", whiteSpace: "nowrap", color: "#9aa3b5" }}>
                      {row.at.replace("T", " ").slice(0, 19)}
                    </td>
                    <td style={{ padding: "6px 8px" }}>{kindLabel(row.kind)}</td>
                    <td style={{ padding: "6px 8px" }}>
                      <Link to={`/devices/${row.device_id}`} state={deviceLinkState({ path: "/postmortem", label: "Postmortem" })}>
                        {row.device_name ?? row.device_id}
                      </Link>
                      {row.if_index != null && <span style={{ color: "#7a8499" }}> · if {row.if_index}</span>}
                    </td>
                    <td style={{ padding: "6px 8px" }}>
                      {row.kind === "event" ? formatEventTypeLabel(row.summary) : row.summary}
                      {row.severity && (
                        <span style={{ marginLeft: 6, fontSize: "0.75rem", color: "#7a8499" }}>{row.severity}</span>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </>
      )}
    </div>
  );
}

const inputStyle: CSSProperties = {
  background: "#12161f",
  border: "1px solid #2e3648",
  borderRadius: 6,
  color: "#c8d0e0",
  padding: "8px 10px",
  fontSize: "0.9rem",
};

const btnPrimary: CSSProperties = {
  background: "#3b82f6",
  border: "none",
  borderRadius: 6,
  color: "#fff",
  padding: "10px 16px",
  fontWeight: 600,
  cursor: "pointer",
  width: "fit-content",
};
