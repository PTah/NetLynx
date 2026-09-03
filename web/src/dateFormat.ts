/** Локальное время: ДД/ММ/ГГГГ ЧЧ:мм:сс */
export function formatDateTimeRU(value: string | null | undefined): string {
  if (!value) return "";
  const d = new Date(value);
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 1970) return "";
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
}

/** Подпись для stale-линка топологии. */
export function formatTopologyStaleAt(value: string | null | undefined): string {
  const at = formatDateTimeRU(value);
  return at ? `устарело с ${at}` : "устарело";
}
