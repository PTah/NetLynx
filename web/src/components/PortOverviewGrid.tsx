import type { CSSProperties } from "react";
import { useEffect, useMemo, useState } from "react";
import { formatLinkSpeedMbps, linkMbps } from "../linkSpeedFormat";
import { resolvePortModelFromHint } from "../portModelCatalog";
import { PortUtilBar } from "./TrafficSparkline";

export type PortRow = {
  if_index: number;
  if_name?: string | null;
  if_descr?: string | null;
  /** IANAifType (SNMP ifType), для эвристики медь/оптика. */
  if_type?: number | null;
  if_speed?: number | null;
  if_high_speed?: number | null;
  port_role?: string | null;
  admin_status?: number | null;
  oper_status?: number | null;
  /** PoE: PSE-MIB и/или SSH; LLDP-PD — только если оба молчат. */
  poe_active?: boolean | null;
  /** Текущая мощность PoE в ваттах (если доступна из MIB). */
  poe_power_w?: number | null;
  descr_override?: string | null;
  cli_description?: string | null;
  util_in_pct?: number | null;
  util_out_pct?: number | null;
  util_max_pct?: number | null;
};

function portDisplayDescr(p: PortRow): string {
  const ov = (p.descr_override ?? "").trim();
  if (ov) return ov;
  const cli = (p.cli_description ?? "").trim();
  if (cli) return cli;
  return (p.if_descr ?? "").trim();
}

type SpeedTier = "10g" | "1g" | "100m" | "10m" | "other";

function speedTier(mbps: number): SpeedTier {
  if (mbps >= 10_000) return "10g";
  if (mbps >= 1_000) return "1g";
  if (mbps >= 100) return "100m";
  if (mbps >= 10) return "10m";
  return "other";
}

function boxStyle(tier: SpeedTier, operUp: boolean, fiber: boolean): CSSProperties {
  const base: CSSProperties = {
    width: 32,
    height: 32,
    borderRadius: 3,
    position: "relative",
    display: "flex",
    alignItems: "center",
    justifyContent: "center",
    flexShrink: 0,
    opacity: operUp ? 1 : 0.42,
    border: fiber ? "1px dashed" : "1px solid",
  };
  if (tier === "10g") {
    return {
      ...base,
      background: "#e9eef6",
      borderColor: fiber ? "#7a8aad" : "#aab4c5",
      boxShadow: "inset 0 1px 0 rgba(255,255,255,0.65)",
    };
  }
  if (tier === "1g") {
    return { ...base, background: "#2f8f4a", borderColor: fiber ? "#0f4a24" : "#1e6b32" };
  }
  if (tier === "100m") {
    return { ...base, background: "#c9a227", borderColor: fiber ? "#6b5516" : "#9a7b1a" };
  }
  if (tier === "10m") {
    return { ...base, background: "#8a6f1f", borderColor: fiber ? "#4a3a12" : "#6b5516" };
  }
  return { ...base, background: "#3d4555", borderColor: fiber ? "#5a6578" : "#2a303c" };
}

/** Молния только при подтвержденной активности PoE (без эвристик по тексту). */
export function showPoEIndicator(p: Pick<PortRow, "poe_active" | "poe_power_w">): boolean {
  if (p.poe_active === true) {
    return true;
  }
  if (p.poe_power_w != null && p.poe_power_w > 0) {
    return true;
  }
  return false;
}

/**
 * Номер порта для подписи над квадратом: только число (например 5 для Ethernet1/0/5).
 * Берём последний числовой сегмент из ifName; иначе — из ifDescr; иначе ifIndex.
 */
function gridPortNumber(p: PortRow): string {
  const name = (p.if_name ?? "").trim();
  const mTail = name.match(/(\d+)\s*$/);
  if (mTail) return mTail[1];
  const mDescr = (p.if_descr ?? "").match(/port:\s*(\d+)/i);
  if (mDescr) return mDescr[1];
  return String(p.if_index);
}

function ubntLikelySfpPort(sysDescr: string, portNum: number): boolean {
  const s = sysDescr.toLowerCase();
  if (!s.includes("edgeswitch") && !s.includes("ubnt")) return false;
  // Ubiquiti EdgeSwitch: модельные диапазоны SFP по данным эксплуатации.
  // Модель может встречаться как "12F Fiber", "12F-Fiber", "12f_fiber" и т.п.
  if (/\b12f\b/.test(s) && /\bfiber\b/.test(s)) return portNum >= 1 && portNum <= 12;
  if (/\b12f\b/.test(s) && /\bfibre\b/.test(s)) return portNum >= 1 && portNum <= 12;
  // На ряде прошивок в sysDescr модель бывает без слова fiber (например "EdgeSwitch 12F").
  // Для `12F Fiber` по факту корректно маркировать SFP как порты 1-12.
  if (/\b12f\b/.test(s)) return portNum >= 1 && portNum <= 12;
  if (s.includes("edgeswitch 8")) return portNum >= 9 && portNum <= 10;
  if (s.includes("edgeswitch 16")) return portNum >= 17 && portNum <= 18;
  if (s.includes("edgeswitch 24")) return portNum >= 25 && portNum <= 26;
  if (s.includes("edgeswitch 48")) return portNum >= 49 && portNum <= 52;
  // Осторожный fallback для неизвестной модели EdgeSwitch.
  return portNum >= 49;
}

