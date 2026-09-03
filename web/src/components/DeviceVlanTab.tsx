import { FormEvent, useEffect, useMemo, useState } from "react";
import { apiDeleteJson, apiGet, apiPatch, apiPost } from "../api";

export type VlanPortRef = {
  if_index: number;
  if_name: string;
  role?: string;
};

export type VlanRow = {
  vlan_id: number;
  name?: string;
  in_database?: boolean;
  access_ports: VlanPortRef[];
  tagged_ports: VlanPortRef[];
  fdb_ports?: VlanPortRef[];
};

type PortHint = {
  if_index: number;
  if_name?: string | null;
  port_role?: string | null;
  cli_port_mode?: string | null;
  cli_access_vlan?: number | null;
  vlan_id?: number | null;
};

type VlansPayload = {
  source?: string;
  vlans?: VlanRow[];
  ok?: boolean;
};

type PortVlanOp = "set_access" | "trunk_allow" | "remove";
type AllowedMode = "add" | "remove" | "all" | "except";

function isSwitchPort(p: PortHint): boolean {
  const n = (p.if_name ?? "").trim().toLowerCase();
  if (n.startsWith("vlan") || n.includes("loopback")) return false;
  return p.if_index > 0;
}

function portLabel(p: PortHint): string {
  return (p.if_name ?? "").trim() || `ifIndex ${p.if_index}`;
}

function inferVlansFromPorts(ports: PortHint[]): VlanRow[] {
  const rows = new Map<number, VlanRow>();
  const ensure = (id: number): VlanRow => {
    let r = rows.get(id);
    if (!r) {
      r = { vlan_id: id, access_ports: [], tagged_ports: [], fdb_ports: [] };
      rows.set(id, r);
    }
    return r;
  };
  for (const p of ports) {
    if (!isSwitchPort(p)) continue;
    const role = (p.cli_port_mode ?? p.port_role ?? "").toLowerCase();
    const access = p.cli_access_vlan && p.cli_access_vlan > 0 ? p.cli_access_vlan : role !== "trunk" && p.vlan_id && p.vlan_id > 0 ? p.vlan_id : null;
    if (access) {
      const r = ensure(access);
      if (!r.access_ports.some((x) => x.if_index === p.if_index)) {
        r.access_ports.push({ if_index: p.if_index, if_name: portLabel(p), role: "access" });
      }
    } else if (p.vlan_id && p.vlan_id > 0) {
      const r = ensure(p.vlan_id);
      if (!r.fdb_ports) r.fdb_ports = [];
      if (!r.fdb_ports.some((x) => x.if_index === p.if_index)) {
        r.fdb_ports.push({ if_index: p.if_index, if_name: portLabel(p), role: role || undefined });
      }
    }
  }
  return [...rows.values()].sort((a, b) => a.vlan_id - b.vlan_id);
}

function formatPorts(list: VlanPortRef[] | undefined): string {
  if (!list?.length) return "—";
  return list.map((p) => p.if_name || String(p.if_index)).join(", ");
}

function vlanNotInDbHint(r: VlanRow): { text: string; title: string } | null {
  if (r.in_database) return null;
  const hasCfg = (r.access_ports?.length ?? 0) > 0 || (r.tagged_ports?.length ?? 0) > 0;
  if (hasCfg) {
    return {
      text: " · с портов",
      title: "Нет в vlan database show run, но есть access/tagged на портах (конфиг или кэш)",
    };
  }
  return {
    text: " · вне DB",
    title: "Нет в vlan database show run",
  };
}

function vlanConfiguredOnPorts(r: VlanRow): boolean {
  return (r.access_ports?.length ?? 0) > 0 || (r.tagged_ports?.length ?? 0) > 0;
}

