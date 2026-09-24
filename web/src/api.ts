import { clearAccessToken, getAccessToken, isLoggingOut, notifyAuthLost, refreshAccessToken } from "./auth";

/** Тело ошибки API: `{"error":"..."}` → текст; иначе исходная строка. */
export function parseApiErrorBody(raw: string, fallback = "Ошибка запроса"): string {
  const t = (raw || "").trim();
  if (!t) return fallback;
  try {
    const j = JSON.parse(t) as { error?: unknown };
    if (typeof j?.error === "string" && j.error.trim()) return j.error.trim();
  } catch {
    /* not JSON */
  }
  return t;
}

function throwIfNotOk(res: Response, body: string): void {
  if (res.ok) return;
  throw new Error(parseApiErrorBody(body, res.statusText));
}

function authHeaders(): Record<string, string> {
  const tok = getAccessToken();
  if (tok) return { Authorization: "Bearer " + tok };
  return {};
}

async function request(path: string, init?: RequestInit, retry401 = true): Promise<Response> {
  if (isLoggingOut() && !path.startsWith("/api/v1/auth/logout")) {
    return new Response(JSON.stringify({ error: "logging out" }), { status: 401 });
  }
  const res = await fetch(path, {
    credentials: "include",
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.headers ?? {}),
      ...authHeaders(),
    },
  });
  if (res.status === 401 && retry401 && !path.startsWith("/api/v1/auth/")) {
    if (isLoggingOut()) return res;
    const ok = await refreshAccessToken();
    if (ok) {
      const retry = await request(path, init, false);
      if (retry.status === 401) {
        clearAccessToken();
        notifyAuthLost();
      }
      return retry;
    }
    clearAccessToken();
    notifyAuthLost();
  }
  return res;
}

export async function apiGet<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await request(path, init);
  const t = await res.text();
  throwIfNotOk(res, t);
  return (t.trim() ? JSON.parse(t) : {}) as T;
}

/** JSON null / не-массив → пустой массив (безопасно для .length / .map). */
export function asArray<T>(value: T[] | null | undefined): T[] {
  return Array.isArray(value) ? value : [];
}

export async function apiPatch<T>(path: string, body: unknown, init?: RequestInit): Promise<T> {
  const res = await request(path, {
    method: "PATCH",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body ?? {}),
    ...init,
  });
  const t = await res.text();
  throwIfNotOk(res, t);
  return (t.trim() ? JSON.parse(t) : {}) as T;
}

export async function apiDelete(path: string, init?: RequestInit): Promise<void> {
  const res = await request(path, {
    method: "DELETE",
    ...init,
  });
  const t = await res.text();
  throwIfNotOk(res, t);
}

/** DELETE с телом ответа JSON (например удаление всех узлов). */
export async function apiDeleteJson<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await request(path, { method: "DELETE", ...init });
  const t = await res.text();
  throwIfNotOk(res, t);
  if (!t.trim()) return {} as T;
  return JSON.parse(t) as T;
}

export async function apiPut<T>(path: string, body: unknown, init?: RequestInit): Promise<T> {
  const res = await request(path, {
    method: "PUT",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body ?? {}),
    ...init,
  });
  const t = await res.text();
  throwIfNotOk(res, t);
  if (!t.trim()) return {} as T;
  return JSON.parse(t) as T;
}

export async function apiPost<T>(path: string, body: unknown, init?: RequestInit): Promise<T> {
  const { headers: initHeaders, ...rest } = init ?? {};
  const res = await request(path, {
    method: "POST",
    body: JSON.stringify(body ?? {}),
    ...rest,
    headers: {
      "Content-Type": "application/json",
      ...(initHeaders ?? {}),
    },
  });
  const t = await res.text();
  throwIfNotOk(res, t);
  return (t.trim() ? JSON.parse(t) : {}) as T;
}

/** POST multipart (не ставить Content-Type — граница form-data ставит браузер). */
export async function apiUpload<T>(path: string, form: FormData): Promise<T> {
  const res = await request(path, { method: "POST", body: form });
  const t = await res.text();
  throwIfNotOk(res, t);
  return (t.trim() ? JSON.parse(t) : {}) as T;
}