/** Эвристика SFP/SFP+/оптика: ifType + вендорные диапазоны + ключевые слова. */
export function isLikelyFiberPort(p: PortRow, sysDescr?: string | null): boolean {
  const name = (p.if_name ?? "").trim();
  const mTailPort = name.match(/(\d+)\s*$/);
  const portNum = mTailPort ? Number(mTailPort[1]) : null;

  // 1) Если модель распознана — приоритетно используем каталог компоновки портов.
  // Это гарантирует, что для Ubiquiti 12F Fiber SFP помечаются строго как 1-12,
  // даже если по ifType/ключевым словам порт выглядит "как гигабит".
  const model = resolvePortModelFromHint(sysDescr ?? null);
  if (model && portNum != null && Number.isFinite(portNum) && portNum > 0) {
    if (model.sfpPorts.includes(portNum)) return true;
    if (model.copperPorts.includes(portNum)) return false;
  }

  const t = p.if_type;
  // 2) Иначе: если ifType указывает на тип интерфейса (часто так бывает с SFP), используем эвристику.
  // 117 gigabitEthernet(часто SFP), 131 tunnel, 56 sonet, 62 fast — на части железа SFP тоже 117.
  if (t === 117 || t === 56 || t === 131) return true;

  // 3) Vendor-specific fallback по sysDescr (если модель не распознана каталогом).
  if (portNum != null && sysDescr) {
    if (ubntLikelySfpPort(sysDescr, portNum)) return true;
  }
  // SNR: uplink-порты 49-52 обычно SFP/оптика (например Ethernet1/0/49..52).
  const mSNRUplink = name.match(/(?:ethernet|gi|gigabitethernet)\d+\/\d+\/(\d+)$/i);
  if (mSNRUplink) {
    const portNum = Number(mSNRUplink[1]);
    if (portNum >= 49 && portNum <= 52) return true;
  }
  const n = `${p.if_name ?? ""} ${p.if_descr ?? ""}`.toLowerCase();
  return (
    n.includes("sfp") ||
    n.includes("sfpp") ||
    n.includes("xfp") ||
    n.includes("qsfp") ||
    n.includes("fiber") ||
    n.includes("optical") ||
    n.includes("/c") ||
    (n.includes("combo") && (n.includes("fiber") || n.includes("sfp")))
  );
}

/** Ключ для дедупликации в сетке: учитываем префикс (gi/te/eth), чтобы не сливать разные типы портов. */
function gridPortKey(p: PortRow): string {
  const name = (p.if_name ?? "").trim().toLowerCase();
  if (name) return `name:${name}`;
  return `ifindex:${p.if_index}`;
}

/** Число для сортировки слева направо по «физическому» номеру. */
function gridSortKey(p: PortRow): number {
  const num = +gridPortNumber(p);
  if (!Number.isNaN(num) && num > 0) return num;
  return 9_000_000_000 + p.if_index;
}

/** Приоритет типа порта для визуального порядка в сетке (как обычно в ELTEX: gi, потом te). */
function gridPortTypeRank(p: PortRow): number {
  const n = (p.if_name ?? "").trim().toLowerCase();
  if (n.startsWith("fe") || n.startsWith("fastethernet")) return 0;
  if (n.startsWith("gi") || n.startsWith("gigabitethernet")) return 1;
  if (n.startsWith("te") || n.startsWith("tengigabitethernet") || n.startsWith("xe")) return 2;
  return 3;
}

function pickPreferredPort(a: PortRow, b: PortRow): PortRow {
  const up = (x: PortRow) => (x.oper_status === 1 ? 1 : 0);
  const du = up(a) - up(b);
  if (du !== 0) return du > 0 ? a : b;
  return a.if_index <= b.if_index ? a : b;
}

