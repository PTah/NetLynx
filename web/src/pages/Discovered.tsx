import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { apiGet, apiPost } from "../api";
import { deviceLinkState } from "../navigation";
import { PromoteDiscoveredForm, type PromoteFormValues, type PromotePreview } from "../components/PromoteDiscoveredForm";
import type { DeviceCategory } from "../deviceCategories";
import {
  copyTextToClipboard,
  formatMacDisplay,
  looksLikeMac,
  normalizeMacQuery,
  selectElementText,
} from "../macUtil";
import type { Device } from "../types";

type Discovered = {
  id: number;
  identity_key: string;
  remote_sys_name?: string | null;
  remote_chassis_id?: string | null;
  remote_mgmt_addr?: string | null;
  last_seen_if_index?: number | null;
  last_seen_from_device_id?: number | null;
  last_protocol?: string | null;
  status: string;
  promoted_device_id?: number | null;
  last_seen_at: string;
  seen_from_name?: string | null;
};

type StatusFilter = "new" | "ignored" | "added" | "all";

const emptyPromote: PromoteFormValues = {
  host: "",
  name: "",
  location: "",
  category: "other",
  community: "public",
};

const NOT_IN_NODES_TIP =
  "Это сосед, увиденный по LLDP/CDP — его ещё нет в списке Узлы. Кнопка «Добавить» в этой строке как раз создаёт узел для него.";

const SEEN_FROM_MISSING_TIP =
  "Это свитч, на чьём порту замечен сосед. Его нет в списке Узлы, поэтому карточку открыть нельзя. Кнопка «Добавить» в строке добавляет соседа на порту, а не этот свитч — свитч нужно завести на странице «Узлы».";

function looksLikeIP(s: string): boolean {
  const t = s.trim();
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(t) || (t.includes(":") && t.includes("::"));
}

/**
 * Если строка похожа на MAC (в т.ч. 10 hex → 01:c0:a8:aa:49), вернуть colon-форму.
 * Иначе null (имя узла и т.п. не трогаем).
 */
function tryFormatMacSearchInput(raw: string): string | null {
  const t = raw.trim();
  if (!t || looksLikeIP(t)) return null;
  if (!looksLikeMac(t)) return null;
  const formatted = formatMacDisplay(t);
  if (formatted === t || !formatted.includes(":")) return null;
  return formatted;
}

function AddressLine({ text }: { text: string }) {
  if (!text) {
    return <div style={{ fontSize: "0.8rem", color: "#9aa3b5" }}>—</div>;
  }
  if (looksLikeMac(text)) {
    const display = formatMacDisplay(text);
    return (
      <div
        style={{ fontSize: "0.8rem", color: "#9aa3b5", cursor: "pointer", userSelect: "all" }}
        title="Клик — выделить MAC; двойной клик — скопировать"
        onClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          selectElementText(e.currentTarget);
        }}
        onDoubleClick={(e) => {
          e.preventDefault();
          e.stopPropagation();
          selectElementText(e.currentTarget);
          copyTextToClipboard(display);
        }}
      >
        {display}
      </div>
    );
  }
  return <div style={{ fontSize: "0.8rem", color: "#9aa3b5" }}>{text}</div>;
}

/** IP обнаруженного соседа (mgmt LLDP/CDP, иначе addr: из identity или sysName-как-IP). */
function discoveredIP(d: Discovered): string {
  const mgmt = (d.remote_mgmt_addr || "").trim();
  if (mgmt && looksLikeIP(mgmt)) return mgmt;
  const key = (d.identity_key || "").trim();
  if (key.toLowerCase().startsWith("addr:")) {
    const a = key.slice(5).trim();
    if (looksLikeIP(a)) return a;
  }
  const sys = (d.remote_sys_name || "").trim();
  if (sys && looksLikeIP(sys)) return sys;
  return "";
}

/** Подстрока под именем: MAC/chassis, без дублирования IP (IP — отдельный столбец). */
function discoveredAddressUnderName(d: Discovered): string {
  const chassis = (d.remote_chassis_id || "").trim();
  if (chassis) return chassis;
  const mgmt = (d.remote_mgmt_addr || "").trim();
  if (mgmt && looksLikeMac(mgmt)) return mgmt;
  const key = (d.identity_key || "").trim();
  if (!key) return "";
  if (key.toLowerCase().startsWith("addr:")) return "";
  if (looksLikeIP(key)) return "";
  return key;
}

