/** Сигнал для страницы топологии: перезагрузить граф после связи с порта / ручной линк. */
export const TOPOLOGY_REFRESH_EVENT = "netlynx:topology-refresh";

const TOPOLOGY_REFRESH_PENDING_KEY = "netlynx-topology-refresh-pending";

export function requestTopologyRefresh(): void {
  try {
    sessionStorage.setItem(TOPOLOGY_REFRESH_PENDING_KEY, String(Date.now()));
  } catch {
    /* ignore */
  }
  window.dispatchEvent(new CustomEvent(TOPOLOGY_REFRESH_EVENT));
}

/** При монтировании топологии: был ли запрос обновления из другой вкладки/карточки узла. */
export function consumeTopologyRefreshPending(): boolean {
  try {
    if (sessionStorage.getItem(TOPOLOGY_REFRESH_PENDING_KEY)) {
      sessionStorage.removeItem(TOPOLOGY_REFRESH_PENDING_KEY);
      return true;
    }
  } catch {
    /* ignore */
  }
  return false;
}