function isGridPort(p: PortRow): boolean {
  if (p.if_index <= 0) return false;
  const role = (p.port_role ?? "auto").toLowerCase();
  if (role === "ignore") return false;
  // В описании пользователь может хранить "VLAN..." (например имя комнаты/сегмента) для обычного физического порта.
  // Поэтому исключаем только явные интерфейсы VLAN/loopback по имени интерфейса, а не по ifDescr.
  const ifName = `${p.if_name ?? ""}`.trim().toLowerCase();
  if (ifName.startsWith("vlan") || ifName.includes("loopback")) return false;
  return true;
}

export function PortOverviewLegend() {
  return (
    <p style={{ fontSize: "0.8rem", color: "#9aa3b5", marginTop: 0, marginBottom: "0.65rem", lineHeight: 1.5 }}>
      <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
        <span style={{ width: 14, height: 14, borderRadius: 2, background: "#2f8f4a", border: "1px solid #1e6b32" }} />{" "}
        1 Гбит/с
      </span>
      {" · "}
      <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
        <span style={{ width: 14, height: 14, borderRadius: 2, background: "#e9eef6", border: "1px solid #aab4c5" }} />{" "}
        10 Гбит/с (светлый / пастельный)
      </span>
      {" · "}
      <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
        <span style={{ width: 14, height: 14, borderRadius: 2, background: "#c9a227", border: "1px solid #9a7b1a" }} />{" "}
        100 Мбит/с
      </span>
      {" · "}
      <span style={{ display: "inline-flex", alignItems: "center", gap: 5 }}>
        <span
          style={{
            minWidth: 22,
            height: 14,
            borderRadius: 2,
            border: "1px dashed #7a8aad",
            color: "#b8c4dc",
            fontSize: 9,
            fontWeight: 700,
            display: "inline-flex",
            alignItems: "center",
            justifyContent: "center",
            letterSpacing: 0.2,
          }}
        >
          SFP
        </span>
        SFP/оптика
      </span>
      {" · "}
      <span title="PoE: SNMP/SSH (выдача питания). LLDP-PD — только если MIB и SSH молчат">
        ⚡ — активный PoE (только фактическая подача питания)
      </span>
      {" · "}
      <span title="Текущая утилизация линка (max in/out) по последнему опросу">
        полоска под портом — утилизация трафика
      </span>
    </p>
  );
}

