/** TTL кэша «не дергать SSH при повторном открытии карточки» (согласовано с CardCLISyncMaxAge на сервере). */
export const DEVICE_CLI_SYNC_SESSION_MS = 6 * 60 * 60 * 1000;

const storageKey = (deviceId: number) => `netlynx.deviceCliSync.${deviceId}`;

export function readDeviceCliSyncSession(deviceId: number): number | null {
  if (typeof window === "undefined" || deviceId <= 0) return null;
  try {
    const raw = sessionStorage.getItem(storageKey(deviceId));
    if (!raw) return null;
    const ts = Number(raw);
    return Number.isFinite(ts) && ts > 0 ? ts : null;
  } catch {
    return null;
  }
}

export function writeDeviceCliSyncSession(deviceId: number): void {
  if (typeof window === "undefined" || deviceId <= 0) return;
  try {
    sessionStorage.setItem(storageKey(deviceId), String(Date.now()));
  } catch {
    /* ignore */
  }
}

export function shouldSkipDeviceCliSyncSession(deviceId: number): boolean {
  const ts = readDeviceCliSyncSession(deviceId);
  if (ts == null) return false;
  return Date.now() - ts < DEVICE_CLI_SYNC_SESSION_MS;
}
