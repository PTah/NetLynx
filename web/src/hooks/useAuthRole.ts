import { useEffect, useState } from "react";
import { apiGet } from "../api";

export type AuthMe = { id: number; username: string; role: string };

export function roleCanWrite(role: string | null | undefined): boolean {
  const r = (role ?? "").trim().toLowerCase();
  return r === "operator" || r === "admin";
}

/** Роль из /api/v1/auth/me. Пока не загрузилась — canWrite=false (зритель). */
export function useAuthRole(): { role: string | null; canWrite: boolean } {
  const [role, setRole] = useState<string | null>(null);
  useEffect(() => {
    let cancelled = false;
    apiGet<AuthMe>("/api/v1/auth/me")
      .then((m) => {
        if (!cancelled) setRole((m.role ?? "").trim().toLowerCase() || "viewer");
      })
      .catch(() => {
        if (!cancelled) setRole("viewer");
      });
    return () => {
      cancelled = true;
    };
  }, []);
  return { role, canWrite: roleCanWrite(role) };
}
