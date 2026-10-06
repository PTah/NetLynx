/** Поля достижимости: Device или узел топологии. */
export type ReachabilityFields = {
  last_snmp_ok?: boolean | null;
  last_ping_ok?: boolean | null;
  last_ping_rtt_ms?: number | null;
  /** null/undefined = авто; true/false = ручная отметка (legacy) */
  online_override?: boolean | null;
  /** auto | ping | online | offline */
  reachability_mode?: string | null;
  device_category?: string | null;
  /** На топологии категория в kind (switch/other/…). */
  kind?: string | null;
  uisp_device_id?: string | null;
  uisp_overview_status?: string | null;
  virtual?: boolean;
};

export type OnlineOverrideMode = "auto" | "ping" | "online" | "offline";

export function onlineOverrideMode(d: ReachabilityFields): OnlineOverrideMode {
  const m = (d.reachability_mode ?? "").trim().toLowerCase();
  if (m === "auto" || m === "ping" || m === "online" || m === "offline") {
    return m;
  }
  if (d.online_override === true) return "online";
  if (d.online_override === false) return "offline";
  return "auto";
}

function categoryHint(d: ReachabilityFields): string {
  return (d.device_category ?? d.kind ?? "").trim().toLowerCase();
}

/** Свитчи/роутеры ожидают SNMP; пинг на IP может отвечать уже другое устройство. */
export function isSnmpExpectedCategory(cat?: string | null): boolean {
  const c = (cat ?? "").trim().toLowerCase();
  return c === "switch" || c === "router";
}

/**
 * Онлайн:
 * — online/offline → как задано вручную
 * — ping → только ICMP
 * — auto: SNMP ok → онлайн; свитч/роутер: только SNMP; прочие: пинг OR SNMP
 */
export function isDeviceOnline(d: ReachabilityFields): boolean {
  if (d.virtual) return false;
  const mode = onlineOverrideMode(d);
  if (mode === "online") return true;
  if (mode === "offline") return false;
  if (mode === "ping") {
    return d.last_ping_ok === true;
  }
  if (d.last_snmp_ok === true) return true;
  if (isSnmpExpectedCategory(categoryHint(d))) {
    return false;
  }
  return d.last_ping_ok === true;
}

export function deviceReachabilityLabel(d: ReachabilityFields): { text: string; color: string } {
  const mode = onlineOverrideMode(d);
  if (mode === "online") {
    return { text: "онлайн (вручную)", color: "#6d6" };
  }
  if (mode === "offline") {
    return { text: "оффлайн (вручную)", color: "#f88" };
  }
  if (mode === "ping") {
    if (d.last_ping_ok === true) {
      const rtt = d.last_ping_rtt_ms != null ? `ping ${d.last_ping_rtt_ms}ms` : "ping";
      return { text: `онлайн (${rtt})`, color: "#6d6" };
    }
    if (d.last_ping_ok == null) {
      return { text: "ещё не опрошен", color: "#9aa3b5" };
    }
    return { text: "оффлайн (нет ping)", color: "#f88" };
  }
  if (isDeviceOnline(d)) {
    const bits: string[] = [];
    if (d.last_snmp_ok === true) bits.push("SNMP");
    if (d.last_ping_ok === true) {
      bits.push(d.last_ping_rtt_ms != null ? `ping ${d.last_ping_rtt_ms}ms` : "ping");
    }
    return { text: bits.length ? `онлайн (${bits.join(" · ")})` : "онлайн", color: "#6d6" };
  }
  if (d.last_ping_ok == null && d.last_snmp_ok == null) {
    return { text: "ещё не опрошен", color: "#9aa3b5" };
  }
  // Инфра: пинг есть, SNMP нет — типичный признак смены владельца IP
  if (isSnmpExpectedCategory(categoryHint(d)) && d.last_ping_ok === true && d.last_snmp_ok === false) {
    return { text: "оффлайн (нет SNMP, ping есть — IP занят?)", color: "#f88" };
  }
  return { text: "оффлайн", color: "#f88" };
}
