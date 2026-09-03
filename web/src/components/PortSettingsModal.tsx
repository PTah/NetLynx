import type { CSSProperties } from "react";
import { FormEvent, useEffect, useRef, useState } from "react";
import { createPortal } from "react-dom";

export type PoEModeValue = "off" | "24v" | "poe+";
export type EdgePortValue = "auto" | "enable" | "disable";

export type PortSettingsTarget = {
  if_index: number;
  if_name?: string | null;
  admin_status?: number | null;
  poe_mode?: PoEModeValue | null;
  isolate?: boolean | null;
  label?: string;
  cli_access_vlan?: number | null;
  cli_port_mode?: string | null;
};

export type PortLiveSettings = {
  admin_up: boolean;
  isolate: boolean;
  poe_mode: PoEModeValue;
  dhcp_trusted?: boolean;
  flow_control?: boolean;
  stp_enabled?: boolean;
  edge_port?: EdgePortValue | string;
  port_priority?: number;
  path_cost?: number;
  poe_24v?: boolean;
  via?: string;
  live?: boolean;
  live_err?: string | null;
  access_vlan?: number | null;
  port_mode?: string | null;
};

type Baseline = {
  enable: boolean;
  isolate: boolean;
  dhcpTrusted: boolean;
  flowControl: boolean;
  stpEnabled: boolean;
  edgePort: EdgePortValue;
  portPriority: number;
  pathCost: number;
  poeMode: PoEModeValue;
  accessVlan: number;
};

type Props = {
  open: boolean;
  port: PortSettingsTarget | null;
  canWrite: boolean;
  settingsWritable?: boolean;
  poe24vSupported?: boolean;
  loadLiveSettings?: () => Promise<PortLiveSettings>;
  /** VLAN ID из vlan database свитча (in_database). null/undefined — проверка только на сервере. */
  loadKnownVlans?: () => Promise<number[]>;
  onClose: () => void;
  onSave: (opts: {
    adminUp: boolean;
    poeMode: PoEModeValue;
    isolate: boolean;
    dhcpTrusted: boolean;
    flowControl: boolean;
    stpEnabled: boolean;
    edgePort: EdgePortValue;
    portPriority: number;
    pathCost: number;
    enableDirty: boolean;
    poeDirty: boolean;
    isolateDirty: boolean;
    dhcpTrustedDirty: boolean;
    flowControlDirty: boolean;
    stpDirty: boolean;
    vlanDirty: boolean;
    accessVlan: number;
  }) => Promise<void>;
};

function normEdge(v: string | undefined | null): EdgePortValue {
  if (v === "enable" || v === "disable" || v === "auto") return v;
  return "auto";
}

function baselineFromPort(port: PortSettingsTarget): Baseline {
  return {
    enable: port.admin_status === 1,
    isolate: port.isolate === true,
    dhcpTrusted: false,
    flowControl: false,
    stpEnabled: true,
    edgePort: "auto",
    portPriority: 128,
    pathCost: 0,
    poeMode: port.poe_mode === "off" || port.poe_mode === "24v" || port.poe_mode === "poe+" ? port.poe_mode : "poe+",
    accessVlan: port.cli_access_vlan && port.cli_access_vlan > 0 ? port.cli_access_vlan : 1,
  };
}