/** Разбор «10,20-22» / «10;20» для проверки предупреждений в UI. */
function parseVlanListClient(s: string): number[] {
  const norm = s.replace(/;/g, ",").replace(/\s+/g, ",");
  const seen = new Set<number>();
  const out: number[] = [];
  for (const raw of norm.split(",")) {
    const part = raw.trim();
    if (!part) continue;
    const dash = part.indexOf("-");
    if (dash > 0) {
      const a = Number(part.slice(0, dash).trim());
      const b = Number(part.slice(dash + 1).trim());
      if (!Number.isFinite(a) || !Number.isFinite(b) || a < 1 || b > 4094 || a > b) continue;
      for (let n = a; n <= b; n++) {
        if (!seen.has(n)) {
          seen.add(n);
          out.push(n);
        }
      }
      continue;
    }
    const n = Number(part);
    if (!Number.isFinite(n) || n < 1 || n > 4094 || seen.has(n)) continue;
    seen.add(n);
    out.push(n);
  }
  return out;
}

/** Предупреждение о риске потери SSH/Web (обычно native / VLAN 1). */
function trunkAllowAccessWarning(mode: AllowedMode, listRaw: string): string | null {
  const ids = parseVlanListClient(listRaw);
  if (mode === "all") return null;
  if (mode === "remove" && ids.includes(1)) {
    return (
      "Снимаете VLAN 1 с trunk (allowed vlan remove …).\n" +
      "Если управление свитчем (SSH/Web) идёт в native/VLAN 1 — можно потерять доступ к устройству.\n\nПродолжить?"
    );
  }
  if (mode === "except") {
    return (
      "except оставляет на trunk только VLAN вне указанного списка.\n" +
      "Если VLAN управления (часто 1 / native) попадёт под except — SSH и Web могут пропасть.\n\nПродолжить?"
    );
  }
  if (mode === "add" && ids.length > 0 && !ids.includes(1)) {
    const one = ids.length === 1 ? `только VLAN ${ids[0]}` : `VLAN ${ids.join(", ")} без 1`;
    return (
      `В списке ${one} — нет VLAN 1 (обычно native / управление).\n\n` +
      "Пока на trunk было allowed vlan all, первый add иногда сужает список; " +
      "если останется не VLAN управления — можно лишиться SSH/Web.\n\n" +
      "Надёжнее указать «1,16» (или свой VLAN управления + нужные).\n" +
      "В CLI без add («allowed vlan 16») список заменяется целиком — тоже опасно.\n\nПродолжить запись?"
    );
  }
  return null;
}

type Props = {
  deviceId: number;
  canWrite: boolean;
  settingsWritable: boolean;
  ports: PortHint[];
  reloadToken?: string | number | null;
  onApplied?: () => void;
};

