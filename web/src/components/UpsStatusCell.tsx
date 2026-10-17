import type { UPSSnapshot } from "../types";
import { UPS_LOAD_HIGH_PCT, UPS_TEMP_HIGH_C, formatUpsCharge, upsOnBattery, upsOutputSourceLabel } from "../upsMetrics";

/** Компактная ячейка ИБП для списка на дашборде. */
export function UpsStatusCell({ ups, muted }: { ups?: UPSSnapshot | null; muted: string }) {
  if (!ups || (!ups.ident_name && !ups.model && ups.battery_charge_pct == null)) {
    return <span style={{ color: muted }}>—</span>;
  }
  const onBattery = upsOnBattery(ups);
  const alarms = (ups.alarms_present ?? 0) > 0;
  const hot = ups.battery_temp_c != null && ups.battery_temp_c >= UPS_TEMP_HIGH_C;
  const overload = ups.output_load_pct != null && ups.output_load_pct >= UPS_LOAD_HIGH_PCT;
  const color = onBattery || alarms || hot || overload ? "#ff5252" : undefined;
  const bits = [
    onBattery ? "БАТАРЕЯ" : upsOutputSourceLabel(ups.output_source),
    formatUpsCharge(ups),
  ];
  if (ups.runtime_min != null) bits.push(`${Math.round(ups.runtime_min)} мин`);
  if (ups.output_load_pct != null) bits.push(`${Math.round(ups.output_load_pct)}%`);
  return (
    <span
      title={[
        ups.ident_name || ups.model || "ИБП",
        `питание: ${upsOutputSourceLabel(ups.output_source)}`,
        `заряд: ${formatUpsCharge(ups)}`,
        ups.battery_temp_c != null ? `АКБ: ${Math.round(ups.battery_temp_c)} °C` : null,
        ups.alarms_present != null ? `аварий: ${ups.alarms_present}` : null,
      ]
        .filter(Boolean)
        .join(" · ")}
      style={{ color, fontWeight: color ? 700 : undefined }}
    >
      {bits.join(" · ")}
    </span>
  );
}
