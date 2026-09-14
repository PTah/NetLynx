import type { EventRow } from "./types";
import { formatLinkSpeedMbps } from "./linkSpeedFormat";

/** Источник события: опрос SNMP или SNMP trap (для LINK_*). */
export function formatEventSourceLabel(payload?: Record<string, unknown> | null): string {
  const p = payload ?? {};
  if (p.trap_confirmed === true) return "trap+опрос";
  if (str(p.source) === "trap") return "trap";
  if (str(p.source) === "syslog") return "syslog";
  if (str(p.source) === "ui") return "UI";
  return "опрос";
}

/** Короткое название типа события для таблиц и фильтров (русский UI). */
export function formatEventTypeLabel(eventType: string): string {
  switch (eventType) {
    case "LINK_UP":
      return "Порт подключен";
    case "LINK_DOWN":
      return "Порт отключен";
    case "DEVICE_OFFLINE":
      return "Устройство ушло оффлайн";
    case "DEVICE_ONLINE":
      return "Устройство снова онлайн";
    case "PORT_UTILIZATION_HIGH":
      return "Повышенная нагрузка на порту";
    case "PORT_UTILIZATION_OK":
      return "Загрузка порта в норме";
    case "PORT_SPEED_DOWN":
      return "Скорость порта снизилась";
    case "PORT_SPEED_OK":
      return "Скорость порта восстановилась";
    case "UNKNOWN_MAC_ON_ACCESS_PORT":
      return "Неизвестный MAC";
    case "MAC_MOVED":
      return "MAC перешёл на другой порт";
    case "MAC_FLAPPING":
      return "MAC flapping";
    case "PORT_FLAP":
      return "Порт flapping (link bounce)";
    case "L2_LOOP_APPEARED":
      return "Обнаружена L2-петля";
    case "MAC_MULTI_ACCESS":
      return "MAC на нескольких access";
    case "STP_TOPOLOGY_CHANGE":
      return "STP: смена топологии";
    case "STP_ROOT_CHANGED":
      return "STP: сменился root";
    case "BROADCAST_STORM_SUSPECTED":
      return "Подозрение на broadcast storm";
    case "BROADCAST_STORM_OK":
      return "Broadcast storm: норма";
    case "MAC_REMOVED":
      return "MAC исчез с порта";
    case "SNMP_TRAP":
      return "SNMP trap";
    case "ACCESS_PORT_MAC_SUBSTITUTED":
      return "Смена MAC на access-порту (возможное вторжение)";
    case "ACCESS_PORT_LONG_IDLE_DEVICE":
      return "Устройство на порту после долгого простоя";
    case "MANUAL_LINK_SUPERSEDED":
      return "Ручная связь заменена LLDP/CDP";
    case "CONFIG_EDIT":
      return "Правка конфига";
    case "PORT_ADMIN_DOWN_ACTION":
      return "Авто-shutdown порта";
    default:
      return eventType;
  }
}

/** Краткая аббревиатура, как в колонке «Последнее событие» на дашборде. */
export function formatDashboardEventAbbrev(eventType: string): string | null {
  switch (eventType) {
    case "PORT_UTILIZATION_HIGH":
      return "PUH";
    case "PORT_UTILIZATION_OK":
      return "PUO";
    case "PORT_SPEED_DOWN":
      return "PSD";
    case "PORT_SPEED_OK":
      return "PSO";
    case "MAC_REMOVED":
      return "MR";
    case "PORT_FLAP":
      return "PFL";
    case "L2_LOOP_APPEARED":
      return "L2L";
    case "UNKNOWN_MAC_ON_ACCESS_PORT":
      return "UMAC!";
    default:
      return null;
  }
}

/** Подсказка при наведении на «Последнее событие»: расшифровка аббревиатуры + суть. */
export function formatDashboardLastEventTooltip(
  ev: Pick<EventRow, "event_type" | "payload" | "if_index">,
): string {
  const label = formatEventTypeLabel(ev.event_type);
  const abbrev = formatDashboardEventAbbrev(ev.event_type);
  const summary = formatEventSummary(ev);
  const head = abbrev ? `${abbrev} — ${label}` : label;
  if (!summary || summary === "—" || summary === label) return head;
  if (summary.startsWith(label) || summary.includes(label)) return `${head}. ${summary}`;
  return `${head}. ${summary}`;
}

function asNum(v: unknown): number | null {
  if (typeof v === "number" && Number.isFinite(v)) return v;
  if (typeof v === "string" && v.trim() !== "") {
    const n = Number(v);
    if (Number.isFinite(n)) return n;
  }
  return null;
}