function discoveredMatchesQuery(d: Discovered, raw: string): boolean {
  const q = raw.trim().toLowerCase();
  if (!q) return true;
  const name = (d.remote_sys_name || "").toLowerCase();
  const mgmt = (d.remote_mgmt_addr || "").toLowerCase();
  const identity = (d.identity_key || "").toLowerCase();
  const chassis = (d.remote_chassis_id || "").toLowerCase();
  const ip = discoveredIP(d).toLowerCase();
  if (name.includes(q) || mgmt.includes(q) || identity.includes(q) || chassis.includes(q) || ip.includes(q)) {
    return true;
  }
  const macQ = normalizeMacQuery(q);
  if (macQ.length >= 4) {
    const macHay = normalizeMacQuery(`${d.remote_chassis_id || ""} ${d.identity_key || ""}`);
    if (macHay.includes(macQ)) return true;
  }
  return false;
}

/** Имя кандидата: ссылка только если уже добавлен в Узлы. */
function CandidateName({ d }: { d: Discovered }) {
  const sys = (d.remote_sys_name || "").trim();
  if (!sys) return <div>—</div>;
  if (d.status === "added" && d.promoted_device_id != null) {
    return (
      <div>
        <Link
          to={`/devices/${d.promoted_device_id}`}
          state={deviceLinkState({ path: "/discovered", label: "Обнаружено" })}
        >
          {sys}
        </Link>
      </div>
    );
  }
  return (
    <div title={NOT_IN_NODES_TIP} style={{ cursor: "help" }}>
      {sys}
    </div>
  );
}

/** «Увиден с»: ссылка только если узел есть в inventory. */
function SeenFromCell({ d, deviceIds }: { d: Discovered; deviceIds: Set<number> }) {
  const id = d.last_seen_from_device_id;
  const ifPart = d.last_seen_if_index != null ? ` / if${d.last_seen_if_index}` : "";
  if (id == null) return <>—</>;
  const label = (d.seen_from_name || "").trim() || `#${id}`;
  if (deviceIds.has(id)) {
    return (
      <>
        <Link to={`/devices/${id}`} state={deviceLinkState({ path: "/discovered", label: "Обнаружено" })}>
          {label}
        </Link>
        {ifPart}
      </>
    );
  }
  return (
    <span title={SEEN_FROM_MISSING_TIP} style={{ cursor: "help", color: "#9aa3b5" }}>
      {label}
      {ifPart}
    </span>
  );
}

