import { deviceCategoryBlink, deviceCategoryColor, type DeviceCategoryDef } from "./deviceCategories";

/** Цвета точки статуса на топологии (системные + fallback). Категории — из справочника. */
export const TOPOLOGY_DOT = {
  selected: "#ffffff",
  virtual: "#111111",
  offline: "#9aa0a6",
  server: "#f0c14a",
  phone: "#e45c9a",
  mfu: "#8b5a2b",
  camera: "#ff8c00",
  ap: "#18c8d6",
  other: "#c5c9d0",
  computer: "#9b59d0",
  router: "#2f9e6f",
  switch: "#4a90e2",
} as const;

/** Builtin kinds + custom category id (tv, rack, …) + virtual. */
export type TopologyNodeKind = string;

export function topologyNodeKind(n: {
  kind?: string | null;
  virtual?: boolean;
}): TopologyNodeKind {
  if (n.virtual) return "virtual";
  const k = (n.kind ?? "").trim().toLowerCase();
  if (!k) return "switch";
  // Совпадает с normalizeDeviceCategory: builtins и пользовательские slug.
  if (/^[a-z][a-z0-9_]{0,31}$/.test(k)) return k;
  return "switch";
}

/** Точка слева на карточке: выбранный → offline → virtual → категория. */
export function topologyDotColor(
  n: { kind?: string | null; virtual?: boolean; last_snmp_ok?: boolean | null },
  opts: { selected?: boolean; offline?: boolean; categories?: DeviceCategoryDef[] } = {},
): string {
  if (opts.selected) return TOPOLOGY_DOT.selected;
  if (n.virtual || topologyNodeKind(n) === "virtual") return TOPOLOGY_DOT.virtual;
  if (opts.offline) return TOPOLOGY_DOT.offline;
  const kind = topologyNodeKind(n);
  if (kind !== "virtual" && opts.categories?.length) {
    return deviceCategoryColor(kind, opts.categories);
  }
  switch (kind) {
    case "server":
      return TOPOLOGY_DOT.server;
    case "phone":
      return TOPOLOGY_DOT.phone;
    case "mfu":
      return TOPOLOGY_DOT.mfu;
    case "camera":
      return TOPOLOGY_DOT.camera;
    case "ap":
      return TOPOLOGY_DOT.ap;
    case "other":
      return TOPOLOGY_DOT.other;
    case "computer":
      return TOPOLOGY_DOT.computer;
    case "router":
      return TOPOLOGY_DOT.router;
    case "switch":
    default:
      return TOPOLOGY_DOT.switch;
  }
}

export function topologyDotBlink(
  n: { kind?: string | null; virtual?: boolean },
  selected?: boolean,
  categories?: DeviceCategoryDef[],
): boolean {
  if (selected || n.virtual) return false;
  const kind = topologyNodeKind(n);
  if (kind === "virtual") return false;
  if (categories?.length) return deviceCategoryBlink(kind, categories);
  return kind === "camera";
}

/** Компьютеры — квадрат; остальные — круг. */
export function topologyMarkerIsSquare(n: { kind?: string | null; virtual?: boolean }): boolean {
  return !n.virtual && topologyNodeKind(n) === "computer";
}

/** Рамка карточки: путь/выбор — акцент; иначе как точка (чуть приглушённее для selected-белого). */
export function topologyCardStroke(
  n: { kind?: string | null; virtual?: boolean; last_snmp_ok?: boolean | null },
  opts: { selected?: boolean; onPath?: boolean; offline?: boolean; categories?: DeviceCategoryDef[] } = {},
): string {
  if (opts.selected) return "#ffffff";
  if (opts.onPath) return "#f0c14a";
  return topologyDotColor(n, { offline: opts.offline, categories: opts.categories });
}

export function topologyCardFill(
  n: { kind?: string | null; virtual?: boolean },
  offline: boolean,
): string {
  // Нейтральный серый (не коричневый — иначе путают с МФУ).
  if (offline) return "#2a2e38";
  switch (topologyNodeKind(n)) {
    case "router":
      return "#0f2218";
    case "server":
      return "#2a2410";
    case "phone":
      return "#2a1420";
    case "mfu":
      return "#241c14";
    case "camera":
      return "#2a1c10";
    case "ap":
      return "#10262a";
    case "virtual":
      return "#1a1a1a";
    case "other":
      return "#222830";
    case "computer":
      return "#221828";
    case "switch":
    default:
      return "#152033";
  }
}

/** Для layout: только switch/router — инфраструктура. */
export function topologyLayoutKind(n: { kind?: string | null; virtual?: boolean }): "switch" | "router" | "other" {
  const k = topologyNodeKind(n);
  if (k === "router") return "router";
  if (k === "switch") return "switch";
  return "other";
}
