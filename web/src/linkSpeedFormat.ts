/** Скорость линка в Мбит/с (ifHighSpeed приоритетнее ifSpeed). */
export function linkMbps(ifHighSpeed?: number | null, ifSpeed?: number | null): number {
  if (ifHighSpeed && ifHighSpeed > 0) return ifHighSpeed;
  if (ifSpeed && ifSpeed > 0) return ifSpeed / 1_000_000;
  return 0;
}

/** Отображение: ≥1 Гбит/с — в Гбит/с, иначе в Мбит/с. */
export function formatLinkSpeedMbps(mbps: number): string {
  if (!Number.isFinite(mbps) || mbps <= 0) return "—";
  if (mbps >= 1000) {
    const g = mbps / 1000;
    const rounded = Math.round(g * 10) / 10;
    const text = Number.isInteger(rounded) ? String(Math.round(rounded)) : String(rounded).replace(/\.0$/, "");
    return `${text} Гбит/с`;
  }
  return `${Math.round(mbps)} Мбит/с`;
}

export function formatPortSpeedFromRow(ifHighSpeed?: number | null, ifSpeed?: number | null): string {
  return formatLinkSpeedMbps(linkMbps(ifHighSpeed, ifSpeed));
}
