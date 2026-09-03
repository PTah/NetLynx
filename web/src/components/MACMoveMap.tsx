import { useMemo } from "react";
import { Link } from "react-router-dom";
import { deviceLinkState } from "../navigation";

export type MoveGraphNode = {
  id: string;
  device_id: number;
  device_name?: string;
  device_host?: string;
  if_index: number;
  if_name?: string;
  label: string;
};

export type MoveGraphEdge = {
  from: string;
  to: string;
  count: number;
  last_seen: string;
  sources?: string[];
};

/** Одна запись timeline FDB/syslog с from→to. */
export type MoveTimelineStep = {
  device_id: number;
  device_name?: string;
  from_if_index?: number;
  to_if_index?: number;
  seen_at: string;
  source: string;
};

type Props = {
  nodes: MoveGraphNode[];
  edges: MoveGraphEdge[];
  timeline?: MoveTimelineStep[];
  mac: string;
};

function nodeKey(deviceId: number, ifIndex: number): string {
  return `${deviceId}:${ifIndex}`;
}

function sourceLabel(src: string): string {
  switch (src) {
    case "fdb_poll":
      return "FDB";
    case "syslog":
      return "syslog";
    default:
      return src || "—";
  }
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

/** Цепочка перемещений MAC: нумерованный список hop'ов + где сейчас без истории переходов. */
export default function MACMoveMap({ nodes, edges, timeline, mac }: Props) {
  const steps = timeline ?? [];
  const labelByKey = useMemo(() => {
    const m = new Map<string, MoveGraphNode>();
    for (const n of nodes) {
      m.set(n.id, n);
    }
    return m;
  }, [nodes]);

  const hops = useMemo(() => {
    return steps
      .filter(
        (t) =>
          t.from_if_index != null &&
          t.to_if_index != null &&
          t.from_if_index !== t.to_if_index,
      )
      .slice()
      .sort((a, b) => new Date(a.seen_at).getTime() - new Date(b.seen_at).getTime());
  }, [steps]);

  const portLabel = (deviceId: number, ifIndex: number, deviceName?: string): string => {
    const n = labelByKey.get(nodeKey(deviceId, ifIndex));
    if (n?.label) return n.label;
    const dev = deviceName?.trim() || `#${deviceId}`;
    return `${dev} · if${ifIndex}`;
  };

  const usedPortKeys = useMemo(() => {
    const s = new Set<string>();
    for (const h of hops) {
      if (h.from_if_index != null) s.add(nodeKey(h.device_id, h.from_if_index));
      if (h.to_if_index != null) s.add(nodeKey(h.device_id, h.to_if_index));
    }
    for (const e of edges) {
      s.add(e.from);
      s.add(e.to);
    }
    return s;
  }, [hops, edges]);

  /** Порты из FDB footprint, для которых нет hop в истории — «одиночные круги» в старой карте. */
  const footprintOnly = useMemo(
    () => nodes.filter((n) => !usedPortKeys.has(n.id)),
    [nodes, usedPortKeys],
  );

  const pairSummary = useMemo(() => {
    if (edges.length === 0) return [];
    return edges
      .slice()
      .sort((a, b) => new Date(b.last_seen).getTime() - new Date(a.last_seen).getTime())
      .map((e) => {
        const fromN = labelByKey.get(e.from);
        const toN = labelByKey.get(e.to);
        return {
          key: `${e.from}-${e.to}`,
          fromLabel: fromN?.label ?? e.from,
          toLabel: toN?.label ?? e.to,
          count: e.count,
          lastSeen: e.last_seen,
          sources: e.sources?.join(", ") ?? "",
        };
      });
  }, [edges, labelByKey]);

  const backState = deviceLinkState({
    path: `/investigate/mac?mac=${encodeURIComponent(mac)}`,
    label: "Расследование MAC",
  });

  const PortLink = ({
    deviceId,
    text,
  }: {
    deviceId: number;
    ifIndex?: number;
    text: string;
  }) => (
    <Link to={`/devices/${deviceId}`} state={backState} style={{ color: "#9ec1ff" }}>
      {text}
    </Link>
  );

  if (hops.length === 0 && nodes.length === 0) {
    return (
      <p style={{ color: "#9aa3b5" }}>
        Нет перемещений между портами — нужны записи FDB/syslog с from→to в истории.
      </p>
    );
  }

  return (
    <div style={{ border: "1px solid #2a3344", borderRadius: 8, background: "#0f131a", padding: "0.75rem 1rem" }}>
      <p style={{ margin: "0 0 0.75rem", color: "#9aa3b5", fontSize: "0.85rem", lineHeight: 1.45 }}>
        Каждый шаг — один зафиксированный hop MAC <strong>с порта A на порт B</strong> (на том же или другом
        свитче). Нумерация по времени: от старых к новым. Это не топология L2 — только история FDB.
      </p>

      {hops.length > 0 ? (
        <>
          {hops.length <= 8 && (
            <div
              style={{
                display: "flex",
                flexWrap: "wrap",
                alignItems: "center",
                gap: "0.35rem 0.5rem",
                marginBottom: "1rem",
                padding: "0.5rem 0.65rem",
                background: "#141820",
                borderRadius: 6,
                fontSize: "0.82rem",
                color: "#c5cedd",
              }}
            >
              {hops.map((h, i) => {
                const fromIdx = h.from_if_index!;
                const toIdx = h.to_if_index!;
                const fromText = portLabel(h.device_id, fromIdx, h.device_name);
                const toText = portLabel(h.device_id, toIdx, h.device_name);
                return (
                  <span key={`${h.seen_at}-${i}`} style={{ display: "inline-flex", alignItems: "center", gap: 4 }}>
                    {i > 0 && <span style={{ color: "#5a6478" }}>→</span>}
                    <span
                      style={{
                        display: "inline-flex",
                        alignItems: "center",
                        gap: 4,
                        padding: "2px 6px",
                        borderRadius: 4,
                        background: "#1c2433",
                        border: "1px solid #3a4558",
                      }}
                    >
                      <span style={{ color: "#8ab4ff", fontWeight: 600 }}>{i + 1}</span>
                      <span style={{ color: "#7a8499" }}>{fromText}</span>
                      <span style={{ color: "#5a6478" }}>→</span>
                      <span>{toText}</span>
                    </span>
                  </span>
                );
              })}
            </div>
          )}

          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", borderCollapse: "collapse", fontSize: "0.88rem" }}>
              <thead>
                <tr style={{ textAlign: "left", color: "#9aa3b5", borderBottom: "1px solid #2a3344" }}>
                  <th style={{ padding: "0.4rem 0.5rem", width: 36 }}>№</th>
                  <th style={{ padding: "0.4rem 0.5rem" }}>Когда</th>
                  <th style={{ padding: "0.4rem 0.5rem" }}>От (узел · порт)</th>
                  <th style={{ padding: "0.4rem 0.35rem", width: 28 }} />
                  <th style={{ padding: "0.4rem 0.5rem" }}>На (узел · порт)</th>
                  <th style={{ padding: "0.4rem 0.5rem" }}>Источник</th>
                </tr>
              </thead>
              <tbody>
                {hops.map((h, i) => {
                  const fromIdx = h.from_if_index!;
                  const toIdx = h.to_if_index!;
                  const fromText = portLabel(h.device_id, fromIdx, h.device_name);
                  const toText = portLabel(h.device_id, toIdx, h.device_name);
                  return (
                    <tr key={`${h.seen_at}-${fromIdx}-${toIdx}-${i}`} style={{ borderBottom: "1px solid #1e2636" }}>
                      <td style={{ padding: "0.45rem 0.5rem", color: "#8ab4ff", fontWeight: 600 }}>{i + 1}</td>
                      <td style={{ padding: "0.45rem 0.5rem", whiteSpace: "nowrap", color: "#b8c0d0" }}>
                        {fmtTime(h.seen_at)}
                      </td>
                      <td style={{ padding: "0.45rem 0.5rem" }}>
                        <PortLink deviceId={h.device_id} ifIndex={fromIdx} text={fromText} />
                      </td>
                      <td style={{ padding: "0.45rem 0.35rem", color: "#6d7689" }}>→</td>
                      <td style={{ padding: "0.45rem 0.5rem" }}>
                        <PortLink deviceId={h.device_id} ifIndex={toIdx} text={toText} />
                      </td>
                      <td style={{ padding: "0.45rem 0.5rem", color: "#9aa3b5" }}>{sourceLabel(h.source)}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        </>
      ) : (
        <p style={{ color: "#9aa3b5", margin: "0 0 0.75rem" }}>
          В timeline нет hop with from→to; см. сводку пар портов ниже или таблицу «Timeline FDB».
        </p>
      )}

      {pairSummary.length > 0 && (
        <details style={{ marginTop: "1rem" }}>
          <summary style={{ cursor: "pointer", color: "#9aa3b5", fontSize: "0.85rem" }}>
            Сводка пар портов (сколько раз MAC «скакал» между двумя портами)
          </summary>
          <ul style={{ margin: "0.5rem 0 0", paddingLeft: "1.25rem", color: "#b8c0d0", fontSize: "0.85rem" }}>
            {pairSummary.map((p) => (
              <li key={p.key} style={{ marginBottom: 4 }}>
                <strong>{p.fromLabel}</strong> ⇄ <strong>{p.toLabel}</strong> — {p.count}×, последний{" "}
                {fmtTime(p.lastSeen)}
                {p.sources ? ` (${p.sources})` : ""}
              </li>
            ))}
          </ul>
        </details>
      )}

      {footprintOnly.length > 0 && (
        <div style={{ marginTop: "1rem", paddingTop: "0.75rem", borderTop: "1px solid #2a3344" }}>
          <div style={{ color: "#9aa3b5", fontSize: "0.85rem", marginBottom: "0.35rem" }}>
            Сейчас в FDB (без hop в истории — раньше на карте это были «одиночные круги»):
          </div>
          <ul style={{ margin: 0, paddingLeft: "1.25rem", fontSize: "0.85rem", color: "#c5cedd" }}>
            {footprintOnly.map((n) => (
              <li key={n.id}>
                <Link to={`/devices/${n.device_id}`} state={backState} style={{ color: "#9ec1ff" }}>
                  {n.label}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