/** Сетка портов как в UISP: цвет по скорости линка, молния только при фактическом активном PoE. */
export function PortOverviewGrid({
  interfaces,
  sysDescr,
  onPortHoverChange,
  onPortClick,
  onPortEditSettings,
  expandedIfIndex,
}: {
  interfaces: PortRow[];
  sysDescr?: string | null;
  onPortHoverChange?: (ifIndex: number | null) => void;
  onPortClick?: (ifIndex: number) => void;
  /** ПКМ → «Редактировать настройки порта». */
  onPortEditSettings?: (ifIndex: number) => void;
  expandedIfIndex?: number | null;
}) {
  const ports = useMemo(() => {
    const filtered = interfaces.filter(isGridPort);
    const byLabel = new Map<string, PortRow>();
    for (const p of filtered) {
      const key = gridPortKey(p);
      const prev = byLabel.get(key);
      if (!prev) byLabel.set(key, p);
      else byLabel.set(key, pickPreferredPort(prev, p));
    }
    return [...byLabel.values()].sort((a, b) => {
      const tr = gridPortTypeRank(a) - gridPortTypeRank(b);
      if (tr !== 0) return tr;
      const d = gridSortKey(a) - gridSortKey(b);
      if (d !== 0) return d;
      return a.if_index - b.if_index;
    });
  }, [interfaces]);

  const [menu, setMenu] = useState<{ ifIndex: number; x: number; y: number } | null>(null);

  useEffect(() => {
    if (!menu) return;
    // Откладываем подписку, чтобы клик «Редактировать» не закрыл меню в том же событии.
    const close = () => setMenu(null);
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") close();
    };
    const timer = window.setTimeout(() => {
      window.addEventListener("click", close);
      window.addEventListener("scroll", close, true);
      window.addEventListener("keydown", onKey);
    }, 0);
    return () => {
      window.clearTimeout(timer);
      window.removeEventListener("click", close);
      window.removeEventListener("scroll", close, true);
      window.removeEventListener("keydown", onKey);
    };
  }, [menu]);

  if (ports.length === 0) return null;

  return (
    <div style={{ marginBottom: "1rem" }}>
      <PortOverviewLegend />
      <div style={{ display: "flex", flexWrap: "wrap", gap: "12px 16px", alignItems: "flex-end" }}>
        {ports.map((p) => {
          const mbps = linkMbps(p.if_high_speed, p.if_speed);
          const tier = speedTier(mbps);
          const up = p.oper_status === 1;
          const fiber = isLikelyFiberPort(p, sysDescr);
          const poe = showPoEIndicator(p) && !fiber;
          const num = gridPortNumber(p);
          const nm = (p.if_name ?? "").trim();
          const expanded = expandedIfIndex === p.if_index;
          const util =
            p.util_max_pct ??
            (p.util_in_pct != null || p.util_out_pct != null
              ? Math.max(p.util_in_pct ?? 0, p.util_out_pct ?? 0)
              : null);
          return (
            <div
              key={p.if_index}
              style={{ display: "flex", flexDirection: "column", alignItems: "center", gap: 3 }}
              onMouseEnter={() => onPortHoverChange?.(p.if_index)}
              onMouseLeave={() => onPortHoverChange?.(null)}
            >
              <div style={{ fontSize: 11, color: "#8b95a8", fontWeight: 600, lineHeight: 1 }}>{num}</div>
              <div
                role={onPortClick || onPortEditSettings ? "button" : undefined}
                tabIndex={onPortClick || onPortEditSettings ? 0 : undefined}
                onClick={onPortClick ? () => onPortClick(p.if_index) : undefined}
                onContextMenu={
                  onPortEditSettings
                    ? (e) => {
                        e.preventDefault();
                        e.stopPropagation();
                        setMenu({ ifIndex: p.if_index, x: e.clientX, y: e.clientY });
                      }
                    : undefined
                }
                onKeyDown={
                  onPortClick
                    ? (e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          onPortClick(p.if_index);
                        }
                      }
                    : undefined
                }
                style={{
                  ...boxStyle(tier, up, fiber),
                  cursor: onPortClick || onPortEditSettings ? "pointer" : undefined,
                  outline: expanded ? "2px solid rgba(120, 188, 255, 0.85)" : undefined,
                  outlineOffset: 2,
                }}
                title={`${nm || "—"} · ${portDisplayDescr(p) || "без описания"} · ${formatLinkSpeedMbps(mbps)} · oper ${p.oper_status ?? "—"}${fiber ? " · вероятно SFP/оптика" : ""}${onPortEditSettings ? " · ПКМ: настройки" : ""}`}
              >
                {fiber ? (
                  <span
                    style={{
                      position: "absolute",
                      top: 2,
                      right: 2,
                      minWidth: 18,
                      height: 10,
                      padding: "0 2px",
                      borderRadius: 2,
                      border: "1px dashed rgba(255,255,255,0.75)",
                      background: "rgba(30,36,48,0.35)",
                      fontSize: 7,
                      fontWeight: 700,
                      color: "rgba(255,255,255,0.95)",
                      letterSpacing: 0.2,
                      display: "inline-flex",
                      alignItems: "center",
                      justifyContent: "center",
                      lineHeight: 1,
                      pointerEvents: "none",
                    }}
                  >
                    SFP
                  </span>
                ) : null}
                {poe ? (
                  <span
                    style={{
                      fontSize: 17,
                      lineHeight: 1,
                      color: "#ffffff",
                      filter: "drop-shadow(0 0 2px rgba(255,255,255,0.45))",
                      textShadow:
                        "0.35px 0 0 #ffffff, -0.35px 0 0 #ffffff, 0 0.35px 0 #ffffff, 0 -0.35px 0 #ffffff",
                    }}
                  >
                    ⚡
                  </span>
                ) : null}
              </div>
              <PortUtilBar pct={up ? util : null} />
            </div>
          );
        })}
      </div>
      {menu && onPortEditSettings ? (
        <div
          role="menu"
          style={{
            position: "fixed",
            left: menu.x,
            top: menu.y,
            zIndex: 90,
            minWidth: 220,
            background: "#1a1f2b",
            border: "1px solid #3a4558",
            borderRadius: 8,
            boxShadow: "0 8px 24px rgba(0,0,0,0.45)",
            padding: "4px 0",
          }}
          onClick={(e) => e.stopPropagation()}
        >
          <button
            type="button"
            role="menuitem"
            style={{
              display: "block",
              width: "100%",
              textAlign: "left",
              background: "transparent",
              border: "none",
              color: "#e8ecf4",
              padding: "8px 14px",
              fontSize: "0.9rem",
              cursor: "pointer",
            }}
            onMouseEnter={(e) => {
              (e.currentTarget as HTMLButtonElement).style.background = "rgba(88,164,255,0.15)";
            }}
            onMouseLeave={(e) => {
              (e.currentTarget as HTMLButtonElement).style.background = "transparent";
            }}
            onClick={() => {
              const idx = menu.ifIndex;
              setMenu(null);
              onPortEditSettings(idx);
            }}
          >
            Редактировать настройки порта
          </button>
        </div>
      ) : null}
    </div>
  );
}
