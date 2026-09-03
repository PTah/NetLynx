import { useCallback, useEffect, useMemo, useRef, useState, type MouseEvent } from "react";

function loadWidths(key: string, defaults: number[]): number[] {
  if (typeof window === "undefined") return [...defaults];
  try {
    const raw = localStorage.getItem(key);
    if (!raw) return [...defaults];
    const arr = JSON.parse(raw) as unknown;
    if (!Array.isArray(arr) || arr.length !== defaults.length) return [...defaults];
    return arr.map((v, i) => {
      const n = Number(v);
      return Number.isFinite(n) && n >= 40 ? n : defaults[i];
    });
  } catch {
    return [...defaults];
  }
}

function saveWidths(key: string, widths: number[]) {
  if (typeof window === "undefined") return;
  try {
    localStorage.setItem(key, JSON.stringify(widths));
  } catch {
    /* ignore */
  }
}

const storageKey = (tableId: string) => `invetor_table_cols_${tableId}`;

/**
 * Фиксированные ширины колонок + colgroup + перетаскивание границы столбца (сохранение в localStorage).
 */
export function usePersistedColumnWidths(tableId: string, defaults: number[]) {
  const key = storageKey(tableId);
  const [widths, setWidths] = useState<number[]>(() => loadWidths(key, defaults));
  const widthsRef = useRef(widths);
  useEffect(() => {
    widthsRef.current = widths;
  }, [widths]);

  const colgroup = useMemo(
    () => (
      <colgroup>
        {widths.map((w, i) => (
          <col key={i} style={{ width: w, minWidth: 40 }} />
        ))}
      </colgroup>
    ),
    [widths],
  );

  const startResize = useCallback(
    (colIndex: number) => (e: MouseEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const startX = e.clientX;
      const startW = widthsRef.current[colIndex] ?? defaults[colIndex];

      const onMove = (ev: globalThis.MouseEvent) => {
        const nextW = Math.max(40, Math.round(startW + (ev.clientX - startX)));
        setWidths((prev) => {
          const next = [...prev];
          next[colIndex] = nextW;
          widthsRef.current = next;
          return next;
        });
      };

      const onUp = () => {
        window.removeEventListener("mousemove", onMove);
        window.removeEventListener("mouseup", onUp);
        saveWidths(key, widthsRef.current);
      };

      window.addEventListener("mousemove", onMove);
      window.addEventListener("mouseup", onUp);
    },
    [defaults, key],
  );

  const ResizeHandle = useCallback(
    ({ colIndex }: { colIndex: number }) => (
      <span
        aria-hidden
        title="Потянуть, чтобы изменить ширину столбца"
        onMouseDown={startResize(colIndex)}
        style={{
          position: "absolute",
          right: 0,
          top: 0,
          bottom: 0,
          width: 6,
          cursor: "col-resize",
          zIndex: 1,
        }}
      />
    ),
    [startResize],
  );

  return { colgroup, ResizeHandle, widths };
}
