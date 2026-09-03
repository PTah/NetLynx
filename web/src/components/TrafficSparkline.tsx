import type { CSSProperties } from "react";

/** Мини-спарклайн (столбики) для Rx/Tx в таблице портов. */
export function TrafficSparkline({
  values,
  color = "#7c6cf0",
  width = 72,
  height = 22,
  title,
}: {
  values: number[];
  color?: string;
  width?: number;
  height?: number;
  title?: string;
}) {
  if (!values.length) {
    return <span style={{ color: "#5a6478", fontSize: "0.75rem" }}>—</span>;
  }
  const max = Math.max(...values, 1e-9);
  const n = values.length;
  const gap = 1;
  const barW = Math.max(1, (width - gap * (n - 1)) / n);
  const bars = values.map((v, i) => {
    const h = Math.max(1, (v / max) * (height - 2));
    const x = i * (barW + gap);
    const y = height - h;
    return <rect key={i} x={x} y={y} width={barW} height={h} fill={color} rx={0.5} />;
  });
  return (
    <svg width={width} height={height} viewBox={`0 0 ${width} ${height}`} aria-hidden style={{ display: "block" }}>
      {title ? <title>{title}</title> : null}
      {bars}
    </svg>
  );
}

export function formatBitRate(bps: number | null | undefined): string {
  if (bps == null || !Number.isFinite(bps) || bps < 0) return "—";
  if (bps < 1000) return `${bps.toFixed(0)} bps`;
  const kbps = bps / 1000;
  if (kbps < 1000) return `${kbps < 10 ? kbps.toFixed(1) : kbps.toFixed(0)} kbps`;
  const mbps = kbps / 1000;
  if (mbps < 1000) return `${mbps < 10 ? mbps.toFixed(1) : mbps.toFixed(0)} Mbps`;
  return `${(mbps / 1000).toFixed(2)} Gbps`;
}

/** Компактная полоска утилизации под иконкой порта (всегда 3px высоты — чтобы сетка не «прыгала»). */
export function PortUtilBar({ pct, style }: { pct: number | null | undefined; style?: CSSProperties }) {
  const has = pct != null && Number.isFinite(pct) && pct > 0;
  const w = has ? Math.min(100, Math.max(4, pct as number)) : 0;
  return (
    <div
      title={has ? `утилизация ~${(pct as number).toFixed(0)}%` : undefined}
      style={{
        width: 32,
        height: 3,
        borderRadius: 1,
        background: has ? "rgba(255,255,255,0.12)" : "transparent",
        overflow: "hidden",
        flexShrink: 0,
        ...style,
      }}
    >
      {has ? (
        <div style={{ width: `${w}%`, height: "100%", background: (pct as number) >= 80 ? "#e07040" : "#6b8af0" }} />
      ) : null}
    </div>
  );
}
