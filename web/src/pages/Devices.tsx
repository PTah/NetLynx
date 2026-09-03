import type { CSSProperties } from "react";
import { FormEvent, useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { apiDelete, apiGet, apiPost } from "../api";
import { usePersistedColumnWidths } from "../hooks/usePersistedColumnWidths";
import { deviceLinkState } from "../navigation";
import {
  DEVICES_LIST_SCROLL_RESTORE_KEY,
  DEVICES_LIST_SCROLL_Y_KEY,
  DEVICES_RESTORE_MAX_ATTEMPTS,
  DEVICES_RESTORE_RETRY_MS,
  applyDevicesListScrollY,
  parsePositiveNumber,
  readDevicesListScrollY,
} from "../storage/devicesListScroll";
import type { Device } from "../types";
import {
  type CategoryFilterState,
  type DeviceCategory,
  deviceCategoryLabel,
  normalizeDeviceCategory,
  readCategoryFilter,
  writeCategoryFilter,
} from "../deviceCategories";
import { DeviceCategoryIcon } from "../components/DeviceCategoryIcon";
import { useDeviceCategories } from "../hooks/useDeviceCategories";
import { isDeviceOnline } from "../deviceOnline";
import { formatMacDisplay, macVendorLabel } from "../macUtil";

type SnmpTestResult = {
  ok: boolean;
  sys_name?: string;
  sys_descr?: string;
  error?: string;
};

type PortSearchHit = {
  device_id: number;
  device_name: string;
  device_host: string;
  location?: string | null;
  if_index: number;
  if_name?: string | null;
  if_descr?: string | null;
  port_role: string;
  match_type?: string;
  mac?: string | null;
  mac_vendor?: string | null;
  ip?: string | null;
  arp_if_index?: number | null;
  note?: string | null;
};

function portLabel(h: PortSearchHit): string {
  const parts = [h.if_name?.trim(), h.if_descr?.trim()].filter(Boolean);
  return parts.length > 0 ? parts.join(" · ") : `ifIndex ${h.if_index}`;
}

type DeviceSortCol = "name" | "host" | "category" | "location";
type SortDir = "asc" | "desc";

function compareHost(a: string, b: string): number {
  const pa = a.trim();
  const pb = b.trim();
  const na = pa.split(".").map((x) => parseInt(x, 10));
  const nb = pb.split(".").map((x) => parseInt(x, 10));
  if (na.length === 4 && nb.length === 4 && na.every((n) => !Number.isNaN(n)) && nb.every((n) => !Number.isNaN(n))) {
    for (let i = 0; i < 4; i++) {
      if (na[i] !== nb[i]) return na[i] - nb[i];
    }
    return 0;
  }
  return pa.localeCompare(pb, undefined, { sensitivity: "base", numeric: true });
}

function compareDevices(a: Device, b: Device, col: DeviceSortCol): number {
  switch (col) {
    case "name":
      return a.name.localeCompare(b.name, undefined, { sensitivity: "base", numeric: true });
    case "host":
      return compareHost(a.host, b.host);
    case "category":
      return deviceCategoryLabel(a.device_category).localeCompare(
        deviceCategoryLabel(b.device_category),
        "ru",
        { sensitivity: "base" },
      );
    case "location": {
      const la = (a.location ?? "").trim();
      const lb = (b.location ?? "").trim();
      if (!la && !lb) return 0;
      if (!la) return 1;
      if (!lb) return -1;
      return la.localeCompare(lb, undefined, { sensitivity: "base", numeric: true });
    }
    default:
      return 0;
  }
}

function sortIndicator(active: boolean, dir: SortDir): string {
  if (!active) return " ⇅";
  return dir === "asc" ? " ▲" : " ▼";
}

export default function Devices() {
  const { categories } = useDeviceCategories();
  const { colgroup, ResizeHandle } = usePersistedColumnWidths("devices-list", [52, 140, 130, 110, 220, 72, 80, 150, 100, 88]);
  const [list, setList] = useState<Device[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [msg, setMsg] = useState<string | null>(null);

  const [name, setName] = useState("");
  const [host, setHost] = useState("");
  const [location, setLocation] = useState("");
  const [deviceCategory, setDeviceCategory] = useState<DeviceCategory>("switch");
  const [categoryFilter, setCategoryFilter] = useState<CategoryFilterState>(() => readCategoryFilter());

  useEffect(() => {
    setCategoryFilter((prev) => {
      const next = readCategoryFilter(categories);
      for (const c of categories) {
        if (typeof prev[c.id] === "boolean") next[c.id] = prev[c.id]!;
      }
      return next;
    });
  }, [categories]);
  const [ver, setVer] = useState<"v1" | "v2c" | "v3">("v2c");
  const [community, setCommunity] = useState("public");
  const [v3User, setV3User] = useState("");
  const [v3AuthProto, setV3AuthProto] = useState<"SHA" | "MD5" | "SHA256" | "SHA512">("SHA");
  const [v3AuthPass, setV3AuthPass] = useState("");
  const [v3PrivProto, setV3PrivProto] = useState<"AES" | "AES256" | "DES" | "NONE">("AES");
  const [v3PrivPass, setV3PrivPass] = useState("");

  const [snmpDlg, setSnmpDlg] = useState<string | null>(null);
  const [uispEnabled, setUispEnabled] = useState(false);
  const [portQ, setPortQ] = useState("");
  const [portHits, setPortHits] = useState<PortSearchHit[]>([]);
  const [portSearchErr, setPortSearchErr] = useState<string | null>(null);
  const [portSearching, setPortSearching] = useState(false);
  const [portSearched, setPortSearched] = useState(false);
  const [sortCol, setSortCol] = useState<DeviceSortCol>("name");
  const [sortDir, setSortDir] = useState<SortDir>("asc");

  const sortedList = useMemo(() => {
    const rows = list.filter((d) => {
      const id = normalizeDeviceCategory(d.device_category);
      return categoryFilter[id] !== false;
    });
    rows.sort((a, b) => {
      const c = compareDevices(a, b, sortCol);
      return sortDir === "asc" ? c : -c;
    });
    return rows;
  }, [list, sortCol, sortDir, categoryFilter]);

  const toggleCategoryFilter = (id: DeviceCategory) => {
    setCategoryFilter((prev) => {
      const next = { ...prev, [id]: !(prev[id] !== false) };
      writeCategoryFilter(next);
      return next;
    });
  };

  const toggleSort = (col: DeviceSortCol) => {
    if (sortCol === col) {
      setSortDir((d) => (d === "asc" ? "desc" : "asc"));
    } else {
      setSortCol(col);
      setSortDir("asc");
    }
  };

  const [pendingRestoreY, setPendingRestoreY] = useState<number | null>(() => {
    if (sessionStorage.getItem(DEVICES_LIST_SCROLL_RESTORE_KEY) !== "1") return null;
    const y = parsePositiveNumber(sessionStorage.getItem(DEVICES_LIST_SCROLL_Y_KEY));
    return y > 0 ? y : null;
  });

  const reload = (signal?: AbortSignal) =>
    apiGet<Device[] | null>("/api/v1/devices", signal ? { signal } : undefined)
      .then((rows) => setList(Array.isArray(rows) ? rows : []))
      .catch((e: Error) => {
        if (e.name !== "AbortError") setErr(e.message);
      });

  const removeDevice = (id: number, name: string) => {
    if (!window.confirm(`Удалить узел «${name}» (id ${id})? События и порты по этому узлу будут удалены из базы.`)) {
      return;
    }
    setErr(null);
    apiDelete(`/api/v1/devices/${id}`)
      .then(() => {
        setMsg(`Узел ${id} удалён.`);
        reload();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const runSnmpTest = (id: number) => {
    setErr(null);
    apiPost<SnmpTestResult>(`/api/v1/devices/${id}/snmp-test`, {})
      .then((r) => {
        if (r.ok) {
          setSnmpDlg(`SNMP OK\nsysName: ${r.sys_name ?? ""}\n\nsysDescr:\n${r.sys_descr ?? ""}`);
        } else {
          setSnmpDlg(`SNMP ошибка: ${r.error ?? "неизвестно"}`);
        }
      })
      .catch((e: Error) => setErr(e.message));
  };

  useEffect(() => {
    const ac = new AbortController();
    void reload(ac.signal);
    return () => ac.abort();
  }, []);

  useEffect(() => {
    if (sessionStorage.getItem(DEVICES_LIST_SCROLL_RESTORE_KEY) !== "1") {
      sessionStorage.removeItem(DEVICES_LIST_SCROLL_Y_KEY);
    }
  }, []);

  useEffect(() => {
    // Восстанавливаем скролл только после загрузки списка, иначе браузер может зажать в 0.
    if (pendingRestoreY == null) return;
    if (list.length === 0) return;
    let raf2 = 0;
    let retryTimer = 0;
    let attempts = 0;

    const tryRestore = () => {
      applyDevicesListScrollY(pendingRestoreY);
      const nowY = readDevicesListScrollY();
      attempts += 1;
      // Если контент ещё «растёт», браузер может срезать top. Пробуем ещё несколько раз.
      if (nowY + 2 < pendingRestoreY && attempts < DEVICES_RESTORE_MAX_ATTEMPTS) {
        retryTimer = window.setTimeout(tryRestore, DEVICES_RESTORE_RETRY_MS);
        return;
      }
      setPendingRestoreY(null);
      sessionStorage.removeItem(DEVICES_LIST_SCROLL_Y_KEY);
      sessionStorage.removeItem(DEVICES_LIST_SCROLL_RESTORE_KEY);
    };

    const raf1 = requestAnimationFrame(() => {
      raf2 = requestAnimationFrame(() => {
        tryRestore();
      });
    });
    return () => {
      cancelAnimationFrame(raf1);
      if (raf2) cancelAnimationFrame(raf2);
      if (retryTimer) window.clearTimeout(retryTimer);
    };
  }, [pendingRestoreY, list.length]);

  useEffect(() => {
    const ac = new AbortController();
    apiGet<{ enabled: boolean }>("/api/v1/settings/uisp", { signal: ac.signal })
      .then((x) => setUispEnabled(Boolean(x.enabled)))
      .catch((e: Error) => {
        if (e.name !== "AbortError") setUispEnabled(false);
      });
    return () => ac.abort();
  }, []);

  const runPortSearch = (e?: FormEvent) => {
    e?.preventDefault();
    const q = portQ.trim();
    if (q.length < 2) {
      setPortSearchErr("Введите минимум 2 символа (описание, MAC или IP).");
      setPortHits([]);
      return;
    }
    setPortSearchErr(null);
    setPortSearching(true);
    apiGet<{ hits: PortSearchHit[] }>(`/api/v1/ports/search?q=${encodeURIComponent(q)}&limit=100`)
      .then((r) => {
        setPortHits(Array.isArray(r.hits) ? r.hits : []);
        setPortSearched(true);
      })
      .catch((er: Error) => {
        setPortSearchErr(er.message);
        setPortHits([]);
        setPortSearched(true);
      })
      .finally(() => setPortSearching(false));
  };

  const importFromUisp = () => {
    setErr(null);
    setMsg(null);
    apiPost<{ ok: boolean; created: number; updated: number; total: number }>("/api/v1/devices/import-uisp", {})
      .then((r) => {
        setMsg(`Импорт UISP: добавлено ${r.created}, обновлено ${r.updated}, коммутаторов в UISP (role=switch): ${r.total}.`);
        reload();
      })
      .catch((e: Error) => setErr(e.message));
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    setErr(null);
    setMsg(null);
    const body: Record<string, unknown> = {
      name,
      host,
      snmp_version: ver,
      poll_interval_seconds: 60,
      device_category: deviceCategory,
    };
    if (location.trim()) {
      body.location = location.trim();
    }
    if (ver === "v1" || ver === "v2c") {
      body.community = community;
    } else {
      body.v3_user = v3User;
      body.v3_auth_protocol = v3AuthProto;
      body.v3_auth_pass = v3AuthPass;
      body.v3_priv_protocol = v3PrivProto;
      if (v3PrivProto !== "NONE") body.v3_priv_pass = v3PrivPass;
    }
    apiPost<{ id: number }>("/api/v1/devices", body)
      .then((r) => {
        setMsg(`Создан узел id=${r.id}`);
        setName("");
        setHost("");
        setLocation("");
        setDeviceCategory("switch");
        setV3User("");
        setV3AuthPass("");
        setV3AuthProto("SHA");
        setV3PrivProto("AES");
        setV3PrivPass("");
        reload();
      })
      .catch((e: Error) => setErr(e.message));
  };

  return (
    <div className="devices-page">
      <h1 style={{ marginTop: 0 }}>Узлы</h1>
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          gap: "0.65rem 1.1rem",
          marginBottom: "1rem",
          fontSize: "0.92rem",
        }}
      >
        {categories.map((o) => (
          <label key={o.id} style={{ display: "inline-flex", alignItems: "center", gap: 6, cursor: "pointer" }}>
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
      {uispEnabled && (
        <p style={{ marginBottom: "1rem" }}>
          <button type="button" onClick={importFromUisp}>
            Подгрузить устройства из UISP
          </button>{" "}
          <span style={{ color: "#9aa3b5", fontSize: "0.9rem" }}>
            Подтягиваются только коммутаторы в сети (SNMP v2c, community из настроек UISP); в UISP со статусом
            disconnected не попадают. Повторный импорт обновляет имя, адрес и расположение.
          </span>
        </p>
      )}
      {err && <p style={{ color: "#f88" }}>{err}</p>}
      {msg && <p style={{ color: "#8d8" }}>{msg}</p>}
      {snmpDlg && (
        <div style={{ marginBottom: "1rem" }}>
          <pre
            style={{
              background: "#1a1f2a",
              padding: "0.75rem",
              borderRadius: 8,
              whiteSpace: "pre-wrap",
              maxHeight: 280,
              overflow: "auto",
              margin: "0 0 0.5rem",
            }}
          >
            {snmpDlg}
          </pre>
          <button type="button" onClick={() => setSnmpDlg(null)}>
            Закрыть
          </button>
        </div>
      )}

      <h2 id="add">Добавить</h2>
      <form onSubmit={onSubmit} style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "flex-end" }}>
        <label>
          Имя
          <br />
          <input value={name} onChange={(e) => setName(e.target.value)} required />
        </label>
        <label>
          IP адрес
          <br />
          <input value={host} onChange={(e) => setHost(e.target.value)} required />
        </label>
        <label>
          Расположение (опционально)
          <br />
          <input
            value={location}
            onChange={(e) => setLocation(e.target.value)}
            placeholder="например, сайт в UISP или этаж"
            style={{ minWidth: 220 }}
          />
        </label>
        <label>
          Тип устройства
          <br />
          <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
            <DeviceCategoryIcon category={deviceCategory} height={22} />
            <select
              value={deviceCategory}
              onChange={(e) => setDeviceCategory(e.target.value)}
            >
              {categories.map((o) => (
                <option key={o.id} value={o.id}>
                  {o.label}
                </option>
              ))}
            </select>
          </span>
        </label>
        <label>
          SNMP
          <br />
          <select value={ver} onChange={(e) => setVer(e.target.value as "v1" | "v2c" | "v3")}>
            <option value="v1">v1</option>
            <option value="v2c">v2c</option>
            <option value="v3">v3 (authPriv SHA+AES)</option>
          </select>
        </label>
        {ver === "v1" || ver === "v2c" ? (
          <label>
            Community
            <br />
            <input value={community} onChange={(e) => setCommunity(e.target.value)} />
          </label>
        ) : null}
        {ver === "v3" && (
          <>
            <label>
              User
              <br />
              <input value={v3User} onChange={(e) => setV3User(e.target.value)} required />
            </label>
            <label>
              Auth protocol
              <br />
              <select value={v3AuthProto} onChange={(e) => setV3AuthProto(e.target.value as "SHA" | "MD5" | "SHA256" | "SHA512")}>
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
                value={v3AuthPass}
                onChange={(e) => setV3AuthPass(e.target.value)}
                minLength={8}
                required
              />
            </label>
            <label>
              Privacy protocol
              <br />
              <select value={v3PrivProto} onChange={(e) => setV3PrivProto(e.target.value as "AES" | "AES256" | "DES" | "NONE")}>
                <option value="AES">AES</option>
                <option value="AES256">AES256</option>
                <option value="DES">DES</option>
                <option value="NONE">NONE (authNoPriv)</option>
              </select>
            </label>
            {v3PrivProto !== "NONE" && (
              <label>
                Priv password
                <br />
                <input
                  type="password"
                  value={v3PrivPass}
                  onChange={(e) => setV3PrivPass(e.target.value)}
                  minLength={8}
                  required
                />
              </label>
            )}
          </>
        )}
        <button type="submit">Создать</button>
      </form>

      <h2 style={{ marginTop: "1.5rem" }}>Поиск на порту</h2>
      <p style={{ marginTop: 0, color: "#9aa3b5", fontSize: "0.9rem" }}>
        Описание — ifName, SNMP ifDescr/ifAlias и своя подпись порта в карточке узла; MAC — таблица FDB после опроса; IP — ARP на свиче, порт из FDB по MAC (может быть на другом
        узле).
      </p>
      <form onSubmit={runPortSearch} style={{ marginBottom: "0.25rem" }}>
        <label style={{ display: "block", marginBottom: "0.35rem", fontSize: "0.9rem" }}>
          Поиск (описание, MAC, IP)
        </label>
        <div className="port-search-inline">
          <input
            value={portQ}
            onChange={(e) => setPortQ(e.target.value)}
            placeholder="DAK-PC / aa:bb:cc:dd:ee:ff / 192.168.1.50"
            style={{ flex: "1 1 280px", minWidth: 200, maxWidth: 480 }}
          />
          <button type="submit" disabled={portSearching}>
            {portSearching ? "Поиск…" : "Найти"}
          </button>
        </div>
      </form>
      {portSearchErr && <p style={{ color: "#f88", marginTop: "0.5rem" }}>{portSearchErr}</p>}
      {portHits.length > 0 && (
        <div className="port-search-results">
          <table style={{ tableLayout: "fixed", width: "100%", fontSize: "0.9rem" }}>
            <thead>
              <tr>
                <th>Узел</th>
                <th>Host</th>
                <th>ifIndex</th>
                <th>Порт</th>
                <th>MAC</th>
                <th>IP</th>
                <th>Роль</th>
                <th>Топология</th>
              </tr>
            </thead>
            <tbody>
              {portHits.map((h, i) => {
                const vendor = macVendorLabel(h.mac_vendor, Boolean(h.ip));
                return (
                <tr key={`${h.device_id}-${h.if_index}-${h.mac ?? ""}-${h.ip ?? ""}-${i}`}>
                  <td>
                    <Link
                      to={`/devices/${h.device_id}`}
                      state={deviceLinkState({ path: "/devices", label: "Все узлы" })}
                    >
                      {h.device_name}
                    </Link>
                  </td>
                  <td>{h.device_host}</td>
                  <td>{h.if_index > 0 ? h.if_index : h.arp_if_index ?? "—"}</td>
                  <td style={{ overflow: "hidden", textOverflow: "ellipsis" }} title={h.note ?? undefined}>
                    {portLabel(h)}
                    {h.note ? <span style={{ display: "block", color: "#9aa3b5", fontSize: "0.8rem" }}>{h.note}</span> : null}
                  </td>
                  <td style={{ fontSize: "0.85rem" }}>
                    {h.mac ? (
                      <>
                        <Link to={`/investigate/mac?mac=${encodeURIComponent(h.mac)}`} title="Расследовать MAC">
                          {formatMacDisplay(h.mac)}
                        </Link>
                        {vendor ? (
                          <span className="mac-vendor" title={vendor}>
                            {" "}
                            ({vendor})
                          </span>
                        ) : null}
                      </>
                    ) : (
                      "—"
                    )}
                  </td>
                  <td style={{ fontSize: "0.85rem" }}>{h.ip ?? "—"}</td>
                  <td>{h.port_role || "—"}</td>
                  <td>
                    <Link
                      to={`/topology?focus=${h.device_id}&q=${encodeURIComponent(h.mac || h.ip || h.device_name)}`}
                      title="Найти узел на топологии"
                      style={{ fontSize: "0.85rem" }}
                    >
                      ↗ топология
                    </Link>
                  </td>
                </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
      {portSearched && !portSearching && portHits.length === 0 && !portSearchErr && (
        <p style={{ color: "#9aa3b5", fontSize: "0.9rem" }}>Совпадений нет. Убедитесь, что узлы уже опрашивались SNMP.</p>
      )}

      <h2 style={{ marginTop: "0.75rem" }}>Список</h2>
      <div id="devices-list-scroll" className="devices-list-scroll">
      <table style={{ tableLayout: "fixed", width: "100%" }}>
        {colgroup}
        <thead>
          <tr>
            <th style={{ userSelect: "none" }}>
              id
              <ResizeHandle colIndex={0} />
            </th>
            <th style={{ userSelect: "none" }}>
              <button type="button" className="devices-sort-th" onClick={() => toggleSort("name")}>
                Имя{sortIndicator(sortCol === "name", sortDir)}
              </button>
              <ResizeHandle colIndex={1} />
            </th>
            <th style={{ userSelect: "none" }}>
              <button type="button" className="devices-sort-th" onClick={() => toggleSort("host")}>
                IP адрес{sortIndicator(sortCol === "host", sortDir)}
              </button>
              <ResizeHandle colIndex={2} />
            </th>
            <th style={{ userSelect: "none" }}>
              <button type="button" className="devices-sort-th" onClick={() => toggleSort("category")}>
                Тип{sortIndicator(sortCol === "category", sortDir)}
              </button>
              <ResizeHandle colIndex={3} />
            </th>
            <th style={{ userSelect: "none" }}>
              <button type="button" className="devices-sort-th" onClick={() => toggleSort("location")}>
                Расположение{sortIndicator(sortCol === "location", sortDir)}
              </button>
              <ResizeHandle colIndex={4} />
            </th>
            <th style={{ userSelect: "none" }}>
              SNMP
              <ResizeHandle colIndex={5} />
            </th>
            <th style={{ userSelect: "none" }}>
              Опрос (с)
              <ResizeHandle colIndex={6} />
            </th>
            <th style={{ userSelect: "none" }}>
              SNMP / ping
              <ResizeHandle colIndex={7} />
            </th>
            <th style={{ userSelect: "none" }}>
              Проверка SNMP
              <ResizeHandle colIndex={8} />
            </th>
            <th style={{ userSelect: "none" }}>
              Удалить
              <ResizeHandle colIndex={9} />
            </th>
          </tr>
        </thead>
        <tbody>
          {sortedList.map((d) => {
            // Как на дашборде: ПК/сервер онлайн по ping; свитч/роутер — только SNMP.
            const offline = !isDeviceOnline(d);
            const dim: CSSProperties = offline
              ? { background: "#141820", color: "#5f6778" }
              : {};
            const linkColor = offline ? { color: "#6d7689" } : undefined;
            const muted = offline ? "#4d5566" : "#9aa3b5";
            return (
              <tr key={d.id} style={offline ? { background: "#141820" } : undefined}>
                <td style={dim}>{d.id}</td>
                <td style={dim}>
                  <Link
                    to={`/devices/${d.id}`}
                    state={deviceLinkState({ path: "/devices", label: "Все узлы" })}
                    style={linkColor}
                    onClick={() => {
                      sessionStorage.setItem(DEVICES_LIST_SCROLL_Y_KEY, String(readDevicesListScrollY()));
                      sessionStorage.setItem(DEVICES_LIST_SCROLL_RESTORE_KEY, "1");
                    }}
                  >
                    {d.name}
                  </Link>
                </td>
                <td style={dim}>{d.host}</td>
                <td style={{ fontSize: "0.9rem", ...dim }}>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: 8 }}>
                    <DeviceCategoryIcon category={d.device_category} height={20} title={deviceCategoryLabel(d.device_category, categories)} />
                    {deviceCategoryLabel(d.device_category, categories)}
                  </span>
                </td>
                <td style={{ fontSize: "0.9rem", overflow: "hidden", textOverflow: "ellipsis", ...dim }}>
                  {d.location?.trim() ? d.location : "—"}
                </td>
                <td style={dim}>{d.snmp_version}</td>
                <td style={dim}>{d.poll_interval_seconds}</td>
                <td style={{ fontSize: "0.9rem", lineHeight: 1.35, ...dim }}>
                  <div>SNMP: {d.last_snmp_ok == null ? "—" : d.last_snmp_ok ? "да" : "нет"}</div>
                  <div style={{ color: muted }}>
                    ping:{" "}
                    {d.last_ping_ok == null
                      ? "—"
                      : d.last_ping_ok
                        ? d.last_ping_rtt_ms != null
                          ? `да (${d.last_ping_rtt_ms}ms)`
                          : "да"
                        : "нет"}
                  </div>
                </td>
                <td style={dim}>
                  <button type="button" onClick={() => runSnmpTest(d.id)}>
                    Тест
                  </button>
                </td>
                <td style={dim}>
                  <button type="button" onClick={() => removeDevice(d.id, d.name)}>
                    Удалить
                  </button>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
      </div>
    </div>
  );
}
