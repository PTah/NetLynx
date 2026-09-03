import { useEffect, useMemo, useRef, useState } from "react";

type Props = {
  value: string;
  options: string[];
  onChange: (value: string) => void;
  onSubmit?: () => void;
  placeholder?: string;
  width?: number | string;
  id?: string;
};

/** Поле расположения: ввод вручную или выбор из существующих по подстроке. */
export function LocationCombobox({
  value,
  options,
  onChange,
  onSubmit,
  placeholder = "введите или выберите расположение",
  width = "100%",
  id,
}: Props) {
  const rootRef = useRef<HTMLDivElement>(null);
  const [open, setOpen] = useState(false);
  const [hi, setHi] = useState(0);

  const filtered = useMemo(() => {
    const q = value.trim().toLowerCase();
    if (!q) return options;
    return options.filter((loc) => loc.toLowerCase().includes(q));
  }, [options, value]);

  useEffect(() => {
    setHi(0);
  }, [value, open]);

  useEffect(() => {
    if (!open) return;
    const onDoc = (e: MouseEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", onDoc);
    return () => document.removeEventListener("mousedown", onDoc);
  }, [open]);

  const pick = (loc: string) => {
    onChange(loc);
    setOpen(false);
  };

  return (
    <div ref={rootRef} style={{ position: "relative", width, maxWidth: "100%" }}>
      <input
        id={id}
        value={value}
        autoComplete="off"
        aria-autocomplete="list"
        aria-expanded={open}
        role="combobox"
        placeholder={placeholder}
        onFocus={() => setOpen(true)}
        onClick={() => setOpen(true)}
        onChange={(e) => {
          onChange(e.target.value);
          setOpen(true);
        }}
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            setOpen(false);
            return;
          }
          if (e.key === "ArrowDown") {
            e.preventDefault();
            setOpen(true);
            setHi((i) => Math.min(i + 1, Math.max(0, filtered.length - 1)));
            return;
          }
          if (e.key === "ArrowUp") {
            e.preventDefault();
            setHi((i) => Math.max(0, i - 1));
            return;
          }
          if (e.key === "Enter") {
            e.preventDefault();
            if (open && filtered[hi] && filtered[hi] !== value.trim()) {
              pick(filtered[hi]);
              return;
            }
            setOpen(false);
            onSubmit?.();
          }
        }}
        style={{ width: "100%", boxSizing: "border-box" }}
      />
      {open && (
        <ul
          role="listbox"
          style={{
            position: "absolute",
            zIndex: 30,
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
            <li style={{ padding: "8px 10px", color: "#9aa3b5", fontSize: "0.85rem" }}>
              {value.trim() ? "Новое расположение — сохраните как есть" : "Нет сохранённых расположений"}
            </li>
          )}
          {filtered.map((loc, i) => {
            const active = i === hi;
            const isSel = loc === value.trim();
            return (
              <li
                key={loc}
                role="option"
                aria-selected={isSel}
                onMouseEnter={() => setHi(i)}
                onMouseDown={(e) => {
                  e.preventDefault();
                  pick(loc);
                }}
                style={{
                  padding: "7px 10px",
                  cursor: "pointer",
                  background: active ? "#1e3a5f" : "transparent",
                  color: isSel ? "#f0c14a" : "#e8eaef",
                  fontSize: "0.9rem",
                }}
              >
                {loc}
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
