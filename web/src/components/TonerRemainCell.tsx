import type { Device } from "../types";
import {
  TONER_CRITICAL_PCT,
  TONER_LOW_PCT,
  formatTonerPct,
  tonerLetter,
  tonerSwatch,
} from "../printerMetrics";

export function TonerRemainCell({
  toners,
  muted,
}: {
  toners?: Device["last_toners"];
  muted: string;
}) {
  if (!toners?.length) return <span style={{ color: muted }}>—</span>;
  return (
    <span style={{ display: "inline-flex", flexWrap: "wrap", gap: "0.35rem 0.65rem" }}>
      {toners.map((t, i) => {
        const critical = t.pct != null && t.pct <= TONER_CRITICAL_PCT;
        const low = t.pct != null && t.pct <= TONER_LOW_PCT;
        return (
          <span
            key={`${t.key}-${i}`}
            title={t.description || t.label}
            style={{
              color: critical ? "#ff5252" : low ? "#f88" : undefined,
              fontWeight: critical || low ? 700 : undefined,
            }}
          >
            <span style={{ color: tonerSwatch(t.key), fontWeight: 700 }}>{tonerLetter(t.key)}</span>{" "}
            {formatTonerPct(t)}
          </span>
        );
      })}
    </span>
  );
}