/** Модалка настроек порта (UISP-like). EdgeSwitch / Eltex / SNR. */
export function PortSettingsModal({
  open,
  port,
  canWrite,
  settingsWritable = true,
  poe24vSupported = true,
  loadLiveSettings,
  loadKnownVlans,
  onClose,
  onSave,
}: Props) {
  const [enable, setEnable] = useState(true);
  const [isolate, setIsolate] = useState(false);
  const [dhcpTrusted, setDhcpTrusted] = useState(false);
  const [flowControl, setFlowControl] = useState(false);
  const [stpEnabled, setStpEnabled] = useState(true);
  const [edgePort, setEdgePort] = useState<EdgePortValue>("auto");
  const [portPriority, setPortPriority] = useState(128);
  const [pathCost, setPathCost] = useState(0);
  const [poeMode, setPoeMode] = useState<PoEModeValue>("poe+");
  const [accessVlan, setAccessVlan] = useState(1);
  const [poe24v, setPoe24v] = useState(poe24vSupported);
  const [baseline, setBaseline] = useState<Baseline>(baselineFromPort({ if_index: 0 }));
  const [loading, setLoading] = useState(false);
  const [loadNote, setLoadNote] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  /** null = ещё не загрузили / нет данных; Set = известные VLAN из database */
  const [knownVlans, setKnownVlans] = useState<Set<number> | null>(null);
  const loadLiveRef = useRef(loadLiveSettings);
  const loadKnownVlansRef = useRef(loadKnownVlans);
  const vlanInputRef = useRef<HTMLInputElement | null>(null);
  /** Закрывать по клику на фон только если pointerdown тоже был на фоне (не drag-выделение из input). */
  const backdropPtrDownRef = useRef(false);
  loadLiveRef.current = loadLiveSettings;
  loadKnownVlansRef.current = loadKnownVlans;

  useEffect(() => {
    if (!open || !port) return;
    let cancelled = false;
    const poll = baselineFromPort(port);
    setBaseline(poll);
    setEnable(poll.enable);
    setIsolate(poll.isolate);
    setDhcpTrusted(poll.dhcpTrusted);
    setFlowControl(poll.flowControl);
    setStpEnabled(poll.stpEnabled);
    setEdgePort(poll.edgePort);
    setPortPriority(poll.portPriority);
    setPathCost(poll.pathCost);
    setPoeMode(poll.poeMode);
    setAccessVlan(poll.accessVlan);
    setPoe24v(poe24vSupported);
    setErr(null);
    setSaving(false);
    setLoadNote(null);
    setKnownVlans(null);
    backdropPtrDownRef.current = false;

    const knownFn = loadKnownVlansRef.current;
    if (knownFn) {
      knownFn()
        .then((ids) => {
          if (cancelled) return;
          const s = new Set<number>([1, ...ids.filter((n) => n >= 1 && n <= 4094)]);
          setKnownVlans(s);
        })
        .catch(() => {
          if (!cancelled) setKnownVlans(null);
        });
    }

    const loadFn = loadLiveRef.current;
    if (!loadFn) {
      setLoading(false);
      return;
    }

    setLoading(true);
    (async () => {
      try {
        const live = await loadFn();
        if (cancelled) return;
        const next: Baseline = {
          enable: live.admin_up,
          isolate: live.isolate === true,
          dhcpTrusted: live.dhcp_trusted === true,
          flowControl: live.flow_control === true,
          stpEnabled: live.stp_enabled !== false,
          edgePort: normEdge(live.edge_port),
          portPriority: typeof live.port_priority === "number" ? live.port_priority : 128,
          pathCost: typeof live.path_cost === "number" ? live.path_cost : 0,
          poeMode: live.poe_mode === "off" || live.poe_mode === "24v" || live.poe_mode === "poe+" ? live.poe_mode : "poe+",
          accessVlan: live.access_vlan && live.access_vlan > 0 ? live.access_vlan : poll.accessVlan,
        };
        if (live.poe_24v === false && next.poeMode === "24v") next.poeMode = "poe+";
        setBaseline(next);
        setEnable(next.enable);
        setIsolate(next.isolate);
        setDhcpTrusted(next.dhcpTrusted);
        setFlowControl(next.flowControl);
        setStpEnabled(next.stpEnabled);
        setEdgePort(next.edgePort);
        setPortPriority(next.portPriority);
        setPathCost(next.pathCost);
        setPoeMode(next.poeMode);
        setAccessVlan(next.accessVlan);
        if (typeof live.poe_24v === "boolean") setPoe24v(live.poe_24v);
        if (live.live) {
          const viaNote =
            live.via === "config_cache" || live.via === "snapshot"
              ? "из конфига при открытии карточки"
              : live.via
                ? `(${live.via})`
                : "";
          setLoadNote(viaNote ? `Настройки ${viaNote}` : "Считано с устройства");
        } else if (live.live_err) {
          setLoadNote(`Live недоступен: ${live.live_err}. Показан опрос SNMP.`);
        } else {
          setLoadNote("Показаны данные последнего опроса");
        }
      } catch (ex) {
        if (cancelled) return;
        setLoadNote(ex instanceof Error ? ex.message : "Не удалось прочитать настройки");
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, [open, port?.if_index, port?.admin_status, poe24vSupported]);

  useEffect(() => {
    if (!open || loading || !settingsWritable) return;
    const t = window.setTimeout(() => {
      const active = document.activeElement;
      if (active instanceof HTMLElement && active.closest('[role="dialog"][aria-labelledby="port-settings-title"]')) {
        return;
      }
      vlanInputRef.current?.focus();
    }, 0);
    return () => window.clearTimeout(t);
  }, [open, loading, settingsWritable, port?.if_index]);

  if (!open || !port) return null;

  const w = settingsWritable;
  const enableDirty = enable !== baseline.enable;
  const isolateDirty = w && isolate !== baseline.isolate;
  const dhcpTrustedDirty = w && dhcpTrusted !== baseline.dhcpTrusted;
  const flowControlDirty = w && flowControl !== baseline.flowControl;
  const poeDirty = w && poeMode !== baseline.poeMode;
  const vlanDirty = w && accessVlan > 0 && accessVlan !== baseline.accessVlan;
  const vlanNotInDb =
    vlanDirty && knownVlans != null && accessVlan > 0 && accessVlan !== 1 && !knownVlans.has(accessVlan);
  const stpDirty =
    w &&
    (stpEnabled !== baseline.stpEnabled ||
      edgePort !== baseline.edgePort ||
      portPriority !== baseline.portPriority ||
      pathCost !== baseline.pathCost);
  const dirty = enableDirty || isolateDirty || dhcpTrustedDirty || flowControlDirty || poeDirty || stpDirty || vlanDirty;
  const canSubmit = dirty && !vlanNotInDb;
  const portTitle = (port.if_name?.trim() || `ifIndex ${port.if_index}`) + (port.label ? ` — ${port.label}` : "");
  const controlsDisabled = !canWrite || saving || loading;

  async function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!canWrite || !canSubmit || saving || loading) return;
    if (vlanNotInDb) {
      setErr(`VLAN ${accessVlan} нет в vlan database — сначала создайте на вкладке VLAN`);
      return;
    }
    setSaving(true);
    setErr(null);
    try {
      await onSave({
        adminUp: enable,
        poeMode,
        isolate,
        dhcpTrusted,
        flowControl,
        stpEnabled,
        edgePort,
        portPriority,
        pathCost,
        enableDirty,
        poeDirty,
        isolateDirty,
        dhcpTrustedDirty,
        flowControlDirty,
        stpDirty,
        vlanDirty,
        accessVlan,
      });
      onClose();
    } catch (ex) {
      setErr(ex instanceof Error ? ex.message : "Ошибка сохранения");
    } finally {
      setSaving(false);
    }
  }

  // Portal в body: иначе модалка внутри .device-detail-body (overflow-y:auto) при фокусе/вводе
  // в VLAN уезжает вместе со скроллом / клипается overflow:hidden — выглядит как «закрылась».
  return createPortal(
    <div
      role="dialog"
      aria-modal="true"
      aria-labelledby="port-settings-title"
      style={{
        position: "fixed",
        inset: 0,
        zIndex: 200,
        background: "rgba(0,0,0,0.55)",
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        padding: 16,
      }}
      onPointerDown={(e) => {
        backdropPtrDownRef.current = e.target === e.currentTarget;
      }}
      onClick={(e) => {
        // Не закрывать, если pointerdown был на форме (выделение текста с mouseup на фоне).
        if (backdropPtrDownRef.current && e.target === e.currentTarget) onClose();
        backdropPtrDownRef.current = false;
      }}
      onKeyDown={(e) => {
        if (e.key === "Escape") {
          e.stopPropagation();
          onClose();
        }
      }}
    >
      <form
        onSubmit={onSubmit}
        style={{
          width: "min(440px, 100%)",
          maxHeight: "92vh",
          overflowY: "auto",
          background: "#1a1f2b",
          border: "1px solid #2e3648",
          borderRadius: 10,
          padding: "1.1rem 1.25rem 1rem",
          boxShadow: "0 12px 40px rgba(0,0,0,0.45)",
          color: "#e8ecf4",
          boxSizing: "border-box",
        }}
        onClick={(e) => e.stopPropagation()}
        onPointerDown={(e) => e.stopPropagation()}
      >
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, marginBottom: "1rem" }}>
          <h2 id="port-settings-title" style={{ margin: 0, fontSize: "1.15rem", fontWeight: 650 }}>
            Расширенные настройки
          </h2>
          <button type="button" onClick={onClose} aria-label="Закрыть" style={closeBtn}>
            ×
          </button>
        </div>
        <p style={{ margin: "0 0 0.35rem", fontSize: "0.85rem", color: "#9aa3b5" }}>{portTitle}</p>
        {loading ? (
          <p style={{ margin: "0 0 0.85rem", fontSize: "0.8rem", color: "#7ab0ff" }}>Читаем настройки с устройства…</p>
        ) : loadNote ? (
          <p style={{ margin: "0 0 0.85rem", fontSize: "0.75rem", color: "#7a8499" }}>{loadNote}</p>
        ) : (
          <div style={{ marginBottom: "0.85rem" }} />
        )}

        <label style={{ ...rowStyle, opacity: loading ? 0.55 : 1 }}>
          <input type="checkbox" checked={enable} disabled={controlsDisabled} onChange={(e) => setEnable(e.target.checked)} />
          <span>
            Включить порт <strong>{port.if_name?.trim() || port.if_index}</strong>
          </span>
        </label>

        <label style={{ ...fieldLabel, opacity: w && !loading ? 1 : 0.55, margin: "0.35rem 0 0.85rem" }}>
          Access VLAN / PVID
          <input
            ref={vlanInputRef}
            type="number"
            min={1}
            max={4094}
            value={accessVlan || ""}
            disabled={controlsDisabled || !w}
            onChange={(e) => setAccessVlan(Number(e.target.value) || 0)}
            onKeyDown={(e) => {
              // Не отдавать Enter форме (случайный submit) — только кнопка «Сохранить».
              if (e.key === "Enter") e.preventDefault();
            }}
            style={{
              ...inputStyle,
              borderColor: vlanNotInDb ? "#c45c5c" : "#2e3648",
            }}
          />
          {vlanNotInDb ? (
            <span style={{ fontSize: "0.75rem", color: "#f88", fontWeight: 500 }} role="alert">
              VLAN {accessVlan} нет в vlan database — сначала создайте на вкладке VLAN
            </span>
          ) : (
            <span style={{ fontSize: "0.75rem", color: "#7a8499", fontWeight: 400 }}>
              switchport access vlan, иначе vlan pvid. Tagged — на вкладке VLAN.
            </span>
          )}
        </label>

        <FeatureCheck
          label="Изолировать порт"
          checked={isolate}
          disabled={controlsDisabled || !w}
          writable={w}
          loading={loading}
          onChange={setIsolate}
          hint="Не видит другие изолированные порты; до uplink и шлюза — может."
        />
        <FeatureCheck
          label="DHCP Snooping (Trusted)"
          checked={dhcpTrusted}
          disabled={controlsDisabled || !w}
          writable={w}
          loading={loading}
          onChange={setDhcpTrusted}
          hint="Trusted — только туда, откуда должны приходить ответы DHCP. Всё остальное — untrusted. Trust включает глобальный ip dhcp snooping; если trust нигде нет — глобальный снимается."
        />
        <FeatureCheck
          label="Flow Control"
          checked={flowControl}
          disabled={controlsDisabled || !w}
          writable={w}
          loading={loading}
          onChange={setFlowControl}
          hint="PAUSE-кадры при переполнении буфера. На uplink часто лучше выкл."
        />

        <label style={{ ...rowStyle, opacity: w && !loading ? 1 : 0.55, marginBottom: "0.25rem" }}>
          <input
            type="checkbox"
            checked={stpEnabled}
            disabled={controlsDisabled || !w}
            onChange={(e) => setStpEnabled(e.target.checked)}
          />
          <span>
            Spanning Tree Protocol
            {!w && <em style={soonStyle}> (недоступно)</em>}
          </span>
        </label>
        <div
          style={{
            display: "grid",
            gridTemplateColumns: "minmax(0, 1fr) minmax(0, 1fr) minmax(0, 1fr)",
            gap: 8,
            margin: "0.35rem 0 0.85rem 0",
            paddingLeft: "1.6rem",
            maxWidth: "100%",
            boxSizing: "border-box",
            opacity: w && stpEnabled && !loading ? 1 : 0.45,
            pointerEvents: w && stpEnabled && !loading && !controlsDisabled ? "auto" : "none",
          }}
        >
          <label style={fieldLabel}>
            Edge Port
            <select
              value={edgePort}
              disabled={controlsDisabled || !w || !stpEnabled}
              onChange={(e) => setEdgePort(e.target.value as EdgePortValue)}
              style={inputStyle}
            >
              <option value="auto">Auto</option>
              <option value="enable">Enable</option>
              <option value="disable">Disable</option>
            </select>
          </label>
          <label style={fieldLabel}>
            Port Priority
            <input
              type="number"
              min={0}
              max={240}
              step={16}
              value={portPriority}
              disabled={controlsDisabled || !w || !stpEnabled}
              onChange={(e) => setPortPriority(Number(e.target.value) || 0)}
              style={inputStyle}
            />
          </label>
          <label style={fieldLabel}>
            Path Cost
            <input
              type="number"
              min={0}
              value={pathCost}
              disabled={controlsDisabled || !w || !stpEnabled}
              onChange={(e) => setPathCost(Number(e.target.value) || 0)}
              style={inputStyle}
              title="0 = auto"
            />
          </label>
        </div>

        <div style={{ marginBottom: "1rem", opacity: w && !loading ? 1 : 0.55 }}>
          <div style={{ fontSize: "0.8rem", color: "#9aa3b5", marginBottom: 4 }}>PoE Mode</div>
          <select
            value={poeMode === "24v" && !poe24v ? "poe+" : poeMode}
            disabled={controlsDisabled || !w}
            onChange={(e) => setPoeMode(e.target.value as PoEModeValue)}
            style={{ ...inputStyle, width: "100%" }}
          >
            <option value="off">Off</option>
            {poe24v && <option value="24v">24 V</option>}
            <option value="poe+">PoE+</option>
          </select>
          <div style={{ fontSize: "0.75rem", color: "#7a8499", marginTop: 4 }}>
            {poe24v
              ? "EdgeSwitch: Off / 24V / PoE+. Eltex/SNR: Off / PoE+ (без 24V)."
              : "Off / PoE+ (24V только на EdgeSwitch Fastpath)"}
          </div>
        </div>

        {err && (
          <p style={{ color: "#f88", fontSize: "0.85rem", margin: "0 0 0.75rem" }} role="alert">
            {err}
          </p>
        )}

        <div style={{ display: "flex", justifyContent: "flex-end", gap: 8 }}>
          <button type="button" onClick={onClose} style={btnSecondary}>
            Отмена
          </button>
          <button
            type="submit"
            disabled={!canWrite || !canSubmit || saving || loading}
            style={{
              ...btnPrimary,
              opacity: !canWrite || !canSubmit || saving || loading ? 0.45 : 1,
              cursor: !canWrite || !canSubmit || saving || loading ? "not-allowed" : "pointer",
            }}
          >
            {saving ? "Сохранение…" : "Сохранить"}
          </button>
        </div>
      </form>
    </div>,
    document.body,
  );
}

