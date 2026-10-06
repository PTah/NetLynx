/** sysUpTime (сотые доли секунды) с поправкой на время с last_poll_at; null если нет данных. */
export function sysUptimeCsNow(
  uptimeCs: number | null | undefined,
  polledAt: string | null | undefined,
  nowMs: number = Date.now(),
): number | null {
  if (uptimeCs == null || uptimeCs < 0) return null;
  const polled = polledAt?.trim();
  let totalCs = uptimeCs;
  if (polled) {
    const polledMs = new Date(polled).getTime();
    if (!Number.isNaN(polledMs)) {
      totalCs += Math.max(0, Math.floor((nowMs - polledMs) / 10));
    }
  }
  return totalCs;
}

/** sysUpTime из SNMP (сотые доли секунды) + момент опроса → человекочитаемый uptime. */
export function formatSysUptime(
  uptimeCs: number | null | undefined,
  polledAt: string | null | undefined,
  nowMs: number = Date.now(),
): string {
  const totalCs = sysUptimeCsNow(uptimeCs, polledAt, nowMs);
  if (totalCs == null) return "";
  const sec = Math.floor(totalCs / 100);
  const days = Math.floor(sec / 86400);
  const hours = Math.floor((sec % 86400) / 3600);
  const mins = Math.floor((sec % 3600) / 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days} ${days === 1 ? "день" : days < 5 ? "дня" : "дней"}`);
  if (hours > 0) parts.push(`${hours} ч`);
  if (mins > 0 || parts.length === 0) parts.push(`${mins} мин`);
  return parts.join(" ");
}
