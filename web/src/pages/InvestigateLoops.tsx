import { useCallback, useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { apiGet } from "../api";
import { deviceLinkState } from "../navigation";

type CycleHop = {
  from_device_id: number;
  from_device_name?: string;
  from_if_index: number;
  from_if_name?: string;
  to_device_id: number;
  to_device_name?: string;
  to_if_index?: number;
  to_if_name?: string;
  protocol: string;
};

type Cycle = {
  length: number;
  device_ids: number[];
  device_names: string[];
  hops: CycleHop[];
  summary: string;
};

type LoopReport = {
  cycles: Cycle[];
  node_count: number;
  edge_count: number;
  protocol: string;
  generated_at: string;
};

export default function InvestigateLoops() {
  const [report, setReport] = useState<LoopReport | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    setErr(null);
    apiGet<LoopReport>("/api/v1/investigate/loops?protocol=lldp")
      .then(setReport)
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    load();
  }, [load]);

  return (
    <div style={{ padding: "1rem 1.25rem", maxWidth: 1100 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 12, flexWrap: "wrap" }}>
        <div>
          <h1 style={{ margin: 0, fontSize: "1.35rem" }}>Петли топологии (LLDP)</h1>
          <p style={{ color: "#9aa3b5", margin: "0.35rem 0 0", fontSize: "0.9rem" }}>
            DFS по графу LLDP-соседей inventory. Отдельно от MAC flapping — ищем циклы устройств и параллельные
            аплинки.
          </p>
        </div>
        <button type="button" onClick={load} disabled={loading}>
          {loading ? "Обновление…" : "Обновить"}
        </button>
      </div>

      {err ? <p style={{ color: "#e8a0a0" }}>{err}</p> : null}

      {report ? (
        <p style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>
          Узлов в графе: {report.node_count}, рёбер: {report.edge_count}, протокол: {report.protocol}.{" "}
          Сформировано: {new Date(report.generated_at).toLocaleString("ru-RU")}
        </p>
      ) : null}

      {!loading && report && (report.cycles ?? []).length === 0 ? (
        <p style={{ color: "#8d8" }}>Циклов не найдено — по LLDP топология без петель (или мало соседей).</p>
      ) : null}

      {report?.cycles?.map((c, idx) => (
        <section
          key={`${c.summary}-${idx}`}
          style={{
            border: "1px solid #2a3344",
            borderRadius: 6,
            padding: "0.75rem 1rem",
            marginBottom: "0.75rem",
            background: "#141820",
          }}
        >
          <div style={{ display: "flex", gap: 10, flexWrap: "wrap", alignItems: "baseline" }}>
            <strong>{c.length === 2 ? "Параллельные линки" : `Цикл длины ${c.length}`}</strong>
            <span style={{ color: "#e8c98a", fontSize: "0.85rem" }}>{c.summary}</span>
          </div>
          <div style={{ marginTop: 8, fontSize: "0.9rem" }}>
            Узлы:{" "}
            {c.device_ids.map((id, i) => (
              <span key={id}>
                {i > 0 ? " → " : null}
                <Link
                  to={`/devices/${id}`}
                  state={deviceLinkState({ path: "/investigate/loops", label: "Петли LLDP" })}
                >
                  {c.device_names[i] || `#${id}`}
                </Link>
              </span>
            ))}
            {c.length > 2 ? ` → ${c.device_names[0] || `#${c.device_ids[0]}`}` : null}
          </div>
          {c.hops?.length ? (
            <table style={{ width: "100%", fontSize: "0.85rem", marginTop: 8 }}>
              <thead>
                <tr>
                  <th>От</th>
                  <th>Порт</th>
                  <th>К</th>
                  <th>Порт</th>
                  <th>Протокол</th>
                </tr>
              </thead>
              <tbody>
                {c.hops.map((h, i) => (
                  <tr key={i}>
                    <td>{h.from_device_name || h.from_device_id}</td>
                    <td>{h.from_if_name || h.from_if_index || "—"}</td>
                    <td>{h.to_device_name || h.to_device_id}</td>
                    <td>{h.to_if_name || h.to_if_index || "—"}</td>
                    <td>{h.protocol}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
          <p style={{ color: "#9aa3b5", fontSize: "0.85rem", marginBottom: 0 }}>
            Проверьте STP/RSTP, LACP на аплинках, лишние патч-корды; сверьте с{" "}
            <Link to="/topology">топологией</Link> и событиями STP / broadcast storm.
          </p>
        </section>
      ))}
    </div>
  );
}
