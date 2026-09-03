import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { apiGet, asArray, apiPatch } from "../api";
import MACMoveMap, { type MoveGraphEdge, type MoveGraphNode } from "../components/MACMoveMap";
import { formatDateTimeRU } from "../dateFormat";
import { formatEventTypeLabel } from "../eventFormat";
import { useAuthRole } from "../hooks/useAuthRole";
import { formatMacDisplay } from "../macUtil";
import { deviceLinkState } from "../navigation";

type Confidence = "high" | "medium" | "low";

type Hypothesis = {
  id: string;
  confidence: Confidence;
  title: string;
  evidence: string[];
  suggested_checks: string[];
};

type Neighbor = {
  protocol: string;
  remote_sys_name?: string;
  remote_port_id?: string;
  remote_mgmt_addr?: string;
};

type Footprint = {
  device_id: number;
  device_name: string;
  device_host: string;
  device_category?: string;
  if_index: number;
  if_name?: string;
  if_descr?: string;
  port_role: string;
  vlan_id?: number;
  macs_on_port: number;
  last_seen_at: string;
  neighbors?: Neighbor[];
};

type Timeline = {
  id: number;
  mac: string;
  device_id: number;
  device_name?: string;
  device_host?: string;
  from_if_index?: number;
  to_if_index?: number;
  vlan_id?: number;
  seen_at: string;
  source: string;
};

type CorrEvent = {
  id: number;
  device_id: number;
  if_index?: number;
  event_type: string;
  severity: string;
  created_at: string;
};

type InvestigationStatus = {
  mac: string;
  status: "open" | "resolved" | "ignored";
  note?: string;
  updated_at?: string;
  updated_by_name?: string;
};

type FDBHistoryHit = {
  device_id: number;
  device_name?: string;
  device_host?: string;
  snapshot_id: number;
  snapshot_at: string;
  if_index: number;
  if_name?: string;
  if_descr?: string;
  vlan_id?: number;
};

type FDBHistoryPoint = {
  days_ago: number;
  target_at: string;
  hits: FDBHistoryHit[];
};

type L2PathHop = {
  device_id: number;
  device_name?: string;
  via_if_index?: number;
  via_if_name?: string;
};

type L2Path = {
  root_device_id: number;
  root_device_name?: string;
  target_device_id: number;
  target_if_index: number;
  target_if_name?: string;
  target_port_role?: string;
  hops: L2PathHop[];
  summary: string;
  note?: string;
};

type ShutImpactClient = {
  mac: string;
  mac_vendor?: string;
  ips?: string[];
  vlan_id?: number;
  device_id?: number;
  device_name?: string;
  last_seen?: string;
};

type ShutImpactNeighbor = {
  protocol: string;
  remote_sys_name?: string;
  remote_port_id?: string;
  remote_mgmt_addr?: string;
  remote_device_id?: number;
  remote_device_name?: string;
  looks_like_infra?: boolean;
};

type ShutImpact = {
  device_id: number;
  device_name: string;
  device_host?: string;
  if_index: number;
  if_name?: string;
  if_descr?: string;
  port_role: string;
  macs_on_port: number;
  clients: ShutImpactClient[];
  neighbors: ShutImpactNeighbor[];
  uplink_suspected: boolean;
  severity: string;
  warnings: string[];
  summary: string;
};

type Report = {
  identity: {
    mac: string;
    vendor?: string;
    locally_administered: boolean;
    virtualization_hint: boolean;
    ips?: string[];
    inventory_device_id?: number;
    inventory_name?: string;
  };
  investigation: InvestigationStatus;
  hypotheses: Hypothesis[];
  timeline: Timeline[];
  footprint: Footprint[];
  fdb_history?: FDBHistoryPoint[];
  l2_paths?: L2Path[];
  move_graph?: { nodes: MoveGraphNode[]; edges: MoveGraphEdge[] };
  correlated_events: CorrEvent[];
  wifi_untracked?: boolean;
  wifi_untracked_note?: string;
  generated_at: string;
};

type Flapper = {
  mac: string;
  move_count: number;
  device_count: number;
  last_seen_at: string;
  mac_vendor?: string;
  has_flap_event: boolean;
  investigation_status?: string;
};

function confColor(c: Confidence): string {
  switch (c) {
    case "high":
      return "#e8a0a0";
    case "medium":
      return "#e8c98a";
    default:
      return "#9aa3b5";
  }
}

