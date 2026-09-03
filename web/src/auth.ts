/** Access-токен только в памяти модуля (не sessionStorage — меньше утечка при XSS). */

export type AuthEvent = "auth-lost" | "logout" | "login";

type AuthListener = (ev: AuthEvent) => void;

let accessToken: string | null = null;
let refreshInFlight: Promise<boolean> | null = null;
let loggingOut = false;
const listeners = new Set<AuthListener>();

const LEGACY_ACCESS_KEY = "invetor_access_token";

function migrateLegacyToken(): void {
  if (accessToken) return;
  try {
    const legacy = sessionStorage.getItem(LEGACY_ACCESS_KEY);
    if (legacy) {
      accessToken = legacy;
      sessionStorage.removeItem(LEGACY_ACCESS_KEY);
    }
  } catch {
    /* ignore */
  }
}

function emit(ev: AuthEvent): void {
  for (const l of [...listeners]) {
    try {
      l(ev);
    } catch {
      /* ignore listener errors */
    }
  }
}

/** Подписка на потерю сессии / logout / login (RequireAuth, SSE). */
export function subscribeAuth(listener: AuthListener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function isLoggingOut(): boolean {
  return loggingOut;
}

export function getAccessToken(): string | null {
  migrateLegacyToken();
  return accessToken;
}

export function setAccessToken(token: string): void {
  accessToken = token;
}

export function clearAccessToken(): void {
  accessToken = null;
  try {
    sessionStorage.removeItem(LEGACY_ACCESS_KEY);
  } catch {
    /* ignore */
  }
}

function clearBasicCreds(): void {
  try {
    sessionStorage.removeItem("invetor_api_user");
    sessionStorage.removeItem("invetor_api_pass");
  } catch {
    /* ignore */
  }
}

/** После неудачного refresh / 401 — единая точка «сессия умерла». */
export function notifyAuthLost(): void {
  if (loggingOut) return;
  clearAccessToken();
  clearBasicCreds();
  emit("auth-lost");
}

export async function refreshAccessToken(): Promise<boolean> {
  if (loggingOut) return false;
  if (refreshInFlight) return refreshInFlight;
  refreshInFlight = (async () => {
    try {
      const res = await fetch("/api/v1/auth/refresh", {
        method: "POST",
        credentials: "include",
        headers: { Accept: "application/json" },
      });
      if (!res.ok) return false;
      const body = (await res.json()) as { access_token?: string };
      if (!body.access_token) return false;
      setAccessToken(body.access_token);
      return true;
    } finally {
      refreshInFlight = null;
    }
  })();
  return refreshInFlight;
}

export async function ensureSession(): Promise<boolean> {
  if (getAccessToken()) return true;
  return refreshAccessToken();
}

export type LoginResult = {
  access_token: string;
  token_type: "Bearer";
  expires_in: number;
  user: { id: number; username: string };
};

export async function login(username: string, password: string): Promise<LoginResult> {
  loggingOut = false;
  const res = await fetch("/api/v1/auth/login", {
    method: "POST",
    credentials: "include",
    headers: {
      "Content-Type": "application/json",
      Accept: "application/json",
    },
    body: JSON.stringify({ username, password }),
  });
  if (!res.ok) {
    const t = await res.text();
    throw new Error(t || res.statusText);
  }
  const out = (await res.json()) as LoginResult;
  setAccessToken(out.access_token);
  emit("login");
  return out;
}

export async function logout(): Promise<void> {
  loggingOut = true;
  clearAccessToken();
  clearBasicCreds();
  try {
    await fetch("/api/v1/auth/logout", {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json" },
    });
  } finally {
    emit("logout");
    // loggingOut остаётся true до следующего login — чтобы 401 во время ухода не делали refresh.
  }
}

/** null = сессия мертва; "" = auth disabled (stream без ticket). */
export async function issueSSETicket(): Promise<string | null> {
  if (loggingOut) return null;

  const parseTicket = async (res: Response): Promise<string | null> => {
    if (!res.ok) return null;
    const body = (await res.json()) as { ticket?: string };
    return typeof body.ticket === "string" ? body.ticket : null;
  };

  const postTicket = (bearer: string | null) =>
    fetch("/api/v1/auth/sse-ticket", {
      method: "POST",
      credentials: "include",
      headers: {
        Accept: "application/json",
        ...(bearer ? { Authorization: "Bearer " + bearer } : {}),
      },
    });

  let tok = getAccessToken();
  if (!tok) {
    const ok = await refreshAccessToken();
    if (!ok) {
      // Lab: NETLYNX_AUTH_DISABLED → 200 + ticket:""
      const probe = await postTicket(null);
      if (probe.ok) {
        const t = await parseTicket(probe);
        if (t !== null) return t;
      }
      notifyAuthLost();
      return null;
    }
    tok = getAccessToken();
  }

  let res = await postTicket(tok);
  if (res.status === 401) {
    const ok = await refreshAccessToken();
    if (!ok) {
      notifyAuthLost();
      return null;
    }
    res = await postTicket(getAccessToken());
  }
  if (!res.ok) {
    if (res.status === 401) notifyAuthLost();
    return null;
  }
  return parseTicket(res);
}