function fmtPct(v: unknown, digits: number): string {
  const n = asNum(v);
  if (n == null) return "—";
  return `${n.toFixed(digits)}%`;
}

function str(v: unknown): string {
  if (v == null) return "";
  return String(v).trim();
}

/**
 * Человекочитаемая подпись порта для событий — как в карточке узла:
 * Ubiquiti «port N: подпись» → только подпись; иначе ifAlias (колонка «Комментарий»); иначе «Порт №» (if_name);
 * иначе if_descr (на бэкенде для новых событий уже с учётом alias, как в БД).
 */
function eventPortDescription(p: Record<string, unknown>): string {
  const ifName = str(p.if_name);
  const ifAlias = str(p.if_alias);
  const descr = str(p.if_descr);

  if (ifName) {
    const ubnt = ifName.match(/^\s*port\s+\d+\s*:\s*(.*)$/i);
    if (ubnt) {
      const tail = ubnt[1].trim();
      if (tail) return tail;
    }
  }
  if (ifAlias) return ifAlias;
  if (descr) return descr;
  if (ifName) return ifName;
  return "";
}

/**
 * Колонка «Порт» в таблице событий: номер (ifIndex или разбор из имени) и подпись порта (см. eventPortDescription).
 */
export function formatEventPortColumn(ev: Pick<EventRow, "if_index" | "payload" | "event_type">): string {
  const p = ev.payload ?? {};
  const idx = ev.if_index;
  let num = "";
  if (idx != null && Number.isFinite(Number(idx)) && Number(idx) > 0) {
    num = String(idx);
  } else {
    const raw = str(p.if_name) || str(p.if_descr);
    const slash = raw.match(/\/(\d+)\s*$/);
    if (slash) num = slash[1];
    else {
      const portWord = raw.match(/Port:\s*(\d+)/i);
      if (portWord) num = portWord[1];
    }
  }
  const descr = eventPortDescription(p);

  if (ev.event_type === "MAC_MOVED") {
    const o = p.old_if_index != null ? String(p.old_if_index) : "?";
    const n = p.new_if_index != null ? String(p.new_if_index) : "?";
    return `${o} → ${n}`;
  }

  if (ev.event_type === "CONFIG_EDIT" && Array.isArray(p.if_indexes) && p.if_indexes.length > 1) {
    const idxs = p.if_indexes.map((x) => String(x)).filter(Boolean);
    if (idxs.length <= 6) return idxs.join(",");
    return `${idxs.slice(0, 4).join(",")}…(+${idxs.length - 4})`;
  }

  if (!num && !descr) return "—";
  if (num && descr) return `${num} · ${descr}`;
  if (num) return num;
  return descr;
}

/** Номер/подпись порта для LINK_*: сначала if_index, иначе разбор if_name (например 0/13 → 13). */
function portLabel(ev: Pick<EventRow, "if_index" | "payload">): string {
  const idx = ev.if_index;
  if (idx != null && Number.isFinite(Number(idx)) && Number(idx) > 0) {
    return `Порт ${idx}`;
  }
  const p = ev.payload ?? {};
  const raw = str(p.if_name) || str(p.if_descr);
  const slash = raw.match(/\/(\d+)\s*$/);
  if (slash) {
    return `Порт ${slash[1]}`;
  }
  const portWord = raw.match(/Port:\s*(\d+)/i);
  if (portWord) {
    return `Порт ${portWord[1]}`;
  }
  return raw ? `Порт (${raw})` : "Порт";
}

