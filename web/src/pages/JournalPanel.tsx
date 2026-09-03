import { useCallback, useEffect, useRef, useState } from "react";
import { apiGet } from "../api";
import { issueSSETicket } from "../auth";

type JournalMeta = {
  unit: string;
  categories: { id: string; label: string }[];
  levels: { id: string; label: string }[];
  limits: number[];
};

type Mode = "live" | "last" | "period";

const JOURNAL_SIZE_KEY = "netlynx.journal.viewSize";
const JOURNAL_SIZE_DEFAULT = { w: 720, h: 360 };
const JOURNAL_SIZE_MIN = { w: 320, h: 160 };

function loadJournalSize(): { w: number; h: number } {
  try {
    const raw = localStorage.getItem(JOURNAL_SIZE_KEY);
    if (!raw) return { ...JOURNAL_SIZE_DEFAULT };
    const j = JSON.parse(raw) as { w?: unknown; h?: unknown };
    const w = typeof j.w === "number" ? j.w : JOURNAL_SIZE_DEFAULT.w;
    const h = typeof j.h === "number" ? j.h : JOURNAL_SIZE_DEFAULT.h;
    return {
      w: Math.max(JOURNAL_SIZE_MIN.w, Math.min(2400, Math.round(w))),
      h: Math.max(JOURNAL_SIZE_MIN.h, Math.min(1400, Math.round(h))),
    };
  } catch {
    return { ...JOURNAL_SIZE_DEFAULT };
  }
}

function saveJournalSize(w: number, h: number) {
  try {
    localStorage.setItem(JOURNAL_SIZE_KEY, JSON.stringify({ w: Math.round(w), h: Math.round(h) }));
  } catch {
    /* ignore */
  }
}

