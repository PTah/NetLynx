import { useEffect, useMemo, useRef, useState } from "react";
import type { CSSProperties } from "react";
import type { Device } from "../types";

type Props = {
  devices: Device[];
  value: string;
  onChange: (deviceId: string) => void;
  excludeId?: number;
  disabled?: boolean;
  ariaLabel?: string;
  placeholder?: string;
  inputStyle?: CSSProperties;
};

function deviceLabel(d: Device): string {
  const name = (d.name || "").trim() || `#${d.id}`;
  const host = (d.host || "").trim();
  return host ? `${name} (${host})` : name;
}

/** Выпадающий список устройств с сортировкой по имени и фильтром по подстроке (имя/host/sysName). */
export function DeviceSearchSelect({
  devices,
  value,
  onChange,
  excludeId,
  disabled,
  ariaLabel = "Узел",
  placeholder = "Поиск по имени…",
  inputStyle,
}: Props) {
  const rootRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [hi, setHi] = useState(0);

  const sorted = useMemo(() => {
    return devices
      .filter((d) => excludeId == null || d.id !== excludeId)
      .slice()
      .sort((a, b) => {
        const an = (a.name || a.host || "").localeCompare(b.name || b.host || "", "ru", { sensitivity: "base" });
        if (an !== 0) return an;
        return a.id - b.id;
      });
  }, [devices, excludeId]);

  const selected = useMemo(() => sorted.find((d) => String(d.id) === value) ?? null, [sorted, value]);

  const filtered = useMemo(() => {
    const q = query.trim().toLocaleLowerCase("ru");
    if (!q) return sorted;
    return sorted.filter((d) => {
      const name = (d.name || "").trim().toLocaleLowerCase("ru");
      const host = (d.host || "").trim().toLocaleLowerCase("ru");
      const sys = (d.sys_name || "").trim().toLocaleLowerCase("ru");
      return (
        name.includes(q) ||
        host.includes(q) ||
        sys.includes(q) ||
        String(d.id).includes(q)
      );
    });
  }, [sorted, query]);

  useEffect(() => {
    if (!open) return;
    setHi(0);
    const t = window.setTimeout(() => inputRef.current?.focus(), 0);
    return () => window.clearTimeout(t);
  }, [open, query]);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  const pick = (d: Device) => {
    onChange(String(d.id));
    setQuery("");
    setOpen(false);
  };

  const display = open ? query : selected ? deviceLabel(selected) : "";

  return (
    <div ref={rootRef} style={{ position: "relative", minWidth: 220 }}>
      <input
        ref={inputRef}
        type="search"
        disabled={disabled}
        value={display}
        placeholder={selected ? deviceLabel(selected) : placeholder}
        aria-label={ariaLabel}
        aria-expanded={open}
        aria-autocomplete="list"
        role="combobox"
        autoComplete="off"
        onFocus={() => {
          setOpen(true);
          setQuery("");
        }}
        onClick={() => {
          setOpen(true);
          setQuery("");
        }}
        onChange={(e) => {
          setOpen(true);
          setQuery(e.target.value);
          setHi(0);
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            setOpen(false);
            setQuery("");
            return;
          }
          if (!open && (e.key === "ArrowDown" || e.key === "Enter")) {
            e.preventDefault();
            setOpen(true);
            setQuery("");
            return;
          }
          if (!open) return;
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setHi((i) => Math.min(i + 1, Math.max(0, filtered.length - 1)));
          } else if (e.key === "ArrowUp") {
            e.preventDefault();
            setHi((i) => Math.max(0, i - 1));
          } else if (e.key === "Enter") {
            e.preventDefault();
            const d = filtered[hi];
            if (d) pick(d);
          }
        }}
        style={{ width: "100%", boxSizing: "border-box", ...inputStyle }}
      />
      {open && (
        <ul
          role="listbox"
          style={{
            position: "absolute",
            zIndex: 20,
            left: 0,
            right: 0,
            top: "100%",
            margin: 0,
            padding: 0,
            listStyle: "none",
            maxHeight: 260,
            overflowY: "auto",
            background: "#12151c",
            border: "1px solid #40506a",
            borderRadius: 6,
            boxShadow: "0 8px 24px #0008",
          }}
        >
          {filtered.length === 0 && (
            <li style={{ padding: "8px 10px", color: "#9aa3b5", fontSize: "0.85rem" }}>Нет совпадений</li>
          )}
          {filtered.map((d, i) => {
            const active = i === hi;
            const isSel = String(d.id) === value;
            return (
              <li
                key={d.id}
                role="option"
                aria-selected={isSel}
                onMouseEnter={() => setHi(i)}
                onMouseDown={(e) => {
                  e.preventDefault();
                  pick(d);
                }}
                style={{
                  padding: "7px 10px",
                  cursor: "pointer",
                  background: active ? "#1e3a5f" : "transparent",
                  color: isSel ? "#f0c14a" : "#e8eaef",
                  fontSize: "0.9rem",
                }}
              >
                {deviceLabel(d)}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
