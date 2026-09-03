import { useEffect, useRef, useState } from "react";
import { apiGet } from "../api";

type HostStats = {
  cpu_pct?: number | null;
  mem_used_pct?: number | null;
  disk_free_pct?: number | null;
  disk_free_gb?: number | null;
};

type DBStats = {
  total_conns?: number;
  acquired_conns?: number;
  idle_conns?: number;
  acquire_total?: number;
  max_conns?: number;
};

type SystemStatsResponse = {
  host?: HostStats;
  db?: DBStats;
};

function fmtPct(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(v)) return "—";
  return `${v.toFixed(0)}%`;
}

function fmtGB(v: number | null | undefined): string {
  if (v == null || !Number.isFinite(v)) return "";
  if (v >= 10) return `${v.toFixed(0)} ГБ`;
  return `${v.toFixed(1)} ГБ`;
}

function StatRow({ label, value, hint }: { label: string; value: string; hint?: string }) {
  return (
    <div className="sidebar-stat-row" title={hint}>
      <span className="sidebar-stat-label">{label}</span>
      <span className="sidebar-stat-value">{value}</span>
    </div>
  );
}

export default function SidebarSystemStats() {
  const [host, setHost] = useState<HostStats | null>(null);
  const [db, setDb] = useState<DBStats | null>(null);
  const prevAcquire = useRef<number | null>(null);
  const [sqlPerSec, setSqlPerSec] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;

    const load = () => {
      apiGet<SystemStatsResponse>("/api/v1/system/stats")
        .then((data) => {
          if (cancelled) return;
          setHost(data.host ?? null);
          const d = data.db ?? null;
          setDb(d);
          const total = d?.acquire_total;
          if (total != null && Number.isFinite(total)) {
            const prev = prevAcquire.current;
            prevAcquire.current = total;
            if (prev != null && total >= prev) {
              setSqlPerSec((total - prev) / 5);
            }
          }
        })
        .catch(() => {
          if (!cancelled) {
            setHost(null);
            setDb(null);
          }
        });
    };

    load();
    const t = setInterval(load, 5000);
    return () => {
      cancelled = true;
      clearInterval(t);
    };
  }, []);

  const diskValue =
    host?.disk_free_pct != null
      ? `${fmtPct(host.disk_free_pct)}${host.disk_free_gb != null ? ` (${fmtGB(host.disk_free_gb)})` : ""}`
      : "—";

  const sqlActive = db?.acquired_conns ?? null;
  const sqlMax = db?.max_conns ?? null;
  const sqlValue =
    sqlActive != null && sqlMax != null
      ? `${sqlActive}/${sqlMax}${sqlPerSec != null ? ` · ${sqlPerSec.toFixed(1)}/с` : ""}`
      : "—";

  return (
    <div className="sidebar-stats" aria-label="Загрузка сервера">
      <StatRow label="CPU" value={fmtPct(host?.cpu_pct)} hint="Загрузка процессора хоста" />
      <StatRow label="RAM" value={fmtPct(host?.mem_used_pct)} hint="Использование оперативной памяти" />
      <StatRow
        label="SQL"
        value={sqlValue}
        hint="Активные соединения с PostgreSQL (занято/макс.) и запросов к пулу в секунду"
      />
      <StatRow label="Диск" value={diskValue} hint="Свободное место на корневом разделе" />
    </div>
  );
}
