/** Типы узлов: встроенные + пользовательские (API /settings/device-categories). */

export type DeviceCategory = string;

export type DeviceCategoryDef = {
  id: string;
  label: string;
  color: string;
  blink?: boolean;
  builtin: boolean;
  sort_order?: number;
};

/** Запасной список = цвета точки на топологии (TOPOLOGY_DOT). */
export const BUILTIN_DEVICE_CATEGORIES: DeviceCategoryDef[] = [
  { id: "switch", label: "Коммутаторы", color: "#4a90e2", blink: false, builtin: true, sort_order: 10 },
  { id: "router", label: "Роутеры", color: "#2f9e6f", blink: false, builtin: true, sort_order: 20 },
  { id: "ap", label: "Точки доступа", color: "#18c8d6", blink: false, builtin: true, sort_order: 30 },
  { id: "server", label: "Серверы", color: "#f0c14a", blink: false, builtin: true, sort_order: 40 },
  { id: "computer", label: "Компьютеры", color: "#9b59d0", blink: false, builtin: true, sort_order: 50 },
  { id: "phone", label: "Телефоны", color: "#e45c9a", blink: false, builtin: true, sort_order: 60 },
  { id: "mfu", label: "МФУ", color: "#8b5a2b", blink: false, builtin: true, sort_order: 70 },
  { id: "camera", label: "Камеры", color: "#ff8c00", blink: true, builtin: true, sort_order: 80 },
  { id: "other", label: "Иные устройства", color: "#c5c9d0", blink: false, builtin: true, sort_order: 90 },
];

/** @deprecated используйте BUILTIN / список из API */
export const DEVICE_CATEGORY_OPTIONS: { id: DeviceCategory; label: string }[] =
  BUILTIN_DEVICE_CATEGORIES.map((c) => ({ id: c.id, label: c.label }));

const BUILTIN_IDS = new Set(BUILTIN_DEVICE_CATEGORIES.map((c) => c.id));

export function normalizeDeviceCategory(raw?: string | null): DeviceCategory {
  const c = (raw ?? "").trim().toLowerCase();
  if (!c) return "switch";
  switch (c) {
    case "switch":
    case "router":
    case "ap":
    case "server":
    case "computer":
    case "phone":
    case "mfu":
    case "camera":
    case "other":
      return c;
    default:
      if (/^[a-z][a-z0-9_]{0,31}$/.test(c)) return c;
      return "other";
  }
}

export function categoryById(
  list: DeviceCategoryDef[],
  raw?: string | null,
): DeviceCategoryDef | undefined {
  const id = normalizeDeviceCategory(raw);
  return list.find((c) => c.id === id);
}

export function deviceCategoryLabel(raw?: string | null, list?: DeviceCategoryDef[]): string {
  const id = normalizeDeviceCategory(raw);
  if (list?.length) {
    const hit = list.find((c) => c.id === id);
    if (hit) return hit.label;
  }
  const builtin = BUILTIN_DEVICE_CATEGORIES.find((c) => c.id === id);
  return builtin?.label ?? id;
}

export function deviceCategoryColor(raw?: string | null, list?: DeviceCategoryDef[]): string {
  const id = normalizeDeviceCategory(raw);
  if (list?.length) {
    const hit = list.find((c) => c.id === id);
    if (hit?.color) return hit.color;
  }
  const builtin = BUILTIN_DEVICE_CATEGORIES.find((c) => c.id === id);
  return builtin?.color ?? "#c5c9d0";
}

export function deviceCategoryBlink(raw?: string | null, list?: DeviceCategoryDef[]): boolean {
  const id = normalizeDeviceCategory(raw);
  if (list?.length) {
    const hit = list.find((c) => c.id === id);
    if (hit) return !!hit.blink;
  }
  const builtin = BUILTIN_DEVICE_CATEGORIES.find((c) => c.id === id);
  return !!builtin?.blink;
}

export function isBuiltinCategory(id: string): boolean {
  return BUILTIN_IDS.has(normalizeDeviceCategory(id));
}

const FILTER_STORAGE_KEY = "invetor.devices.categoryFilter";
const DASHBOARD_FILTER_STORAGE_KEY = "invetor.dashboard.categoryFilter";
const TOPOLOGY_FILTER_STORAGE_KEY = "invetor.topology.categoryFilter";

export type CategoryFilterState = Record<string, boolean>;

