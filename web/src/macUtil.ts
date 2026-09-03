/** Нормализация MAC для сравнения: aa:bb → aabb */
export function normalizeMacQuery(s: string): string {
  return s.toLowerCase().replace(/[^0-9a-f]/g, "");
}

/**
 * MAC / chassis: классический aa:bb:…:ff (12 hex) или компактный hex без разделителей.
 */
export function looksLikeMac(s: string): boolean {
  const t = s.trim();
  if (!t) return false;
  if (/^\d{1,3}(\.\d{1,3}){3}$/.test(t)) return false;
  if (t.includes("::")) return false; // IPv6
  if (/^chassis:/i.test(t)) {
    const hex = normalizeMacQuery(t.slice(t.indexOf(":") + 1));
    return hex.length >= 6 && hex.length <= 16 && hex.length % 2 === 0;
  }
  const colons = (t.match(/:/g) || []).length;
  if (colons > 5) return false;
  if (!/^[0-9a-fA-F:.\-]+$/.test(t)) return false;
  const n = normalizeMacQuery(t);
  return n.length >= 6 && n.length <= 16 && n.length % 2 === 0;
}

/** Чётная длина hex 6–16 → aa:bb:…; иначе исходная. */
export function formatMacDisplay(raw: string): string {
  const t = raw.trim();
  if (!t) return t;
  let hex = normalizeMacQuery(t);
  if ((hex.length < 6 || hex.length % 2 !== 0) && /^chassis:/i.test(t)) {
    hex = normalizeMacQuery(t.slice(t.indexOf(":") + 1));
  }
  if (hex.length === 10 && hex.startsWith("01")) {
    const octets = [2, 4, 6, 8].map((i) => parseInt(hex.slice(i, i + 2), 16));
    if (octets.every((n) => Number.isFinite(n) && n >= 0 && n <= 255)) {
      return octets.join(".");
    }
  }
  if (hex.length < 6 || hex.length > 16 || hex.length % 2 !== 0) return t;
  return hex.match(/.{1,2}/g)!.join(":");
}

/** Подпись вендора по OUI: только если нет IP и имя известно. */
export function macVendorLabel(vendor?: string | null, hasIP?: boolean): string | null {
  if (hasIP) return null;
  const v = vendor?.trim();
  return v ? v : null;
}

export function selectElementText(el: HTMLElement) {
  const range = document.createRange();
  range.selectNodeContents(el);
  const sel = window.getSelection();
  sel?.removeAllRanges();
  sel?.addRange(range);
}

export function copyTextToClipboard(text: string) {
  const t = text.trim();
  if (!t) return;
  if (navigator.clipboard?.writeText) {
    void navigator.clipboard.writeText(t);
    return;
  }
  try {
    document.execCommand("copy");
  } catch {
    /* ignore */
  }
}