/** Краткое человекочитаемое описание события для таблиц. */
export function formatEventSummary(ev: Pick<EventRow, "event_type" | "payload" | "if_index">): string {
  const p = ev.payload ?? {};
  switch (ev.event_type) {
    case "PORT_UTILIZATION_HIGH": {
      const port = eventPortDescription(p) || "порт";
      const max = fmtPct(p.util_max_pct, 1);
      const th = fmtPct(p.threshold_pct, 0);
      const inn = fmtPct(p.util_in_pct, 1);
      const out = fmtPct(p.util_out_pct, 1);
      return `Утилизация выше порога ${th}: максимум ${max} (TX ${inn}, RX ${out}). ${port}`;
    }
    case "PORT_UTILIZATION_OK": {
      return `Загрузка порта на момент события: ${fmtPct(p.util_max_pct, 1)}`;
    }
    case "PORT_SPEED_DOWN":
    case "PORT_SPEED_OK": {
      const oldM = asNum(p.old_mbps);
      const newM = asNum(p.new_mbps);
      const port = eventPortDescription(p);
      const from = oldM != null ? formatLinkSpeedMbps(oldM) : "—";
      const to = newM != null ? formatLinkSpeedMbps(newM) : "—";
      const base =
        ev.event_type === "PORT_SPEED_DOWN"
          ? `Скорость снизилась: ${from} → ${to}`
          : `Скорость выросла: ${from} → ${to}`;
      return port ? `${base}. ${port}` : base;
    }
    case "LINK_UP": {
      const base = `${portLabel(ev)} подключен`;
      if (p.trap_confirmed) return `${base} (подтверждено: trap+опрос)`;
      if (str(p.source) === "trap") return `${base} (по SNMP trap)`;
      return base;
    }
    case "LINK_DOWN": {
      const base = `${portLabel(ev)} отключен`;
      if (p.trap_confirmed) return `${base} (подтверждено: trap+опрос)`;
      if (str(p.source) === "trap") return `${base} (по SNMP trap)`;
      return base;
    }
    case "DEVICE_OFFLINE": {
      const host = str(p.host);
      const reason = str(p.reason);
      const why =
        reason === "snmp"
          ? "SNMP недоступен"
          : reason === "ping"
            ? "ICMP недоступен"
            : reason === "override"
              ? "ручная отметка"
              : "нет связи";
      return host ? `Узел ${host} оффлайн (${why})` : `Устройство оффлайн (${why})`;
    }
    case "DEVICE_ONLINE": {
      const host = str(p.host);
      const sec = asNum(p.offline_duration_sec);
      const dur =
        sec != null && sec >= 3600
          ? `${(sec / 3600).toFixed(1)} ч`
          : sec != null && sec >= 60
            ? `${Math.round(sec / 60)} мин`
            : sec != null
              ? `${Math.round(sec)} с`
              : "";
      if (host && dur) return `Узел ${host} снова онлайн (был оффлайн ${dur})`;
      if (host) return `Узел ${host} снова онлайн`;
      return "Устройство снова онлайн";
    }
    case "UNKNOWN_MAC_ON_ACCESS_PORT": {
      const mac = str(p.mac);
      return mac ? `Неизвестный MAC на access: ${mac}` : "Неизвестный MAC на access-порту";
    }
    case "MAC_MOVED": {
      const mac = str(p.mac);
      return mac ? `MAC ${mac}: порт ${p.old_if_index ?? "?"} → ${p.new_if_index ?? "?"}` : "MAC перешёл на другой порт";
    }
    case "MAC_FLAPPING": {
      const mac = str(p.mac);
      const src = str(p.source) || "опрос";
      const ports = Array.isArray(p.ports) ? p.ports.join(", ") : "";
      if (mac && ports) return `Flapping ${mac} [${ports}] (${src})`;
      if (mac) return `Flapping ${mac} (${src})`;
      return "MAC flapping";
    }
    case "PORT_FLAP": {
      const n = asNum(p.bounce_count);
      const win = asNum(p.window_sec);
      const sources = Array.isArray(p.sources) ? p.sources.join("+") : "";
      const iface = str(p.if_name) || str(p.if_descr) || (p.if_index != null ? `if ${p.if_index}` : "порт");
      const winLabel = win != null ? ` за ${Math.round(win / 60)} мин` : "";
      const srcLabel = sources ? ` (${sources})` : "";
      return n != null
        ? `Порт flapping: ${iface}, ${n} bounce${winLabel}${srcLabel}`
        : `Порт flapping: ${iface}${srcLabel}`;
    }
    case "L2_LOOP_APPEARED": {
      const summary = str(p.summary);
      const len = asNum(p.length);
      if (summary) return `L2-петля: ${summary}`;
      if (len != null) return `Обнаружена L2-петля (длина ${len})`;
      return "Обнаружена L2-петля";
    }
    case "MAC_MULTI_ACCESS": {
      const mac = str(p.mac);
      const n = asNum(p.count);
      return mac
        ? `MAC ${mac} на ${n ?? "нескольких"} access-портах`
        : "Один MAC на нескольких access-портах";
    }
    case "STP_ROOT_CHANGED": {
      const root = str(p.designated_root);
      return root ? `STP root: ${root}` : "STP: сменился root";
    }
    case "BROADCAST_STORM_SUSPECTED": {
      const n = asNum(p.high_util_ports);
      const delta = asNum(p.fdb_delta);
      const parts: string[] = [];
      if (n != null) parts.push(`${n} порт(ов) >${asNum(p.util_threshold_pct) ?? 80}%`);
      if (delta != null && delta > 0) parts.push(`FDB +${delta}`);
      return parts.length ? `Broadcast storm?: ${parts.join(", ")}` : "Подозрение на broadcast storm";
    }
    case "BROADCAST_STORM_OK": {
      const n = asNum(p.high_util_ports);
      return n != null ? `Broadcast storm: норма (${n} порт(ов) с высокой util)` : "Broadcast storm: норма";
    }
    case "MAC_REMOVED": {
      const mac = str(p.mac);
      return mac ? `MAC исчез с FDB: ${mac}` : "MAC исчез с FDB";
    }
    case "SNMP_TRAP": {
      const summary = str(p.trap_summary);
      const label = str(p.trap_label);
      const src = str(p.source_ip);
      if (summary && src) return `${summary} (${src})`;
      if (summary) return summary;
      if (label && src) return `${label} от ${src}`;
      const oid = str(p.trap_oid);
      if (oid && src) return `Trap ${oid} от ${src}`;
      if (oid) return `Trap ${oid}`;
      if (src) return `Trap от ${src}`;
      return "SNMP trap";
    }
    case "ACCESS_PORT_MAC_SUBSTITUTED": {
      const o = str(p.old_mac);
      const n = str(p.new_mac);
      const port = eventPortDescription(p);
      if (o && n) {
        return port ? `MAC ${o} → ${n}. ${port}` : `MAC ${o} → ${n}`;
      }
      return "Смена MAC на access-порту";
    }
    case "ACCESS_PORT_LONG_IDLE_DEVICE": {
      const mac = str(p.mac);
      const ih = asNum(p.idle_hours);
      const idle =
        ih != null && ih >= 24
          ? `${(ih / 24).toFixed(1)} сут.`
          : ih != null
            ? `${ih.toFixed(1)} ч`
            : "";
      const port = eventPortDescription(p);
      if (mac && idle) {
        return port ? `После простоя ${idle}: MAC ${mac}. ${port}` : `После простоя ${idle}: MAC ${mac}`;
      }
      return mac ? `Новое устройство: ${mac}` : "Активность на порту после долгого простоя";
    }
    case "MANUAL_LINK_SUPERSEDED": {
      const id = p.manual_link_id != null ? String(p.manual_link_id) : "?";
      const proto = str(p.discovered_protocol) || "LLDP/CDP";
      return `Ручная связь #${id} снята: появился ${proto.toUpperCase()}`;
    }
    case "CONFIG_EDIT": {
      const user = str(p.username) || "неизвестный";
      const change = formatConfigEditChange(str(p.change));
      const detail = formatConfigEditDetail(p);
      if (detail) return `${user}: ${change} — ${detail}`;
      return `${user}: ${change}`;
    }
    default:
      return Object.keys(p).length ? JSON.stringify(p) : "—";
  }
}

