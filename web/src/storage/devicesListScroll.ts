/** Скролл списка «Узлы» внутри #devices-list-scroll при уходе на /devices/:id и обратно */
export const DEVICES_LIST_SCROLL_Y_KEY = "devices-page-scroll-y";
export const DEVICES_LIST_SCROLL_RESTORE_KEY = "devices-page-scroll-restore";
export const DEVICES_RESTORE_MAX_ATTEMPTS = 8;
export const DEVICES_RESTORE_RETRY_MS = 40;

function getDevicesListScrollEl(): HTMLElement | null {
  return document.getElementById("devices-list-scroll");
}

function getMainScrollEl(): HTMLElement | null {
  return document.getElementById("app-main-scroll");
}

export function parsePositiveNumber(value: string | null): number {
  const n = value == null ? 0 : Number(value);
  return Number.isFinite(n) && n > 0 ? n : 0;
}

export function readDevicesListScrollY(): number {
  const listY = getDevicesListScrollEl()?.scrollTop ?? 0;
  const mainY = getMainScrollEl()?.scrollTop ?? 0;
  const winY = window.scrollY ?? 0;
  const docY = document.documentElement?.scrollTop ?? 0;
  const bodyY = document.body?.scrollTop ?? 0;
  return Math.max(listY, mainY, winY, docY, bodyY, 0);
}

export function applyDevicesListScrollY(y: number): void {
  const top = Number.isFinite(y) && y > 0 ? y : 0;
  const list = getDevicesListScrollEl();
  const main = getMainScrollEl();
  // Список «Узлы» скроллится вместе со страницей (#app-main-scroll), не во внутреннем контейнере.
  if (list && list.scrollHeight > list.clientHeight + 2) {
    list.scrollTo({ top, behavior: "auto" });
  }
  if (main) {
    main.scrollTo({ top, behavior: "auto" });
  }
  window.scrollTo({ top, behavior: "auto" });
  if (document.documentElement) document.documentElement.scrollTop = top;
  if (document.body) document.body.scrollTop = top;
}
