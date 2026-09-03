import { useCallback, useEffect, useState } from "react";
import { apiGet, apiPost } from "../api";

type ConfigSnapshot = {
  id: number;
  device_id: number;
  fetched_at: string;
  config_hash: string;
  source: string;
  byte_size: number;
};

type DiffLine = {
  kind: "equal" | "insert" | "delete";
  text: string;
  old_line?: number;
  new_line?: number;
};

type DiffStats = { equal: number; insert: number; delete: number };

const sourceLabel: Record<string, string> = {
  scheduled: "планировщик",
  backup: "бэкап",
  manual: "вручную",
  port_sync: "sync портов",
};

/** ~10 строк таблицы + заголовок */
const SNAPSHOT_LIST_MAX_HEIGHT = "22.5rem";

export function DeviceConfigHistory({
  deviceId,
  canWrite,
}: {
  deviceId: number;
  canWrite: boolean;
}) {
  const [snaps, setSnaps] = useState<ConfigSnapshot[]>([]);
  const [loading, setLoading] = useState(true);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [toId, setToId] = useState<number | "">("");
  const [fromId, setFromId] = useState<number | "">("");
  const [diffLines, setDiffLines] = useState<DiffLine[] | null>(null);
  const [diffStats, setDiffStats] = useState<DiffStats | null>(null);
  const [diffMsg, setDiffMsg] = useState("");
  const [listExpanded, setListExpanded] = useState(false);

  const load = useCallback(() => {
    setLoading(true);
    setErr("");
    apiGet<{ snapshots: ConfigSnapshot[] }>(`/api/v1/devices/${deviceId}/config/snapshots?limit=30`)
      .then((r) => {
        const list = r.snapshots ?? [];
        setSnaps(list);
        if (list.length > 0) {
          setToId((prev) => (prev === "" ? list[0].id : prev));
          if (list.length > 1) {
            setFromId((prev) => (prev === "" ? list[1].id : prev));
          }
        }
      })
      .catch((e: Error) => setErr(e.message))
      .finally(() => setLoading(false));
  }, [deviceId]);

  useEffect(() => {
    load();
  }, [load]);

  const fetchNow = () => {
    setBusy(true);
    setErr("");
    apiPost<{ ok: boolean; saved: boolean; id: number }>(`/api/v1/devices/${deviceId}/config/snapshot`, {})
      .then(() => load())
      .catch((e: Error) => setErr(e.message))
      .finally(() => setBusy(false));
  };

  const closeDiff = () => {
    setDiffLines(null);
    setDiffStats(null);
    setDiffMsg("");
  };

  const diffVisible = diffLines !== null || diffStats !== null || diffMsg !== "";

  const runDiff = () => {
    if (diffVisible) {
      closeDiff();
      return;
    }
    setDiffMsg("");
    setDiffLines(null);
    setDiffStats(null);
    const q = new URLSearchParams();
    if (toId !== "") q.set("to", String(toId));
    if (fromId !== "") q.set("from", String(fromId));
    apiGet<{ lines: DiffLine[]; stats: DiffStats }>(
      `/api/v1/devices/${deviceId}/config/diff?${q.toString()}`,
    )
      .then((r) => {
        setDiffLines(r.lines ?? []);
        setDiffStats(r.stats ?? null);
        if (!r.lines?.length) setDiffMsg("Нет отличий или пустой diff");
      })
      .catch((e: Error) => setDiffMsg(e.message));
  };

  const diffPrev = () => {
    if (toId === "" || snaps.length < 2) return;
    const idx = snaps.findIndex((s) => s.id === toId);
    const cur = idx >= 0 ? snaps[idx] : snaps[0];
    const prev = snaps[idx + 1];
    if (!cur || !prev) return;
    setToId(cur.id);
    setFromId(prev.id);
    setDiffMsg("");
    apiGet<{ lines: DiffLine[]; stats: DiffStats }>(
      `/api/v1/devices/${deviceId}/config/diff?to=${cur.id}&from=${prev.id}`,
    )
      .then((r) => {
        setDiffLines(r.lines ?? []);
        setDiffStats(r.stats ?? null);
      })
      .catch((e: Error) => setDiffMsg(e.message));
  };

  return (
    <div style={{ marginBottom: "1rem", marginTop: "0.75rem" }}>
      <div style={{ display: "flex", flexWrap: "wrap", alignItems: "baseline", gap: "0.5rem 0.75rem" }}>
        <strong>История конфига (show run) — сохраняется при изменении конфига</strong>
        {snaps.length > 0 && !loading && (
          <span style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>
            {snaps.length} снимк.
            {snaps[0] && (
              <>
                {" "}
                · последний {new Date(snaps[0].fetched_at).toLocaleString()} (
                {sourceLabel[snaps[0].source] ?? snaps[0].source})
              </>
            )}
          </span>
        )}
        {snaps.length > 0 && !loading && (
          <button
            type="button"
            onClick={() => setListExpanded((v) => !v)}
            style={{
              fontSize: "0.85rem",
              padding: "2px 8px",
              background: "transparent",
              border: "1px solid #3a4558",
              color: "#9aa3b5",
              cursor: "pointer",
            }}
          >
            {listExpanded ? "Свернуть список ▲" : `Развернуть список (${snaps.length}) ▼`}
          </button>
        )}
      </div>
      <p style={{ margin: "0.35rem 0", color: "#9aa3b5", fontSize: "0.85rem" }}>
        Снимки <strong>running-config</strong> по SSH (<code>show run</code>) — не FDB и не MAC-таблица. Нужны для diff
        «что изменилось на свитче»; хранятся по расписанию, при бэкапе и sync портов (обычно десятки, не сотни).
      </p>
      {!loading && snaps.length > 0 && !listExpanded && (
        <p style={{ margin: "0 0 0.5rem", color: "#7a8499", fontSize: "0.8rem" }}>
          Список снимков свёрнут — разверните для просмотра (до {snaps.length}, прокрутка ~10 строк).
        </p>
      )}
      {err && <p style={{ color: "#f88" }}>{err}</p>}
      <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", marginBottom: "0.5rem" }}>
        {canWrite && (
          <button type="button" onClick={fetchNow} disabled={busy}>
            {busy ? "Снимаем…" : "Сохранить снимок сейчас"}
          </button>
        )}
        <button type="button" onClick={load} disabled={loading}>
          Обновить список
        </button>
        <button type="button" onClick={diffPrev} disabled={snaps.length < 2}>
          Diff с предыдущим
        </button>
      </div>
      {loading ? (
        <p style={{ color: "#9aa3b5" }}>Загрузка…</p>
      ) : snaps.length === 0 ? (
        <p style={{ color: "#9aa3b5" }}>Снимков пока нет. Нужны SSH-учётные данные и успешный show run.</p>
      ) : (
        <>
          {listExpanded && (
            <div
              style={{
                maxHeight: SNAPSHOT_LIST_MAX_HEIGHT,
                overflowY: "auto",
                overflowX: "auto",
                marginBottom: "0.5rem",
                border: "1px solid #2e3648",
                borderRadius: 6,
              }}
            >
              <table style={{ width: "100%", fontSize: "0.9rem", borderCollapse: "collapse" }}>
                <thead>
                  <tr style={{ position: "sticky", top: 0, background: "#1a1f2b", zIndex: 1 }}>
                    <th style={{ textAlign: "left", padding: "0.35rem 0.5rem" }}>Когда</th>
                    <th style={{ textAlign: "left", padding: "0.35rem 0.5rem" }}>Источник</th>
                    <th style={{ textAlign: "right", padding: "0.35rem 0.5rem" }}>Размер</th>
                    <th style={{ textAlign: "left", padding: "0.35rem 0.5rem" }}>Hash</th>
                  </tr>
                </thead>
                <tbody>
                  {snaps.map((s) => (
                    <tr key={s.id}>
                      <td style={{ padding: "0.3rem 0.5rem" }}>{new Date(s.fetched_at).toLocaleString()}</td>
                      <td style={{ padding: "0.3rem 0.5rem" }}>{sourceLabel[s.source] ?? s.source}</td>
                      <td style={{ textAlign: "right", padding: "0.3rem 0.5rem" }}>
                        {s.byte_size.toLocaleString()} B
                      </td>
                      <td style={{ fontFamily: "monospace", fontSize: "0.8rem", padding: "0.3rem 0.5rem" }}>
                        {s.config_hash.slice(0, 12)}…
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
          <div style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "flex-end" }}>
            <label>
              К (новый)
              <br />
              <select
                value={toId === "" ? "" : String(toId)}
                onChange={(e) => setToId(e.target.value ? Number(e.target.value) : "")}
              >
                {snaps.map((s) => (
                  <option key={s.id} value={s.id}>
                    {new Date(s.fetched_at).toLocaleString()} ({s.source})
                  </option>
                ))}
              </select>
            </label>
            <label>
              От (старый)
              <br />
              <select
                value={fromId === "" ? "" : String(fromId)}
                onChange={(e) => setFromId(e.target.value ? Number(e.target.value) : "")}
              >
                {snaps.map((s) => (
                  <option key={s.id} value={s.id}>
                    {new Date(s.fetched_at).toLocaleString()} ({s.source})
                  </option>
                ))}
              </select>
            </label>
            <button type="button" onClick={runDiff}>
              {diffVisible ? "Свернуть diff" : "Показать diff"}
            </button>
          </div>
        </>
      )}
      {diffVisible && diffStats && (
        <p style={{ fontSize: "0.85rem", color: "#9aa3b5" }}>
          +{diffStats.insert} / −{diffStats.delete} / ={diffStats.equal}
        </p>
      )}
      {diffVisible && diffMsg && <p style={{ color: "#f88" }}>{diffMsg}</p>}
      {diffVisible && diffLines && diffLines.length > 0 && (
        <pre
          style={{
            maxHeight: 420,
            overflow: "auto",
            background: "#0d1117",
            padding: "0.75rem",
            fontSize: "0.78rem",
            lineHeight: 1.35,
            border: "1px solid #333",
          }}
        >
          {diffLines.map((l, i) => {
            const prefix = l.kind === "insert" ? "+ " : l.kind === "delete" ? "- " : "  ";
            const color = l.kind === "insert" ? "#6d6" : l.kind === "delete" ? "#f88" : "#ccc";
            return (
              <div key={i} style={{ color, whiteSpace: "pre-wrap" }}>
                {prefix}
                {l.text}
              </div>
            );
          })}
        </pre>
      )}
    </div>
  );
}
