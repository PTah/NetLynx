import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { apiDelete, apiGet, apiPatch, apiPost } from "../api";
import { formatTopologyStaleAt } from "../dateFormat";
import { deviceLinkState } from "../navigation";
import { CARD_H, computeEdgeBends, edgeLabelPos, edgePath, edgeRenderable, estimateCardWidth, layoutTopology, type LayoutMode, type Pos } from "../topologyLayout";
import {
  buildExpandedLocationView,
  buildLocationGraph,
  devicesUnderLocation,
  layoutExpandedLocationView,
  pickLocationRoot,
  type ExpandedLayout,
  type LocationNode,
} from "../topologyLocations";
import { findShortestPath } from "../topologyPath";
import { TOPOLOGY_REFRESH_EVENT, consumeTopologyRefreshPending } from "../topologyRefresh";
import type { ManualTopologyLink, PortSearchHit, TopologyEdge, TopologyGraph, TopologyNode } from "../topologyTypes";
import { isDeviceOnline } from "../deviceOnline";
import {
  type CategoryFilterState,
  type DeviceCategory,
  isInfraDeviceCategory,
  normalizeDeviceCategory,
  readTopologyCategoryFilter,
  writeTopologyCategoryFilter,
} from "../deviceCategories";
import { useDeviceCategories } from "../hooks/useDeviceCategories";
import type { DeviceCategoryDef } from "../deviceCategories";
import { PromoteDiscoveredForm, type PromotePreview } from "../components/PromoteDiscoveredForm";
import { DeviceCategoryIcon } from "../components/DeviceCategoryIcon";
import { copyTextToClipboard, formatMacDisplay, looksLikeMac, selectElementText } from "../macUtil";
import {
  topologyCardFill,
  topologyCardStroke,
  topologyDotBlink,
  topologyDotColor,
  topologyLayoutKind,
  topologyMarkerIsSquare,
  TOPOLOGY_DOT,
} from "../topologyDots";

function suggestPromoteCategory(n?: TopologyNode | null): DeviceCategory {
  const k = (n?.kind ?? "").trim().toLowerCase();
  if (/^[a-z][a-z0-9_]{0,31}$/.test(k)) return normalizeDeviceCategory(k);
  return "other";
}

/** Ребро между карточками разного размера (устройство ↔ peer-локация). */
function mixedEdgePath(from: Pos, fromW: number, fromH: number, to: Pos, toW: number, toH: number, bend = 0): string {
  const fromCy = from.y + fromH / 2;
  const toCy = to.y + toH / 2;
  const toRight = to.x + toW / 2 >= from.x + fromW / 2;
  const x1 = toRight ? from.x + fromW : from.x;
  const y1 = fromCy;
  const x2 = toRight ? to.x : to.x + toW;
  const y2 = toCy;
  if (Math.abs(x2 - x1) <= 4) {
    return edgePath(from, to, fromW, fromH, "lr", bend);
  }
  const sx = toRight ? 1 : -1;
  const dx = Math.max(40, Math.abs(x2 - x1) * 0.45);
  const len = Math.hypot(x2 - x1, y2 - y1) || 1;
  const ox = (-(y2 - y1) / len) * bend;
  const oy = ((x2 - x1) / len) * bend;
  return `M ${x1} ${y1} C ${x1 + sx * dx + ox} ${y1 + oy}, ${x2 - sx * dx + ox} ${y2 + oy}, ${x2} ${y2}`;
}

function canDrawEdge(a?: Pos | null, b?: Pos | null, cardW?: number, cardH?: number, mode: LayoutMode = "lr"): a is Pos {
  return !!b && finitePos(a) && finitePos(b) && edgeRenderable(a, b, cardW, cardH, mode);
}

function finitePos(p?: Pos | null): p is Pos {
  return !!p && Number.isFinite(p.x) && Number.isFinite(p.y);
}

type IfaceOpt = { if_index: number; if_name?: string | null };
type ViewMode = "devices" | "locations";

const REFRESH_MS = 20_000;
const LOC_CARD_H = 64;
const dark = { border: "1px solid #242a38", borderRadius: 8, background: "#12151c" };
type View = { x: number; y: number; scale: number };

function edgeKey(e: TopologyEdge): string {
  return `${e.local_device_id}-${e.local_if_index}-${(e.protocols?.length ? e.protocols : [e.protocol]).join("+")}-${e.remote_device_id ?? 0}`;
}
function label(n?: TopologyNode | null): string { return n ? (n.name || n.sys_name || n.host || `#${n.id}`).trim() || "?" : "?"; }
function isInfra(n: TopologyNode): boolean {
  const k = topologyLayoutKind(n);
  return k === "switch" || k === "router";
}
function offline(n: TopologyNode): boolean {
  // Как дашборд/Узлы: для ПК/иных достаточно ping; свитч/роутер — SNMP.
  return !n.virtual && !isDeviceOnline(n);
}
/** Прятать как offline: только свитчи/роутеры. ПК/серверы часто без SNMP — иначе пропадают с карты. */
function hideAsOffline(n: TopologyNode): boolean {
  return offline(n) && isInfra(n);
}

/** MAC virtual-узла для копирования (заголовок или chassis/port из связи). */
function virtualNodeMac(n: TopologyNode, edges: TopologyEdge[]): string | null {
  const candidates = [
    label(n),
    ...label(n).split("·").map((s) => s.trim()),
    ...edges.flatMap((e) => [e.remote_chassis_id, e.remote_port_id].filter(Boolean) as string[]),
  ];
  for (const c of candidates) {
    if (looksLikeMac(c)) return formatMacDisplay(c);
  }
  return null;
}

function StatusDot(props: {
  cx: number;
  cy: number;
  r: number;
  node: TopologyNode;
  selected?: boolean;
  offline?: boolean;
  categories?: DeviceCategoryDef[];
}) {
  const fill = topologyDotColor(props.node, {
    selected: props.selected,
    offline: props.offline,
    categories: props.categories,
  });
  const blink = topologyDotBlink(props.node, props.selected, props.categories);
  const stroke = props.selected ? "#555" : "none";
  const square = topologyMarkerIsSquare(props.node);
  const anim = blink ? <animate attributeName="opacity" values="1;0.22;1" dur="1.15s" repeatCount="indefinite" /> : null;
  if (square) {
    const side = props.r * 2;
    return (
      <rect
        x={props.cx - props.r}
        y={props.cy - props.r}
        width={side}
        height={side}
        rx={1.5}
        fill={fill}
        stroke={stroke}
        strokeWidth={props.selected ? 1 : 0}
      >
        {anim}
      </rect>
    );
  }
  return (
    <circle cx={props.cx} cy={props.cy} r={props.r} fill={fill} stroke={stroke} strokeWidth={props.selected ? 1 : 0}>
      {anim}
    </circle>
  );
}
function speed(v?: number | null): string { return !v || v <= 0 ? "" : v >= 1000 ? `${v / 1000}G` : `${v}M`; }
function protocols(e: TopologyEdge): string { return (e.protocols?.length ? e.protocols : [e.protocol]).map((x) => x.toUpperCase()).join("+"); }
function edgeText(e: TopologyEdge): string {
  return `${e.local_if_name || `if${e.local_if_index}`} → ${e.remote_port_id || "?"} · ${protocols(e)}${speed(e.local_if_speed) ? ` · ${speed(e.local_if_speed)}` : ""}`;
}

/** Подсказка линка: оба устройства + порты (ориентация от viewFromId, если задан). */
function edgeHoverText(
  e: TopologyEdge,
  localLabel: string,
  remoteLabel: string,
  viewFromId?: number | null,
  extraLine?: string | null,
): string {
  const flip = viewFromId != null && e.remote_device_id === viewFromId;
  const aName = flip ? remoteLabel : localLabel;
  const bName = flip ? localLabel : remoteLabel;
  const aPort = flip
    ? e.remote_if_name || e.remote_port_id || "?"
    : e.local_if_name || `if${e.local_if_index}`;
  const bPort = flip
    ? e.local_if_name || `if${e.local_if_index}`
    : e.remote_if_name || e.remote_port_id || "?";
  let detail = `${aPort} → ${bPort} · ${protocols(e)}`;
  const sp = speed(e.local_if_speed);
  if (sp) detail += ` · ${sp}`;
  if (e.vlan_id != null) detail += ` · VLAN ${e.vlan_id}`;
  if (e.poe_active) detail += e.poe_power_w != null ? ` · PoE ${e.poe_power_w}W` : " · PoE";
  if (e.stale) detail += ` · ${formatTopologyStaleAt(e.last_seen_at)}`;
  const lines = [`${aName} ↔ ${bName}`, detail];
  if (extraLine?.trim()) lines.push(extraLine.trim());
  if (e.manual_note?.trim()) lines.push(`Заметка: ${e.manual_note.trim()}`);
  return lines.join("\n");
}

function matchesNode(n: TopologyNode, q: string): boolean { return [n.name, n.host, n.sys_name, n.location, n.sys_descr].filter(Boolean).join(" ").toLowerCase().includes(q); }
function matchesEdge(e: TopologyEdge, q: string): boolean { return [e.local_if_name, e.remote_sys_name, e.remote_port_id, e.remote_if_name, e.remote_chassis_id, e.remote_mgmt_addr, protocols(e)].filter(Boolean).join(" ").toLowerCase().includes(q); }
function looksLikePortSearch(q: string): boolean { return q.includes(":") || /^\d{1,3}(\.\d{1,3}){3}$/.test(q); }
function validMode(v: string | null): LayoutMode { return v === "tb" || v === "radial" ? v : "lr"; }
function validView(v: string | null): ViewMode { return v === "locations" ? "locations" : "devices"; }
function id(v: string | null): number | null {
  if (v == null || v === "") return null;
  const n = Number(v);
  // virtual-узлы имеют отрицательные id
  return Number.isFinite(n) && n !== 0 ? n : null;
}

const TOPOLOGY_ROOT_KEY = "invetor.topology.rootId";
/** Id локаций на карте ≥ 1e9 — их в серверный root_device_id не пишем. */
const LOC_ID_MIN = 1_000_000_000;

function readStoredRoot(): number | null {
  try {
    return id(localStorage.getItem(TOPOLOGY_ROOT_KEY));
  } catch {
    return null;
  }
}

function writeStoredRoot(v: number | null) {
  try {
    if (v == null) localStorage.removeItem(TOPOLOGY_ROOT_KEY);
    else localStorage.setItem(TOPOLOGY_ROOT_KEY, String(v));
  } catch {
    /* ignore */
  }
}

function isDeviceRootId(v: number | null | undefined): v is number {
  return v != null && v > 0 && v < LOC_ID_MIN;
}