export default function Discovered() {
  const nav = useNavigate();
  const formRef = useRef<HTMLFormElement>(null);
  const [status, setStatus] = useState<StatusFilter>("new");
  const [search, setSearch] = useState("");
  const [rows, setRows] = useState<Discovered[]>([]);
  const [devices, setDevices] = useState<Device[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [msg, setMsg] = useState<string | null>(null);

  const filteredRows = useMemo(
    () => rows.filter((d) => discoveredMatchesQuery(d, search)),
    [rows, search],
  );

  const [promoteID, setPromoteID] = useState<number | null>(null);
  const [promote, setPromote] = useState<PromoteFormValues>(emptyPromote);
  const [preview, setPreview] = useState<PromotePreview | null>(null);
  const [busy, setBusy] = useState(false);

  const deviceIds = useMemo(() => new Set(devices.map((d) => d.id)), [devices]);
  const existingLocations = useMemo(() => {
    const set = new Set<string>();
    for (const n of devices) {
      const loc = n.location?.trim();
      if (loc) set.add(loc);
    }
    return [...set].sort((a, b) => a.localeCompare(b, "ru", { sensitivity: "base" }));
  }, [devices]);
  const deviceById = useMemo(() => new Map(devices.map((d) => [d.id, d])), [devices]);

  const load = useCallback((signal?: AbortSignal) => {
    setLoading(true);
    setErr(null);
    Promise.all([
      apiGet<Discovered[]>(`/api/v1/discovered?status=${encodeURIComponent(status)}`, signal ? { signal } : undefined),
      apiGet<Device[] | null>("/api/v1/devices", signal ? { signal } : undefined),
    ])
      .then(([list, inv]) => {
        setRows(Array.isArray(list) ? list : []);
        setDevices(Array.isArray(inv) ? inv : []);
      })
      .catch((e: Error) => {
        if (e.name !== "AbortError") setErr(e.message || String(e));
      })
      .finally(() => {
        if (!signal?.aborted) setLoading(false);
      });
  }, [status]);

  useEffect(() => {
    const ac = new AbortController();
    void load(ac.signal);
    return () => ac.abort();
  }, [load]);

  useEffect(() => {
    if (promoteID == null) return;
    formRef.current?.scrollIntoView({ behavior: "smooth", block: "start" });
    formRef.current?.querySelector<HTMLInputElement>("input[name='promote-host']")?.focus();
  }, [promoteID]);

  function openPromote(d: Discovered) {
    const mgmt = (d.remote_mgmt_addr || "").trim();
    const sys = (d.remote_sys_name || "").trim();
    const seenLoc = d.last_seen_from_device_id != null
      ? deviceById.get(d.last_seen_from_device_id)?.location?.trim() || ""
      : "";
    setPromoteID(d.id);
    setPromote({
      host: mgmt || (looksLikeIP(sys) ? sys : ""),
      name: looksLikeMac(sys) ? "" : sys || (looksLikeMac(mgmt) ? "" : mgmt),
      location: seenLoc,
      category: "other" as DeviceCategory,
      community: "public",
    });
    setPreview(null);
    setMsg(null);
    setErr(null);
  }

  async function onIgnore(id: number) {
    setBusy(true);
    setMsg(null);
    setErr(null);
    try {
      await apiPost(`/api/v1/discovered/${id}/ignore`, {});
      setMsg(`Кандидат #${id} скрыт (ignored)`);
      if (promoteID === id) setPromoteID(null);
      load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function onReopen(id: number) {
    setBusy(true);
    setMsg(null);
    setErr(null);
    try {
      await apiPost(`/api/v1/discovered/${id}/reopen`, {});
      setMsg(`Кандидат #${id} снова в статусе new — можно добавить`);
      load();
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setBusy(false);
    }
  }

  async function runPreview() {
    if (promoteID == null) return;
    if (!promote.host.trim()) {
      setPreview({ ok: false, error: "Укажите IP для проверки SNMP (с сервера NetLynx)" });
      return;
    }
    setBusy(true);
    setPreview(null);
    setErr(null);
    try {
      const res = await apiPost<PromotePreview>(`/api/v1/discovered/${promoteID}/preview`, {
        host: promote.host,
        name: promote.name,
        snmp_version: "v2c",
        community: promote.community,
      });
      setPreview(res);
      if (!res.ok) setErr(res.error || "Проверка SNMP не удалась");
    } catch (e) {
      const m = e instanceof Error ? e.message : String(e);
      setPreview({ ok: false, error: m });
      setErr(m);
    } finally {
      setBusy(false);
    }
  }

  async function onPromote(e: FormEvent) {
    e.preventDefault();
    if (promoteID == null) return;
    if (!promote.name.trim()) {
      setErr("Укажите имя узла.");
      return;
    }
    setBusy(true);
    setMsg(null);
    setErr(null);
    try {
      const res = await apiPost<{ id: number }>(`/api/v1/discovered/${promoteID}/promote`, {
        host: promote.host,
        name: promote.name,
        location: promote.location,
        device_category: promote.category,
        snmp_version: "v2c",
        community: promote.community,
      });
      setMsg(`Узел создан: id=${res.id}. Открываю карточку…`);
      setPromoteID(null);
      setPromote(emptyPromote);
      load();
      nav(`/devices/${res.id}`, { state: deviceLinkState({ path: "/discovered", label: "Обнаружено" }) });
    } catch (err) {
      setErr(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  const closePromote = () => {
    setPromoteID(null);
    setPromote(emptyPromote);
    setPreview(null);
    setErr(null);
    setMsg(null);
  };

  const promoteForm =
    promoteID != null ? (
      <div style={{ margin: "1rem 0", maxWidth: 560 }}>
        <PromoteDiscoveredForm
          formRef={formRef}
          title={`Добавление в список Узлы — кандидат #${promoteID}`}
          values={promote}
          locations={existingLocations}
          preview={preview}
          busy={busy}
          onChange={(patch) => setPromote((v) => ({ ...v, ...patch }))}
          onPreview={() => void runPreview()}
          onSubmit={onPromote}
          onCancel={closePromote}
        />
      </div>
    ) : null;

  return (
    <div className="page">
      <div style={{ display: "flex", gap: "1rem", alignItems: "center", flexWrap: "wrap" }}>
        <h1 style={{ margin: 0 }}>Обнаружено</h1>
        <select value={status} onChange={(e) => setStatus(e.target.value as StatusFilter)}>
          <option value="new">Новые</option>
          <option value="ignored">Игнор</option>
          <option value="added">Добавленные</option>
          <option value="all">Все</option>
        </select>
        <input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          onKeyDown={(e) => {
            if (e.key !== "Enter") return;
            const formatted = tryFormatMacSearchInput(search);
            if (formatted) {
              e.preventDefault();
              setSearch(formatted);
            }
          }}
          placeholder="Поиск: имя, IP или MAC…"
          aria-label="Поиск по имени, IP или MAC"
          style={{ minWidth: "14rem", flex: "1 1 14rem", maxWidth: "28rem" }}
        />
        <button type="button" onClick={() => load()} disabled={loading}>
          Обновить
        </button>
      </div>
      <p style={{ color: "#9aa3b5" }}>
        Кандидаты LLDP/CDP, которых ещё нет в списке <Link to="/devices">Узлы</Link> (inventory). Кнопка «Добавить»
        открывает ту же форму, что и на топологии: имя, тип, расположение, IP по желанию. Узел появляется только
        после «Добавить и сохранить». Если узел потом удалили из Узлов, кандидат снова станет <code>new</code> и
        его можно добавить повторно.
      </p>
      {err && <p style={{ color: "#f88" }}>{err}</p>}
      {msg && <p style={{ color: "#6d6" }}>{msg}</p>}
      {promoteForm}
      {loading && <p>Загрузка…</p>}
      {!loading && rows.length === 0 && <p style={{ color: "#9aa3b5" }}>Нет кандидатов в этом фильтре.</p>}
      {!loading && rows.length > 0 && filteredRows.length === 0 && (
        <p style={{ color: "#9aa3b5" }}>Нет совпадений по «{search.trim()}» (имя / IP / MAC).</p>
      )}
      {filteredRows.length > 0 && (
        <table className="data-table" style={{ width: "100%", marginTop: "0.75rem" }}>
          <thead>
            <tr>
              <th>Имя / адрес</th>
              <th>IP</th>
              <th>Протокол</th>
              <th>Увиден с</th>
              <th>Статус</th>
              <th>Последний раз</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {filteredRows.map((d) => (
              <tr key={d.id} style={promoteID === d.id ? { outline: "1px solid #3d8bfd" } : undefined}>
                <td>
                  <CandidateName d={d} />
                  <AddressLine text={discoveredAddressUnderName(d)} />
                </td>
                <td style={{ whiteSpace: "nowrap", fontVariantNumeric: "tabular-nums" }}>
                  {discoveredIP(d) || "—"}
                </td>
                <td>{(d.last_protocol || "—").toUpperCase()}</td>
                <td>
                  <SeenFromCell d={d} deviceIds={deviceIds} />
                </td>
                <td>{d.status}</td>
                <td style={{ whiteSpace: "nowrap" }}>{new Date(d.last_seen_at).toLocaleString()}</td>
                <td style={{ whiteSpace: "nowrap" }}>
                  {(d.status === "new" ||
                    (d.status === "added" && (d.promoted_device_id == null || !deviceIds.has(d.promoted_device_id)))) && (
                    <>
                      <button type="button" disabled={busy} onClick={() => openPromote(d)}>
                        Добавить
                      </button>{" "}
                      <button type="button" disabled={busy} onClick={() => void onIgnore(d.id)}>
                        Игнор
                      </button>
                    </>
                  )}
                  {d.status === "added" && d.promoted_device_id != null && deviceIds.has(d.promoted_device_id) && (
                    <Link
                      to={`/devices/${d.promoted_device_id}`}
                      state={deviceLinkState({ path: "/discovered", label: "Обнаружено" })}
                    >
                      Узел #{d.promoted_device_id}
                    </Link>
                  )}
                  {d.status === "added" && (d.promoted_device_id == null || !deviceIds.has(d.promoted_device_id)) && (
                    <>
                      {" "}
                      <button type="button" disabled={busy} onClick={() => void onReopen(d.id)} title="Сбросить ошибочный added">
                        Вернуть в новые
                      </button>
                    </>
                  )}
                  {d.status === "ignored" && (
                    <button type="button" disabled={busy} onClick={() => void onReopen(d.id)}>
                      Вернуть в новые
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  );
}
