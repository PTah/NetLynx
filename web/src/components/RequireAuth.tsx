import { useEffect, useState } from "react";
import { Navigate, Outlet, useLocation } from "react-router-dom";
import { ensureSession, subscribeAuth } from "../auth";

export default function RequireAuth() {
  const loc = useLocation();
  const [state, setState] = useState<"loading" | "ok" | "no">("loading");

  useEffect(() => {
    let cancelled = false;
    ensureSession()
      .then((ok) => {
        if (!cancelled) setState(ok ? "ok" : "no");
      })
      .catch(() => {
        if (!cancelled) setState("no");
      });
    const unsub = subscribeAuth((ev) => {
      if (ev === "auth-lost" || ev === "logout") setState("no");
      if (ev === "login") setState("ok");
    });
    return () => {
      cancelled = true;
      unsub();
    };
  }, []);

  if (state === "loading") return <p style={{ padding: "1rem" }}>Проверка сессии...</p>;
  if (state === "no") return <Navigate to="/login" replace state={{ from: loc }} />;
  return <Outlet />;
}
