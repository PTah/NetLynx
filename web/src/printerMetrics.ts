import type { PrinterToner } from "./types";

const TONER_LETTER: Record<string, string> = {
  black: "K",
  cyan: "C",
  magenta: "M",
  yellow: "Y",
};

const TONER_COLOR: Record<string, string> = {
  black: "#c8c8c8",
  cyan: "#3ec7d6",
  magenta: "#e07ad4",
  yellow: "#d4c44a",
};

export function tonerLetter(key: string): string {
  return TONER_LETTER[key] ?? (key.slice(0, 1).toUpperCase() || "?");
}

export function tonerSwatch(key: string): string {
  return TONER_COLOR[key] ?? "#9aa3b5";
}

export function formatPageCount(n: number | null | undefined): string {
  if (n == null || !Number.isFinite(n)) return "—";
  return Math.round(n).toLocaleString("ru-RU");
}

export function formatTonerPct(t: PrinterToner): string {
  if (t.pct != null && Number.isFinite(t.pct)) return `${Math.round(t.pct)}%`;
  if (t.remaining_ok) return "есть";
  if (t.unknown) return "н/д";
  if (t.level != null && t.max != null && t.max > 0) {
    return `${Math.round((t.level / t.max) * 100)}%`;
  }
  return "—";
}

export function tonerMetricType(t: PrinterToner): string {
  return t.metric_type || `toner_${t.key || "other"}_pct`;
}

export function isColorMfu(toners: PrinterToner[] | null | undefined): boolean {
  if (!toners?.length) return false;
  const keys = new Set(toners.map((t) => t.key));
  return keys.has("cyan") || keys.has("magenta") || keys.has("yellow");
}