/** Человекочитаемое имя поля правки конфига. */
function formatConfigEditChange(change: string): string {
  switch (change) {
    case "port.admin":
      return "порт: admin";
    case "port.descr":
      return "порт: описание";
    case "port.poe":
      return "порт: PoE";
    case "port.poe_reset":
      return "порт: PoE reset";
    case "port.bulk":
      return "порты: массово";
    case "port.isolate":
      return "порт: isolate";
    case "port.flow_control":
      return "порт: flow control";
    case "port.stp":
      return "порт: STP";
    case "port.dhcp_snooping":
      return "порт: DHCP snooping";
    case "port.vlan":
      return "порт: VLAN";
    case "port.thresholds":
      return "порт: пороги утилизации";
    case "device.snmp":
      return "узел: SNMP";
    case "device.ssh":
      return "узел: SSH";
    case "device.monitoring":
      return "узел: мониторинг";
    case "device.name":
      return "узел: имя";
    case "device.host":
      return "узел: адрес";
    case "device.location":
      return "узел: локация";
    case "device.category":
      return "узел: тип";
    case "device.poll_interval":
      return "узел: интервал опроса";
    case "device.online_override":
      return "узел: ручной online/offline";
    case "device.trust_link_traps":
      return "узел: trust link traps";
    case "device.chassis_mac":
      return "узел: chassis MAC";
    case "vlan.database.create":
      return "VLAN database: создать";
    case "vlan.database.name":
      return "VLAN database: имя";
    case "vlan.database.delete":
      return "VLAN database: удалить";
    default:
      return change || "изменение";
  }
}

