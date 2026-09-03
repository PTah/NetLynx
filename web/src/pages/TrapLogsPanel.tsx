import { Fragment, useCallback, useEffect, useMemo, useState } from "react";
import { apiDelete, apiGet, apiPatch } from "../api";

type TrapLabelOption = {
  value: string;
  title: string;
  group: string;
};

type TrapSettings = {
  log_enabled: boolean;
  listen_enabled?: boolean;
  listen_port?: number;
  trap_include_labels?: string;
  link_trap_events_mode?: string;
  trap_filter_active?: boolean;
  trap_label_options?: TrapLabelOption[];
  listen_addr?: string;
  receiver_enabled: boolean;
  community_filter: boolean;
  log_count: number;
  log_retain_max: number;
};

type TrapLogRow = {
  id: number;
  received_at: string;
  source_ip: string;
  device_id?: number | null;
  snmp_version?: string;
  community?: string;
  trap_oid?: string;
  trap_label?: string;
  trap_summary?: string;
  if_index?: number | null;
  payload: Record<string, unknown>;
};

type Props = {
  canWrite: boolean;
};

const PORT_TRAP_PRESET = [
  "linkUp",
  "linkDown",
  "topologyChangeInitiatedTrap",
  "loopDetectedTrap",
  "agentSwitchStormControlTrap",
];

