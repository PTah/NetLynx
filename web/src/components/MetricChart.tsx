type Point = { value: number; sampled_at: string };

type Props = {
  title: string;
  samples: Point[];
  unit?: string;
  maxY?: number;
};

export default function MetricChart({ title, samples, unit = "%", maxY = 100 }: Props) {
  const w = 480;
  const h = 120;
  const pad = 8;
  if (!samples.length) {
    return (
      <div style={{ marginBottom: "1rem" }}>
        <div style={{ fontWeight: 600, marginBottom: 4 }}>{title}</div>
        <div style={{ color: "#888", fontSize: "0.9rem" }}>Нет данных за выбранный период</div>
      </div>
    );
  }
  const vals = samples.map((s) => s.value);
  const minV = 0;
  const maxV = Math.max(maxY, ...vals, 1);
  const innerW = w - pad * 2;
  const innerH = h - pad * 2;
  const pts = samples.map((s, i) => {
    const x = pad + (samples.length === 1 ? innerW / 2 : (i / (samples.length - 1)) * innerW);
    const y = pad + innerH - ((s.value - minV) / (maxV - minV)) * innerH;
    return `${x},${y}`;
  });
  const last = samples[samples.length - 1];
  return (
    <div style={{ marginBottom: "1rem" }}>
      <div style={{ fontWeight: 600, marginBottom: 4 }}>
        {title}{" "}
        <span style={{ color: "#8cf", fontWeight: 400 }}>
          {last.value.toFixed(1)}
          {unit}
        </span>
      </div>
      <svg width={w} height={h} style={{ background: "#1a1f2a", borderRadius: 8, display: "block" }}>
        <polyline fill="none" stroke="#5b9bd5" strokeWidth="2" points={pts.join(" ")} />
      </svg>
    </div>
  );
}
