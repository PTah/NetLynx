import type { UPSSnapshot } from "./types";

/** Пороги ИБП; совпадают с internal/poller/ups.go. */
export const UPS_TEMP_HIGH_C = 45;
export const UPS_TEMP_CRITICAL_C = 55;
export const UPS_LOAD_HIGH_PCT = 90;

/** Метрики истории ИБП (metric_samples ups_*), в порядке показа на карточке. */
export const UPS_METRICS: { type: string; label: string; unit: string; digits: number }[] = [
  { type: "ups_battery_charge_pct", label: "Заряд АКБ", unit: "%", digits: 0 },
  { type: "ups_runtime_min", label: "Время работы", unit: "мин", digits: 0 },
  { type: "ups_output_load_pct", label: "Нагрузка выхода", unit: "%", digits: 0 },
  { type: "ups_output_w", label: "Мощность выхода", unit: "Вт", digits: 0 },
  { type: "ups_battery_voltage_v", label: "Напряжение АКБ", unit: "В", digits: 1 },
  { type: "ups_input_voltage_v", label: "Входное напряжение", unit: "В", digits: 0 },
  { type: "ups_output_voltage_v", label: "Выходное напряжение", unit: "В", digits: 0 },
  { type: "ups_battery_temp_c", label: "Температура АКБ", unit: "°C", digits: 0 },
  { type: "ups_on_battery", label: "На батарее", unit: "", digits: 0 },
];

export function upsMetricLabel(type: string): string {
  return UPS_METRICS.find((m) => m.type === type)?.label ?? type;
}

export function upsBatteryStatusLabel(v?: number | null): string {
  switch (v) {
    case 2:
      return "норма";
    case 3:
      return "низкий заряд";
    case 4:
      return "разряжена";
    case 1:
      return "неизвестно";
    default:
      return "—";
  }
}

export function upsOutputSourceLabel(v?: number | null): string {
  switch (v) {
    case 1:
      return "иное";
    case 2:
      return "нет выхода";
    case 3:
      return "сеть";
    case 4:
      return "байпас";
    case 5:
      return "батарея";
    case 6:
      return "booster";
    case 7:
      return "reducer";
    default:
      return "—";
  }
}

export function upsOnBattery(u?: UPSSnapshot | null): boolean {
  if (!u) return false;
  if (u.on_battery === true) return true;
  if (u.seconds_on_battery != null && u.seconds_on_battery > 0) return true;
  return u.output_source === 5;
}

export function formatUpsTitle(u?: UPSSnapshot | null): string {
  if (!u) return "";
  const name = (u.ident_name ?? "").trim();
  if (name) return name;
  const model = (u.model ?? "").trim();
  const maker = (u.manufacturer ?? "").trim();
  if (model && maker) return `${model} (${maker})`;
  return model || maker;
}

export function fmtUpsNum(v: number | null | undefined, digits = 0, unit = ""): string {
  if (v == null || !Number.isFinite(v)) return "—";
  return `${v.toFixed(digits)}${unit}`;
}

/** Заряд АКБ в процентах с учётом «нет данных». */
export function formatUpsCharge(u?: UPSSnapshot | null): string {
  return fmtUpsNum(u?.battery_charge_pct, 0, "%");
}

/** Время автономии в минутах. */
export function formatUpsRuntime(u?: UPSSnapshot | null): string {
  return u?.runtime_min == null ? "—" : `${Math.round(u.runtime_min)} мин`;
}

export function formatUpsLoad(u?: UPSSnapshot | null): string {
  return fmtUpsNum(u?.output_load_pct, 0, "%");
}

/** Нужно ли подсветить узел красным в списках (питание/авария/перегрев/перегрузка). */
export function isUpsCritical(u?: UPSSnapshot | null): boolean {
  if (!u) return false;
  if (upsOnBattery(u)) return true;
  if (u.battery_status != null && u.battery_status >= 3) return true;
  if (u.alarms_present != null && u.alarms_present > 0) return true;
  if (u.battery_temp_c != null && u.battery_temp_c >= UPS_TEMP_HIGH_C) return true;
  return u.output_load_pct != null && u.output_load_pct >= UPS_LOAD_HIGH_PCT;
}

/** Короткая строка статуса для списка устройств. */
export function upsStatusSummary(u?: UPSSnapshot | null): string {
  if (!u) return "—";
  if (upsOnBattery(u)) {
    const parts = ["БАТАРЕЯ"];
    if (u.battery_charge_pct != null) parts.push(`${Math.round(u.battery_charge_pct)}%`);
    if (u.runtime_min != null) parts.push(`${Math.round(u.runtime_min)} мин`);
    return parts.join(" · ");
  }
  const parts: string[] = [upsOutputSourceLabel(u.output_source)];
  if (u.battery_charge_pct != null) parts.push(`${Math.round(u.battery_charge_pct)}%`);
  if (u.output_load_pct != null) parts.push(`${Math.round(u.output_load_pct)}% нагрузки`);
  return parts.join(" · ");
}