function parseLabelsCSV(raw: string): string[] {
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

function joinLabelsCSV(labels: string[]): string {
  const seen = new Set<string>();
  const out: string[] = [];
  for (const l of labels) {
    const t = l.trim();
    if (!t || seen.has(t)) continue;
    seen.add(t);
    out.push(t);
  }
  return out.join(",");
}

function fmtTime(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return iso;
  return d.toLocaleString();
}

export default function TrapLogsPanel({ canWrite }: Props) {
  const [settings, setSettings] = useState<TrapSettings | null>(null);
  const [items, setItems] = useState<TrapLogRow[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [expanded, setExpanded] = useState<number | null>(null);
  const [draftLabels, setDraftLabels] = useState<string>("");
  const [filterDirty, setFilterDirty] = useState(false);

  const labelOptions = settings?.trap_label_options ?? [];
  const selectedSet = useMemo(() => new Set(parseLabelsCSV(draftLabels)), [draftLabels]);

  const groupedOptions = useMemo(() => {
    const groups = new Map<string, TrapLabelOption[]>();
    for (const opt of labelOptions) {
      const g = opt.group || "Прочее";
      const list = groups.get(g) ?? [];
      list.push(opt);
      groups.set(g, list);
    }
    return [...groups.entries()];
  }, [labelOptions]);

  const load = useCallback(async () => {
    setErr(null);
    try {
      const [s, logs] = await Promise.all([
        apiGet("/api/v1/settings/snmp-traps") as Promise<TrapSettings>,
        apiGet("/api/v1/settings/snmp-traps/logs?limit=100") as Promise<{ items: TrapLogRow[] }>,
      ]);
      setSettings(s);
      setItems(logs.items ?? []);
      if (!filterDirty) {
        setDraftLabels(s.trap_include_labels ?? "");
      }
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, [filterDirty]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!settings?.log_enabled) return;
    const id = window.setInterval(() => {
      void load();
    }, 4000);
    return () => window.clearInterval(id);
  }, [settings?.log_enabled, load]);

  async function setLogEnabled(enabled: boolean) {
    if (!canWrite) return;
    setBusy(true);
    setErr(null);
    try {
      const s = (await apiPatch("/api/v1/settings/snmp-traps", {
        log_enabled: enabled,
      })) as TrapSettings;
      setSettings(s);
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function saveFilter(csv: string) {
    if (!canWrite) return;
    setBusy(true);
    setErr(null);
    try {
      const s = (await apiPatch("/api/v1/settings/snmp-traps", {
        trap_include_labels: csv,
      })) as TrapSettings;
      setSettings(s);
      setDraftLabels(s.trap_include_labels ?? "");
      setFilterDirty(false);
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  function toggleLabel(value: string) {
    const next = new Set(selectedSet);
    if (next.has(value)) next.delete(value);
    else next.add(value);
    const ordered = labelOptions.map((o) => o.value).filter((v) => next.has(v));
    for (const v of next) {
      if (!ordered.includes(v)) ordered.push(v);
    }
    setDraftLabels(joinLabelsCSV(ordered));
    setFilterDirty(true);
  }

  async function clearLogs() {
    if (!canWrite) return;
    if (!window.confirm("Очистить весь журнал trap логов?")) return;
    setBusy(true);
    setErr(null);
    try {
      await apiDelete("/api/v1/settings/snmp-traps/logs");
      await load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  const listen = settings?.listen_addr?.trim() || "—";
  const recvOn = !!settings?.receiver_enabled;
  const filterActive = parseLabelsCSV(draftLabels).length > 0;

  return (
    <div>
      <section className="settings-card">
        <h2>Trap logs</h2>
        <p className="settings-lead" style={{ marginTop: 0 }}>
          Журнал входящих SNMP traps. Приёмник, порт и мгновенные link-события — во вкладке{" "}
          <strong>Уведомления → Принимать traps</strong> (по умолчанию UDP <strong>9162</strong>).
        </p>
        {err && (
          <p style={{ color: "#f88" }} role="alert">
            {err}
          </p>
        )}
        <p style={{ margin: "0.35rem 0", color: "#9aa3b5", fontSize: "0.9rem" }}>
          Приёмник:{" "}
          {recvOn ? (
            <>
              слушает <code>{listen}</code>
              {settings?.community_filter ? " (есть фильтр community)" : " (без фильтра community)"}
            </>
          ) : (
            <>выключен (Настройки → Уведомления → «Принимать traps»)</>
          )}
          . В журнале до {settings?.log_retain_max ?? 2000} записей (сейчас {settings?.log_count ?? 0}
          {filterActive ? ", фильтр типов включён" : ", все типы"}).
        </p>
        {(settings?.link_trap_events_mode ?? "off") === "off" ? (
          <p style={{ margin: "0.35rem 0 0", color: "#c5a572", fontSize: "0.88rem" }}>
            <code>linkUp</code>/<code>linkDown</code> сейчас только в trap logs. Чтобы дублировать в таблицу{" "}
            <strong>«События»</strong>, включите режим в{" "}
            <strong>Настройки → Уведомления → Принимать traps</strong> («на всех узлах» или «только с флагом на
            устройстве» + флаг на карточке узла).
          </p>
        ) : null}
        <label
          style={{
            display: "inline-flex",
            alignItems: "center",
            gap: "0.5rem",
            marginTop: "0.5rem",
            cursor: canWrite ? "pointer" : "default",
            opacity: canWrite ? 1 : 0.7,
          }}
        >
          <input
            type="checkbox"
            checked={!!settings?.log_enabled}
            disabled={!canWrite || busy || !recvOn}
            onChange={(e) => void setLogEnabled(e.target.checked)}
          />
          Вести trap логи
        </label>

        {labelOptions.length > 0 ? (
          <div style={{ marginTop: "1rem" }}>
            <h3 style={{ margin: "0 0 0.35rem", fontSize: "1rem" }}>Какие traps отслеживать</h3>
            <p style={{ margin: "0 0 0.5rem", color: "#9aa3b5", fontSize: "0.85rem" }}>
              Пустой выбор = все типы. Невыбранные traps не пишутся в журнал и не создают события.
            </p>
            <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap", marginBottom: "0.5rem" }}>
              <button
                type="button"
                disabled={!canWrite || busy}
                onClick={() => {
                  setDraftLabels(joinLabelsCSV(PORT_TRAP_PRESET));
                  setFilterDirty(true);
                }}
              >
                Порты / линк
              </button>
              <button
                type="button"
                disabled={!canWrite || busy}
                onClick={() => {
                  setDraftLabels("");
                  setFilterDirty(true);
                }}
              >
                Все типы
              </button>
              <button
                type="button"
                disabled={!canWrite || busy || !filterDirty}
                onClick={() => void saveFilter(draftLabels)}
              >
                Сохранить фильтр
              </button>
            </div>
            <div
              style={{
                display: "grid",
                gridTemplateColumns: "repeat(auto-fill, minmax(16rem, 1fr))",
                gap: "0.75rem 1rem",
                maxHeight: 220,
                overflowY: "auto",
                padding: "0.5rem",
                background: "#121820",
                borderRadius: 4,
              }}
            >
              {groupedOptions.map(([group, opts]) => (
                <div key={group}>
                  <div style={{ fontSize: "0.78rem", color: "#7a8499", marginBottom: 4 }}>{group}</div>
                  {opts.map((opt) => (
                    <label
                      key={opt.value}
                      style={{
                        display: "flex",
                        alignItems: "flex-start",
                        gap: "0.4rem",
                        fontSize: "0.85rem",
                        marginBottom: 4,
                        cursor: canWrite ? "pointer" : "default",
                        opacity: canWrite ? 1 : 0.75,
                      }}
                    >
                      <input
                        type="checkbox"
                        checked={selectedSet.has(opt.value)}
                        disabled={!canWrite || busy}
                        onChange={() => toggleLabel(opt.value)}
                      />
                      <span>{opt.title}</span>
                    </label>
                  ))}
                </div>
              ))}
            </div>
          </div>
        ) : null}

        <div style={{ display: "flex", gap: "0.5rem", flexWrap: "wrap", marginTop: "0.75rem" }}>
          <button type="button" onClick={() => void load()} disabled={busy}>
            Обновить
          </button>
          <button type="button" onClick={() => void clearLogs()} disabled={!canWrite || busy}>
            Очистить журнал
          </button>
        </div>
      </section>

      <section className="settings-card" style={{ marginTop: "1rem" }}>
        <h2>Последние traps</h2>
        {items.length === 0 ? (
          <p style={{ color: "#9aa3b5" }}>
            Пока пусто. Включите «Вести trap логи» и отправьте trap со свитча
            {filterActive ? " выбранного типа" : ""}.
          </p>
        ) : (
          <div style={{ overflowX: "auto" }}>
            <table style={{ width: "100%", fontSize: "0.9rem", tableLayout: "fixed" }}>
              <thead>
                <tr>
                  <th style={{ width: "11rem" }}>Время</th>
                  <th style={{ width: "8rem" }}>Source IP</th>
                  <th style={{ width: "4rem" }}>Dev</th>
                  <th style={{ width: "4rem" }}>if</th>
                  <th>Расшифровка</th>
                  <th style={{ width: "5rem" }} />
                </tr>
              </thead>
              <tbody>
                {items.map((row) => (
                  <Fragment key={row.id}>
                    <tr>
                      <td>{fmtTime(row.received_at)}</td>
                      <td>
                        <code>{row.source_ip}</code>
                      </td>
                      <td>{row.device_id ?? "—"}</td>
                      <td>{row.if_index ?? "—"}</td>
                      <td style={{ overflow: "hidden", textOverflow: "ellipsis" }}>
                        <span title={row.trap_oid || ""}>
                          <strong>{row.trap_label || "SNMP trap"}</strong>
                          {row.trap_summary ? (
                            <span style={{ display: "block", color: "#9aa3b5", fontSize: "0.82rem", marginTop: 2 }}>
                              {row.trap_summary}
                            </span>
                          ) : null}
                        </span>
                      </td>
                      <td>
                        <button
                          type="button"
                          style={{ fontSize: "0.8rem" }}
                          onClick={() => setExpanded(expanded === row.id ? null : row.id)}
                        >
                          {expanded === row.id ? "Скрыть" : "JSON"}
                        </button>
                      </td>
                    </tr>
                    {expanded === row.id ? (
                      <tr>
                        <td colSpan={6}>
                          <pre
                            style={{
                              margin: 0,
                              padding: "0.5rem",
                              background: "#121820",
                              borderRadius: 4,
                              overflow: "auto",
                              maxHeight: 280,
                              fontSize: "0.78rem",
                            }}
                          >
                            {JSON.stringify(row.payload, null, 2)}
                          </pre>
                        </td>
                      </tr>
                    ) : null}
                  </Fragment>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  );
}