export function DeviceVlanTab({ deviceId, canWrite, settingsWritable, ports, reloadToken, onApplied }: Props) {
  const inferred = useMemo(() => inferVlansFromPorts(ports), [ports]);
  const [rows, setRows] = useState<VlanRow[]>(inferred);
  const [source, setSource] = useState("ports");
  const [apiReady, setApiReady] = useState(false);
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);
  const [busyNote, setBusyNote] = useState("");
  const [nameDraft, setNameDraft] = useState<Record<number, string>>({});
  const [vlanId, setVlanId] = useState(0);
  const [ifIndex, setIfIndex] = useState(0);
  const [op, setOp] = useState<PortVlanOp>("set_access");
  const [allowedMode, setAllowedMode] = useState<AllowedMode>("add");
  const [allowedList, setAllowedList] = useState("");
  const [createId, setCreateId] = useState(0);
  const [createName, setCreateName] = useState("");
  const [selected, setSelected] = useState<Set<number>>(() => new Set());

  const physical = useMemo(() => ports.filter(isSwitchPort), [ports]);
  const selectedSorted = useMemo(
    () => [...selected].filter((id) => id !== 1).sort((a, b) => a - b),
    [selected]
  );
  const selectableIds = useMemo(
    () => rows.map((r) => r.vlan_id).filter((id) => id !== 1 && rows.find((x) => x.vlan_id === id)?.in_database),
    [rows]
  );
  const allSelectableChecked =
    selectableIds.length > 0 && selectableIds.every((id) => selected.has(id));

  const applyList = (list: VlanRow[], src?: string) => {
    setApiReady(true);
    if (src) setSource(src);
    setRows(list);
    const d: Record<number, string> = {};
    for (const row of list) d[row.vlan_id] = row.name || "";
    setNameDraft(d);
    setSelected((prev) => {
      const keep = new Set<number>();
      for (const id of prev) {
        if (list.some((r) => r.vlan_id === id) && id !== 1) keep.add(id);
      }
      return keep;
    });
    if (vlanId > 0 && !list.some((r) => r.vlan_id === vlanId) && list[0]) {
      setVlanId(list[0].vlan_id);
    }
  };

  const applyPayload = (r: VlansPayload, fallbackPorts = ports) => {
    if (r.vlans) {
      applyList(r.vlans.length ? r.vlans : inferVlansFromPorts(fallbackPorts), r.source || "ports");
      return true;
    }
    return false;
  };

  const load = () => {
    setLoading(true);
    setErr("");
    apiGet<VlansPayload>(`/api/v1/devices/${deviceId}/vlans`)
      .then((r) => {
        if (!applyPayload(r)) {
          applyList(inferVlansFromPorts(ports), "ports");
        }
      })
      .catch((e: Error) => {
        setErr(e.message);
        setApiReady(false);
        setRows(inferVlansFromPorts(ports));
        setSource("ports");
      })
      .finally(() => setLoading(false));
  };

  // Пока /vlans не ответил — черновик с портов. После ответа API не затирать полный список
  // локальным infer (иначе после sync ролей/опроса портов таблица «схлопывается»).
  useEffect(() => {
    if (!apiReady) setRows(inferred);
  }, [inferred, apiReady]);

  useEffect(() => {
    setApiReady(false);
    setSource("ports");
    setRows(inferVlansFromPorts(ports));
    load();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- ports только как стартовый черновик
  }, [deviceId, reloadToken]);

  useEffect(() => {
    if (vlanId <= 0 && rows[0]) setVlanId(rows[0].vlan_id);
  }, [rows, vlanId]);

  useEffect(() => {
    if (ifIndex <= 0 && physical[0]) setIfIndex(physical[0].if_index);
  }, [physical, ifIndex]);

  const sourceNote =
    source === "config_cache"
      ? "vlan database из show run (кэш после записи / открытия карточки)"
      : source === "snapshot"
        ? "vlan database из снимка show run + порты"
        : "пока только с портов (access VLAN / FDB). Имена появятся после show run.";

  const beginBusy = (note: string) => {
    setBusy(true);
    setBusyNote(note);
    setErr("");
  };

  const endBusy = () => {
    setBusy(false);
    setBusyNote("");
  };

  const afterWrite = (r: VlansPayload) => {
    if (!applyPayload(r)) load();
    onApplied?.();
  };

  const onCreate = (e: FormEvent) => {
    e.preventDefault();
    if (!canWrite || !settingsWritable || busy || createId < 1 || createId > 4094) return;
    const name = createName.trim();
    beginBusy(`Идёт обновление конфига… создаём VLAN ${createId}`);
    setRows((prev) => {
      if (prev.some((r) => r.vlan_id === createId)) {
        return prev.map((r) =>
          r.vlan_id === createId ? { ...r, name: name || r.name, in_database: true } : r
        );
      }
      return [...prev, { vlan_id: createId, name, in_database: true, access_ports: [], tagged_ports: [], fdb_ports: [] }].sort(
        (a, b) => a.vlan_id - b.vlan_id
      );
    });
    setNameDraft((prev) => ({ ...prev, [createId]: name }));
    setVlanId(createId);
    apiPost<VlansPayload>(`/api/v1/devices/${deviceId}/vlans`, {
      vlan_id: createId,
      ...(name ? { name } : {}),
    })
      .then(afterWrite)
      .catch((ex: Error) => {
        setErr(ex.message);
        load();
      })
      .finally(() => {
        endBusy();
        setCreateId(0);
        setCreateName("");
      });
  };

  const onSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!canWrite || !settingsWritable || !ifIndex || busy) return;
    if (op === "trunk_allow") {
      if (allowedMode !== "all" && !allowedList.trim()) {
        setErr("Укажите список VLAN в Allow vlan (например 10,20-22)");
        return;
      }
      const warn = trunkAllowAccessWarning(allowedMode, allowedList);
      if (warn && !window.confirm(warn)) return;
      beginBusy("Идёт обновление конфига на свитче (trunk allowed vlan)…");
      apiPatch<VlansPayload>(`/api/v1/devices/${deviceId}/interfaces/${ifIndex}/vlan`, {
        op: "trunk_allow",
        allowed_mode: allowedMode,
        allowed_vlans: allowedList,
      })
        .then(afterWrite)
        .catch((ex: Error) => setErr(ex.message))
        .finally(endBusy);
      return;
    }
    if (vlanId < 1 || vlanId > 4094) return;
    beginBusy("Идёт обновление конфига на свитче (VLAN на порту)…");
    apiPatch<VlansPayload>(`/api/v1/devices/${deviceId}/interfaces/${ifIndex}/vlan`, {
      op,
      vlan_id: vlanId,
    })
      .then(afterWrite)
      .catch((ex: Error) => setErr(ex.message))
      .finally(endBusy);
  };

  const w = canWrite && settingsWritable;

  const saveName = (id: number) => {
    if (!w || busy || id < 1) return;
    const name = nameDraft[id] ?? "";
    beginBusy(`Идёт обновление конфига… записываем имя VLAN ${id}`);
    // Сразу показать имя в таблице; ответ API уточнит полный список (в т.ч. новый VLAN).
    setRows((prev) => prev.map((row) => (row.vlan_id === id ? { ...row, name, in_database: true } : row)));
    apiPatch<VlansPayload>(`/api/v1/devices/${deviceId}/vlans/${id}`, { name })
      .then(afterWrite)
      .catch((ex: Error) => {
        setErr(ex.message);
        load();
      })
      .finally(endBusy);
  };

  const toggleSelect = (id: number) => {
    if (id === 1) return;
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const toggleSelectAll = () => {
    if (allSelectableChecked) {
      setSelected(new Set());
      return;
    }
    setSelected(new Set(selectableIds));
  };

  const deleteVlans = (ids: number[]) => {
    if (!w || busy) return;
    const clean = [...new Set(ids)].filter((id) => id > 1 && id <= 4094).sort((a, b) => a - b);
    if (!clean.length) return;
    const blocked: string[] = [];
    for (const id of clean) {
      const row = rows.find((r) => r.vlan_id === id);
      if (!row) continue;
      if (vlanConfiguredOnPorts(row)) {
        const ports = [
          ...(row.access_ports || []).map((p) => `${p.if_name || p.if_index} (access)`),
          ...(row.tagged_ports || []).map((p) => `${p.if_name || p.if_index} (tagged)`),
        ];
        blocked.push(`VLAN ${id}: ${ports.join(", ")}`);
      }
    }
    if (blocked.length) {
      setErr(
        "Сначала снимите VLAN с портов («Убрать с порта» или Access на другой VLAN), потом удаляйте из vlan database. " +
          blocked.join("; ")
      );
      return;
    }
    const list = clean.join(", ");
    const title =
      clean.length === 1
        ? `Удалить VLAN ${list} из vlan database свитча?`
        : `Удалить ${clean.length} VLAN (${list}) из vlan database?\nНа свитч уйдёт: no vlan ${list}`;
    if (!window.confirm(title)) return;
    beginBusy(`Идёт обновление конфига… удаляем VLAN ${list}`);
    setRows((prev) => prev.filter((row) => !clean.includes(row.vlan_id)));
    setNameDraft((prev) => {
      const next = { ...prev };
      for (const id of clean) delete next[id];
      return next;
    });
    setSelected(new Set());
    const req =
      clean.length === 1
        ? apiDeleteJson<VlansPayload>(`/api/v1/devices/${deviceId}/vlans/${clean[0]}`)
        : apiDeleteJson<VlansPayload>(`/api/v1/devices/${deviceId}/vlans`, {
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ vlan_ids: clean }),
          });
    req
      .then(afterWrite)
      .catch((ex: Error) => {
        setErr(ex.message);
        load();
      })
      .finally(endBusy);
  };

  const deleteVlan = (r: VlanRow) => {
    if (!w || busy || r.vlan_id === 1) return;
    // Помеченные + «Удалить» на любом из них → массовое удаление.
    if (selected.has(r.vlan_id) && selectedSorted.length > 1) {
      deleteVlans(selectedSorted);
      return;
    }
    deleteVlans([r.vlan_id]);
  };

  const portSubmitDisabled =
    !w ||
    busy ||
    !ifIndex ||
    (op === "trunk_allow"
      ? allowedMode !== "all" && !allowedList.trim()
      : vlanId < 1);

  return (
    <div className="device-detail-vlan-stub">
      <h2 style={{ marginTop: 0, fontSize: "1.1rem" }}>VLAN database</h2>
      <p style={{ color: "#9aa3b5", fontSize: "0.9rem", marginTop: 0 }}>
        Список VLAN на свитче: <code>vlan database</code> из show run и привязки портов. Колонка FDB — MAC на уже
        известных VLAN (призраки удалённых VLAN из FDB в список не попадают). Удаление из vlan database запрещено, пока
        VLAN висит на портах. VLAN 1 удалить нельзя.
      </p>
      {busy ? (
        <p style={{ color: "#9dd", fontSize: "0.9rem", marginTop: "-0.35rem" }} role="status" aria-live="polite">
          {busyNote || "Идёт обновление конфига…"} Подождите — идёт SSH и запись на свитч; кнопки временно недоступны.
        </p>
      ) : (
        <p style={{ color: "#7a8499", fontSize: "0.8rem", marginTop: "-0.35rem" }}>
          {loading ? "Обновляем список…" : sourceNote}
        </p>
      )}
      {err && (
        <p style={{ color: "#f88" }} role="alert">
          {err}
        </p>
      )}
      {w && (
        <form
          onSubmit={onCreate}
          style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "flex-end", marginBottom: "0.75rem" }}
        >
          <label>
            Новый VLAN ID
            <br />
            <input
              type="number"
              min={1}
              max={4094}
              value={createId || ""}
              disabled={busy}
              onChange={(e) => setCreateId(Number(e.target.value) || 0)}
              style={{ width: 88 }}
            />
          </label>
          <label>
            Имя (необязательно)
            <br />
            <input
              type="text"
              maxLength={32}
              value={createName}
              disabled={busy}
              onChange={(e) => setCreateName(e.target.value)}
              placeholder="Office"
              style={{ minWidth: 140 }}
            />
          </label>
          <button type="submit" disabled={busy || createId < 1 || createId > 4094}>
            {busy ? "Пишем…" : "Добавить в vlan database"}
          </button>
        </form>
      )}
      {w && selectedSorted.length > 0 && (
        <p style={{ margin: "0 0 0.5rem", display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "center" }}>
          <span style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>
            Выбрано: {selectedSorted.join(", ")}
          </span>
          <button type="button" disabled={busy} onClick={() => deleteVlans(selectedSorted)}>
            Удалить выбранные ({selectedSorted.length})
          </button>
          <button type="button" disabled={busy} onClick={() => setSelected(new Set())}>
            Снять выделение
          </button>
        </p>
      )}
      <table>
        <thead>
          <tr>
            {w && (
              <th style={{ width: 36 }} title="Выбрать для массового удаления">
                <input
                  type="checkbox"
                  checked={allSelectableChecked}
                  disabled={busy || selectableIds.length === 0}
                  onChange={toggleSelectAll}
                  aria-label="Выбрать все VLAN кроме 1"
                />
              </th>
            )}
            <th>VLAN ID</th>
            <th>Имя</th>
            <th>Привязка к портам</th>
            <th>Tagged</th>
            <th>FDB</th>
            <th>VLAN DB</th>
          </tr>
        </thead>
        <tbody>
          {rows.length === 0 ? (
            <tr>
              <td colSpan={w ? 7 : 6} style={{ color: "#9aa3b5" }}>
                Пока нет VLAN на портах и нет vlan database в снимке конфига.
              </td>
            </tr>
          ) : (
            rows.map((r) => (
              <tr
                key={r.vlan_id}
                style={{
                  cursor: "pointer",
                  background: selected.has(r.vlan_id) ? "rgba(80, 140, 200, 0.12)" : undefined,
                }}
                onClick={() => {
                  setVlanId(r.vlan_id);
                  if (op === "trunk_allow" && allowedMode !== "all") {
                    setAllowedList((prev) => {
                      const id = String(r.vlan_id);
                      if (!prev.trim()) return id;
                      if (prev.split(/[,;\s]+/).includes(id)) return prev;
                      return `${prev},${id}`;
                    });
                  }
                }}
                title="Подставить VLAN в форму ниже"
              >
                {w && (
                  <td onClick={(e) => e.stopPropagation()}>
                    <input
                      type="checkbox"
                      checked={selected.has(r.vlan_id)}
                      disabled={busy || r.vlan_id === 1}
                      onChange={() => toggleSelect(r.vlan_id)}
                      title={r.vlan_id === 1 ? "VLAN 1 нельзя удалить" : "Пометить для массового удаления"}
                      aria-label={`Выбрать VLAN ${r.vlan_id}`}
                    />
                  </td>
                )}
                <td>
                  {r.vlan_id}
                  {(() => {
                    const hint = vlanNotInDbHint(r);
                    if (!hint) return null;
                    return (
                      <span style={{ color: "#7a8499", fontSize: "0.75rem" }} title={hint.title}>
                        {hint.text}
                      </span>
                    );
                  })()}
                </td>
                <td onClick={(e) => e.stopPropagation()}>
                  {w ? (
                    <input
                      type="text"
                      maxLength={32}
                      value={nameDraft[r.vlan_id] ?? r.name ?? ""}
                      disabled={busy}
                      onChange={(e) => setNameDraft((prev) => ({ ...prev, [r.vlan_id]: e.target.value }))}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") {
                          e.preventDefault();
                          saveName(r.vlan_id);
                        }
                      }}
                      placeholder="имя / description"
                      style={{ width: "100%", minWidth: 140, boxSizing: "border-box" }}
                    />
                  ) : (
                    r.name || "—"
                  )}
                </td>
                <td>{formatPorts(r.access_ports)}</td>
                <td>{formatPorts(r.tagged_ports)}</td>
                <td>{formatPorts(r.fdb_ports)}</td>
                <td onClick={(e) => e.stopPropagation()} style={{ whiteSpace: "nowrap" }}>
                  {w && (
                    <>
                      <button
                        type="button"
                        disabled={busy}
                        onClick={() => saveName(r.vlan_id)}
                        title="Записать имя в vlan database"
                      >
                        Сохранить
                      </button>{" "}
                      <button
                        type="button"
                        disabled={
                          busy ||
                          r.vlan_id === 1 ||
                          !r.in_database ||
                          vlanConfiguredOnPorts(r) ||
                          (selected.has(r.vlan_id) &&
                            selectedSorted.some((id) => {
                              const row = rows.find((x) => x.vlan_id === id);
                              return !!row && vlanConfiguredOnPorts(row);
                            }))
                        }
                        onClick={() => deleteVlan(r)}
                        title={
                          r.vlan_id === 1
                            ? "VLAN 1 нельзя удалить"
                            : !r.in_database
                              ? "Уже нет в vlan database (остаток FDB/кэша — удалять нечего)"
                              : vlanConfiguredOnPorts(r)
                                ? "Сначала снимите VLAN с портов"
                                : selected.has(r.vlan_id) && selectedSorted.length > 1
                                  ? `Удалить выбранные: ${selectedSorted.join(", ")}`
                                  : "Удалить VLAN из vlan database свитча"
                        }
                      >
                        Удалить
                        {selected.has(r.vlan_id) && selectedSorted.length > 1
                          ? ` (${selectedSorted.length})`
                          : ""}
                      </button>
                    </>
                  )}
                </td>
              </tr>
            ))
          )}
        </tbody>
      </table>
      <form
        onSubmit={onSubmit}
        style={{ display: "flex", flexWrap: "wrap", gap: "0.5rem", alignItems: "flex-end", marginTop: "0.75rem" }}
      >
        {op !== "trunk_allow" && (
          <label>
            VLAN ID
            <br />
            <input
              type="number"
              min={1}
              max={4094}
              value={vlanId || ""}
              disabled={!w || busy}
              onChange={(e) => setVlanId(Number(e.target.value) || 0)}
              style={{ width: 88 }}
            />
          </label>
        )}
        <label>
          Порт
          <br />
          <select
            value={ifIndex || ""}
            disabled={!w || busy}
            onChange={(e) => setIfIndex(Number(e.target.value) || 0)}
            style={{ minWidth: 140 }}
          >
            {physical.map((p) => (
              <option key={p.if_index} value={p.if_index}>
                {portLabel(p)}
              </option>
            ))}
          </select>
        </label>
        <label>
          Тип VLAN
          <br />
          <select
            value={op}
            disabled={!w || busy}
            onChange={(e) => {
              const next = e.target.value as PortVlanOp;
              setOp(next);
              if (next === "trunk_allow" && vlanId > 0 && !allowedList.trim()) {
                setAllowedList(String(vlanId));
              }
            }}
            style={{ minWidth: 200 }}
          >
            <option value="set_access">Access / untagged</option>
            <option value="trunk_allow">Trunk / tagged</option>
            <option value="remove">Убрать с порта</option>
          </select>
        </label>
        {op === "trunk_allow" && (
          <>
            <label>
              Allow vlan
              <br />
              <select
                value={allowedMode}
                disabled={!w || busy}
                onChange={(e) => setAllowedMode(e.target.value as AllowedMode)}
                style={{ minWidth: 120 }}
                title="switchport trunk allowed vlan …"
              >
                <option value="add">add</option>
                <option value="remove">remove</option>
                <option value="all">all</option>
                <option value="except">except</option>
              </select>
            </label>
            {allowedMode !== "all" && (
              <label>
                Список VLAN
                <br />
                <input
                  type="text"
                  value={allowedList}
                  disabled={!w || busy}
                  onChange={(e) => setAllowedList(e.target.value)}
                  placeholder="10,20-22 или 10;20"
                  style={{ minWidth: 160 }}
                  title="Как на EdgeSwitch/ELTEX/SNR: add/remove/except + список"
                />
              </label>
            )}
          </>
        )}
        <button type="submit" disabled={portSubmitDisabled} title={!w ? "Нужны SSH и роль operator" : undefined}>
          {busy ? "Пишем…" : "Применить на порт"}
        </button>
      </form>
      {op === "trunk_allow" && (
        <>
          <p style={{ color: "#7a8499", fontSize: "0.8rem", marginTop: "0.35rem" }}>
            На свитч: <code>switchport mode trunk</code> и{" "}
            <code>
              switchport trunk allowed vlan{" "}
              {allowedMode === "all" ? "all" : `${allowedMode} ${allowedList.trim() || "…"}`}
            </code>
            . ELTEX: add/remove/all; SNR также except и списки через «;».
          </p>
          <p style={{ color: "#c9a227", fontSize: "0.85rem", marginTop: "0.25rem" }}>
            Управление свитчем (SSH/Web) обычно идёт в <strong>native / VLAN 1</strong>. Пока allowed не трогали —
            фактически были все VLAN, включая 1. Если прописать только «рабочий» VLAN без 1 (или remove/except
            затронет VLAN управления) — можно потерять доступ. Безопаснее:{" "}
            <code>1,&lt;vlan&gt;</code> или режим <code>all</code>.
          </p>
        </>
      )}
    </div>
  );
}
