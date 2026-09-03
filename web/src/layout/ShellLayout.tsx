import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { NavLink, Outlet, useLocation, useNavigate } from "react-router-dom";
import SidebarSystemStats from "../components/SidebarSystemStats";
import { logout } from "../auth";
import {
  DEVICES_LIST_SCROLL_RESTORE_KEY,
  DEVICES_LIST_SCROLL_Y_KEY,
  applyDevicesListScrollY,
  parsePositiveNumber,
  readDevicesListScrollY,
} from "../storage/devicesListScroll";

type HealthInfo = { version?: string };

const nav = [
  { to: "/", label: "Дашборд", end: true },
  { to: "/devices", label: "Узлы" },
  { to: "/topology", label: "Топология" },
  { to: "/discovered", label: "Обнаружено" },
  { to: "/events", label: "События" },
  { to: "/investigate/mac", label: "MAC" },
  { to: "/investigate/loops", label: "Петли" },
  { to: "/postmortem", label: "Postmortem" },
  { to: "/settings", label: "Настройки" },
];

export default function ShellLayout() {
  const navTo = useNavigate();
  const location = useLocation();
  const prevPathRef = useRef<string | null>(null);
  const scrollSaveRafRef = useRef<number | null>(null);
  const [appVersion, setAppVersion] = useState<string>("");

  useEffect(() => {
    fetch("/health")
      .then((r) => (r.ok ? r.json() : Promise.reject()))
      .then((h: HealthInfo) => setAppVersion((h.version ?? "").trim()))
      .catch(() => setAppVersion(""));
  }, []);

  const onLogout = () => {
    logout().finally(() => navTo("/login", { replace: true }));
  };

  useLayoutEffect(() => {
    const path = location.pathname;
    const prev = prevPathRef.current;
    prevPathRef.current = path;

    const isDevicesList = path === "/devices";
    const isDeviceDetail = /^\/devices\/[^/]+$/.test(path);
    const leavingDevicesListForDetail = prev === "/devices" && isDeviceDetail;

    if (leavingDevicesListForDetail) {
      const yToSave = Math.max(
        parsePositiveNumber(sessionStorage.getItem(DEVICES_LIST_SCROLL_Y_KEY)),
        readDevicesListScrollY(),
      );
      if (yToSave > 0) {
        sessionStorage.setItem(DEVICES_LIST_SCROLL_Y_KEY, String(yToSave));
      }
      sessionStorage.setItem(DEVICES_LIST_SCROLL_RESTORE_KEY, "1");
      return;
    }

    if (prev != null && /^\/devices\/[^/]+$/.test(prev) && isDevicesList) {
      return;
    }

    if (!isDevicesList && !isDeviceDetail) {
      sessionStorage.removeItem(DEVICES_LIST_SCROLL_Y_KEY);
      sessionStorage.removeItem(DEVICES_LIST_SCROLL_RESTORE_KEY);
      applyDevicesListScrollY(0);
    }
  }, [location.pathname]);

  useEffect(() => {
    if (location.pathname !== "/devices") return;

    const saveNow = () => {
      sessionStorage.setItem(DEVICES_LIST_SCROLL_Y_KEY, String(readDevicesListScrollY()));
    };

    const onScroll = () => {
      if (scrollSaveRafRef.current != null) return;
      scrollSaveRafRef.current = requestAnimationFrame(() => {
        scrollSaveRafRef.current = null;
        saveNow();
      });
    };

    if (sessionStorage.getItem(DEVICES_LIST_SCROLL_RESTORE_KEY) !== "1") {
      saveNow();
    }
    const list = document.getElementById("devices-list-scroll");
    const main = document.getElementById("app-main-scroll");
    list?.addEventListener("scroll", onScroll, { passive: true });
    main?.addEventListener("scroll", onScroll, { passive: true });
    return () => {
      list?.removeEventListener("scroll", onScroll);
      main?.removeEventListener("scroll", onScroll);
      if (scrollSaveRafRef.current != null) {
        cancelAnimationFrame(scrollSaveRafRef.current);
        scrollSaveRafRef.current = null;
      }
    };
  }, [location.pathname]);

  return (
    <div className="app-shell">
      <aside className="app-sidebar">
        <div className="app-sidebar-brand">
          <img className="app-sidebar-logo" src="/logo.png" alt="" width={36} height={36} />
          <span>NetLynx{appVersion ? ` ${appVersion}` : ""}</span>
        </div>
        <nav className="app-sidebar-nav">
          {nav.map((n) => (
            <NavLink
              key={n.to}
              to={n.to}
              end={n.end}
              className={({ isActive }) => (isActive ? "app-sidebar-link app-sidebar-link--active" : "app-sidebar-link")}
            >
              {n.label}
            </NavLink>
          ))}
        </nav>
        <div className="app-sidebar-footer">
          <SidebarSystemStats />
          <button type="button" className="app-sidebar-logout" onClick={onLogout}>
            Выйти
          </button>
        </div>
      </aside>
      <main id="app-main-scroll" className="app-main">
        <Outlet />
      </main>
    </div>
  );
}
