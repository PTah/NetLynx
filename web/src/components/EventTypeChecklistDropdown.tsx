import type { CSSProperties } from "react";
import { useEffect, useRef, useState } from "react";
import { formatEventTypeLabel } from "../eventFormat";

/** Типы событий, доступные для фильтров уведомлений. */
export const NOTIFY_EVENT_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: "LINK_UP", label: formatEventTypeLabel("LINK_UP") },
  { value: "LINK_DOWN", label: formatEventTypeLabel("LINK_DOWN") },
  { value: "DEVICE_OFFLINE", label: formatEventTypeLabel("DEVICE_OFFLINE") },
  { value: "DEVICE_ONLINE", label: formatEventTypeLabel("DEVICE_ONLINE") },
  { value: "PORT_UTILIZATION_HIGH", label: formatEventTypeLabel("PORT_UTILIZATION_HIGH") },
  { value: "PORT_UTILIZATION_OK", label: formatEventTypeLabel("PORT_UTILIZATION_OK") },
  { value: "PORT_SPEED_DOWN", label: formatEventTypeLabel("PORT_SPEED_DOWN") },
  { value: "PORT_SPEED_OK", label: formatEventTypeLabel("PORT_SPEED_OK") },
  { value: "UNKNOWN_MAC_ON_ACCESS_PORT", label: formatEventTypeLabel("UNKNOWN_MAC_ON_ACCESS_PORT") },
  { value: "MAC_MOVED", label: formatEventTypeLabel("MAC_MOVED") },
  { value: "MAC_FLAPPING", label: formatEventTypeLabel("MAC_FLAPPING") },
  { value: "MAC_MULTI_ACCESS", label: formatEventTypeLabel("MAC_MULTI_ACCESS") },
  { value: "MAC_REMOVED", label: formatEventTypeLabel("MAC_REMOVED") },
  { value: "ACCESS_PORT_MAC_SUBSTITUTED", label: formatEventTypeLabel("ACCESS_PORT_MAC_SUBSTITUTED") },
  { value: "ACCESS_PORT_LONG_IDLE_DEVICE", label: formatEventTypeLabel("ACCESS_PORT_LONG_IDLE_DEVICE") },
  { value: "SNMP_TRAP", label: formatEventTypeLabel("SNMP_TRAP") },
  { value: "MANUAL_LINK_SUPERSEDED", label: formatEventTypeLabel("MANUAL_LINK_SUPERSEDED") },
];

export function parseEventTypesCSV(raw: string): string[] {
  return raw
    .split(",")
    .map((s) => s.trim())
    .filter(Boolean);
}

export function joinEventTypesCSV(types: string[]): string {
  return types.join(",");
}

type Props = {
  value: string;
  onChange: (csv: string) => void;
  disabled?: boolean;
};