function confLabel(c: Confidence): string {
  switch (c) {
    case "high":
      return "высокая";
    case "medium":
      return "средняя";
    default:
      return "низкая";
  }
}

function statusLabel(s: string): string {
  switch (s) {
    case "resolved":
      return "закрыто";
    case "ignored":
      return "игнор";
    default:
      return "открыто";
  }
}

function statusColor(s: string): string {
  switch (s) {
    case "resolved":
      return "#8d8";
    case "ignored":
      return "#9aa3b5";
    default:
      return "#e8c98a";
  }
}

export default function InvestigateMAC() {
  const { canWrite } = useAuthRole();
  const [params, setParams] = useSearchParams();
  const initial = (params.get("mac") ?? "").trim();
  const [macInput, setMacInput] = useState(initial);
  const [report, setReport] = useState<Report | null>(null);
  const [flappers, setFlappers] = useState<Flapper[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [hideIgnored, setHideIgnored] = useState(true);
  const [statusNote, setStatusNote] = useState("");
  const [statusBusy, setStatusBusy] = useState(false);
  const [shutBusy, setShutBusy] = useState<string | null>(null);
  const [actionMsg, setActionMsg] = useState<string | null>(null);
  const [shutImpact, setShutImpact] = useState<ShutImpact | null>(null);
  const [shutImpactLoading, setShutImpactLoading] = useState(false);
  const [shutPreviewKey, setShutPreviewKey] = useState<string | null>(null);
  const [shutAckUplink, setShutAckUplink] = useState(false);

  const multiAccessBanner = useMemo(() => {
    if (!report?.footprint?.length) return null;
    const accessByDev = new Map<number, Footprint[]>();
    for (const f of report.footprint) {
      const role = f.port_role || "access";
      if (role !== "access") continue;
      if ((f.macs_on_port ?? 0) >= 8) continue;
      const list = accessByDev.get(f.device_id) ?? [];
      list.push(f);
      accessByDev.set(f.device_id, list);
    }
    if (accessByDev.size < 2) return null;
    const n = [...accessByDev.values()].reduce((s, xs) => s + xs.length, 0);
    return `MAC одновременно на ${n} access-порт(ах) ${accessByDev.size} свитчей → дубликат MAC или петля через чужой сегмент`;
  }, [report]);

  const openShutPreview = async (f: Footprint) => {
    if (!canWrite) return;
    const key = `${f.device_id}:${f.if_index}`;
    setErr(null);
    setActionMsg(null);
    setShutAckUplink(false);
    setShutPreviewKey(key);
    setShutImpactLoading(true);
    setShutImpact(null);
    try {
      const impact = await apiGet<ShutImpact>(
        `/api/v1/devices/${f.device_id}/interfaces/${f.if_index}/shut-impact`,
      );
      setShutImpact(impact);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Не удалось загрузить превью shutdown");
      setShutPreviewKey(null);
    } finally {
      setShutImpactLoading(false);
    }
  };

  const confirmShut = async () => {
    if (!canWrite || !shutImpact) return;
    if (shutImpact.uplink_suspected && !shutAckUplink) {
      setErr("Подтвердите, что понимаете риск uplink (галочка в диалоге).");
      return;
    }
    const key = `${shutImpact.device_id}:${shutImpact.if_index}`;
    const label = shutImpact.if_name || String(shutImpact.if_index);
    setShutBusy(key);
    setErr(null);
    try {
      const res = await apiPatch<{ ok: boolean; via?: string }>(
        `/api/v1/devices/${shutImpact.device_id}/interfaces/${shutImpact.if_index}/admin`,
        { admin_up: false },
      );
      const via = res.via ? ` (${res.via})` : "";
      setActionMsg(`Порт ${label} на ${shutImpact.device_name}: выключен${via}. Обновите отчёт через минуту.`);
      setShutImpact(null);
      load(report?.identity.mac ?? macInput);
    } catch (e) {
      setErr(e instanceof Error ? e.message : "Ошибка shutdown порта");
    } finally {
      setShutBusy(null);
    }
  };

  const loadFlappers = useCallback(() => {
    const q = hideIgnored ? "hide_ignored=1" : "hide_ignored=0";
    apiGet<{ items?: Flapper[] }>(`/api/v1/investigate/mac/flappers?hours=24&min_moves=2&limit=40&${q}`)
      .then((r) => setFlappers(Array.isArray(r.items) ? r.items : []))
      .catch(() => setFlappers([]));
  }, [hideIgnored]);

  const load = useCallback(
    (mac: string) => {
      const q = mac.trim();
      if (!q) {
        setErr("Укажите MAC");
        return;
      }
      setLoading(true);
      setErr(null);
      setParams({ mac: q }, { replace: true });
      apiGet<Report>(`/api/v1/investigate/mac?mac=${encodeURIComponent(q)}`)
        .then((r) => {
          const report: Report = {
            ...r,
            footprint: asArray(r.footprint),
            timeline: asArray(r.timeline),
            hypotheses: asArray(r.hypotheses),
            fdb_history: asArray(r.fdb_history),
            l2_paths: asArray(r.l2_paths),
            correlated_events: asArray(r.correlated_events),
            move_graph: {
              nodes: asArray(r.move_graph?.nodes),
              edges: asArray(r.move_graph?.edges),
            },
          };
          setReport(report);
          setStatusNote(report.investigation?.note ?? "");
        })
        .catch((e: Error) => {
          setReport(null);
          if (e.name === "AbortError") return;
          let msg = e.message;
          try {
            const j = JSON.parse(msg) as { error?: string };
            if (j.error) msg = j.error;
          } catch {
            /* не JSON */
          }
          setErr(msg);
        })
        .finally(() => setLoading(false));
    },
    [setParams],
  );

  useEffect(() => {
    loadFlappers();
  }, [loadFlappers]);

  useEffect(() => {
    if (initial) load(initial);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  const setInvestigationStatus = (status: "open" | "resolved" | "ignored") => {
    if (!report?.identity.mac) return;
    setStatusBusy(true);
    apiPatch<InvestigationStatus>(
      `/api/v1/investigate/mac/status?mac=${encodeURIComponent(report.identity.mac)}`,
      { status, note: statusNote.trim() || undefined },
    )
      .then((inv) => {
        setReport((prev) => (prev ? { ...prev, investigation: inv } : prev));
        loadFlappers();
      })
      .catch((e: Error) => setErr(e.message))
      .finally(() => setStatusBusy(false));
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    load(macInput);
  };

  return (
    <div>
      <h1 style={{ marginTop: 0 }}>Расследование MAC</h1>
      <p style={{ color: "#9aa3b5", maxWidth: 780 }}>
        Карта перемещений и гипотезы причин flapping: цепочка hop&apos;ов FDB + syslog. Выберите «горячий» MAC ниже или введите адрес вручную.
      </p>
      <form onSubmit={onSubmit} style={{ display: "flex", gap: 8, flexWrap: "wrap", marginBottom: "1.25rem" }}>
        <input
          value={macInput}
          onChange={(e) => setMacInput(e.target.value)}
          placeholder="52:54:4c:83:09:e0"
          style={{ minWidth: 220, fontFamily: "ui-monospace, monospace" }}
          aria-label="MAC"
        />
        <button type="submit" disabled={loading}>
          {loading ? "…" : "Расследовать"}
        </button>
      </form>
      {err && <p style={{ color: "#f88" }}>{err}</p>}

      <section style={{ marginBottom: "1.5rem" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, flexWrap: "wrap" }}>
          <h2 style={{ fontSize: "1.1rem", margin: 0 }}>Горячие MAC (24 ч)</h2>
          <label style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>
            <input
              type="checkbox"
              checked={hideIgnored}
              onChange={(e) => setHideIgnored(e.target.checked)}
            />{" "}
            скрыть игнорируемые
          </label>
        </div>
        {flappers.length === 0 ? (
          <p style={{ color: "#9aa3b5" }}>
            Пока нет частых перемещений. После опросов FDB или syslog здесь появятся кандидаты на flapping.
          </p>
        ) : (
          <table style={{ width: "100%", fontSize: "0.9rem" }}>
            <thead>
              <tr>
                <th>MAC</th>
                <th>Переходов</th>
                <th>Узлов</th>
                <th>Последний</th>
                <th>Статус</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {flappers.map((f) => (
                <tr key={f.mac}>
                  <td style={{ fontFamily: "ui-monospace, monospace" }}>
                    {formatMacDisplay(f.mac)}
                    {f.mac_vendor ? (
                      <span style={{ color: "#9aa3b5", marginLeft: 6 }}>({f.mac_vendor})</span>
                    ) : null}
                    {f.has_flap_event ? (
                      <span style={{ color: "#e8a0a0", marginLeft: 8, fontSize: "0.8rem" }}>FLAP</span>
                    ) : null}
                  </td>
                  <td>{f.move_count}</td>
                  <td>{f.device_count}</td>
                  <td style={{ whiteSpace: "nowrap" }}>{new Date(f.last_seen_at).toLocaleString()}</td>
                  <td style={{ color: statusColor(f.investigation_status ?? "open"), fontSize: "0.85rem" }}>
                    {statusLabel(f.investigation_status ?? "open")}
                  </td>
                  <td>
                    <button
                      type="button"
                      onClick={() => {
                        setMacInput(f.mac);
                        load(f.mac);
                      }}
                    >
                      Карта
                    </button>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>

      {report && (
        <>
          {report.wifi_untracked && report.wifi_untracked_note ? (
            <div
              role="status"
              style={{
                marginBottom: "1rem",
                padding: "0.75rem 1rem",
                borderRadius: 6,
                border: "1px solid #4a5568",
                background: "#1a2030",
                color: "#b8c0d0",
              }}
            >
              {report.wifi_untracked_note}{" "}
              <Link to="/settings?tab=mac">Настройки → MAC</Link>
            </div>
          ) : null}
          {multiAccessBanner ? (
            <div
              role="alert"
              style={{
                marginBottom: "1rem",
                padding: "0.75rem 1rem",
                borderRadius: 6,
                border: "1px solid #8a3030",
                background: "#2a1515",
                color: "#f0b4b4",
              }}
            >
              <strong>Аномалия footprint:</strong> {multiAccessBanner}
            </div>
          ) : null}
          {actionMsg ? (
            <p style={{ color: "#8d8", marginBottom: "1rem" }}>{actionMsg}</p>
          ) : null}

          <section style={{ marginBottom: "1.5rem", padding: "0.75rem 1rem", background: "#141820", borderRadius: 6, border: "1px solid #2a3344" }}>
            <div style={{ display: "flex", gap: 12, alignItems: "center", flexWrap: "wrap" }}>
              <strong>Расследование:</strong>
              <span style={{ color: statusColor(report.investigation?.status ?? "open") }}>
                {statusLabel(report.investigation?.status ?? "open")}
              </span>
              {(() => {
                const at = formatDateTimeRU(report.investigation?.updated_at);
                return at ? (
                  <span style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>
                    {at}
                    {report.investigation?.updated_by_name ? ` · ${report.investigation.updated_by_name}` : ""}
                  </span>
                ) : null;
              })()}
            </div>
            {canWrite ? (
              <div style={{ marginTop: 10, display: "flex", flexWrap: "wrap", gap: 8, alignItems: "flex-end" }}>
                <label style={{ flex: "1 1 200px" }}>
                  Заметка
                  <br />
                  <input
                    value={statusNote}
                    onChange={(e) => setStatusNote(e.target.value)}
                    placeholder="причина / что сделали"
                    style={{ width: "100%" }}
                  />
                </label>
                <button type="button" disabled={statusBusy} onClick={() => setInvestigationStatus("resolved")}>
                  Закрыть (resolved)
                </button>
                <button type="button" disabled={statusBusy} onClick={() => setInvestigationStatus("ignored")}>
                  Игнорировать
                </button>
                {(report.investigation?.status === "resolved" || report.investigation?.status === "ignored") && (
                  <button type="button" disabled={statusBusy} onClick={() => setInvestigationStatus("open")}>
                    Снова открыть
                  </button>
                )}
              </div>
            ) : report.investigation?.note ? (
              <p style={{ margin: "0.5rem 0 0", color: "#c5cedd" }}>{report.investigation.note}</p>
            ) : null}
          </section>

          <section style={{ marginBottom: "1.5rem" }}>
            <h2 style={{ fontSize: "1.1rem" }}>Идентичность</h2>
            <div style={{ fontFamily: "ui-monospace, monospace" }}>{formatMacDisplay(report.identity.mac)}</div>
            <ul style={{ color: "#c5cedd", lineHeight: 1.5 }}>
              {report.identity.vendor ? <li>Vendor: {report.identity.vendor}</li> : null}
              <li>LAA: {report.identity.locally_administered ? "да" : "нет"}</li>
              <li>Виртуализация (эвристика): {report.identity.virtualization_hint ? "да" : "нет"}</li>
              {report.identity.ips && report.identity.ips.length > 0 ? (
                <li>ARP IP: {report.identity.ips.join(", ")}</li>
              ) : null}
              {report.identity.inventory_device_id != null ? (
                <li>
                  Inventory:{" "}
                  <Link
                    to={`/devices/${report.identity.inventory_device_id}`}
                    state={deviceLinkState({
                      path: `/investigate/mac?mac=${encodeURIComponent(report.identity.mac)}`,
                      label: "Расследование MAC",
                    })}
                  >
                    {report.identity.inventory_name || `#${report.identity.inventory_device_id}`}
                  </Link>
                </li>
              ) : null}
            </ul>
          </section>

          <section style={{ marginBottom: "1.5rem" }}>
            <h2 style={{ fontSize: "1.1rem" }}>Цепочка перемещений MAC</h2>
            <p style={{ color: "#9aa3b5", fontSize: "0.85rem", marginTop: 0 }}>
              Пошаговая история hop&apos;ов между портами (FDB/syslog), не схема топологии.
            </p>
            <MACMoveMap
              mac={report.identity.mac}
              nodes={report.move_graph?.nodes ?? []}
              edges={report.move_graph?.edges ?? []}
              timeline={report.timeline}
            />
          </section>

          <section style={{ marginBottom: "1.5rem" }}>
            <h2 style={{ fontSize: "1.1rem" }}>Гипотезы — откуда flapping</h2>
            {report.hypotheses.map((h) => (
              <div
                key={h.id}
                style={{
                  border: "1px solid #2a3344",
                  borderRadius: 6,
                  padding: "0.75rem 1rem",
                  marginBottom: "0.75rem",
                  background: "#141820",
                }}
              >
                <div style={{ display: "flex", gap: 10, alignItems: "baseline", flexWrap: "wrap" }}>
                  <strong>{h.title}</strong>
                  <span style={{ color: confColor(h.confidence), fontSize: "0.85rem" }}>
                    уверенность: {confLabel(h.confidence)}
                  </span>
                  <span style={{ color: "#6d7689", fontSize: "0.8rem" }}>{h.id}</span>
                </div>
                <div style={{ marginTop: 8 }}>
                  <div style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>Доказательства</div>
                  <ul style={{ margin: "0.25rem 0 0.5rem" }}>
                    {h.evidence.map((e, i) => (
                      <li key={i}>{e}</li>
                    ))}
                  </ul>
                  <div style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>Что проверить</div>
                  <ul style={{ margin: "0.25rem 0 0" }}>
                    {h.suggested_checks.map((e, i) => (
                      <li key={i}>{e}</li>
                    ))}
                  </ul>
                </div>
              </div>
            ))}
          </section>

          <section style={{ marginBottom: "1.5rem" }}>
            <h2 style={{ fontSize: "1.1rem" }}>Путь к устройству (L2 / LLDP)</h2>
            <p style={{ color: "#9aa3b5", fontSize: "0.85rem", marginTop: 0 }}>
              BFS от узла с наибольшим числом LLDP-связей (эвристика core) до access-порта, где MAC в текущем
              FDB. Не путать с «физическим патч-кордом» — только видимая LLDP-топология.
            </p>
            {!report.l2_paths || report.l2_paths.length === 0 ? (
              <p style={{ color: "#9aa3b5" }}>Нет access-хитов с малым числом MAC или нет LLDP-графа.</p>
            ) : (
              report.l2_paths.map((p, i) => (
                <div
                  key={`${p.target_device_id}-${p.target_if_index}-${i}`}
                  style={{
                    border: "1px solid #2a3344",
                    borderRadius: 6,
                    padding: "0.6rem 0.85rem",
                    marginBottom: "0.5rem",
                    background: "#141820",
                  }}
                >
                  <div style={{ fontFamily: "ui-monospace, monospace", fontSize: "0.9rem" }}>{p.summary}</div>
                  {p.note ? <div style={{ color: "#9aa3b5", fontSize: "0.8rem", marginTop: 4 }}>{p.note}</div> : null}
                </div>
              ))
            )}
          </section>

          <section style={{ marginBottom: "1.5rem" }}>
            <h2 style={{ fontSize: "1.1rem" }}>
              Где сейчас (FDB){" "}
              <span style={{ fontSize: "0.85rem", color: "#9aa3b5", fontWeight: 400 }}>
                (подозрительные access сверху)
              </span>
            </h2>
            <p style={{ color: "#9aa3b5", fontSize: "0.85rem", marginTop: 0 }}>
              Снимок = <em>последний</em> порт, который увидел poller (не «железо навечно»). Читайте{" "}
              <strong>роль</strong> + <strong>MAC на порту</strong>: access + мало MAC → сильный сигнал
              физического подключения; access + ≥8 MAC → скорее uplink; trunk → идите upstream по соседям.
            </p>
            {report.footprint.length === 0 ? (
              <p style={{ color: "#9aa3b5" }}>Сейчас в снимках FDB не найден.</p>
            ) : (
              <table style={{ width: "100%", fontSize: "0.9rem" }}>
                <thead>
                  <tr>
                    <th>Узел</th>
                    <th>Порт</th>
                    <th>Роль</th>
                    <th>VLAN</th>
                    <th>MAC на порту</th>
                    <th>Соседи</th>
                    {canWrite ? <th>Действие</th> : null}
                  </tr>
                </thead>
                <tbody>
                  {report.footprint.map((f) => {
                    const busyKey = `${f.device_id}:${f.if_index}`;
                    const role = f.port_role || "access";
                    const suspect = role === "access" && (f.macs_on_port ?? 0) <= 5;
                    const hint =
                      role === "access" && (f.macs_on_port ?? 0) >= 8
                        ? "похож на uplink"
                        : role === "trunk"
                          ? "через uplink"
                          : suspect
                            ? "⚠ источник?"
                            : role === "access"
                              ? "сильный сигнал"
                              : "";
                    return (
                      <tr
                        key={`${f.device_id}-${f.if_index}`}
                        style={suspect ? { background: "rgba(232, 201, 138, 0.12)" } : undefined}
                      >
                        <td>
                          <Link
                            to={`/devices/${f.device_id}`}
                            state={deviceLinkState({
                              path: `/investigate/mac?mac=${encodeURIComponent(report.identity.mac)}`,
                              label: "Расследование MAC",
                            })}
                          >
                            {f.device_name}
                          </Link>
                          <div style={{ color: "#9aa3b5", fontSize: "0.8rem" }}>{f.device_host}</div>
                        </td>
                        <td>
                          {f.if_name || f.if_index}
                          {f.if_descr ? (
                            <div style={{ color: "#9aa3b5", fontSize: "0.8rem" }}>{f.if_descr}</div>
                          ) : null}
                        </td>
                        <td>
                          {f.port_role || "—"}
                          {hint ? (
                            <div style={{ color: "#e8c98a", fontSize: "0.75rem" }}>{hint}</div>
                          ) : null}
                        </td>
                        <td>{f.vlan_id ?? "—"}</td>
                        <td>{f.macs_on_port}</td>
                        <td style={{ fontSize: "0.85rem" }}>
                          {(f.neighbors ?? []).length === 0
                            ? "—"
                            : (f.neighbors ?? []).map((n, i) => (
                                <div key={i}>
                                  {n.protocol}: {n.remote_sys_name || n.remote_mgmt_addr || "?"}
                                  {n.remote_port_id ? ` (${n.remote_port_id})` : ""}
                                </div>
                              ))}
                        </td>
                        {canWrite ? (
                          <td>
                            <button
                              type="button"
                              disabled={shutBusy === busyKey || shutPreviewKey === busyKey}
                              title="Сначала показать, кто отвалится за портом"
                              onClick={() => openShutPreview(f)}
                            >
                              {shutBusy === busyKey || shutPreviewKey === busyKey ? "…" : "Shut"}
                            </button>
                          </td>
                        ) : null}
                      </tr>
                    );
                  })}
                </tbody>
              </table>
            )}
          </section>

          <section style={{ marginBottom: "1.5rem" }}>
            <h2 style={{ fontSize: "1.1rem" }}>История FDB (ежедневные снимки)</h2>
            {!report.fdb_history || report.fdb_history.length === 0 ? (
              <p style={{ color: "#9aa3b5" }}>
                Пока нет архивных снимков. Они накапливаются после первого суточного FDB-poll (интервал{" "}
                <code>FDB_SNAPSHOT_INTERVAL_HOURS</code>).
              </p>
            ) : (
              report.fdb_history.map((pt) => (
                <div key={pt.days_ago} style={{ marginBottom: "1rem" }}>
                  <h3 style={{ fontSize: "0.95rem", color: "#c8d0e0" }}>
                    ~{pt.days_ago} сут. назад ({new Date(pt.target_at).toLocaleString("ru-RU")})
                  </h3>
                  {pt.hits.length === 0 ? (
                    <p style={{ color: "#9aa3b5", margin: "0.25rem 0 0" }}>В снимках не найден.</p>
                  ) : (
                    <table style={{ width: "100%", fontSize: "0.9rem" }}>
                      <thead>
                        <tr>
                          <th>Узел</th>
                          <th>Порт</th>
                          <th>VLAN</th>
                          <th>Снимок</th>
                        </tr>
                      </thead>
                      <tbody>
                        {pt.hits.map((h) => (
                          <tr key={`${pt.days_ago}-${h.device_id}-${h.if_index}`}>
                            <td>
                              <Link
                                to={`/devices/${h.device_id}`}
                                state={deviceLinkState({
                                  path: `/investigate/mac?mac=${encodeURIComponent(report.identity.mac)}`,
                                  label: "Расследование MAC",
                                })}
                              >
                                {h.device_name || `#${h.device_id}`}
                              </Link>
                              {h.device_host ? (
                                <div style={{ color: "#9aa3b5", fontSize: "0.8rem" }}>{h.device_host}</div>
                              ) : null}
                            </td>
                            <td>
                              {h.if_name || h.if_index}
                              {h.if_descr ? (
                                <div style={{ color: "#9aa3b5", fontSize: "0.8rem" }}>{h.if_descr}</div>
                              ) : null}
                            </td>
                            <td>{h.vlan_id ?? "—"}</td>
                            <td style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>
                              {new Date(h.snapshot_at).toLocaleString("ru-RU")}
                            </td>
                          </tr>
                        ))}
                      </tbody>
                    </table>
                  )}
                </div>
              ))
            )}
          </section>

          <section style={{ marginBottom: "1.5rem" }}>
            <h2 style={{ fontSize: "1.1rem" }}>История перемещений</h2>
            {report.timeline.length === 0 ? (
              <p style={{ color: "#9aa3b5" }}>Пока нет записей — дождитесь FDB-опросов или syslog.</p>
            ) : (
              <table style={{ width: "100%", fontSize: "0.9rem" }}>
                <thead>
                  <tr>
                    <th>Время</th>
                    <th>Узел</th>
                    <th>Переход</th>
                    <th>Источник</th>
                  </tr>
                </thead>
                <tbody>
                  {report.timeline.map((m) => (
                    <tr key={m.id}>
                      <td style={{ whiteSpace: "nowrap" }}>{new Date(m.seen_at).toLocaleString()}</td>
                      <td>
                        <Link to={`/devices/${m.device_id}`}>{m.device_name || `#${m.device_id}`}</Link>
                      </td>
                      <td>
                        {m.from_if_index != null ? m.from_if_index : "—"} →{" "}
                        {m.to_if_index != null ? m.to_if_index : "—"}
                        {m.vlan_id != null ? ` (vlan ${m.vlan_id})` : ""}
                      </td>
                      <td>{m.source}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
          </section>

          {report.correlated_events && report.correlated_events.length > 0 ? (
            <section>
              <h2 style={{ fontSize: "1.1rem" }}>Связанные события (24 ч)</h2>
              <ul style={{ fontSize: "0.9rem" }}>
                {report.correlated_events.slice(0, 30).map((ev) => (
                  <li key={ev.id}>
                    {new Date(ev.created_at).toLocaleString()} · {formatEventTypeLabel(ev.event_type)}
                    {ev.if_index != null ? ` · if ${ev.if_index}` : ""} · {ev.severity}
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </>
      )}

      {(shutImpact || shutImpactLoading) && (
        <div
          role="dialog"
          aria-modal="true"
          style={{
            position: "fixed",
            inset: 0,
            background: "rgba(0,0,0,0.55)",
            zIndex: 1000,
            display: "flex",
            alignItems: "center",
            justifyContent: "center",
            padding: 16,
          }}
          onClick={() => {
            if (!shutBusy) {
              setShutImpact(null);
              setShutPreviewKey(null);
            }
          }}
        >
          <div
            style={{
              width: "min(640px, 100%)",
              maxHeight: "90vh",
              overflow: "auto",
              background: "#12161e",
              border: "1px solid #2a3344",
              borderRadius: 8,
              padding: "1rem 1.1rem",
              boxShadow: "0 12px 40px rgba(0,0,0,0.45)",
            }}
            onClick={(e) => e.stopPropagation()}
          >
            <h2 style={{ marginTop: 0, fontSize: "1.15rem" }}>Перед shutdown порта</h2>
            {shutImpactLoading && !shutImpact ? (
              <p style={{ color: "#9aa3b5" }}>Смотрим FDB и LLDP за портом…</p>
            ) : shutImpact ? (
              <>
                <p style={{ marginTop: 0 }}>
                  <strong>
                    {shutImpact.device_name}
                    {shutImpact.device_host ? ` (${shutImpact.device_host})` : ""}
                  </strong>
                  {" · "}
                  {shutImpact.if_name || `if${shutImpact.if_index}`}
                  {shutImpact.if_descr ? ` — ${shutImpact.if_descr}` : ""}
                  {" · роль "}
                  {shutImpact.port_role}
                </p>
                <p
                  style={{
                    color:
                      shutImpact.severity === "critical"
                        ? "#f0b4b4"
                        : shutImpact.severity === "warning"
                          ? "#e8c98a"
                          : "#9aa3b5",
                  }}
                >
                  {shutImpact.summary}
                </p>
                {shutImpact.warnings?.length ? (
                  <ul style={{ color: "#f0b4b4", fontSize: "0.9rem" }}>
                    {shutImpact.warnings.map((w, i) => (
                      <li key={i}>{w}</li>
                    ))}
                  </ul>
                ) : null}

                <h3 style={{ fontSize: "0.95rem", marginBottom: 6 }}>Устройства в FDB за портом</h3>
                {shutImpact.clients.length === 0 ? (
                  <p style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>Пусто в текущем снимке FDB.</p>
                ) : (
                  <table style={{ width: "100%", fontSize: "0.85rem", marginBottom: 12 }}>
                    <thead>
                      <tr>
                        <th>MAC</th>
                        <th>IP / узел</th>
                        <th>Vendor</th>
                      </tr>
                    </thead>
                    <tbody>
                      {shutImpact.clients.map((c) => (
                        <tr key={c.mac}>
                          <td style={{ fontFamily: "ui-monospace, monospace" }}>{c.mac}</td>
                          <td>
                            {c.device_name ? (
                              <Link to={`/devices/${c.device_id}`}>{c.device_name}</Link>
                            ) : null}
                            {c.ips?.length ? (
                              <div style={{ color: "#9aa3b5" }}>{c.ips.join(", ")}</div>
                            ) : !c.device_name ? (
                              "—"
                            ) : null}
                          </td>
                          <td>{c.mac_vendor || "—"}</td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                )}

                <h3 style={{ fontSize: "0.95rem", marginBottom: 6 }}>LLDP/CDP соседи</h3>
                {shutImpact.neighbors.length === 0 ? (
                  <p style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>Соседей нет.</p>
                ) : (
                  <ul style={{ fontSize: "0.85rem", marginTop: 0 }}>
                    {shutImpact.neighbors.map((n, i) => (
                      <li key={i} style={{ color: n.looks_like_infra ? "#f0b4b4" : undefined }}>
                        {n.protocol}:{" "}
                        {n.remote_device_name ? (
                          <Link to={`/devices/${n.remote_device_id}`}>{n.remote_device_name}</Link>
                        ) : (
                          n.remote_sys_name || n.remote_mgmt_addr || "?"
                        )}
                        {n.remote_port_id ? ` · ${n.remote_port_id}` : ""}
                        {n.looks_like_infra ? " · похож на коммутатор" : ""}
                      </li>
                    ))}
                  </ul>
                )}

                {shutImpact.uplink_suspected ? (
                  <label style={{ display: "flex", gap: 8, alignItems: "flex-start", margin: "12px 0", color: "#f0b4b4" }}>
                    <input
                      type="checkbox"
                      checked={shutAckUplink}
                      onChange={(e) => setShutAckUplink(e.target.checked)}
                    />
                    <span>
                      Понимаю риск: это может быть uplink — могу потерять доступ к свитчу{" "}
                      <strong>{shutImpact.device_name}</strong> и/или сегменту за портом.
                    </span>
                  </label>
                ) : null}

                <div style={{ display: "flex", gap: 8, justifyContent: "flex-end", marginTop: 16 }}>
                  <button
                    type="button"
                    disabled={!!shutBusy}
                    onClick={() => {
                      setShutImpact(null);
                      setShutPreviewKey(null);
                    }}
                  >
                    Отмена
                  </button>
                  <button
                    type="button"
                    disabled={
                      !!shutBusy || (shutImpact.uplink_suspected && !shutAckUplink)
                    }
                    style={{
                      background: shutImpact.uplink_suspected ? "#6a2020" : undefined,
                      color: shutImpact.uplink_suspected ? "#fff" : undefined,
                    }}
                    onClick={() => confirmShut()}
                  >
                    {shutBusy ? "Выключаю…" : "Всё равно выключить порт"}
                  </button>
                </div>
              </>
            ) : null}
          </div>
        </div>
      )}
    </div>
  );
}
