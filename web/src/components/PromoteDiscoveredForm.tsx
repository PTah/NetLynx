import { FormEvent, Ref } from "react";
import { type DeviceCategory } from "../deviceCategories";
import { useDeviceCategories } from "../hooks/useDeviceCategories";
import { DeviceCategoryIcon } from "./DeviceCategoryIcon";
import { LocationCombobox } from "./LocationCombobox";

export type PromotePreview = {
  ok: boolean;
  error?: string;
  sys_name?: string;
  sys_descr?: string;
  host?: string;
};

export type PromoteFormValues = {
  host: string;
  name: string;
  location: string;
  category: DeviceCategory;
  community: string;
};

type Props = {
  values: PromoteFormValues;
  locations: string[];
  preview: PromotePreview | null;
  busy: boolean;
  title?: string;
  formRef?: Ref<HTMLFormElement>;
  onChange: (patch: Partial<PromoteFormValues>) => void;
  onPreview: () => void;
  onSubmit: (e: FormEvent) => void;
  onCancel: () => void;
};

/** Форма promote discovered → Узлы (Топология и Обнаружено). */
export function PromoteDiscoveredForm({
  values,
  locations,
  preview,
  busy,
  title = "Добавление в список Узлы",
  formRef,
  onChange,
  onPreview,
  onSubmit,
  onCancel,
}: Props) {
  const { categories } = useDeviceCategories();
  return (
    <form
      ref={formRef}
      onSubmit={onSubmit}
      style={{
        border: "1px solid #3d8bfd",
        borderRadius: 8,
        background: "#12151c",
        padding: 12,
        display: "grid",
        gap: 6,
      }}
    >
      <strong>{title}</strong>
      <p style={{ margin: "2px 0 4px", color: "#9aa3b5", fontSize: "0.85rem" }}>
        Имя и тип обязательны. IP необязателен (другой офис / нет SNMP с сервера) — тогда узел создаётся по MAC
        из LLDP. Community нужен только если указали IP.
      </p>
      <input
        name="promote-host"
        value={values.host}
        onChange={(e) => onChange({ host: e.target.value })}
        placeholder="Host / IP (необязательно)"
        style={{ width: "100%" }}
      />
      <input
        value={values.name}
        onChange={(e) => onChange({ name: e.target.value })}
        placeholder="Имя"
        required
        style={{ width: "100%" }}
      />
      <LocationCombobox
        value={values.location}
        options={locations}
        onChange={(location) => onChange({ location })}
        placeholder="Расположение (необязательно)"
        width="100%"
      />
      <div style={{ display: "flex", alignItems: "center", gap: 8, width: "100%" }}>
        <DeviceCategoryIcon category={values.category} height={22} />
        <select
          value={values.category}
          onChange={(e) => onChange({ category: e.target.value })}
          style={{ flex: 1, minWidth: 0 }}
          aria-label="Тип устройства"
        >
          {categories.map((o) => (
            <option key={o.id} value={o.id}>
              {o.label}
            </option>
          ))}
        </select>
      </div>
      <input
        value={values.community}
        onChange={(e) => onChange({ community: e.target.value })}
        placeholder="Community v2c"
        style={{ width: "100%" }}
      />
      {!values.host.trim() && (
        <p style={{ margin: 0, color: "#c9a227", fontSize: "0.85rem" }}>
          Mgmt IP неизвестен — узел можно добавить без адреса (по chassis MAC). SNMP/ping с сервера NetLynx
          работать не будут, пока не укажете IP позже.
        </p>
      )}
      <div style={{ display: "flex", gap: 6, flexWrap: "wrap" }}>
        <button type="button" onClick={onPreview} disabled={busy}>
          Проверить SNMP
        </button>
        <button type="submit" disabled={busy}>
          Добавить и сохранить
        </button>
        <button type="button" disabled={busy} onClick={onCancel}>
          Отменить добавление
        </button>
      </div>
      {preview && (
        <div style={{ color: preview.ok ? "#9bd08b" : "#f88" }}>
          {preview.ok ? `${preview.sys_name || "OK"} ${preview.host || ""}` : preview.error}
        </div>
      )}
    </form>
  );
}
