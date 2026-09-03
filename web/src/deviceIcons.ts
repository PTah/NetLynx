/** Иконки типов устройств.
 * UI: корень пака + вырезанная светлая/белая подложка → /device-icons (тёмная тема).
 * Email: White layer без правок → internal/notify/assets/device-icons.
 */

import { normalizeDeviceCategory } from "./deviceCategories";

/** Сброс кэша браузера при смене набора иконок. */
const ICON_CACHE_BUST = "0.37.18";

/** category id → имя файла без .png */
const ICON_FILE_BY_CATEGORY: Record<string, string> = {
  switch: "switch",
  router: "router",
  ap: "ap",
  server: "server",
  computer: "computer",
  phone: "phone",
  mfu: "mfu",
  camera: "camera",
  other: "other",
  tv: "tv",
  rack: "rack",
  industrial: "industrial",
  ilo: "ilo-idrac-ipmi",
  ipmi: "ilo-idrac-ipmi",
};

/** Имя файла иконки (без .png); неизвестный тип → other. */
export function deviceIconFileStem(raw?: string | null): string {
  const id = normalizeDeviceCategory(raw);
  return ICON_FILE_BY_CATEGORY[id] ?? "other";
}

/** @deprecated используйте deviceIconFileStem */
export function deviceIconId(raw?: string | null): string {
  return deviceIconFileStem(raw);
}

/** URL UI-иконки (без white layer). */
export function deviceIconUrl(category?: string | null): string {
  return `/device-icons/${deviceIconFileStem(category)}.png?v=${ICON_CACHE_BUST}`;
}

export function hasDeviceIcon(category?: string | null): boolean {
  const id = normalizeDeviceCategory(category);
  return id in ICON_FILE_BY_CATEGORY;
}