function todayLocal(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function daysAgoLocal(n: number): string {
  const d = new Date();
  d.setDate(d.getDate() - n);
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

export default function JournalPanel() {
  const [meta, setMeta] = useState<JournalMeta | null>(null);
  const [mode, setMode] = useState<Mode>("live");
  const [lines, setLines] = useState<string[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [liveOn, setLiveOn] = useState(false);
  const [liveOk, setLiveOk] = useState(false);
  const [limit, setLimit] = useState(100);
  const [level, setLevel] = useState("info");
  const [cats, setCats] = useState<string[]>([]);
  const [since, setSince] = useState(daysAgoLocal(1));
  const [until, setUntil] = useState(todayLocal());
  const preRef = useRef<HTMLPreElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const esRef = useRef<EventSource | null>(null);
  const stickBottom = useRef(true);
  const resizingRef = useRef(false);

  // Размер только в DOM + localStorage — не в React state, иначе setLines при live сбрасывает resize.
  useEffect(() => {
    const el = wrapRef.current;
    if (!el) return;
    const s = loadJournalSize();
    el.style.width = `${s.w}px`;
    el.style.height = `${s.h}px`;

    const persist = () => {
      const w = el.offsetWidth;
      const h = el.offsetHeight;
      if (w < JOURNAL_SIZE_MIN.w || h < JOURNAL_SIZE_MIN.h) return;
      saveJournalSize(w, h);
    };

    const onPointerDown = (e: PointerEvent) => {
      // угол resize обычно у правого/нижнего края
      const r = el.getBoundingClientRect();
      const nearCorner = e.clientX > r.right - 22 && e.clientY > r.bottom - 22;
      if (nearCorner) resizingRef.current = true;
    };
    const onPointerUp = () => {
      if (!resizingRef.current) return;
      resizingRef.current = false;
      persist();
    };

    el.addEventListener("pointerdown", onPointerDown);
    window.addEventListener("pointerup", onPointerUp);
    return () => {
      el.removeEventListener("pointerdown", onPointerDown);
      window.removeEventListener("pointerup", onPointerUp);
    };
  }, []);

  useEffect(() => {
    apiGet<JournalMeta>("/api/v1/settings/journal")
      .then(setMeta)
      .catch((e: Error) => setErr(e.message));
  }, []);

  const scrollIfNeeded = () => {
    const el = preRef.current;
    if (!el || !stickBottom.current) return;
    el.scrollTop = el.scrollHeight;
  };

  const stopLive = useCallback(() => {
    if (esRef.current) {
      esRef.current.close();
      esRef.current = null;
    }
    setLiveOn(false);
    setLiveOk(false);
  }, []);

  useEffect(() => () => stopLive(), [stopLive]);

  const loadLast = async () => {
    setErr(null);
    setLoading(true);
    stopLive();
    try {
      const p = new URLSearchParams();
      p.set("limit", String(limit));
      if (level) p.set("level", level);
      if (cats.length) p.set("categories", cats.join(","));
      const res = await apiGet<{ lines: string[]; count: number }>(`/api/v1/settings/journal/lines?${p}`);
      setLines(Array.isArray(res.lines) ? res.lines : []);
      stickBottom.current = true;
      requestAnimationFrame(scrollIfNeeded);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const loadPeriod = async () => {
    setErr(null);
    setLoading(true);
    stopLive();
    try {
      const p = new URLSearchParams();
      p.set("limit", String(Math.max(limit, 500)));
      p.set("since", since);
      if (until) p.set("until", until + " 23:59:59");
      if (level) p.set("level", level);
      if (cats.length) p.set("categories", cats.join(","));
      const res = await apiGet<{ lines: string[]; count: number }>(`/api/v1/settings/journal/lines?${p}`);
      setLines(Array.isArray(res.lines) ? res.lines : []);
      stickBottom.current = true;
      requestAnimationFrame(scrollIfNeeded);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  };

  const startLive = async () => {
    setErr(null);
    stopLive();
    setLines([]);
    setLiveOn(true);
    try {
      const ticket = await issueSSETicket();
      if (ticket === null) {
        setLiveOn(false);
        setErr("нет сессии для потока журнала");
        return;
      }
      const p = new URLSearchParams();
      if (ticket) p.set("ticket", ticket);
      p.set("limit", "50");
      if (level) p.set("level", level);
      if (cats.length) p.set("categories", cats.join(","));
      const es = new EventSource(`/api/v1/settings/journal/stream?${p}`);
      esRef.current = es;
      es.addEventListener("log", (ev) => {
        try {
          const line = JSON.parse((ev as MessageEvent).data) as string;
          setLines((prev) => {
            const next = prev.length > 2000 ? prev.slice(-1500) : prev.slice();
            next.push(line);
            return next;
          });
          requestAnimationFrame(scrollIfNeeded);
        } catch {
          /* ignore */
        }
      });
      es.addEventListener("error", (ev) => {
        try {
          const msg = JSON.parse((ev as MessageEvent).data) as string;
          setErr(msg);
        } catch {
          /* EventSource connection error also fires "error" */
        }
        setLiveOk(false);
      });
      es.onopen = () => setLiveOk(true);
      es.onerror = () => {
        setLiveOk(false);
        if (es.readyState === EventSource.CLOSED) {
          setLiveOn(false);
        }
      };
    } catch (e) {
      setLiveOn(false);
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  useEffect(() => {
    if (mode === "last") void loadLast();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [mode]);

  const toggleCat = (id: string) => {
    setCats((prev) => (prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]));
  };

  const limits = meta?.limits ?? [50, 100, 200, 500, 1000];

  return (
    <div role="tabpanel">
      <p className="settings-lead">
        Журнал службы <code>{meta?.unit ?? "NetLynx.service"}</code> (systemd journal). Сетевые события портов — на
        странице «События».
      </p>
      {err && <p style={{ color: "#f88" }}>{err}</p>}

      <section className="settings-card settings-card--journal">
        <div className="journal-modes" role="tablist" aria-label="Режим журнала">
          {(
            [
              ["live", "В реальном времени"],
              ["last", "Последние записи"],
              ["period", "За период"],
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              type="button"
              className={mode === id ? "settings-tab settings-tab--active" : "settings-tab"}
              onClick={() => {
                stopLive();
                setMode(id);
              }}
            >
              {label}
            </button>
          ))}
        </div>

        <div className="journal-filters">
          {(mode === "last" || mode === "period") && (
            <label>
              Сколько строк
              <br />
              <select value={limit} onChange={(e) => setLimit(Number(e.target.value))}>
                {limits.map((n) => (
                  <option key={n} value={n}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
          )}
          <label>
            Уровень
            <br />
            <select value={level} onChange={(e) => setLevel(e.target.value)}>
              {(meta?.levels ?? []).map((l) => (
                <option key={l.id} value={l.id}>
                  {l.label}
                </option>
              ))}
            </select>
          </label>
          {mode === "period" && (
            <>
              <label>
                С
                <br />
                <input type="date" value={since} onChange={(e) => setSince(e.target.value)} />
              </label>
              <label>
                По
                <br />
                <input type="date" value={until} onChange={(e) => setUntil(e.target.value)} />
              </label>
            </>
          )}
        </div>

        {mode === "period" && (
          <div className="journal-cats">
            <div style={{ marginBottom: "0.35rem", color: "#9aa3b5", fontSize: "0.9rem" }}>
              Темы (пусто = все). Можно выбрать несколько:
            </div>
            <div className="journal-cats-list">
              {(meta?.categories ?? []).map((c) => (
                <label key={c.id} className="journal-cat">
                  <input type="checkbox" checked={cats.includes(c.id)} onChange={() => toggleCat(c.id)} />
                  {c.label}
                </label>
              ))}
            </div>
          </div>
        )}

        <div className="journal-actions">
          {mode === "live" && !liveOn && (
            <button type="button" onClick={() => void startLive()}>
              Подключить поток
            </button>
          )}
          {mode === "live" && liveOn && (
            <button type="button" onClick={stopLive}>
              Остановить
            </button>
          )}
          {mode === "last" && (
            <button type="button" onClick={() => void loadLast()} disabled={loading}>
              {loading ? "Загрузка…" : "Обновить"}
            </button>
          )}
          {mode === "period" && (
            <button type="button" onClick={() => void loadPeriod()} disabled={loading}>
              {loading ? "Загрузка…" : "Показать"}
            </button>
          )}
          {mode === "live" && (
            <span style={{ color: liveOk ? "#8d8" : "#9aa3b5", fontSize: "0.9rem" }}>
              {liveOn ? (liveOk ? "● поток активен" : "○ подключение…") : "поток выключен"}
            </span>
          )}
          <span style={{ color: "#9aa3b5", fontSize: "0.85rem" }}>{lines.length} строк</span>
        </div>

        <div
          ref={wrapRef}
          className="journal-view-wrap"
          title="Потяните за правый нижний угол, чтобы изменить размер"
        >
          <pre
            ref={preRef}
            className="journal-view"
            onScroll={(e) => {
              const el = e.currentTarget;
              stickBottom.current = el.scrollHeight - el.scrollTop - el.clientHeight < 40;
            }}
          >
            {lines.length === 0 ? (loading ? "…" : "Нет записей") : lines.join("\n")}
          </pre>
        </div>
        <p className="journal-resize-hint">Размер окна запоминается в этом браузере. Угол справа снизу — изменить размер.</p>
      </section>
    </div>
  );
}
