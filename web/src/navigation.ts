/** Откуда открыли карточку устройства — для ссылки «← …». */
export type DeviceBackRef = {
  path: string;
  label: string;
};

export const DEVICE_BACK_DEFAULT: DeviceBackRef = {
  path: "/devices",
  label: "Все узлы",
};

export function deviceLinkState(back: DeviceBackRef): { deviceBack: DeviceBackRef } {
  return { deviceBack: back };
}
