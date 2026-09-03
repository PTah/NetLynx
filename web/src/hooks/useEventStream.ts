import { useEffect, useRef, useState } from "react";
import { issueSSETicket, subscribeAuth } from "../auth";
import type { EventRow } from "../types";

const BACKOFF_MS = [1000, 2000, 4000, 8000, 15000, 30000];

/** Подписка на SSE /api/v1/events/stream через одноразовый ticket.
 *  При обрыве — новый ticket + backoff. Без ticket (auth lost) — стоп, без silent 401-loop.
 */
export function useEventStream(onEvent: (ev: EventRow) => void, enabled = true) {
  const cb = useRef(onEvent);
  cb.current = onEvent;
  const [connected, setConnected] = useState(false);

  useEffect(() => {
    if (!enabled) return;
    let es: EventSource | null = null;
    let cancelled = false;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    let attempt = 0;
    let authDead = false;

    const clearTimer = () => {
      if (retryTimer != null) {
        clearTimeout(retryTimer);
        retryTimer = null;
      }
    };

    const closeES = () => {
      if (es) {
        es.onopen = null;
        es.onerror = null;
        es.close();
        es = null;
      }
    };

    const scheduleRetry = () => {
      if (cancelled || authDead) return;
      const delay = BACKOFF_MS[Math.min(attempt, BACKOFF_MS.length - 1)];
      attempt += 1;
      clearTimer();
      retryTimer = setTimeout(() => {
        void connect();
      }, delay);
    };

    const attachHandlers = (source: EventSource) => {
      source.onopen = () => {
        attempt = 0;
        setConnected(true);
      };
      source.onerror = () => {
        setConnected(false);
        closeES();
        scheduleRetry();
      };
      source.addEventListener("invetor_event", (msg) => {
        try {
          const raw = JSON.parse((msg as MessageEvent).data) as {
            event_id: number;
            device_id: number;
            if_index?: number | null;
            event_type: string;
            severity: string;
            payload?: Record<string, unknown>;
          };
          cb.current({
            id: raw.event_id,
            device_id: raw.device_id,
            if_index: raw.if_index ?? null,
            event_type: raw.event_type,
            severity: raw.severity,
            payload: raw.payload,
            created_at: new Date().toISOString(),
          });
        } catch {
          /* ignore malformed */
        }
      });
    };

    const connect = async () => {
      if (cancelled || authDead) return;
      closeES();
      const ticket = await issueSSETicket();
      if (cancelled || authDead) return;
      // null = auth lost (notifyAuthLost уже вызван); "" = auth disabled → stream без ticket.
      if (ticket === null) {
        authDead = true;
        setConnected(false);
        return;
      }
      const url =
        ticket !== ""
          ? `/api/v1/events/stream?ticket=${encodeURIComponent(ticket)}`
          : `/api/v1/events/stream`;
      es = new EventSource(url, { withCredentials: true });
      attachHandlers(es);
    };

    const unsub = subscribeAuth((ev) => {
      if (ev === "auth-lost" || ev === "logout") {
        authDead = true;
        clearTimer();
        closeES();
        setConnected(false);
      }
      if (ev === "login") {
        authDead = false;
        attempt = 0;
        void connect();
      }
    });

    void connect();

    return () => {
      cancelled = true;
      unsub();
      clearTimer();
      closeES();
      setConnected(false);
    };
  }, [enabled]);

  return connected;
}