function FeatureCheck({
  label,
  checked,
  disabled,
  writable,
  loading,
  onChange,
  hint,
}: {
  label: string;
  checked: boolean;
  disabled: boolean;
  writable: boolean;
  loading: boolean;
  onChange: (v: boolean) => void;
  hint: string;
}) {
  return (
    <>
      <label style={{ ...rowStyle, opacity: writable && !loading ? 1 : 0.55, marginBottom: "0.25rem" }}>
        <input type="checkbox" checked={checked} disabled={disabled} onChange={(e) => onChange(e.target.checked)} />
        <span>
          {label}
          {!writable && <em style={soonStyle}> (недоступно)</em>}
        </span>
      </label>
      {writable && <p style={{ margin: "0 0 0.55rem 1.6rem", fontSize: "0.75rem", color: "#7a8499", lineHeight: 1.35 }}>{hint}</p>}
    </>
  );
}

const rowStyle: CSSProperties = {
  display: "flex",
  alignItems: "center",
  gap: 10,
  marginBottom: "0.55rem",
  fontSize: "0.92rem",
  cursor: "default",
};

const soonStyle: CSSProperties = { fontStyle: "normal", fontSize: "0.78rem", color: "#7a8499" };

const fieldLabel: CSSProperties = {
  display: "flex",
  flexDirection: "column",
  gap: 4,
  fontSize: "0.75rem",
  color: "#9aa3b5",
  minWidth: 0,
};

const inputStyle: CSSProperties = {
  background: "#12161f",
  border: "1px solid #2e3648",
  borderRadius: 6,
  color: "#c8d0e0",
  padding: "6px 8px",
  fontSize: "0.85rem",
  width: "100%",
  maxWidth: "100%",
  boxSizing: "border-box",
};

const closeBtn: CSSProperties = {
  background: "transparent",
  border: "none",
  color: "#9aa3b5",
  fontSize: "1.25rem",
  cursor: "pointer",
  lineHeight: 1,
  padding: 4,
};

const btnPrimary: CSSProperties = {
  background: "#3b82f6",
  border: "none",
  borderRadius: 6,
  color: "#fff",
  padding: "8px 16px",
  fontWeight: 600,
  fontSize: "0.9rem",
};

const btnSecondary: CSSProperties = {
  background: "transparent",
  border: "1px solid #3a4558",
  borderRadius: 6,
  color: "#c8d0e0",
  padding: "8px 14px",
  fontSize: "0.9rem",
  cursor: "pointer",
};