export function defaultCategoryFilter(list?: DeviceCategoryDef[]): CategoryFilterState {
  const src = list?.length ? list : BUILTIN_DEVICE_CATEGORIES;
  const out: CategoryFilterState = {};
  for (const c of src) out[c.id] = true;
  return out;
}

export function readCategoryFilter(list?: DeviceCategoryDef[]): CategoryFilterState {
  return readCategoryFilterFromKey(FILTER_STORAGE_KEY, list);
}

export function writeCategoryFilter(state: CategoryFilterState): void {
  try {
    localStorage.setItem(FILTER_STORAGE_KEY, JSON.stringify(state));
  } catch {
    /* ignore */
  }
}

function readCategoryFilterFromKey(storageKey: string, list?: DeviceCategoryDef[]): CategoryFilterState {
  const base = defaultCategoryFilter(list);
  try {
    const raw = localStorage.getItem(storageKey);
    if (!raw) return base;
    const parsed = JSON.parse(raw) as Partial<Record<string, boolean>>;
    for (const id of Object.keys(base)) {
      if (typeof parsed[id] === "boolean") base[id] = parsed[id]!;
    }
  } catch {
    /* ignore */
  }
  return base;
}

export function readDashboardCategoryFilter(list?: DeviceCategoryDef[]): CategoryFilterState {
  return readCategoryFilterFromKey(DASHBOARD_FILTER_STORAGE_KEY, list);
}

export function writeDashboardCategoryFilter(state: CategoryFilterState): void {
  try {
    localStorage.setItem(DASHBOARD_FILTER_STORAGE_KEY, JSON.stringify(state));
  } catch {
    /* ignore */
  }
}

/** На топологии по умолчанию скрыты все типы кроме switch/router (как старый «другие»=off). */
export function defaultTopologyCategoryFilter(list?: DeviceCategoryDef[]): CategoryFilterState {
  const src = list?.length ? list : BUILTIN_DEVICE_CATEGORIES;
  const out: CategoryFilterState = {};
  for (const c of src) {
    out[c.id] = c.id === "switch" || c.id === "router";
  }
  return out;
}

export function readTopologyCategoryFilter(list?: DeviceCategoryDef[]): CategoryFilterState {
  const base = defaultTopologyCategoryFilter(list);
  try {
    const raw = localStorage.getItem(TOPOLOGY_FILTER_STORAGE_KEY);
    if (!raw) return base;
    const parsed = JSON.parse(raw) as Partial<Record<string, boolean>>;
    for (const id of Object.keys(base)) {
      if (typeof parsed[id] === "boolean") base[id] = parsed[id]!;
    }
  } catch {
    /* ignore */
  }
  return base;
}

export function writeTopologyCategoryFilter(state: CategoryFilterState): void {
  try {
    localStorage.setItem(TOPOLOGY_FILTER_STORAGE_KEY, JSON.stringify(state));
  } catch {
    /* ignore */
  }
}

/** Заголовок таблицы узлов на дашборде по выбранным типам. */
export function dashboardDevicesTableTitle(
  filter: CategoryFilterState,
  list: DeviceCategoryDef[],
): string {
  const selected = list.filter((c) => filter[c.id] !== false);
  if (selected.length === 1) return selected[0]!.label;
  return "Выбранные устройства";
}

export function isInfraDeviceCategory(id: string): boolean {
  const c = normalizeDeviceCategory(id);
  return c === "switch" || c === "router";
}

export function suggestCategoryIdFromLabel(label: string): string {
  const map: Record<string, string> = {
    а: "a",
    б: "b",
    в: "v",
    г: "g",
    д: "d",
    е: "e",
    ё: "e",
    ж: "zh",
    з: "z",
    и: "i",
    й: "y",
    к: "k",
    л: "l",
    м: "m",
    н: "n",
    о: "o",
    п: "p",
    р: "r",
    с: "s",
    т: "t",
    у: "u",
    ф: "f",
    х: "h",
    ц: "ts",
    ч: "ch",
    ш: "sh",
    щ: "sch",
    ъ: "",
    ы: "y",
    ь: "",
    э: "e",
    ю: "yu",
    я: "ya",
  };
  let s = label.trim().toLowerCase();
  let out = "";
  for (const ch of s) {
    if (map[ch] != null) out += map[ch];
    else if (/[a-z0-9]/.test(ch)) out += ch;
    else if (ch === " " || ch === "-" || ch === "/") out += "_";
  }
  out = out.replace(/_+/g, "_").replace(/^_|_$/g, "");
  if (!out || !/^[a-z]/.test(out)) out = "type_" + (out || "custom");
  return out.slice(0, 32);
}
