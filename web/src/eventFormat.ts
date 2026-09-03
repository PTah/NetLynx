import type { EventRow } from "./types";
import { formatLinkSpeedMbps } from "./linkSpeedFormat";

/** Источник события: опрос SNMP или SNMP trap (для LINK_*). */
export function formatEventSourceLabel(payload?: Record<string, unknown> | null): string {
  const p = payload ?? {};
  if (p.trap_confirmed === true) return "trap+опрос";
  if (str(p.source) === "trap") return "trap";
  if (str(p.source) === "syslog") return "syslog";
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
    default:
      return Object.keys(p).length ? JSON.stringify(p) : "—";
  }
}