/** Выпадающий список с чекбоксами. Пустой выбор = все типы (как раньше). */
export function EventTypeChecklistDropdown({ value, onChange, disabled }: Props) {
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement>(null);
  const selected = parseEventTypesCSV(value);
  const selectedSet = new Set(selected.map((s) => s.toUpperCase()));

  useEffect(() => {
    if (!open) return;
    function onDoc(e: MouseEvent) {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    }
    function onKey(e: KeyboardEvent) {
      if (e.key === "Escape") setOpen(false);
    }
    document.addEventListener("mousedown", onDoc);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDoc);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  function toggle(code: string) {
    const up = code.toUpperCase();
    const next = new Set(selectedSet);
    if (next.has(up)) next.delete(up);
    else next.add(up);
    const ordered = NOTIFY_EVENT_TYPE_OPTIONS.map((o) => o.value).filter((v) => next.has(v));
    // сохранить неизвестные коды из value (на случай ручного наследия)
    for (const s of selected) {
      const u = s.toUpperCase();
      if (!NOTIFY_EVENT_TYPE_OPTIONS.some((o) => o.value === u) && next.has(u)) {
        ordered.push(s);
      }
    }
    onChange(joinEventTypesCSV(ordered));
  }

  function selectAll() {
    onChange("");
  }

  function clearToNone() {
    // «ничего» как пустой список без типов — для фильтра пусто = все.
    // Явный «снять все галки» = пусто = все. Для «только выбранные» нужны галки.
    onChange("");
  }

  const summary =
    selected.length === 0
      ? "Все типы"
      : selected.length === 1
        ? formatEventTypeLabel(selected[0])
        : `Выбрано: ${selected.length}`;

  return (
    <div ref={rootRef} style={{ position: "relative", width: "100%" }}>
      <button
        type="button"
        disabled={disabled}
        onClick={() => setOpen((v) => !v)}
        aria-haspopup="listbox"
        aria-expanded={open}
        style={{
          width: "100%",
          textAlign: "left",
          display: "flex",
          justifyContent: "space-between",
          alignItems: "center",
          gap: 8,
          padding: "6px 10px",
          background: "#12161f",
          border: "1px solid #2e3648",
          borderRadius: 6,
          color: "#e8ecf4",
          cursor: disabled ? "not-allowed" : "pointer",
          fontSize: "0.9rem",
        }}
      >
        <span style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{summary}</span>
        <span style={{ color: "#9aa3b5", flexShrink: 0 }}>{open ? "▲" : "▼"}</span>
      </button>
      {open && (
        <div
          role="listbox"
          aria-multiselectable
          style={{
            position: "absolute",
            zIndex: 40,
            left: 0,
            right: 0,
            top: "calc(100% + 4px)",
            maxHeight: 280,
            overflowY: "auto",
            background: "#1a1f2b",
            border: "1px solid #2e3648",
            borderRadius: 8,
            boxShadow: "0 10px 28px rgba(0,0,0,0.45)",
            padding: "6px 0",
          }}
        >
          <div style={{ display: "flex", gap: 8, padding: "4px 10px 8px", borderBottom: "1px solid #2e3648" }}>
            <button type="button" onClick={selectAll} style={linkBtn}>
              Все типы
            </button>
            <button
              type="button"
              onClick={() => {
                onChange(joinEventTypesCSV(NOTIFY_EVENT_TYPE_OPTIONS.map((o) => o.value)));
              }}
              style={linkBtn}
            >
              Отметить все
            </button>
            <button type="button" onClick={clearToNone} style={linkBtn}>
              Сбросить
            </button>
          </div>
          {NOTIFY_EVENT_TYPE_OPTIONS.map((opt) => {
            const checked = selectedSet.has(opt.value);
            return (
              <label
                key={opt.value}
                style={{
                  display: "flex",
                  alignItems: "flex-start",
                  gap: 8,
                  padding: "6px 10px",
                  cursor: "pointer",
                  fontSize: "0.88rem",
                  color: "#e8ecf4",
                }}
                onMouseEnter={(e) => {
                  (e.currentTarget as HTMLElement).style.background = "#242b3a";
                }}
                onMouseLeave={(e) => {
                  (e.currentTarget as HTMLElement).style.background = "transparent";
                }}
              >
                <input type="checkbox" checked={checked} onChange={() => toggle(opt.value)} style={{ marginTop: 2 }} />
                <span>
                  <span style={{ display: "block" }}>{opt.label}</span>
                  <span style={{ display: "block", fontSize: "0.72rem", color: "#7a8499" }}>{opt.value}</span>
                </span>
              </label>
            );
          })}
          <p style={{ margin: "6px 10px 4px", fontSize: "0.72rem", color: "#7a8499" }}>
            Без отметок = отправлять все типы событий.
          </p>
        </div>
      )}
    </div>
  );
}

const linkBtn: CSSProperties = {
  background: "transparent",
  border: "none",
  color: "#7ab0ff",
  cursor: "pointer",
  fontSize: "0.78rem",
  padding: 0,
};