function formatBulkNotesHuman(notes: unknown): string[] {
  if (!Array.isArray(notes)) return [];
  const out: string[] = [];
  for (const raw of notes) {
    const n = str(raw);
    if (!n) continue;
    if (n === "admin=true") out.push("admin вкл");
    else if (n === "admin=false") out.push("admin выкл (shutdown)");
    else if (n.startsWith("poe=")) out.push(`PoE ${n.slice(4)}`);
    else if (n.startsWith("vlan=no_vlan")) out.push("VLAN: No VLAN");
    else if (n.startsWith("vlan=set_access/")) out.push(`VLAN access ${n.slice("vlan=set_access/".length)}`);
    else if (n.startsWith("vlan=")) out.push(`VLAN ${n.slice(5)}`);
    else if (n.startsWith("poe_reset=")) out.push(`PoE reset ${n.slice("poe_reset=".length)}`);
    else out.push(n);
  }
  return out;
}

function formatConfigEditDetail(p: Record<string, unknown>): string {
  const parts: string[] = [];
  const change = str(p.change);

  if (change === "port.bulk") {
    const summary = str(p.summary);
    if (summary) return summary;
    const idxs = Array.isArray(p.if_indexes) ? p.if_indexes.map((x) => String(x)).filter(Boolean) : [];
    let actions = formatBulkNotesHuman(p.notes);
    const applied = p.applied && typeof p.applied === "object" ? (p.applied as Record<string, unknown>) : null;
    if (actions.length === 0 && applied) {
      actions = [];
      if (typeof applied.admin_up === "boolean") actions.push(applied.admin_up ? "admin вкл" : "admin выкл");
      if (applied.poe_mode != null) actions.push(`PoE ${str(applied.poe_mode)}`);
      if (applied.vlan_op === "no_vlan") actions.push("VLAN: No VLAN");
      else if (applied.vlan_op === "set_access" && applied.vlan_id != null) actions.push(`VLAN access ${applied.vlan_id}`);
      if (applied.poe_reset_seconds != null) actions.push(`PoE reset ${applied.poe_reset_seconds}s`);
    }
    const head = idxs.length ? `${idxs.length} порт(ов): if ${idxs.join(",")}` : "несколько портов";
    return actions.length ? `${head} — ${actions.join("; ")}` : head;
  }

  if (change === "port.poe_reset") {
    const sec = asNum(p.seconds);
    if (sec != null) parts.push(`${sec} с`);
  }

  const action = str(p.action);
  if (action === "shutdown") parts.push("выкл");
  else if (action === "no_shutdown") parts.push("вкл");
  if (p.poe_mode != null) parts.push(`PoE ${str(p.poe_mode)}`);
  if (typeof p.isolate === "boolean") parts.push(p.isolate ? "isolate on" : "isolate off");
  if (typeof p.flow_control === "boolean") parts.push(p.flow_control ? "FC on" : "FC off");
  if (typeof p.trusted === "boolean") parts.push(p.trusted ? "trusted" : "untrusted");
  if (p.op != null) {
    const vlan = p.vlan_id != null ? ` VLAN ${p.vlan_id}` : "";
    parts.push(`${str(p.op)}${vlan}`);
  }
  if (p.vlan_id != null && p.op == null && change !== "port.bulk") parts.push(`VLAN ${p.vlan_id}`);
  if (Array.isArray(p.vlan_ids) && p.vlan_ids.length) parts.push(`VLAN ${p.vlan_ids.join(", ")}`);
  if (p.descr != null) {
    const d = str(p.descr);
    parts.push(d ? `«${d}»` : "(очищено)");
  }
  if (p.name != null && (change.startsWith("device.") || change.startsWith("vlan."))) {
    const n = str(p.name);
    if (n) parts.push(n);
  }
  if (p.host != null) parts.push(str(p.host) || "(пусто)");
  if (p.location != null) parts.push(str(p.location) || "(пусто)");
  if (p.device_category != null) parts.push(str(p.device_category));
  if (p.poll_interval_seconds != null) parts.push(`${p.poll_interval_seconds} с`);
  if (p.mode != null) parts.push(str(p.mode));
  if (typeof p.trust_link_traps === "boolean") parts.push(p.trust_link_traps ? "вкл" : "выкл");
  if (p.chassis_mac != null) parts.push(str(p.chassis_mac) || "(очищено)");
  if (p.snmp_version != null) parts.push(str(p.snmp_version));
  if (p.via != null) parts.push(`via ${str(p.via)}`);
  return parts.join(", ");
}
