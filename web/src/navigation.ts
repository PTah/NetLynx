/** Откуда открыли карточку устройства — для ссылки «← …». */
export type DeviceBackRef = {
  path: string;
  label: string;
};

export const DEVICE_BACK_DEFAULT: DeviceBackRef = {
  path: "/devices",
  label: "Все узлы",
};

const DEVICE_BACK_STORAGE_KEY = "netlynx.deviceBack";

function isDeviceBackRef(v: unknown): v is DeviceBackRef {
  if (!v || typeof v !== "object") return false;
  const o = v as DeviceBackRef;
  return typeof o.path === "string" && o.path.length > 0 && typeof o.label === "string" && o.label.length > 0;
}

/** Запомнить раздел-источник (вкладки карточки и F5 не должны сбрасывать «← …»). */
export function rememberDeviceBack(back: DeviceBackRef): void {
  try {
    sessionStorage.setItem(DEVICE_BACK_STORAGE_KEY, JSON.stringify(back));
  } catch {
    /* private mode / quota */
  }
}

export function readStoredDeviceBack(): DeviceBackRef | null {
  try {
    const raw = sessionStorage.getItem(DEVICE_BACK_STORAGE_KEY);
    if (!raw) return null;
    const parsed: unknown = JSON.parse(raw);
    return isDeviceBackRef(parsed) ? parsed : null;
  } catch {
    return null;
  }
}

/** location.state → sessionStorage → список узлов. */
export function resolveDeviceBack(fromLocation: unknown): DeviceBackRef {
  const stateBack = (fromLocation as { deviceBack?: unknown } | null)?.deviceBack;
  if (isDeviceBackRef(stateBack)) {
    rememberDeviceBack(stateBack);
    return stateBack;
  }
  return readStoredDeviceBack() ?? DEVICE_BACK_DEFAULT;
}

export function deviceLinkState(back: DeviceBackRef): { deviceBack: DeviceBackRef } {
  return { deviceBack: back };
}
