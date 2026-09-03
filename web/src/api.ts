import { clearAccessToken, getAccessToken, isLoggingOut, notifyAuthLost, refreshAccessToken } from "./auth";

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
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  return res.json() as Promise<T>;
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
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  return res.json() as Promise<T>;
}

export async function apiDelete(path: string, init?: RequestInit): Promise<void> {
  const res = await request(path, {
    method: "DELETE",
    ...init,
  });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
}

/** DELETE с телом ответа JSON (например удаление всех узлов). */
export async function apiDeleteJson<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await request(path, { method: "DELETE", ...init });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  const text = await res.text();
  if (!text.trim()) return {} as T;
  return JSON.parse(text) as T;
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
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  const text = await res.text();
  if (!text.trim()) return {} as T;
  return JSON.parse(text) as T;
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
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  return res.json() as Promise<T>;
}

/** POST multipart (не ставить Content-Type — граница form-data ставит браузер). */
export async function apiUpload<T>(path: string, form: FormData): Promise<T> {
  const res = await request(path, { method: "POST", body: form });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  return res.json() as Promise<T>;
}