export default function Topology() {
  const { categories } = useDeviceCategories();
  const nav = useNavigate();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState(params.get("q") || "");
  const [focus, setFocus] = useState<number | null>(() => id(params.get("focus")));
  const [layoutMode, setLayoutMode] = useState<LayoutMode>(() => validMode(params.get("layout")));
  const [viewMode, setViewMode] = useState<ViewMode>(() => validView(params.get("view")));
  const [expandedPath, setExpandedPath] = useState<string | null>(() => {
    const v = params.get("expand");
    return v && v.trim() ? v : null;
  });
  const [preferredRootId, setPreferredRootId] = useState<number | null>(() => id(params.get("root")) ?? readStoredRoot());
  const [protocol, setProtocol] = useState(params.get("proto") || "");
  const [includeStale, setIncludeStale] = useState(params.get("stale") === "1");
  const [vlan, setVlan] = useState(params.get("vlan") || "");
  const [location, setLocation] = useState(params.get("location") || "");
  const [depth, setDepth] = useState(params.get("depth") || "");
  const [deviceId, setDeviceId] = useState(params.get("device_id") || "");
  const [pathA, setPathA] = useState<number | null>(() => id(params.get("pathA")));
  const [pathB, setPathB] = useState<number | null>(() => id(params.get("pathB")));
  const [showLabels, setShowLabels] = useState(params.get("labels") === "1");
  const [showSwitches, setShowSwitches] = useState(true);
  const [categoryFilter, setCategoryFilter] = useState<CategoryFilterState>(() => readTopologyCategoryFilter());
  const [typeMenuOpen, setTypeMenuOpen] = useState(false);
  const typeMenuRef = useRef<HTMLDivElement>(null);
  const [showOffline, setShowOffline] = useState(false);
  const [graph, setGraph] = useState<TopologyGraph | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [hits, setHits] = useState<PortSearchHit[]>([]);
  const [hitIndex, setHitIndex] = useState(0);
  const [hoverEdge, setHoverEdge] = useState<string | null>(null);
  const [hoverTip, setHoverTip] = useState<{ text: string; x: number; y: number } | null>(null);
  const [view, setView] = useState<View>({ x: 0, y: 0, scale: 1 });
  const emptyPromote = {
    open: false,
    host: "",
    name: "",
    location: "",
    category: "other" as DeviceCategory,
    community: "public",
    preview: null as PromotePreview | null,
    busy: false,
  };
  const [promote, setPromote] = useState(emptyPromote);
  const [linkMode, setLinkMode] = useState(false);
  const [linkA, setLinkA] = useState<{ id: number; ifIndex: number } | null>(null);
  const [linkB, setLinkB] = useState<{ id: number; ifIndex: number } | null>(null);
  const [linkIfaces, setLinkIfaces] = useState<Record<number, IfaceOpt[]>>({});
  const [linkBusy, setLinkBusy] = useState(false);
  const [linkMsg, setLinkMsg] = useState<string | null>(null);
  const [supersededLinks, setSupersededLinks] = useState<ManualTopologyLink[]>([]);
  const canvasRef = useRef<HTMLDivElement>(null);
  const svgRef = useRef<SVGSVGElement>(null);
  const dragRef = useRef<{ cx: number; cy: number; x: number; y: number; moved: boolean } | null>(null);
  const viewRef = useRef(view);
  viewRef.current = view;
  const serverQuery = useMemo(() => ({ q: query.trim(), protocol, includeStale, vlan, location, depth, deviceId }), [query, protocol, includeStale, vlan, location, depth, deviceId]);

  const nonInfraCategories = useMemo(
    () => categories.filter((c) => !isInfraDeviceCategory(c.id)),
    [categories],
  );

  useEffect(() => {
    setCategoryFilter((prev) => {
      const next = readTopologyCategoryFilter(categories);
      for (const c of categories) {
        if (typeof prev[c.id] === "boolean") next[c.id] = prev[c.id]!;
      }
      return next;
    });
  }, [categories]);

  useEffect(() => {
    if (!typeMenuOpen) return;
    const onDocClick = (e: MouseEvent) => {
      if (typeMenuRef.current && !typeMenuRef.current.contains(e.target as Node)) setTypeMenuOpen(false);
    };
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [typeMenuOpen]);

  // Zoom колесом без Ctrl (passive:false — иначе preventDefault не сработает).
  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;
    const onWheel = (e: WheelEvent) => {
      e.preventDefault();
      e.stopPropagation();
      const r = el.getBoundingClientRect();
      const mx = e.clientX - r.left;
      const my = e.clientY - r.top;
      const f = e.deltaY > 0 ? 0.9 : 1.1;
      const v = viewRef.current;
      const scale = Math.max(0.15, Math.min(2.5, v.scale * f));
      setView({
        scale,
        x: mx - ((mx - v.x) / v.scale) * scale,
        y: my - ((my - v.y) / v.scale) * scale,
      });
    };
    el.addEventListener("wheel", onWheel, { passive: false });
    return () => el.removeEventListener("wheel", onWheel);
  }, []);

  useEffect(() => {
    const next = new URLSearchParams();
    const put = (key: string, value: string | number | null, defaultValue = "") => { if (value != null && String(value) !== defaultValue) next.set(key, String(value)); };
    put("q", query.trim()); put("focus", focus); put("layout", layoutMode, "lr"); put("root", preferredRootId);
    put("view", viewMode, "devices");
    put("expand", viewMode === "locations" ? expandedPath : null);
    put("proto", protocol); put("stale", includeStale ? "1" : "0", "0"); put("vlan", vlan); put("location", location);
    put("depth", depth); put("device_id", deviceId); put("pathA", pathA); put("pathB", pathB); put("labels", showLabels ? "1" : "0", "0");
    if (next.toString() !== params.toString()) setParams(next, { replace: true });
  }, [query, focus, layoutMode, preferredRootId, viewMode, expandedPath, protocol, includeStale, vlan, location, depth, deviceId, pathA, pathB, showLabels, params, setParams]);

  const loadSeq = useRef(0);
  const load = useCallback(async (quiet = false, signal?: AbortSignal) => {
    const seq = ++loadSeq.current;
    if (!quiet) { setLoading(true); setError(null); }
    const qs = new URLSearchParams();
    if (serverQuery.q) qs.set("q", serverQuery.q);
    if (serverQuery.protocol) qs.set("protocol", serverQuery.protocol);
    qs.set("include_stale", serverQuery.includeStale ? "1" : "0");
    if (serverQuery.deviceId) qs.set("device_id", serverQuery.deviceId);
    if (serverQuery.depth) qs.set("depth", serverQuery.depth);
    if (serverQuery.vlan) qs.set("vlan_id", serverQuery.vlan);
    if (serverQuery.location) qs.set("location", serverQuery.location);
    try {
      const g = await apiGet<TopologyGraph>(`/api/v1/topology?${qs}`, signal ? { signal } : undefined);
      if (seq !== loadSeq.current) return;
      setGraph(g);
      try {
        const recent = await apiGet<ManualTopologyLink[]>(
          "/api/v1/manual-links?status=superseded&limit=5",
          signal ? { signal } : undefined,
        );
        if (seq === loadSeq.current) setSupersededLinks(Array.isArray(recent) ? recent : []);
      } catch {
        /* баннер необязателен */
      }
    } catch (e) {
      if ((e as Error).name !== "AbortError" && !quiet) setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (!quiet && seq === loadSeq.current) setLoading(false);
    }
  }, [serverQuery]);
  const loadRef = useRef(load);
  loadRef.current = load;

  useEffect(() => {
    const ac = new AbortController();
    void load(false, ac.signal);
    return () => ac.abort();
  }, [load]);

  useEffect(() => {
    if (consumeTopologyRefreshPending()) {
      void loadRef.current(false);
    }
    const onRefresh = () => void loadRef.current(false);
    window.addEventListener(TOPOLOGY_REFRESH_EVENT, onRefresh);
    return () => window.removeEventListener(TOPOLOGY_REFRESH_EVENT, onRefresh);
  }, []);

  // Стабильный интервал: не сбрасывать на каждое изменение load/serverQuery.
  useEffect(() => {
    let refreshAC: AbortController | null = null;
    const t = window.setInterval(() => {
      refreshAC?.abort();
      refreshAC = new AbortController();
      void loadRef.current(true, refreshAC.signal);
    }, REFRESH_MS);
    return () => {
      window.clearInterval(t);
      refreshAC?.abort();
    };
  }, []);
  useEffect(() => {
    const q = query.trim();
    if (!looksLikePortSearch(q)) { setHits([]); return; }
    const ac = new AbortController();
    const t = window.setTimeout(() => apiGet<{ hits?: PortSearchHit[] } | PortSearchHit[]>(`/api/v1/ports/search?q=${encodeURIComponent(q)}&limit=40`, { signal: ac.signal })
      .then((v) => {
        setHits(Array.isArray(v) ? v : Array.isArray(v.hits) ? v.hits : []);
        setHitIndex(0);
      }).catch((e: Error) => { if (e.name !== "AbortError") setHits([]); }), 220);
    return () => { ac.abort(); window.clearTimeout(t); };
  }, [query]);

  const isLocView = viewMode === "locations";
  const anyNonInfraVisible = useMemo(
    () => nonInfraCategories.some((c) => categoryFilter[c.id] !== false),
    [nonInfraCategories, categoryFilter],
  );
  const selectedTypeCount = useMemo(
    () => nonInfraCategories.filter((c) => categoryFilter[c.id] !== false).length,
    [nonInfraCategories, categoryFilter],
  );
  const typeMenuLabel =
    selectedTypeCount === nonInfraCategories.length
      ? "Тип устройства"
      : `Тип устройства (${selectedTypeCount}/${nonInfraCategories.length})`;
  const toggleCategoryFilter = (id: DeviceCategory) => {
    setCategoryFilter((prev) => {
      const next = { ...prev, [id]: !(prev[id] !== false) };
      writeTopologyCategoryFilter(next);
      return next;
    });
  };
  const isOtherVisible = useCallback(
    (n: TopologyNode) => {
      if (n.virtual) return anyNonInfraVisible;
      const cat = normalizeDeviceCategory(n.kind);
      return categoryFilter[cat] !== false;
    },
    [anyNonInfraVisible, categoryFilter],
  );
  const nodes = useMemo(
    () =>
      (graph?.nodes ?? []).filter(
        (n) => (isInfra(n) ? showSwitches : isOtherVisible(n)) && (showOffline || !hideAsOffline(n)),
      ),
    [graph, showSwitches, isOtherVisible, showOffline],
  );
  const nodeById = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);
  const visibleIds = useMemo(() => new Set(nodes.map((n) => n.id)), [nodes]);
  const edges = useMemo(() => (graph?.edges ?? []).filter((e) => (includeStale || !e.stale) && e.remote_device_id != null && visibleIds.has(e.local_device_id) && visibleIds.has(e.remote_device_id)), [graph, includeStale, visibleIds]);

  const locSourceNodes = useMemo(
    () => (graph?.nodes ?? []).filter((n) => !n.virtual && n.id > 0 && (showOffline || !hideAsOffline(n))),
    [graph, showOffline],
  );
  const locSourceEdges = useMemo(
    () =>
      (graph?.edges ?? []).filter(
        (e) =>
          (includeStale || !e.stale) &&
          e.remote_device_id != null &&
          locSourceNodes.some((n) => n.id === e.local_device_id) &&
          locSourceNodes.some((n) => n.id === e.remote_device_id),
      ),
    [graph, includeStale, locSourceNodes],
  );
  const locGraph = useMemo(() => buildLocationGraph(locSourceNodes, locSourceEdges), [locSourceNodes, locSourceEdges]);
  const locById = useMemo(() => new Map(locGraph.locNodes.map((n) => [n.id, n])), [locGraph]);
  const existingLocations = useMemo(() => {
    const set = new Set<string>();
    for (const n of graph?.nodes ?? []) {
      const loc = n.location?.trim();
      if (loc) set.add(loc);
    }
    return [...set].sort((a, b) => a.localeCompare(b, "ru", { sensitivity: "base" }));
  }, [graph]);
  const locRootId = useMemo(
    () => pickLocationRoot(locGraph.locNodes, locGraph.treeEdges, locGraph.linkEdges),
    [locGraph],
  );
  const expandedView = useMemo(() => {
    if (!isLocView || !expandedPath) return null;
    return buildExpandedLocationView(locGraph, locSourceEdges, expandedPath);
  }, [isLocView, expandedPath, locGraph, locSourceEdges]);
  const isExpanded = !!expandedView;
  const expandedDeviceById = useMemo(
    () => new Map(expandedView?.devices.map((d) => [d.id, d]) ?? []),
    [expandedView],
  );
  const expandedPeerById = useMemo(
    () => new Map(expandedView?.peers.map((p) => [p.id, p]) ?? []),
    [expandedView],
  );

  const search = query.trim().toLowerCase();
  const showEdgeTip = (key: string, text: string, ev: { clientX: number; clientY: number }) => {
    setHoverEdge(key);
    setHoverTip({ text, x: ev.clientX, y: ev.clientY });
  };
  const moveEdgeTip = (ev: { clientX: number; clientY: number }) => {
    setHoverTip((t) => (t ? { ...t, x: ev.clientX, y: ev.clientY } : null));
  };
  const hideEdgeTip = () => {
    setHoverEdge(null);
    setHoverTip(null);
  };
  useEffect(() => {
      setHoverEdge(null);
      setHoverTip(null);
  }, [graph, layoutMode, viewMode, expandedPath, search]);
  const hitIds = useMemo(() => new Set(hits.map((h) => h.device_id)), [hits]);
  const matchIds = useMemo(() => {
    if (isLocView && isExpanded && expandedView) {
      if (!search) return [];
      const ids: number[] = [];
      for (const d of expandedView.devices) {
        if (matchesNode(d, search) || hitIds.has(d.id)) ids.push(d.id);
      }
      for (const p of expandedView.peers) {
        if (p.path.toLowerCase().includes(search) || p.label.toLowerCase().includes(search)) ids.push(p.id);
      }
      for (const ex of expandedView.externalEdges) {
        if (matchesEdge(ex.edge, search)) {
          ids.push(ex.device_id, ex.peer_id);
        }
      }
      return ids.filter((v, i, a) => a.indexOf(v) === i);
    }
    if (isLocView) {
      if (!search) return [];
      return locGraph.locNodes
        .filter(
          (n) =>
            n.path.toLowerCase().includes(search) ||
            n.label.toLowerCase().includes(search) ||
            devicesUnderLocation(locGraph, n.path).some((d) => matchesNode(d, search) || hitIds.has(d.id)),
        )
        .map((n) => n.id);
    }
    return nodes
      .filter((n) => search && (matchesNode(n, search) || hitIds.has(n.id)))
      .map((n) => n.id)
      .concat(edges.filter((e) => search && matchesEdge(e, search)).flatMap((e) => [e.local_device_id, e.remote_device_id!]))
      .filter((v, i, a) => a.indexOf(v) === i);
  }, [isLocView, isExpanded, expandedView, locGraph, nodes, edges, search, hitIds]);
  const matchSet = useMemo(() => new Set(matchIds), [matchIds]);

  const cardH = isExpanded ? CARD_H : isLocView ? LOC_CARD_H : CARD_H;
  const cardW = useMemo(() => {
    if (isLocView && !isExpanded) {
      return Math.max(
        200,
        ...locGraph.locNodes.map((n) => estimateCardWidth(n.label, `${n.device_count} устройств`)),
      );
    }
    if (isExpanded) return 188;
    return Math.max(188, ...nodes.map((n) => estimateCardWidth(label(n), n.virtual ? "не в списке Узлы" : n.host || "—")));
  }, [isLocView, isExpanded, locGraph.locNodes, nodes]);

  const layout = useMemo(() => {
    if (isLocView && expandedView) {
      const el = layoutExpandedLocationView(expandedView, layoutMode);
      return {
        pos: el.pos,
        width: el.width,
        height: el.height,
        cardW: el.cardW,
        mode: el.mode,
        expanded: el as ExpandedLayout,
      };
    }
    if (isLocView) {
      const layoutNodes = locGraph.locNodes.map((n) => ({
        id: n.id,
        name: n.label,
        host: `${n.device_count} устройств`,
        kind: "switch",
        link_count: n.device_count,
        last_snmp_ok: n.offline ? false : true,
      }));
      const layoutEdges = [
        ...locGraph.treeEdges.map((e) => ({ local_device_id: e.parent_id, remote_device_id: e.child_id })),
        ...locGraph.linkEdges.map((e) => ({ local_device_id: e.a_id, remote_device_id: e.b_id })),
      ];
      return {
        ...layoutTopology(layoutMode, layoutNodes, layoutEdges, {
          cardW,
          cardH: LOC_CARD_H,
          preferredRootId: preferredRootId != null && locById.has(preferredRootId) ? preferredRootId : locRootId,
        }),
        expanded: null as ExpandedLayout | null,
      };
    }
    return {
      ...layoutTopology(
        layoutMode,
        nodes.map((n) => ({ ...n, name: label(n), kind: topologyLayoutKind(n) })),
        edges.map((e) => ({ local_device_id: e.local_device_id, remote_device_id: e.remote_device_id! })),
        { cardW, cardH: CARD_H, preferredRootId },
      ),
      expanded: null as ExpandedLayout | null,
    };
  }, [isLocView, expandedView, locGraph, locById, locRootId, layoutMode, nodes, edges, cardW, preferredRootId]);

  const path = useMemo(
    () => (!isLocView && pathA && pathB ? findShortestPath(edges, pathA, pathB) : null),
    [isLocView, edges, pathA, pathB],
  );
  const pathNodes = useMemo(() => new Set(path ? [pathA!, ...path.map((h) => h.toId)] : []), [path, pathA]);
  const pathEdges = useMemo(() => new Set(path?.map((h) => `${h.fromId}:${h.toId}`) ?? []), [path]);
  const selected =
    focus == null
      ? null
      : !isLocView
        ? nodeById.get(focus) ?? null
        : isExpanded
          ? expandedDeviceById.get(focus) ?? null
          : null;
  const selectedLoc: LocationNode | null = isLocView
    ? isExpanded && expandedView
      ? locGraph.locNodes.find((n) => n.path === expandedView.path) ?? null
      : focus != null
        ? locById.get(focus) ?? null
        : null
    : null;
  const selectedPeer = isExpanded && focus != null ? expandedPeerById.get(focus) ?? null : null;
  const selectedLocDevices = useMemo(
    () => (selectedLoc ? devicesUnderLocation(locGraph, selectedLoc.path) : []),
    [selectedLoc, locGraph],
  );
  const selectedEdges = useMemo(() => {
    if (isExpanded && expandedView && focus != null && expandedDeviceById.has(focus)) {
      return [
        ...expandedView.internalEdges.filter((e) => e.local_device_id === focus || e.remote_device_id === focus),
        ...expandedView.externalEdges.filter((ex) => ex.device_id === focus).map((ex) => ex.edge),
      ];
    }
    if (!isLocView && focus) return edges.filter((e) => e.local_device_id === focus || e.remote_device_id === focus);
    return [];
  }, [isLocView, isExpanded, expandedView, expandedDeviceById, edges, focus]);
  const selectedLocLinks = useMemo(() => {
    if (!selectedLoc || isExpanded) return [];
    return locGraph.linkEdges.filter((e) => e.a_id === selectedLoc.id || e.b_id === selectedLoc.id);
  }, [selectedLoc, locGraph, isExpanded]);
  const edgeLabelOffsets = useMemo(() => {
    const placed: { x: number; y: number }[] = [];
    const offsets = new Map<string, number>();
    const list = isExpanded && expandedView
      ? [
          ...expandedView.internalEdges.map((e) => ({
            key: `in:${expandedView.path}:${edgeKey(e)}`,
            a: e.local_device_id,
            b: e.remote_device_id!,
          })),
          ...expandedView.externalEdges.map((ex) => ({
            key: `ex:${expandedView.path}:${edgeKey(ex.edge)}:${ex.peer_id}`,
            a: ex.device_id,
            b: ex.peer_id,
          })),
        ]
      : isLocView
        ? locGraph.linkEdges.map((e) => ({ key: `L${e.a_id}-${e.b_id}`, a: e.a_id, b: e.b_id }))
        : edges.map((e) => ({ key: edgeKey(e), a: e.local_device_id, b: e.remote_device_id! }));
    list.forEach((e) => {
      const a = layout.pos.get(e.a);
      const b = layout.pos.get(e.b);
      if (!a || !b) return;
      const x = (a.x + b.x + layout.cardW) / 2;
      const y = (a.y + b.y + cardH) / 2;
      const overlaps = placed.some((p) => Math.abs(p.x - x) <= 12 && Math.abs(p.y - y) <= 12);
      offsets.set(e.key, overlaps ? 10 : 0);
      placed.push({ x, y });
    });
    return offsets;
  }, [isLocView, isExpanded, expandedView, edges, locGraph.linkEdges, layout, cardH]);
  const edgeBends = useMemo(() => {
    const list = isExpanded && expandedView
      ? [
          ...expandedView.internalEdges.map((e) => ({
            key: `in:${expandedView.path}:${edgeKey(e)}`,
            a: e.local_device_id,
            b: e.remote_device_id!,
          })),
          ...expandedView.externalEdges.map((ex) => ({
            key: `ex:${expandedView.path}:${edgeKey(ex.edge)}:${ex.peer_id}`,
            a: ex.device_id,
            b: ex.peer_id,
          })),
        ]
      : isLocView
        ? [
            ...locGraph.treeEdges.map((e) => ({ key: `tree-${e.parent_id}-${e.child_id}`, a: e.parent_id, b: e.child_id })),
            ...locGraph.linkEdges.map((e) => ({ key: `L${e.a_id}-${e.b_id}`, a: e.a_id, b: e.b_id })),
          ]
        : edges.map((e) => ({ key: edgeKey(e), a: e.local_device_id, b: e.remote_device_id! }));
    const el = layout.expanded;
    const w = el ? el.cardW : layout.cardW;
    const h = el ? el.deviceCardH : cardH;
    return computeEdgeBends(list, layout.pos, w, h);
  }, [isLocView, isExpanded, expandedView, edges, locGraph.treeEdges, locGraph.linkEdges, layout, cardH]);

  const center = useCallback((nodeId: number) => {
    const p = layout.pos.get(nodeId); const box = canvasRef.current?.getBoundingClientRect(); if (!p || !box) return;
    const el = layout.expanded;
    const w = el && expandedPeerById.has(nodeId) ? el.peerCardW : layout.cardW;
    const h = el && expandedPeerById.has(nodeId) ? el.peerCardH : (el ? el.deviceCardH : cardH);
    setView((v) => ({ ...v, x: box.width / 2 - (p.x + w / 2) * v.scale, y: box.height / 2 - (p.y + h / 2) * v.scale }));
  }, [layout, cardH, expandedPeerById]);
  const select = useCallback((nodeId: number, doCenter = true) => {
    setPromote(emptyPromote);
    setFocus(nodeId);
    if (doCenter) center(nodeId);
    if (isLocView || !linkMode) return;
    const n = graph?.nodes.find((x) => x.id === nodeId);
    if (!n || n.virtual || nodeId < 0) {
      setLinkMsg("Ручную связь можно создать только между узлами из списка Узлы");
      return;
    }
    if (!linkA) {
      setLinkA({ id: nodeId, ifIndex: 0 });
      setLinkMsg(`Выбран узел A: ${label(n)}. Укажите порт ниже.`);
    } else if (!linkB && nodeId !== linkA.id) {
      setLinkB({ id: nodeId, ifIndex: 0 });
      setLinkMsg(`Выбран узел B: ${label(n)}. Укажите порт ниже.`);
    } else if (nodeId === linkA.id) {
      setLinkMsg("Выберите другой узел для конца B");
    }
  }, [center, linkMode, linkA, graph, isLocView]);

  const openLocation = useCallback((path: string, locId?: number) => {
    setExpandedPath(path);
    if (locId != null) setFocus(locId);
    else {
      const loc = locGraph.locNodes.find((n) => n.path === path);
      setFocus(loc?.id ?? null);
    }
  }, [locGraph.locNodes]);

  const closeExpanded = useCallback(() => {
    const path = expandedPath;
    setExpandedPath(null);
    if (path) {
      const loc = locGraph.locNodes.find((n) => n.path === path);
      if (loc) setFocus(loc.id);
    }
  }, [expandedPath, locGraph.locNodes]);

  const switchToDevicesForLocation = (path: string) => {
    setViewMode("devices");
    setExpandedPath(null);
    setLocation(path === "Без расположения" ? "" : path);
    setFocus(null);
    setLinkMode(false);
  };

  // После смены вида / раскрытия / layout — вписать холст.
  useEffect(() => {
    if (!isLocView || locGraph.locNodes.length === 0) return;
    const t = window.setTimeout(() => {
      const r = canvasRef.current?.getBoundingClientRect();
      if (!r || layout.width <= 0 || layout.height <= 0 || !Number.isFinite(layout.width) || !Number.isFinite(layout.height)) return;
      const s = Math.max(0.15, Math.min(1, (r.width - 60) / layout.width, (r.height - 60) / layout.height));
      setView({ scale: s, x: (r.width - layout.width * s) / 2, y: (r.height - layout.height * s) / 2 });
    }, 0);
    return () => window.clearTimeout(t);
  }, [isLocView, isExpanded, expandedPath, locGraph.locNodes.length, layout.width, layout.height, layoutMode]);

  // Сброс hover при смене раскрытой локации — иначе «залипают» подсветки/старые ключи рёбер.
  useEffect(() => {
      setHoverEdge(null);
      setHoverTip(null);
  }, [expandedPath, viewMode]);

  // Корень дерева устройств — с сервера (переживает перезапуск/другой браузер); localStorage — кэш.
  useEffect(() => {
    let cancelled = false;
    void (async () => {
      try {
        const s = await apiGet<{ root_device_id?: number | null }>("/api/v1/settings/topology");
        if (cancelled) return;
        const serverRoot = isDeviceRootId(s.root_device_id ?? null) ? Number(s.root_device_id) : null;
        if (serverRoot != null) {
          setPreferredRootId(serverRoot);
          writeStoredRoot(serverRoot);
          return;
        }
        const local = readStoredRoot();
        if (isDeviceRootId(local)) {
          setPreferredRootId(local);
          try {
            await apiPatch("/api/v1/settings/topology", { root_device_id: local });
          } catch {
            /* viewer без права PATCH — оставляем локальный кэш */
          }
        }
      } catch {
        /* офлайн / нет прав — остаётся URL/localStorage */
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const persistDeviceRoot = useCallback(async (next: number | null) => {
    writeStoredRoot(next);
    if (next != null && !isDeviceRootId(next)) return;
    try {
      if (next == null) {
        await apiPatch("/api/v1/settings/topology", { clear_root: true });
      } else {
        await apiPatch("/api/v1/settings/topology", { root_device_id: next });
      }
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  const ensureIfaces = useCallback(async (deviceId: number) => {
    if (linkIfaces[deviceId]) return;
    try {
      const rows = await apiGet<IfaceOpt[]>(`/api/v1/devices/${deviceId}/interfaces`);
      const list = Array.isArray(rows) ? rows.filter((r) => r.if_index > 0) : [];
      setLinkIfaces((prev) => ({ ...prev, [deviceId]: list }));
    } catch {
      setLinkIfaces((prev) => ({ ...prev, [deviceId]: [] }));
    }
  }, [linkIfaces]);

  useEffect(() => {
    if (linkA?.id) void ensureIfaces(linkA.id);
    if (linkB?.id) void ensureIfaces(linkB.id);
  }, [linkA?.id, linkB?.id, ensureIfaces]);

  const createManualLink = async () => {
    if (!linkA || !linkB || linkA.ifIndex <= 0 || linkB.ifIndex <= 0) {
      setLinkMsg("Выберите оба узла и порты");
      return;
    }
    setLinkBusy(true);
    setLinkMsg(null);
    try {
      await apiPost("/api/v1/manual-links", {
        a_device_id: linkA.id,
        a_if_index: linkA.ifIndex,
        b_device_id: linkB.id,
        b_if_index: linkB.ifIndex,
      });
      setLinkMsg("Ручная связь создана");
      setLinkA(null);
      setLinkB(null);
      setLinkMode(false);
      await load(false);
    } catch (e) {
      setLinkMsg(e instanceof Error ? e.message : String(e));
    } finally {
      setLinkBusy(false);
    }
  };

  const deleteManualLink = async (id: number) => {
    if (!window.confirm(`Удалить ручную связь #${id}?`)) return;
    try {
      await apiDelete(`/api/v1/manual-links/${id}`);
      setLinkMsg(`Связь #${id} удалена`);
      await load(false);
    } catch (e) {
      setLinkMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const restoreManualLink = async (id: number) => {
    try {
      await apiPatch(`/api/v1/manual-links/${id}`, { status: "active" });
      setLinkMsg(`Связь #${id} восстановлена как active`);
      await load(false);
    } catch (e) {
      setLinkMsg(e instanceof Error ? e.message : String(e));
    }
  };

  const editManualNote = async (id: number, current?: string | null) => {
    const next = window.prompt(`Заметка для ручной связи #${id}`, current ?? "");
    if (next === null) return;
    try {
      await apiPatch(`/api/v1/manual-links/${id}`, { note: next });
      setLinkMsg(`Заметка #${id} обновлена`);
      await load(false);
    } catch (e) {
      setLinkMsg(e instanceof Error ? e.message : String(e));
    }
  };
  const fit = () => { const r = canvasRef.current?.getBoundingClientRect(); if (!r) return; const s = Math.max(.15, Math.min(1, (r.width - 60) / layout.width, (r.height - 60) / layout.height)); setView({ scale: s, x: (r.width - layout.width * s) / 2, y: (r.height - layout.height * s) / 2 }); };
  const panToWorld = (wx: number, wy: number) => {
    const box = canvasRef.current?.getBoundingClientRect();
    if (!box) return;
    setView((v) => ({
      ...v,
      x: box.width / 2 - wx * v.scale,
      y: box.height / 2 - wy * v.scale,
    }));
  };
  const onMinimapClick = (e: React.MouseEvent<SVGSVGElement>) => {
    e.stopPropagation();
    e.preventDefault();
    const svg = e.currentTarget;
    const rect = svg.getBoundingClientRect();
    if (rect.width <= 0 || rect.height <= 0) return;
    const wx = ((e.clientX - rect.left) / rect.width) * layout.width;
    const wy = ((e.clientY - rect.top) / rect.height) * layout.height;
    panToWorld(wx, wy);
  };
  const neighbor = (nodeId: number, key: string) => {
    const p = layout.pos.get(nodeId); if (!p) return;
    const candidates = isExpanded && expandedView
      ? [
          ...expandedView.internalEdges.flatMap((e) =>
            e.local_device_id === nodeId ? [e.remote_device_id!] : e.remote_device_id === nodeId ? [e.local_device_id] : [],
          ),
          ...expandedView.externalEdges.flatMap((ex) =>
            ex.device_id === nodeId ? [ex.peer_id] : ex.peer_id === nodeId ? [ex.device_id] : [],
          ),
        ].filter((v, i, a) => a.indexOf(v) === i)
      : isLocView
      ? [
          ...locGraph.treeEdges.flatMap((e) => (e.parent_id === nodeId ? [e.child_id] : e.child_id === nodeId ? [e.parent_id] : [])),
          ...locGraph.linkEdges.flatMap((e) => (e.a_id === nodeId ? [e.b_id] : e.b_id === nodeId ? [e.a_id] : [])),
        ].filter((v, i, a) => a.indexOf(v) === i)
      : edges.flatMap((e) => e.local_device_id === nodeId ? [e.remote_device_id!] : e.remote_device_id === nodeId ? [e.local_device_id] : []).filter((v, i, a) => a.indexOf(v) === i);
    const direction = { ArrowLeft: [-1, 0], ArrowRight: [1, 0], ArrowUp: [0, -1], ArrowDown: [0, 1] }[key] as number[] | undefined;
    if (!direction) return;
    const best = candidates.map((v) => ({ v, p: layout.pos.get(v) })).filter((x): x is { v: number; p: Pos } => !!x.p).map((x) => ({ ...x, dot: (x.p.x - p.x) * direction[0] + (x.p.y - p.y) * direction[1], d: Math.hypot(x.p.x - p.x, x.p.y - p.y) })).filter((x) => x.dot > 0).sort((a, b) => b.dot / b.d - a.dot / a.d)[0];
    if (best) select(best.v);
  };
  const download = (name: string, blob: Blob) => { const u = URL.createObjectURL(blob); const a = document.createElement("a"); a.href = u; a.download = name; a.click(); URL.revokeObjectURL(u); };
  const exportSvg = () => { const svg = svgRef.current; if (!svg) return; const clone = svg.cloneNode(true) as SVGSVGElement; clone.setAttribute("width", String(layout.width)); clone.setAttribute("height", String(layout.height)); clone.setAttribute("viewBox", `0 0 ${layout.width} ${layout.height}`); const g = clone.querySelector("[data-scene]"); if (g) g.setAttribute("transform", ""); download("topology.svg", new Blob([new XMLSerializer().serializeToString(clone)], { type: "image/svg+xml" })); };
  const exportPng = () => { const svg = svgRef.current; if (!svg) return; const clone = svg.cloneNode(true) as SVGSVGElement; clone.setAttribute("width", String(layout.width)); clone.setAttribute("height", String(layout.height)); clone.setAttribute("viewBox", `0 0 ${layout.width} ${layout.height}`); clone.querySelector("[data-scene]")?.setAttribute("transform", ""); const u = URL.createObjectURL(new Blob([new XMLSerializer().serializeToString(clone)], { type: "image/svg+xml" })); const img = new Image(); img.onload = () => { const c = document.createElement("canvas"); c.width = layout.width; c.height = layout.height; c.getContext("2d")?.drawImage(img, 0, 0); c.toBlob((b) => { if (b) download("topology.png", b); URL.revokeObjectURL(u); }, "image/png"); }; img.src = u; };
  const openPromote = () => {
    if (promote.open) {
      setPromote(emptyPromote);
      return;
    }
    const edgeHint = selectedEdges.find((e) => e.remote_mgmt_addr || e.remote_sys_name);
    const mgmt = (edgeHint?.remote_mgmt_addr || "").trim();
    const hint = (mgmt || selected?.host || "").trim();
    const suggested = label(selected);
    const neighborLoc =
      selectedEdges
        .map((e) => {
          const otherId = e.local_device_id === selected?.id ? e.remote_device_id : e.local_device_id;
          return otherId != null ? nodeById.get(otherId)?.location?.trim() : "";
        })
        .find((v) => !!v) || "";
    const locHint = (expandedPath || neighborLoc || "").trim();
    setPromote({
      open: true,
      host: hint,
      // MAC как имя — плохо; оставляем пустым, чтобы задать нормальное имя
      name: looksLikeMac(suggested) ? "" : suggested,
      location: locHint,
      category: suggestPromoteCategory(selected),
      community: "public",
      preview: null,
      busy: false,
    });
  };
  const preview = async () => {
    if (!selected?.discovered_id) return;
    if (!promote.host.trim()) {
      setPromote((v) => ({ ...v, preview: { ok: false, error: "Укажите IP для проверки SNMP (с сервера NetLynx)" } }));
      return;
    }
    setPromote((v) => ({ ...v, busy: true, preview: null }));
    try {
      const r = await apiPost<PromotePreview>(`/api/v1/discovered/${selected.discovered_id}/preview`, {
        host: promote.host,
        name: promote.name,
        snmp_version: "v2c",
        community: promote.community,
      });
      setPromote((v) => ({ ...v, preview: r }));
    } catch (e) {
      setPromote((v) => ({ ...v, preview: { ok: false, error: e instanceof Error ? e.message : String(e) } }));
    } finally {
      setPromote((v) => ({ ...v, busy: false }));
    }
  };
  const promoteNode = async (e: FormEvent) => {
    e.preventDefault();
    if (!selected?.discovered_id) return;
    setPromote((v) => ({ ...v, busy: true }));
    try {
      const r = await apiPost<{ id: number }>(`/api/v1/discovered/${selected.discovered_id}/promote`, {
        host: promote.host,
        name: promote.name,
        location: promote.location,
        device_category: promote.category,
        snmp_version: "v2c",
        community: promote.community,
      });
      setPromote(emptyPromote);
      setFocus(r.id);
      await load(false);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setPromote((v) => ({ ...v, busy: false }));
    }
  };
  const miniScale = Math.min(128 / layout.width, 96 / layout.height);
  const miniView = { x: Math.max(0, -view.x / view.scale) * miniScale, y: Math.max(0, -view.y / view.scale) * miniScale, w: (canvasRef.current?.clientWidth ?? 320) / view.scale * miniScale, h: (canvasRef.current?.clientHeight ?? 240) / view.scale * miniScale };
  const statusCount = isExpanded && expandedView
    ? expandedView.devices.length
    : isLocView
      ? locGraph.locNodes.length
      : nodes.length;
  const statusLinks = isExpanded && expandedView
    ? expandedView.internalEdges.length + expandedView.externalEdges.length
    : isLocView
      ? locGraph.treeEdges.length + locGraph.linkEdges.length
      : edges.length;

  return (
    <div className="page topology-page">
      <div className="topology-page-toolbar" style={{ display: "flex", gap: 10, alignItems: "baseline", flexWrap: "wrap" }}>
        <h1 style={{ margin: 0 }}>Топология</h1>
        <div style={{ display: "inline-flex", border: "1px solid #40506a", borderRadius: 8, overflow: "hidden" }}>
          <button
            type="button"
            onClick={() => { setViewMode("devices"); setExpandedPath(null); setFocus(null); }}
            style={{ border: 0, borderRadius: 0, background: !isLocView ? "#1e3a5f" : "transparent", fontWeight: !isLocView ? 700 : 400 }}
          >
            Устройства
          </button>
          <button
            type="button"
            onClick={() => { setViewMode("locations"); setExpandedPath(null); setFocus(null); setLinkMode(false); setPathA(null); setPathB(null); }}
            style={{ border: 0, borderRadius: 0, background: isLocView ? "#1e3a5f" : "transparent", fontWeight: isLocView ? 700 : 400 }}
          >
            Расположения
          </button>
        </div>
        {isExpanded && expandedView && (
          <>
            <button type="button" onClick={closeExpanded}>← К карте локаций</button>
            <span style={{ color: "#c5cedd", fontWeight: 600 }}>{expandedView.label}</span>
          </>
        )}
        <button type="button" onClick={() => void load(false)} disabled={loading}>Обновить</button>
        <span style={{ color: "#9aa3b5" }}>
          {statusCount} {isExpanded ? "устройств" : isLocView ? "локаций" : "узлов"}
          {isExpanded && expandedView ? ` · ${expandedView.peers.length} соседних локаций` : ""}
          {" · "}{statusLinks} связей · авто 20с
        </span>
        {!isLocView && (
          <span style={{ color: "#7a8499", fontSize: "0.78rem", display: "inline-flex", flexWrap: "wrap", gap: "0.55rem", alignItems: "center" }} title="Цвет точки слева на карточке; иконка — тип устройства">
            <span><span style={{ color: TOPOLOGY_DOT.selected }}>●</span> выбран</span>
            <span><span style={{ color: TOPOLOGY_DOT.offline }}>●</span> offline</span>
            <span><span style={{ color: TOPOLOGY_DOT.virtual }}>●</span> не в Узлах</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="server" height={14} /><span style={{ color: TOPOLOGY_DOT.server }}>●</span> сервер</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="computer" height={14} /><span style={{ color: TOPOLOGY_DOT.computer }}>■</span> ПК</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="phone" height={14} /><span style={{ color: TOPOLOGY_DOT.phone }}>●</span> телефон</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="mfu" height={14} /><span style={{ color: TOPOLOGY_DOT.mfu }}>●</span> МФУ</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="camera" height={14} /><span style={{ color: TOPOLOGY_DOT.camera }}>●</span> камера</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="ap" height={14} /><span style={{ color: TOPOLOGY_DOT.ap }}>●</span> точка доступа</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="other" height={14} /><span style={{ color: TOPOLOGY_DOT.other }}>●</span> иное</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="switch" height={14} /><span style={{ color: TOPOLOGY_DOT.switch }}>●</span> свитч</span>
            <span style={{ display: "inline-flex", alignItems: "center", gap: 4 }}><DeviceCategoryIcon category="router" height={14} /><span style={{ color: TOPOLOGY_DOT.router }}>●</span> роутер</span>
          </span>
        )}
        {isLocView && locGraph.locNodes.length === 0 && (
          <span style={{ color: "#f0c14a" }}>Нет данных: у узлов пустое «Расположение» или все скрыты фильтром offline</span>
        )}
        {isLocView && expandedPath && !expandedView && (
          <span style={{ color: "#f0c14a" }}>Локация «{expandedPath}» не найдена — <button type="button" onClick={() => setExpandedPath(null)}>закрыть</button></span>
        )}
        {preferredRootId != null && !isExpanded && (
          <span style={{ color: "#5ec8a0", fontSize: "0.9rem" }}>
            корень: #{preferredRootId}{" "}
            <button
              type="button"
              style={{ fontSize: "0.8rem" }}
              onClick={() => {
                const wasDevice = isDeviceRootId(preferredRootId);
                setPreferredRootId(null);
                if (wasDevice) void persistDeviceRoot(null);
                else writeStoredRoot(null);
              }}
            >
              авто
            </button>
          </span>
        )}
      </div>
      <div className="topology-page-toolbar" style={{ ...dark, padding: 10, display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <input type="search" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="Поиск: имя, IP, порт, MAC…" style={{ minWidth: 260, flex: "1 1 260px" }} />
        <select value={layoutMode} onChange={(e) => setLayoutMode(e.target.value as LayoutMode)}>
          <option value="lr">Слева направо</option>
          <option value="tb">Сверху вниз</option>
          <option value="radial">Радиально</option>
        </select>
        <select value={protocol} onChange={(e) => setProtocol(e.target.value)}>
          <option value="">Все протоколы</option>
          <option value="lldp">LLDP</option>
          <option value="cdp">CDP</option>
          <option value="fdb">FDB</option>
          <option value="manual">manual</option>
        </select>
        <input value={vlan} onChange={(e) => setVlan(e.target.value)} placeholder="VLAN" inputMode="numeric" style={{ width: 64 }} />
        <input value={location} onChange={(e) => setLocation(e.target.value)} placeholder="Локация" style={{ width: 100 }} />
        <input value={depth} onChange={(e) => setDepth(e.target.value)} placeholder="Глубина" inputMode="numeric" style={{ width: 74 }} />
        <input value={deviceId} onChange={(e) => setDeviceId(e.target.value)} placeholder="ID узла" inputMode="numeric" style={{ width: 75 }} />
        <label><input type="checkbox" checked={showSwitches} onChange={(e) => setShowSwitches(e.target.checked)} disabled={isLocView} /> коммутаторы / роутеры</label>
        <div ref={typeMenuRef} style={{ position: "relative", flex: "0 0 auto" }}>
          <button
            type="button"
            aria-expanded={typeMenuOpen}
            disabled={isLocView}
            onClick={() => setTypeMenuOpen((open) => !open)}
            style={{
              background: typeMenuOpen ? "#252b38" : "transparent",
              border: "1px solid #333a4a",
              borderRadius: 6,
              padding: "0.35rem 0.75rem",
              cursor: isLocView ? "not-allowed" : "pointer",
              color: isLocView ? "#6b7280" : "#c5cedd",
              whiteSpace: "nowrap",
              opacity: isLocView ? 0.55 : 1,
            }}
          >
            {typeMenuLabel} {typeMenuOpen ? "▴" : "▾"}
          </button>
          {typeMenuOpen && !isLocView && (
            <div
              style={{
                position: "absolute",
                left: 0,
                top: "calc(100% + 0.35rem)",
                minWidth: 220,
                padding: "0.65rem 0.75rem",
                background: "#1a1f2a",
                border: "1px solid #333a4a",
                borderRadius: 8,
                boxShadow: "0 8px 24px rgba(0,0,0,0.35)",
                zIndex: 20,
              }}
            >
              {nonInfraCategories.map((o) => (
                <label
                  key={o.id}
                  style={{
                    display: "flex",
                    alignItems: "center",
                    gap: 8,
                    padding: "0.25rem 0",
                    cursor: "pointer",
                    fontSize: "0.92rem",
                  }}
                >
                  <input
                    type="checkbox"
                    checked={categoryFilter[o.id] !== false}
                    onChange={() => toggleCategoryFilter(o.id)}
                  />
                  <DeviceCategoryIcon category={o.id} height={18} title={o.label} />
                  {o.label}
                </label>
              ))}
            </div>
          )}
        </div>
        <label><input type="checkbox" checked={showOffline} onChange={(e) => setShowOffline(e.target.checked)} /> offline свитчи</label>
        <label title="Показывать связи, которые давно не подтверждались опросом (пунктир). Выкл — только живые.">
          <input type="checkbox" checked={includeStale} onChange={(e) => setIncludeStale(e.target.checked)} /> устаревшие связи
        </label>
        <label><input type="checkbox" checked={showLabels} onChange={(e) => setShowLabels(e.target.checked)} /> подписи</label>
        <button type="button" onClick={() => setView((v) => ({ ...v, scale: Math.min(2.5, v.scale * 1.15) }))}>+</button>
        <button type="button" onClick={() => setView((v) => ({ ...v, scale: Math.max(0.15, v.scale / 1.15) }))}>−</button>
        <button type="button" onClick={fit}>Fit</button>
        <button type="button" onClick={() => setView({ x: 0, y: 0, scale: 1 })}>1:1</button>
        <button type="button" onClick={() => download("topology.json", new Blob([JSON.stringify(graph, null, 2)], { type: "application/json" }))} disabled={!graph}>JSON</button>
        <button type="button" onClick={exportSvg}>SVG</button>
        <button type="button" onClick={exportPng}>PNG</button>
        {!isLocView && (
        <button
          type="button"
          onClick={() => {
            setLinkMode((v) => !v);
            setLinkA(null);
            setLinkB(null);
            setLinkMsg(null);
          }}
          style={linkMode ? { fontWeight: 700, outline: "1px solid #f0c14a" } : undefined}
        >
          {linkMode ? "Отмена связи" : "Связать"}
        </button>
        )}
        <span style={{ color: "#6b7280", fontSize: "0.8rem" }}>колесо — zoom · drag — pan · клик по миникарте — переход</span>
      </div>
      {!isLocView && (linkMode || linkMsg || supersededLinks.length > 0) && (
        <div className="topology-page-toolbar" style={{ ...dark, padding: 10, marginTop: 8 }}>
          {linkMode && (
            <div style={{ display: "flex", flexWrap: "wrap", gap: 8, alignItems: "center" }}>
              <strong style={{ color: "#f0c14a" }}>Режим «Связать»</strong>
              <span style={{ color: "#9aa3b5" }}>кликните узел A → порт, затем узел B → порт</span>
              {linkA && (
                <label>
                  A #{linkA.id} порт{" "}
                  <select
                    value={linkA.ifIndex || ""}
                    onChange={(e) => setLinkA({ ...linkA, ifIndex: Number(e.target.value) || 0 })}
                  >
                    <option value="">—</option>
                    {(linkIfaces[linkA.id] || []).map((p) => (
                      <option key={p.if_index} value={p.if_index}>
                        {p.if_index}
                        {p.if_name ? ` · ${p.if_name}` : ""}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              {linkB && (
                <label>
                  B #{linkB.id} порт{" "}
                  <select
                    value={linkB.ifIndex || ""}
                    onChange={(e) => setLinkB({ ...linkB, ifIndex: Number(e.target.value) || 0 })}
                  >
                    <option value="">—</option>
                    {(linkIfaces[linkB.id] || []).map((p) => (
                      <option key={p.if_index} value={p.if_index}>
                        {p.if_index}
                        {p.if_name ? ` · ${p.if_name}` : ""}
                      </option>
                    ))}
                  </select>
                </label>
              )}
              <button type="button" disabled={linkBusy} onClick={() => void createManualLink()}>
                Создать связь
              </button>
            </div>
          )}
          {linkMsg && <div style={{ color: "#c5cedd", marginTop: linkMode ? 8 : 0 }}>{linkMsg}</div>}
          {supersededLinks.length > 0 && (
            <div style={{ marginTop: 8 }}>
              <strong style={{ color: "#c9a227" }}>Ранее снятые ручные связи (superseded)</strong>
              {supersededLinks.map((ml) => (
                <div key={ml.id} style={{ color: "#9aa3b5", fontSize: "0.85rem", marginTop: 4 }}>
                  #{ml.id}: {ml.a_device_name || ml.a_device_id}:{ml.a_if_index} ↔ {ml.b_device_name || ml.b_device_id}:
                  {ml.b_if_index}
                  {ml.superseded_by ? ` · ${ml.superseded_by}` : ""}{" "}
                  <button type="button" onClick={() => void restoreManualLink(ml.id)}>
                    Восстановить
                  </button>{" "}
                  <button type="button" onClick={() => void deleteManualLink(ml.id)}>
                    Удалить
                  </button>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
      {hits.length > 0 && (
        <div className="topology-page-toolbar" style={{ ...dark, padding: 8, maxWidth: 720, fontSize: ".85rem" }}>
          <div style={{ color: "#9aa3b5", marginBottom: 6 }}>
            Поиск по MAC/IP: сначала прямой LLDP, затем записи FDB (MAC может светиться на uplink’ах всего VLAN).
          </div>
          <div style={{ display: "flex", gap: 8, alignItems: "center", marginBottom: 6 }}>
            <button type="button" onClick={() => {
              const index = (hitIndex - 1 + hits.length) % hits.length;
              setHitIndex(index);
              select(hits[index].device_id);
            }}>←</button>
            <span>{hitIndex + 1} из {hits.length}</span>
            <button type="button" onClick={() => {
              const index = (hitIndex + 1) % hits.length;
              setHitIndex(index);
              select(hits[index].device_id);
            }}>→</button>
          </div>
          {hits.map((h) => (
            <div key={`${h.device_id}-${h.if_index}-${h.match_type}`} style={{ display: "flex", gap: 8, justifyContent: "space-between", alignItems: "baseline", color: hits[hitIndex] === h ? "#f0c14a" : undefined }}>
              <span>
                {h.device_name || h.device_host} · {h.if_name || `if${h.if_index}`} · {h.mac || h.ip || h.match_type}
                {h.match_type === "lldp" ? " · LLDP" : h.match_type === "mac" ? " · FDB" : ""}
                {h.note ? <span style={{ color: "#9aa3b5" }}> — {h.note}</span> : null}
              </span>
              <button type="button" onClick={() => { setHitIndex(hits.indexOf(h)); select(h.device_id); }}>на топологии</button>
            </div>
          ))}
        </div>
      )}
      {!isLocView && (
      <div className="topology-page-toolbar" style={{ ...dark, padding: 10, display: "flex", gap: 8, flexWrap: "wrap", alignItems: "center" }}>
        <strong>Маршрут:</strong>
        <button type="button" onClick={() => setPathA(focus)} disabled={!focus}>Выбрать A {pathA ? `#${pathA}` : ""}</button>
        <button type="button" onClick={() => setPathB(focus)} disabled={!focus}>Выбрать B {pathB ? `#${pathB}` : ""}</button>
        <button type="button" onClick={() => { setPathA(null); setPathB(null); }}>Сбросить</button>
        {pathA && pathB && <span style={{ color: path ? "#9bd08b" : "#f88" }}>{path ? `${path.length} переходов` : "Путь не найден"}</span>}
      </div>
      )}
      {error && <p style={{ color: "#f88", margin: 0 }}>{error}</p>}
      {loading && !graph && <p style={{ margin: 0 }}>Загрузка…</p>}
      <div className="topology-workspace">
        <div
          ref={canvasRef}
          className="topology-canvas"
          onPointerDown={(e) => {
            if (e.button !== 0) return;
            const t = e.target as Element | null;
            if (t?.closest?.("[data-node], [data-edge], [data-minimap]")) return;
            e.preventDefault(); // не выделять текст узлов при pan
            window.getSelection()?.removeAllRanges();
            dragRef.current = { cx: e.clientX, cy: e.clientY, x: view.x, y: view.y, moved: false };
            e.currentTarget.setPointerCapture(e.pointerId);
          }}
          onPointerMove={(e) => {
            const d = dragRef.current;
            if (!d) return;
            if (Math.abs(e.clientX - d.cx) > 3 || Math.abs(e.clientY - d.cy) > 3) d.moved = true;
            setView((v) => ({ ...v, x: d.x + e.clientX - d.cx, y: d.y + e.clientY - d.cy }));
          }}
          onPointerUp={() => { dragRef.current = null; }}
          style={{ cursor: dragRef.current ? "grabbing" : "grab" }}
        >
          <svg id="topology-svg" ref={svgRef} width="100%" height="100%" style={{ display: "block" }}>
            <rect width="100%" height="100%" fill="#0d0f14" />
            <g data-scene="true" transform={`translate(${view.x},${view.y}) scale(${view.scale})`}>
              {isLocView && !isExpanded &&
                locGraph.treeEdges.map((e) => {
                  const a = layout.pos.get(e.parent_id);
                  const b = layout.pos.get(e.child_id);
                  if (!canDrawEdge(a, b, layout.cardW, cardH, layout.mode) || !b) return null;
                  const ordered = layout.mode === "tb" ? [a, b] : a.x <= b.x ? [a, b] : [b, a];
                  const key = `tree-${e.parent_id}-${e.child_id}`;
                  const d = edgePath(ordered[0], ordered[1], layout.cardW, cardH, layout.mode, edgeBends.get(key) ?? 0);
                  return (
                    <path
                      key={`tree-${e.parent_id}-${e.child_id}`}
                      d={d}
                      fill="none"
                      stroke="#3a4558"
                      strokeWidth={1.6}
                      strokeDasharray="4 5"
                      opacity={0.85}
                    />
                  );
                })}
              {isLocView && !isExpanded &&
                locGraph.linkEdges.map((e) => {
                  const a = layout.pos.get(e.a_id);
                  const b = layout.pos.get(e.b_id);
                  if (!canDrawEdge(a, b, layout.cardW, cardH, layout.mode) || !b) return null;
                  const key = `L${e.a_id}-${e.b_id}`;
                  const edgeSelected = focus === e.a_id || focus === e.b_id;
                  const edgeMatch =
                    !search ||
                    matchSet.has(e.a_id) ||
                    matchSet.has(e.b_id) ||
                    (locById.get(e.a_id)?.path.toLowerCase().includes(search) ?? false) ||
                    (locById.get(e.b_id)?.path.toLowerCase().includes(search) ?? false);
                  const ordered = layout.mode === "tb" ? [a, b] : a.x <= b.x ? [a, b] : [b, a];
                  const bend = edgeBends.get(key) ?? 0;
                  const d = edgePath(ordered[0], ordered[1], layout.cardW, cardH, layout.mode, bend);
                  const mid = edgeLabelPos(a, b, layout.cardW, cardH, bend);
                  const protos = e.protocols.map((p) => p.toUpperCase()).join("+");
                  return (
                    <g
                      key={key}
                      data-edge="1"
                      role="button"
                      tabIndex={0}
                      opacity={edgeMatch ? 1 : 0.12}
                      style={{ cursor: "pointer", outline: "none" }}
                      onPointerDown={(event) => {
                        event.stopPropagation();
                        select(e.a_id, false);
                      }}
                    >
                      <path d={d} fill="none" stroke="transparent" strokeWidth={15} />
                      <path
                        d={d}
                        fill="none"
                        stroke={
                          e.stale
                            ? "#5a6170"
                            : e.has_manual && !e.protocols.includes("lldp") && !e.protocols.includes("cdp")
                              ? "#b388ff"
                              : e.protocols.includes("cdp")
                                ? "#c9a227"
                                : "#3d8bfd"
                        }
                        strokeWidth={hoverEdge === key || edgeSelected ? 3.4 : 2.2}
                        strokeDasharray={e.stale ? "6 4" : e.has_manual && !e.protocols.includes("lldp") ? "2 3" : undefined}
                        opacity={hoverEdge && hoverEdge !== key ? 0.25 : 0.9}
                        onMouseEnter={(ev) =>
                          showEdgeTip(
                            key,
                            `${locById.get(e.a_id)?.label ?? "?"} ↔ ${locById.get(e.b_id)?.label ?? "?"}\n${e.link_count} линк(ов) · ${protos}`,
                            ev,
                          )
                        }
                        onMouseMove={moveEdgeTip}
                        onMouseLeave={hideEdgeTip}
                      />
                      <title>{`${locById.get(e.a_id)?.label} ↔ ${locById.get(e.b_id)?.label}\n${e.link_count} линк(ов) · ${protos}`}</title>
                      {showLabels && (
                        <text x={mid.x} y={mid.y - 4 + (edgeLabelOffsets.get(key) ?? 0)} textAnchor="middle" fill="#c5cedd" fontSize="10" style={{ pointerEvents: "none" }}>
                          {e.link_count}× {protos}
                        </text>
                      )}
                    </g>
                  );
                })}
              {isExpanded && expandedView && layout.expanded && (
                <g key={`expand:${expandedView.path}`} data-expanded-location={expandedView.path}>
                  {(() => {
                const el = layout.expanded!;
                const g = el.group;
                const peerIds = new Set(expandedView.peers.map((p) => p.id));
                return (
                  <>
                    <rect
                      x={g.x}
                      y={g.y}
                      width={g.w}
                      height={g.h}
                      rx={14}
                      fill="#101722"
                      stroke="#4a6fa5"
                      strokeWidth={1.6}
                    />
                    <text x={g.x + 16} y={g.y + 22} fill="#e8eaef" fontSize="14" fontWeight="700">
                      {expandedView.label}
                    </text>
                    <text x={g.x + 16} y={g.y + 40} fill="#9aa3b5" fontSize="11">
                      {expandedView.devices.length} устройств · {expandedView.internalEdges.length} внутр. · {expandedView.externalEdges.length} наруж.
                    </text>
                    {expandedView.internalEdges.map((e) => {
                      if (e.local_device_id === e.remote_device_id) return null;
                      const a = layout.pos.get(e.local_device_id);
                      const b = layout.pos.get(e.remote_device_id!);
                      if (!canDrawEdge(a, b, el.cardW, el.deviceCardH, el.mode) || !b) return null;
                      if (!expandedDeviceById.has(e.local_device_id) || !expandedDeviceById.has(e.remote_device_id!)) return null;
                      const key = `in:${expandedView.path}:${edgeKey(e)}`;
                      const edgeSelected = focus === e.local_device_id || focus === e.remote_device_id;
                      const edgeMatch = !search || matchesEdge(e, search) || matchSet.has(e.local_device_id) || matchSet.has(e.remote_device_id!);
                      const ordered = a.x <= b.x ? [a, b] : [b, a];
                      const bend = edgeBends.get(key) ?? 0;
                      const d = edgePath(ordered[0], ordered[1], el.cardW, el.deviceCardH, el.mode, bend);
                      const mid = edgeLabelPos(a, b, el.cardW, el.deviceCardH, bend);
                      return (
                        <g
                          key={key}
                          data-edge="1"
                          role="button"
                          tabIndex={0}
                          opacity={edgeMatch ? 1 : 0.12}
                          style={{ cursor: "pointer", outline: "none" }}
                          onPointerDown={(event) => {
                            event.stopPropagation();
                            select(e.local_device_id, false);
                          }}
                        >
                          <path d={d} fill="none" stroke="transparent" strokeWidth={15} />
                          <path
                            d={d}
                            fill="none"
                            stroke={
                              e.stale
                                ? "#5a6170"
                                : protocols(e).includes("MANUAL") && !protocols(e).includes("LLDP") && !protocols(e).includes("CDP")
                                  ? "#b388ff"
                                  : protocols(e).includes("CDP")
                                    ? "#c9a227"
                                    : "#3d8bfd"
                            }
                            strokeWidth={hoverEdge === key || edgeSelected ? 3.4 : 2}
                            strokeDasharray={e.stale ? "6 4" : protocols(e).includes("MANUAL") && !protocols(e).includes("LLDP") ? "2 3" : undefined}
                            opacity={hoverEdge && hoverEdge !== key ? 0.25 : 0.9}
                            onMouseEnter={(ev) =>
                              showEdgeTip(
                                key,
                                edgeHoverText(
                                  e,
                                  label(expandedDeviceById.get(e.local_device_id) ?? nodeById.get(e.local_device_id)),
                                  label(expandedDeviceById.get(e.remote_device_id!) ?? nodeById.get(e.remote_device_id!)) ||
                                    e.remote_sys_name ||
                                    "?",
                                ),
                                ev,
                              )
                            }
                            onMouseMove={moveEdgeTip}
                            onMouseLeave={hideEdgeTip}
                          />
                          <title>
                            {edgeHoverText(
                              e,
                              label(expandedDeviceById.get(e.local_device_id) ?? nodeById.get(e.local_device_id)),
                              label(expandedDeviceById.get(e.remote_device_id!) ?? nodeById.get(e.remote_device_id!)) ||
                                e.remote_sys_name ||
                                "?",
                            )}
                          </title>
                          {showLabels && (
                            <text x={mid.x} y={mid.y - 4 + (edgeLabelOffsets.get(key) ?? 0)} textAnchor="middle" fill="#c5cedd" fontSize="10" style={{ pointerEvents: "none" }}>
                              {edgeText(e)}
                            </text>
                          )}
                        </g>
                      );
                    })}
                    {expandedView.externalEdges.map((ex) => {
                      if (!peerIds.has(ex.peer_id) || ex.peer_id === 0) return null;
                      const a = layout.pos.get(ex.device_id);
                      const b = layout.pos.get(ex.peer_id);
                      if (!finitePos(a) || !finitePos(b)) return null;
                      if (!expandedDeviceById.has(ex.device_id)) return null;
                      if (Math.hypot(b.x + el.peerCardW / 2 - (a.x + el.cardW / 2), b.y + el.peerCardH / 2 - (a.y + el.deviceCardH / 2)) < 18) return null;
                      const key = `ex:${expandedView.path}:${edgeKey(ex.edge)}:${ex.peer_id}`;
                      const edgeSelected = focus === ex.device_id || focus === ex.peer_id;
                      const edgeMatch = !search || matchesEdge(ex.edge, search) || matchSet.has(ex.device_id) || matchSet.has(ex.peer_id);
                      const bend = edgeBends.get(key) ?? 0;
                      const d = mixedEdgePath(a, el.cardW, el.deviceCardH, b, el.peerCardW, el.peerCardH, bend);
                      const mid = edgeLabelPos(a, b, el.cardW, el.deviceCardH, bend);
                      const peer = expandedPeerById.get(ex.peer_id);
                      const tipText = edgeHoverText(
                        ex.edge,
                        label(nodeById.get(ex.edge.local_device_id) ?? expandedDeviceById.get(ex.edge.local_device_id)),
                        label(nodeById.get(ex.edge.remote_device_id!) ?? expandedDeviceById.get(ex.edge.remote_device_id!)) ||
                          ex.edge.remote_sys_name ||
                          "?",
                        ex.device_id,
                        peer ? `локация: ${peer.label}` : null,
                      );
                      return (
                        <g
                          key={key}
                          data-edge="1"
                          role="button"
                          tabIndex={0}
                          opacity={edgeMatch ? 1 : 0.12}
                          style={{ cursor: "pointer", outline: "none" }}
                          onPointerDown={(event) => {
                            event.stopPropagation();
                            select(ex.device_id, false);
                          }}
                        >
                          <path d={d} fill="none" stroke="transparent" strokeWidth={15} />
                          <path
                            d={d}
                            fill="none"
                            stroke={
                              ex.edge.stale
                                ? "#5a6170"
                                : protocols(ex.edge).includes("MANUAL") && !protocols(ex.edge).includes("LLDP") && !protocols(ex.edge).includes("CDP")
                                  ? "#b388ff"
                                  : protocols(ex.edge).includes("CDP")
                                    ? "#c9a227"
                                    : "#3d8bfd"
                            }
                            strokeWidth={hoverEdge === key || edgeSelected ? 3.4 : 2.2}
                            strokeDasharray={ex.edge.stale ? "6 4" : protocols(ex.edge).includes("MANUAL") && !protocols(ex.edge).includes("LLDP") ? "2 3" : undefined}
                            opacity={hoverEdge && hoverEdge !== key ? 0.25 : 0.9}
                            onMouseEnter={(ev) => showEdgeTip(key, tipText, ev)}
                            onMouseMove={moveEdgeTip}
                            onMouseLeave={hideEdgeTip}
                          />
                          <title>{tipText}</title>
                          {showLabels && (
                            <text x={mid.x} y={mid.y - 4 + (edgeLabelOffsets.get(key) ?? 0)} textAnchor="middle" fill="#c5cedd" fontSize="10" style={{ pointerEvents: "none" }}>
                              {edgeText(ex.edge)}
                            </text>
                          )}
                        </g>
                      );
                    })}
                    {expandedView.devices.map((n) => {
                      const p = layout.pos.get(n.id);
                      if (!finitePos(p)) return null;
                      const isSelected = focus === n.id;
                      const isOff = offline(n);
                      const dim = !!search && !matchSet.has(n.id);
                      const stroke = topologyCardStroke(n, { selected: isSelected, offline: isOff, categories });
                      return (
                        <g
                          key={`dev:${expandedView.path}:${n.id}`}
                          data-node="1"
                          role="button"
                          tabIndex={0}
                          transform={`translate(${p.x},${p.y})`}
                          opacity={dim ? 0.15 : 1}
                          style={{ cursor: "pointer" }}
                          onPointerDown={(e) => {
                            e.stopPropagation();
                            select(n.id, false);
                          }}
                          onClick={(e) => {
                            e.stopPropagation();
                            select(n.id, false);
                          }}
                          onKeyDown={(e) => {
                            if (e.key.startsWith("Arrow")) {
                              e.preventDefault();
                              neighbor(n.id, e.key);
                            }
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              select(n.id, false);
                            }
                          }}
                        >
                          <rect
                            width={el.cardW}
                            height={el.deviceCardH}
                            rx={8}
                            fill={topologyCardFill(n, isOff)}
                            stroke={stroke}
                            strokeWidth={isSelected ? 2.5 : 1.4}
                          />
                          <StatusDot cx={16} cy={el.deviceCardH / 2} r={6} node={n} selected={isSelected} offline={isOff} categories={categories} />
                          <text x={28} y={22} fill="#e8eaef" fontSize="12" fontWeight="600">{label(n).slice(0, 38)}</text>
                          <text x={28} y={39} fill="#9aa3b5" fontSize="10">{(n.host || "—").slice(0, 44)}</text>
                          <title>{`${label(n)} · ${n.host || "—"}`}</title>
                        </g>
                      );
                    })}
                    {expandedView.peers.map((n) => {
                      const p = layout.pos.get(n.id);
                      if (!finitePos(p)) return null;
                      const isSelected = focus === n.id;
                      const dim = !!search && !matchSet.has(n.id);
                      const stroke = isSelected ? "#ffffff" : n.offline ? "#9aa0a6" : "#6cb6ff";
                      const countLabel =
                        n.device_count === 1 ? "1 устройство" : n.device_count < 5 ? `${n.device_count} устройства` : `${n.device_count} устройств`;
                      return (
                        <g
                          key={`peer:${expandedView.path}:${n.id}`}
                          data-node="1"
                          role="button"
                          tabIndex={0}
                          transform={`translate(${p.x},${p.y})`}
                          opacity={dim ? 0.15 : 1}
                          style={{ cursor: "pointer" }}
                          onPointerDown={(e) => {
                            e.stopPropagation();
                            select(n.id, false);
                          }}
                          onClick={(e) => {
                            e.stopPropagation();
                            openLocation(n.path, n.id);
                          }}
                          onKeyDown={(e) => {
                            if (e.key.startsWith("Arrow")) {
                              e.preventDefault();
                              neighbor(n.id, e.key);
                            }
                            if (e.key === "Enter" || e.key === " ") {
                              e.preventDefault();
                              openLocation(n.path, n.id);
                            }
                          }}
                        >
                          <rect
                            width={el.peerCardW}
                            height={el.peerCardH}
                            rx={10}
                            fill={n.offline ? "#2a2e38" : "#152033"}
                            stroke={stroke}
                            strokeWidth={isSelected ? 2.5 : 1.4}
                          />
                          <circle cx={18} cy={el.peerCardH / 2} r={7} fill={stroke} />
                          <text x={32} y={24} fill="#e8eaef" fontSize="13" fontWeight="600">
                            {n.label.slice(0, 36)}
                          </text>
                          <text x={32} y={44} fill="#9aa3b5" fontSize="11">
                            {countLabel}
                          </text>
                          <title>{`${n.path}\nКлик — раскрыть локацию`}</title>
                        </g>
                      );
                    })}
                  </>
                );
                  })()}
                </g>
              )}
              {!isLocView &&
                edges.map((e) => {
                if (e.local_device_id === e.remote_device_id) return null;
                const a = layout.pos.get(e.local_device_id);
                const b = layout.pos.get(e.remote_device_id!);
                if (!canDrawEdge(a, b, layout.cardW, cardH, layout.mode) || !b) return null;
                const key = edgeKey(e);
                const pathActive = pathEdges.has(`${e.local_device_id}:${e.remote_device_id}`) || pathEdges.has(`${e.remote_device_id}:${e.local_device_id}`);
                const edgeMatch = !search || matchesEdge(e, search) || matchSet.has(e.local_device_id) || matchSet.has(e.remote_device_id!);
                const edgeSelected = focus === e.local_device_id || focus === e.remote_device_id;
                const ordered = layout.mode === "tb" ? [a, b] : a.x <= b.x ? [a, b] : [b, a];
                const bend = edgeBends.get(key) ?? 0;
                const d = edgePath(ordered[0], ordered[1], layout.cardW, cardH, layout.mode, bend);
                const mid = edgeLabelPos(a, b, layout.cardW, cardH, bend);
                const tipText = edgeHoverText(
                  e,
                  label(nodeById.get(e.local_device_id)),
                  label(nodeById.get(e.remote_device_id!)) || e.remote_sys_name || "?",
                );
                return (
                  <g
                    key={key}
                    data-edge="1"
                    role="button"
                    tabIndex={0}
                    aria-label={`${label(nodeById.get(e.local_device_id))}: ${e.local_if_name || `if${e.local_if_index}`} — ${label(nodeById.get(e.remote_device_id!))}: ${e.remote_port_id || e.remote_if_name || "?"}`}
                    aria-current={edgeSelected ? "true" : undefined}
                    opacity={edgeMatch ? 1 : 0.12}
                    style={{ cursor: "pointer", outline: "none" }}
                    onFocus={() => setHoverEdge(key)}
                    onBlur={hideEdgeTip}
                    onMouseEnter={(ev) => showEdgeTip(key, tipText, ev)}
                    onMouseMove={moveEdgeTip}
                    onMouseLeave={hideEdgeTip}
                    onPointerDown={(event) => {
                      event.stopPropagation();
                      select(e.local_device_id, false);
                    }}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        select(e.local_device_id);
                      }
                    }}
                  >
                    <path d={d} fill="none" stroke="transparent" strokeWidth={15} />
                    <path
                      d={d}
                      fill="none"
                      stroke={
                        pathActive
                          ? "#f0c14a"
                          : e.stale
                            ? "#5a6170"
                            : protocols(e).includes("MANUAL") && !protocols(e).includes("LLDP") && !protocols(e).includes("CDP")
                              ? "#b388ff"
                              : protocols(e).includes("CDP")
                                ? "#c9a227"
                                : "#3d8bfd"
                      }
                      strokeWidth={hoverEdge === key || pathActive || edgeSelected ? 3.4 : 2}
                      strokeDasharray={e.stale ? "6 4" : protocols(e).includes("MANUAL") && !protocols(e).includes("LLDP") ? "2 3" : undefined}
                      opacity={hoverEdge && hoverEdge !== key ? 0.25 : 0.9}
                    />
                    <title>{tipText}</title>
                    {showLabels && (
                      <text x={mid.x} y={mid.y - 4 + (edgeLabelOffsets.get(key) ?? 0)} textAnchor="middle" fill="#c5cedd" fontSize="10" style={{ pointerEvents: "none" }}>
                        {edgeText(e)}
                      </text>
                    )}
                  </g>
                );
              })}
              {isLocView && !isExpanded &&
                locGraph.locNodes.map((n) => {
                  const p = layout.pos.get(n.id);
                  if (!p) return null;
                  const isSelected = focus === n.id;
                  const dim = !!search && !matchSet.has(n.id);
                  const stroke = isSelected ? "#ffffff" : n.offline ? "#9aa0a6" : "#4a90e2";
                  const countLabel =
                    n.device_count === 1 ? "1 устройство" : n.device_count < 5 ? `${n.device_count} устройства` : `${n.device_count} устройств`;
                  return (
                    <g
                      key={n.id}
                      data-node="1"
                      role="button"
                      tabIndex={0}
                      transform={`translate(${p.x},${p.y})`}
                      opacity={dim ? 0.15 : 1}
                      style={{ cursor: "pointer" }}
                      onPointerDown={(e) => {
                        e.stopPropagation();
                        openLocation(n.path, n.id);
                      }}
                      onClick={(e) => {
                        e.stopPropagation();
                        openLocation(n.path, n.id);
                      }}
                      onKeyDown={(e) => {
                        if (e.key.startsWith("Arrow")) {
                          e.preventDefault();
                          neighbor(n.id, e.key);
                        }
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          openLocation(n.path, n.id);
                        }
                      }}
                    >
                      <rect
                        width={layout.cardW}
                        height={cardH}
                        rx={10}
                        fill={n.offline ? "#2a2e38" : "#152033"}
                        stroke={stroke}
                        strokeWidth={isSelected ? 2.5 : 1.4}
                      />
                      <circle cx={18} cy={cardH / 2} r={7} fill={stroke} />
                      <text x={32} y={24} fill="#e8eaef" fontSize="13" fontWeight="600">
                        {n.label.slice(0, 36)}
                      </text>
                      <text x={32} y={44} fill="#9aa3b5" fontSize="11">
                        {countLabel}
                      </text>
                      <title>{`${n.path}\n${countLabel}${n.offline ? " · есть offline" : ""}\nКлик — раскрыть`}</title>
                    </g>
                  );
                })}
              {!isLocView &&
                nodes.map((n) => {
                const p = layout.pos.get(n.id);
                if (!p) return null;
                const isSelected = focus === n.id;
                const isPath = pathNodes.has(n.id);
                const isOff = offline(n);
                const dim = !!search && !matchSet.has(n.id);
                const stroke = topologyCardStroke(n, { selected: isSelected, onPath: isPath, offline: isOff, categories });
                const sub = n.virtual ? "не в списке Узлы" : n.host || "—";
                return (
                  <g
                    key={n.id}
                    data-node="1"
                    role="button"
                    tabIndex={0}
                    transform={`translate(${p.x},${p.y})`}
                    opacity={dim ? 0.15 : 1}
                    style={{ cursor: "pointer" }}
                    onPointerDown={(e) => {
                      e.stopPropagation();
                      select(n.id, false);
                    }}
                    onClick={(e) => {
                      e.stopPropagation();
                      select(n.id, false);
                    }}
                    onKeyDown={(e) => {
                      if (e.key.startsWith("Arrow")) {
                        e.preventDefault();
                        neighbor(n.id, e.key);
                      }
                      if (e.key === "Enter" || e.key === " ") {
                        e.preventDefault();
                        select(n.id, false);
                      }
                    }}
                  >
                    <rect
                      width={layout.cardW}
                      height={cardH}
                      rx={8}
                      fill={topologyCardFill(n, isOff)}
                      stroke={stroke}
                      strokeWidth={isSelected || isPath ? 2.5 : 1.4}
                    />
                    <StatusDot cx={16} cy={cardH / 2} r={6} node={n} selected={isSelected} offline={isOff} categories={categories} />
                    <text x={28} y={22} fill="#e8eaef" fontSize="12" fontWeight="600">{label(n).slice(0, 38)}</text>
                    <text x={28} y={39} fill="#9aa3b5" fontSize="10">{sub.slice(0, 44)}</text>
                    <title>{`${label(n)} · ${sub}`}</title>
                  </g>
                );
              })}
            </g>
          </svg>
          <svg
            data-minimap="1"
            width="128"
            height="96"
            viewBox={`0 0 ${Math.max(layout.width, 1)} ${Math.max(layout.height, 1)}`}
            onClick={onMinimapClick}
            onPointerDown={(e) => e.stopPropagation()}
            style={{ position: "absolute", right: 10, bottom: 10, background: "#0d0f14dd", border: "1px solid #40506a", cursor: "crosshair", zIndex: 2 }}
          >
            {isExpanded && expandedView && layout.expanded
              ? (
                <>
                  {expandedView.internalEdges.map((e) => {
                    const a = layout.pos.get(e.local_device_id);
                    const b = layout.pos.get(e.remote_device_id!);
                    return a && b ? (
                      <line
                        key={`mi-${edgeKey(e)}`}
                        x1={a.x + layout.expanded!.cardW / 2}
                        y1={a.y + layout.expanded!.deviceCardH / 2}
                        x2={b.x + layout.expanded!.cardW / 2}
                        y2={b.y + layout.expanded!.deviceCardH / 2}
                        stroke="#516078"
                        strokeWidth="5"
                      />
                    ) : null;
                  })}
                  {expandedView.externalEdges.map((ex) => {
                    const a = layout.pos.get(ex.device_id);
                    const b = layout.pos.get(ex.peer_id);
                    return a && b ? (
                      <line
                        key={`mx-${ex.device_id}-${ex.peer_id}`}
                        x1={a.x + layout.expanded!.cardW / 2}
                        y1={a.y + layout.expanded!.deviceCardH / 2}
                        x2={b.x + layout.expanded!.peerCardW / 2}
                        y2={b.y + layout.expanded!.peerCardH / 2}
                        stroke="#516078"
                        strokeWidth="5"
                      />
                    ) : null;
                  })}
                  {expandedView.devices.map((n) => {
                    const p = layout.pos.get(n.id);
                    return p ? (
                      <rect
                        key={n.id}
                        x={p.x}
                        y={p.y}
                        width={layout.expanded!.cardW}
                        height={layout.expanded!.deviceCardH}
                        fill={focus === n.id ? "#f0c14a" : "#4a90e2"}
                      />
                    ) : null;
                  })}
                  {expandedView.peers.map((n) => {
                    const p = layout.pos.get(n.id);
                    return p ? (
                      <rect
                        key={`mp-${n.id}`}
                        x={p.x}
                        y={p.y}
                        width={layout.expanded!.peerCardW}
                        height={layout.expanded!.peerCardH}
                        fill={focus === n.id ? "#f0c14a" : "#6cb6ff"}
                      />
                    ) : null;
                  })}
                </>
              )
              : isLocView
              ? locGraph.linkEdges.map((e) => {
                  const a = layout.pos.get(e.a_id);
                  const b = layout.pos.get(e.b_id);
                  return a && b ? (
                    <line
                      key={`ml-${e.a_id}-${e.b_id}`}
                      x1={a.x + layout.cardW / 2}
                      y1={a.y + cardH / 2}
                      x2={b.x + layout.cardW / 2}
                      y2={b.y + cardH / 2}
                      stroke="#516078"
                      strokeWidth="5"
                    />
                  ) : null;
                })
              : edges.map((e) => {
              const a = layout.pos.get(e.local_device_id);
              const b = layout.pos.get(e.remote_device_id!);
              return a && b ? (
                <line
                  key={edgeKey(e)}
                  x1={a.x + layout.cardW / 2}
                  y1={a.y + cardH / 2}
                  x2={b.x + layout.cardW / 2}
                  y2={b.y + cardH / 2}
                  stroke="#516078"
                  strokeWidth="5"
                />
              ) : null;
            })}
            {!isExpanded && (isLocView ? locGraph.locNodes : nodes).map((n) => {
              const p = layout.pos.get(n.id);
              return p ? <rect key={n.id} x={p.x} y={p.y} width={layout.cardW} height={cardH} fill={focus === n.id ? "#f0c14a" : "#4a90e2"} /> : null;
            })}
            <rect
              x={miniView.x / Math.max(miniScale, 1e-6)}
              y={miniView.y / Math.max(miniScale, 1e-6)}
              width={miniView.w / Math.max(miniScale, 1e-6)}
              height={miniView.h / Math.max(miniScale, 1e-6)}
              fill="rgba(240,193,74,0.12)"
              stroke="#f0c14a"
              strokeWidth={Math.max(8, layout.width / 80)}
            />
          </svg>
        </div>
        <aside className="topology-inspector" style={{ ...dark, padding: 12, fontSize: ".9rem" }}>
          {!selected && !selectedLoc && !selectedPeer && (
            <p style={{ margin: 0, color: "#9aa3b5" }}>
              {isExpanded
                ? "Клик по устройству — инспектор. Клик по соседней локации — раскрыть её. Синие линии наружу — связи с другими площадками."
                : isLocView
                ? "Клик по расположению раскрывает устройства внутри и связи наружу. Пунктир — иерархия сайтов, цветные линии — LLDP/CDP/manual между локациями."
                : "Выберите узел для инспектора. Стрелки переходят к соседям, Enter оставляет узел открытым."}
            </p>
          )}
          {isExpanded && expandedView && (
            <div style={{ marginBottom: 12 }}>
              <div style={{ display: "flex", justifyContent: "space-between", gap: 8, alignItems: "flex-start" }}>
                <strong>{expandedView.label}</strong>
                <button type="button" onClick={closeExpanded}>×</button>
              </div>
              <div style={{ color: "#9aa3b5", marginTop: 4, fontSize: "0.85rem" }}>{expandedView.path}</div>
              <div style={{ color: "#c5cedd", marginTop: 6 }}>
                {expandedView.devices.length} устройств · {expandedView.peers.length} соседних локаций
              </div>
              <div style={{ display: "flex", gap: 8, marginTop: 10, flexWrap: "wrap" }}>
                <button type="button" onClick={closeExpanded}>К карте локаций</button>
                <button type="button" onClick={() => switchToDevicesForLocation(expandedView.path)}>
                  Устройства (фильтр)
                </button>
              </div>
            </div>
          )}
          {selectedPeer && (
            <>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <strong>{selectedPeer.label}</strong>
                <button type="button" onClick={() => setFocus(null)}>×</button>
              </div>
              <div style={{ color: "#9aa3b5", marginTop: 4, fontSize: "0.85rem" }}>{selectedPeer.path}</div>
              <div style={{ color: "#c5cedd", marginTop: 6 }}>
                {selectedPeer.device_count} устройств{selectedPeer.offline ? " · есть offline" : ""}
              </div>
              <button
                type="button"
                style={{ marginTop: 10 }}
                onClick={() => openLocation(selectedPeer.path, selectedPeer.id)}
              >
                Раскрыть эту локацию
              </button>
            </>
          )}
          {selectedLoc && !isExpanded && (
            <>
              <div style={{ display: "flex", justifyContent: "space-between" }}>
                <strong>{selectedLoc.label}</strong>
                <button type="button" onClick={() => setFocus(null)}>×</button>
              </div>
              <div style={{ color: "#9aa3b5", marginTop: 4, fontSize: "0.85rem" }}>{selectedLoc.path}</div>
              <div style={{ color: "#c5cedd", marginTop: 6 }}>
                {selectedLoc.device_count} устройств
                {selectedLoc.direct_count !== selectedLoc.device_count
                  ? ` (прямо здесь: ${selectedLoc.direct_count})`
                  : ""}
                {selectedLoc.offline ? " · есть offline" : ""}
              </div>
              <div style={{ display: "flex", gap: 8, marginTop: 10, flexWrap: "wrap" }}>
                <button type="button" onClick={() => openLocation(selectedLoc.path, selectedLoc.id)}>
                  Раскрыть локацию
                </button>
                <button type="button" onClick={() => switchToDevicesForLocation(selectedLoc.path)}>
                  Открыть устройства этой локации
                </button>
                <button
                  type="button"
                  onClick={() => {
                    const next = preferredRootId === selectedLoc.id ? null : selectedLoc.id;
                    setPreferredRootId(next);
                    writeStoredRoot(next);
                  }}
                >
                  {preferredRootId === selectedLoc.id ? "Сбросить корень дерева" : "Сделать корнем дерева"}
                </button>
              </div>
              <h3 style={{ fontSize: ".95rem", margin: "14px 0 6px" }}>
                Связи с другими локациями ({selectedLocLinks.length})
              </h3>
              {selectedLocLinks.map((e) => {
                const otherId = e.a_id === selectedLoc.id ? e.b_id : e.a_id;
                const other = locById.get(otherId);
                return (
                  <div key={`sl-${e.a_id}-${e.b_id}`} style={{ borderTop: "1px solid #242a38", padding: "7px 0" }}>
                    <button
                      type="button"
                      style={{ padding: 0, border: 0, background: "none", color: "#6cb6ff", cursor: "pointer" }}
                      onClick={() => select(otherId, false)}
                    >
                      {other?.label || "—"}
                    </button>
                    <div style={{ color: "#9aa3b5", fontSize: ".8rem" }}>
                      {e.link_count}× · {e.protocols.map((p) => p.toUpperCase()).join("+")}
                      {e.stale ? ` · ${formatTopologyStaleAt(e.last_seen_at)}` : ""}
                    </div>
                  </div>
                );
              })}
              <h3 style={{ fontSize: ".95rem", margin: "14px 0 6px" }}>Устройства ({selectedLocDevices.length})</h3>
              {selectedLocDevices.map((d) => (
                <div key={d.id} style={{ borderTop: "1px solid #242a38", padding: "6px 0" }}>
                  <Link to={`/devices/${d.id}`} state={deviceLinkState({ path: "/topology?view=locations", label: "Топология" })}>
                    {d.name || d.host}
                  </Link>
                  <div style={{ color: "#9aa3b5", fontSize: ".8rem" }}>
                    {d.host}
                    {offline(d) ? " · offline" : ""}
                  </div>
                </div>
              ))}
            </>
          )}
          {selected && (
            <>
              <div style={{ display: "flex", justifyContent: "space-between", gap: 8, alignItems: "flex-start" }}>
                {(() => {
                  const mac = selected.virtual ? virtualNodeMac(selected, selectedEdges) : null;
                  if (mac) {
                    return (
                      <strong
                        style={{ cursor: "pointer", userSelect: "all" }}
                        title="Клик — выделить MAC; двойной клик — скопировать"
                        onClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          selectElementText(e.currentTarget);
                        }}
                        onDoubleClick={(e) => {
                          e.preventDefault();
                          e.stopPropagation();
                          selectElementText(e.currentTarget);
                          copyTextToClipboard(mac);
                        }}
                      >
                        {looksLikeMac(label(selected)) ? mac : label(selected)}
                      </strong>
                    );
                  }
                  return <strong>{label(selected)}</strong>;
                })()}
                <button type="button" onClick={() => setFocus(null)}>×</button>
              </div>
              <div style={{ color: "#9aa3b5", marginTop: 4 }}>
                {selected.virtual
                  ? "не в списке Узлы · виден по LLDP/CDP"
                  : [
                      selected.host || "—",
                      selected.location || null,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
              </div>
              <div style={{ display: "flex", gap: 8, marginTop: 10, flexWrap: "wrap" }}>
                {!selected.virtual && (
                  <button
                    type="button"
                    onClick={() => nav(`/devices/${selected.id}`, { state: deviceLinkState({ path: isLocView ? "/topology?view=locations" : "/topology", label: "Топология" }) })}
                  >
                    Карточка устройства
                  </button>
                )}
                {!selected.virtual && (
                  <button
                    type="button"
                    title="Дерево L→R / T→B / radial строится от этого узла (сохраняется на сервере)"
                    onClick={() => {
                      const next = preferredRootId === selected.id ? null : selected.id;
                      setPreferredRootId(next);
                      void persistDeviceRoot(next);
                    }}
                  >
                    {preferredRootId === selected.id ? "Сбросить корень дерева" : "Сделать корнем дерева"}
                  </button>
                )}
                {selected.virtual && selected.discovered_id && (
                  <button type="button" onClick={openPromote}>
                    {promote.open ? "Сбросить форму" : "Добавить в Узлы…"}
                  </button>
                )}
              </div>
              {selected.virtual && selected.discovered_id && promote.open && (
                <div style={{ marginTop: 10 }}>
                  <PromoteDiscoveredForm
                    values={promote}
                    locations={existingLocations}
                    preview={promote.preview}
                    busy={promote.busy}
                    onChange={(patch) => setPromote((v) => ({ ...v, ...patch }))}
                    onPreview={() => void preview()}
                    onSubmit={promoteNode}
                    onCancel={() => setPromote(emptyPromote)}
                  />
                </div>
              )}
              <h3 style={{ fontSize: ".95rem", margin: "14px 0 6px" }}>Связи ({selectedEdges.length})</h3>
              {selectedEdges.map((e) => {
                const otherId = e.local_device_id === selected.id ? e.remote_device_id! : e.local_device_id;
                const other = nodeById.get(otherId);
                return (
                  <div key={edgeKey(e)} style={{ borderTop: "1px solid #242a38", padding: "7px 0" }}>
                    <button type="button" style={{ padding: 0, border: 0, background: "none", color: "#6cb6ff", cursor: "pointer" }} onClick={() => select(otherId, false)}>
                      {label(other) || e.remote_sys_name || "—"}
                    </button>
                    <div style={{ color: "#9aa3b5", fontSize: ".8rem" }}>
                      {edgeText(e)}
                      {e.poe_active ? ` · PoE${e.poe_power_w ? ` ${e.poe_power_w}W` : ""}` : ""}
                      {e.vlan_id ? ` · VLAN ${e.vlan_id}` : ""}
                      {e.stale ? ` · ${formatTopologyStaleAt(e.last_seen_at)}` : ""}
                    </div>
                    {e.manual_link_id != null && e.manual_note && (
                      <div style={{ color: "#c5cedd", fontSize: ".85rem", marginTop: 4, whiteSpace: "pre-wrap" }}>
                        Заметка: {e.manual_note}
                      </div>
                    )}
                    {e.manual_link_id != null && (
                      <div style={{ marginTop: 4, display: "flex", flexWrap: "wrap", gap: 6 }}>
                        <button
                          type="button"
                          onClick={() => void editManualNote(e.manual_link_id!, e.manual_note)}
                        >
                          Изменить заметку
                        </button>
                        <button type="button" onClick={() => void deleteManualLink(e.manual_link_id!)}>
                          Удалить ручную #{e.manual_link_id}
                        </button>
                      </div>
                    )}
                    {other && !other.virtual && (
                      <Link to={`/devices/${other.id}`} state={deviceLinkState({ path: "/topology", label: "Топология" })}>карточка</Link>
                    )}
                  </div>
                );
              })}
            </>
          )}
          {path && (
            <>
              <h3 style={{ fontSize: ".95rem", margin: "14px 0 6px" }}>Путь</h3>
              {path.map((h, i) => (
                <div key={`${h.fromId}-${h.toId}-${i}`} style={{ borderTop: "1px solid #242a38", padding: "6px 0" }}>
                  <button type="button" style={{ padding: 0, border: 0, background: "none", color: "#6cb6ff", cursor: "pointer" }} onClick={() => select(h.fromId, false)}>
                    {label(nodeById.get(h.fromId))}
                  </button>
                  {" → "}
                  <button type="button" style={{ padding: 0, border: 0, background: "none", color: "#6cb6ff", cursor: "pointer" }} onClick={() => select(h.toId, false)}>
                    {label(nodeById.get(h.toId))}
                  </button>
                  <div style={{ color: "#9aa3b5", fontSize: ".8rem" }}>{h.fromPort} → {h.toPort} · {h.protocol}</div>
                </div>
              ))}
            </>
          )}
        </aside>
      </div>
      {hoverTip && (
        <div
          role="tooltip"
          style={{
            position: "fixed",
            left: Math.min(hoverTip.x + 14, typeof window !== "undefined" ? window.innerWidth - 360 : hoverTip.x + 14),
            top: Math.min(hoverTip.y + 14, typeof window !== "undefined" ? window.innerHeight - 120 : hoverTip.y + 14),
            zIndex: 40,
            maxWidth: 340,
            padding: "8px 10px",
            borderRadius: 8,
            border: "1px solid #3a455c",
            background: "rgba(18, 24, 36, 0.96)",
            color: "#e8eaef",
            fontSize: "0.8rem",
            lineHeight: 1.45,
            whiteSpace: "pre-wrap",
            pointerEvents: "none",
            boxShadow: "0 8px 24px rgba(0,0,0,0.35)",
          }}
        >
          {hoverTip.text}
        </div>
      )}
    </div>
  );
}
