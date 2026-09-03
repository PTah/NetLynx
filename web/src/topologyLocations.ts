import type { TopologyEdge, TopologyNode } from "./topologyTypes";
import { isDeviceOnline } from "./deviceOnline";
import {
  CARD_H,
  estimateCardWidth,
  layoutTopology,
  type LayoutMode,
  type Pos,
} from "./topologyLayout";

export const NO_LOCATION = "Без расположения";
export const LOC_SEP = " / ";

export type LocationNode = {
  id: number;
  path: string;
  label: string;
  /** устройства в этом узле и ниже по дереву */
  device_count: number;
  /** устройства ровно с этим location */
  direct_count: number;
  offline: boolean;
  depth: number;
  parentPath: string | null;
};

export type LocationTreeEdge = {
  parent_id: number;
  child_id: number;
};

export type LocationLinkEdge = {
  a_id: number;
  b_id: number;
  link_count: number;
  protocols: string[];
  stale: boolean;
  last_seen_at?: string | null;
  has_manual: boolean;
};

export type LocationGraph = {
  locNodes: LocationNode[];
  treeEdges: LocationTreeEdge[];
  linkEdges: LocationLinkEdge[];
  /** path → devices assigned to that exact location */
  devicesByPath: Map<string, TopologyNode[]>;
  pathByDeviceId: Map<number, string>;
};

function normalizeLocation(raw?: string | null): string {
  const t = (raw ?? "").trim();
  return t || NO_LOCATION;
}

/** Разбор пути UISP-стиля: «A / B / C». */
export function locationSegments(path: string): string[] {
  if (path === NO_LOCATION) return [NO_LOCATION];
  return path
    .split(LOC_SEP)
    .map((s) => s.trim())
    .filter(Boolean);
}

export function locationPrefixes(path: string): string[] {
  const segs = locationSegments(path);
  const out: string[] = [];
  for (let i = 0; i < segs.length; i++) {
    out.push(segs.slice(0, i + 1).join(LOC_SEP));
  }
  return out;
}

/** Стабильный положительный id для пути (не пересекается с device id и virtual −1,-2,…). */
export function locationPathId(path: string, used: Set<number>): number {
  let h = 2166136261;
  for (let i = 0; i < path.length; i++) {
    h ^= path.charCodeAt(i);
    h = Math.imul(h, 16777619);
  }
  let id = 1_000_000_000 + (Math.abs(h) % 700_000_000);
  let n = 0;
  while (used.has(id) && n < 10_000) {
    id += 1;
    n++;
  }
  used.add(id);
  return id;
}

function underPath(deviceLoc: string, nodePath: string): boolean {
  if (deviceLoc === nodePath) return true;
  if (nodePath === NO_LOCATION) return deviceLoc === NO_LOCATION;
  return deviceLoc.startsWith(nodePath + LOC_SEP);
}

export function buildLocationGraph(nodes: TopologyNode[], edges: TopologyEdge[]): LocationGraph {
  const inventory = nodes.filter((n) => !n.virtual && n.id > 0);
  const devicesByPath = new Map<string, TopologyNode[]>();
  const pathByDeviceId = new Map<number, string>();

  for (const n of inventory) {
    const path = normalizeLocation(n.location);
    pathByDeviceId.set(n.id, path);
    const list = devicesByPath.get(path) ?? [];
    list.push(n);
    devicesByPath.set(path, list);
  }

  const pathSet = new Set<string>();
  for (const path of devicesByPath.keys()) {
    for (const p of locationPrefixes(path)) pathSet.add(p);
  }
  if (pathSet.size === 0) {
    return { locNodes: [], treeEdges: [], linkEdges: [], devicesByPath, pathByDeviceId };
  }

  const usedIds = new Set<number>();
  const idByPath = new Map<string, number>();
  for (const path of [...pathSet].sort((a, b) => a.localeCompare(b, "ru"))) {
    idByPath.set(path, locationPathId(path, usedIds));
  }

  const locNodes: LocationNode[] = [];
  for (const path of pathSet) {
    const segs = locationSegments(path);
    const depth = segs.length - 1;
    const parentPath = depth > 0 ? segs.slice(0, -1).join(LOC_SEP) : null;
    let device_count = 0;
    let direct_count = 0;
    let offline = false;
    for (const [devPath, list] of devicesByPath) {
      if (!underPath(devPath, path)) continue;
      device_count += list.length;
      if (devPath === path) direct_count += list.length;
      if (list.some((d) => !d.virtual && !isDeviceOnline(d))) offline = true;
    }
    locNodes.push({
      id: idByPath.get(path)!,
      path,
      label: segs[segs.length - 1] || path,
      device_count,
      direct_count,
      offline,
      depth,
      parentPath,
    });
  }

  const treeEdges: LocationTreeEdge[] = [];
  for (const n of locNodes) {
    if (!n.parentPath) continue;
    const pid = idByPath.get(n.parentPath);
    if (pid == null) continue;
    treeEdges.push({ parent_id: pid, child_id: n.id });
  }

  type Acc = { count: number; protocols: Set<string>; stale: boolean; has_manual: boolean; last_seen_at?: string | null };
  const linkAcc = new Map<string, Acc>();
  for (const e of edges) {
    if (e.remote_device_id == null) continue;
    const aPath = pathByDeviceId.get(e.local_device_id);
    const bPath = pathByDeviceId.get(e.remote_device_id);
    if (!aPath || !bPath || aPath === bPath) continue;
    const aId = idByPath.get(aPath);
    const bId = idByPath.get(bPath);
    if (aId == null || bId == null) continue;
    const lo = Math.min(aId, bId);
    const hi = Math.max(aId, bId);
    const key = `${lo}|${hi}`;
    let acc = linkAcc.get(key);
    if (!acc) {
      acc = { count: 0, protocols: new Set(), stale: false, has_manual: false };
      linkAcc.set(key, acc);
    }
    acc.count += 1;
    for (const p of e.protocols?.length ? e.protocols : [e.protocol]) {
      if (p) acc.protocols.add(p.toLowerCase());
    }
    if (e.stale) acc.stale = true;
    if (e.last_seen_at && (!acc.last_seen_at || e.last_seen_at > acc.last_seen_at)) {
      acc.last_seen_at = e.last_seen_at;
    }
    if (e.manual_link_id != null || (e.protocol || "").toLowerCase() === "manual") acc.has_manual = true;
  }

  const linkEdges: LocationLinkEdge[] = [];
  for (const [key, acc] of linkAcc) {
    const [a, b] = key.split("|").map(Number);
    linkEdges.push({
      a_id: a,
      b_id: b,
      link_count: acc.count,
      protocols: [...acc.protocols].sort(),
      stale: acc.stale,
      last_seen_at: acc.last_seen_at,
      has_manual: acc.has_manual,
    });
  }

  return { locNodes, treeEdges, linkEdges, devicesByPath, pathByDeviceId };
}

/** Корень для layout: верхний уровень, предпочтение «серверн»/«офис»/«ук», иначе макс. степень. */
export function pickLocationRoot(
  locNodes: LocationNode[],
  treeEdges: LocationTreeEdge[],
  linkEdges: LocationLinkEdge[],
): number | null {
  if (locNodes.length === 0) return null;
  const degree = new Map<number, number>();
  for (const n of locNodes) degree.set(n.id, 0);
  const bump = (a: number, b: number) => {
    degree.set(a, (degree.get(a) ?? 0) + 1);
    degree.set(b, (degree.get(b) ?? 0) + 1);
  };
  for (const e of treeEdges) bump(e.parent_id, e.child_id);
  for (const e of linkEdges) bump(e.a_id, e.b_id);

  const tops = locNodes.filter((n) => n.depth === 0);
  const pool = tops.length > 0 ? tops : locNodes;
  const prefer = (label: string) => {
    const s = label.toLowerCase();
    if (/серверн/.test(s)) return 3000;
    if (/офис|ук\b|server/.test(s)) return 2000;
    return 0;
  };
  let best = pool[0];
  let bestScore = -1;
  for (const n of pool) {
    const score = prefer(n.label) + prefer(n.path) + (degree.get(n.id) ?? 0) * 10 + n.device_count;
    if (score > bestScore) {
      bestScore = score;
      best = n;
    }
  }
  return best.id;
}

export function devicesUnderLocation(graph: LocationGraph, path: string): TopologyNode[] {
  const out: TopologyNode[] = [];
  for (const [devPath, list] of graph.devicesByPath) {
    if (underPath(devPath, path)) out.push(...list);
  }
  return out.sort((a, b) => (a.name || a.host).localeCompare(b.name || b.host, "ru"));
}

export type ExpandedPeerLoc = {
  id: number;
  path: string;
  label: string;
  device_count: number;
  offline: boolean;
};

export type ExpandedExternalEdge = {
  device_id: number;
  peer_path: string;
  peer_id: number;
  edge: TopologyEdge;
};

export type ExpandedLocationView = {
  path: string;
  label: string;
  devices: TopologyNode[];
  internalEdges: TopologyEdge[];
  peers: ExpandedPeerLoc[];
  externalEdges: ExpandedExternalEdge[];
};

/** Раскрытие локации: устройства внутри + peer-локации снаружи. */
export function buildExpandedLocationView(
  locGraph: LocationGraph,
  edges: TopologyEdge[],
  path: string,
): ExpandedLocationView | null {
  const loc = locGraph.locNodes.find((n) => n.path === path);
  if (!loc) return null;
  const devices = devicesUnderLocation(locGraph, path);
  const inside = new Set(devices.map((d) => d.id));
  const internalEdges: TopologyEdge[] = [];
  const peerPaths = new Set<string>();
  const externalEdges: ExpandedExternalEdge[] = [];

  for (const e of edges) {
    if (e.remote_device_id == null) continue;
    const aIn = inside.has(e.local_device_id);
    const bIn = inside.has(e.remote_device_id);
    if (aIn && bIn) {
      internalEdges.push(e);
      continue;
    }
    if (!aIn && !bIn) continue;
    const localId = aIn ? e.local_device_id : e.remote_device_id;
    const remoteId = aIn ? e.remote_device_id : e.local_device_id;
    const peerPath = locGraph.pathByDeviceId.get(remoteId);
    if (!peerPath || peerPath === path || underPath(peerPath, path)) continue;
    peerPaths.add(peerPath);
    externalEdges.push({
      device_id: localId,
      peer_path: peerPath,
      peer_id: 0,
      edge: e,
    });
  }

  const used = new Set(locGraph.locNodes.map((n) => n.id));
  const idByPeerPath = new Map<string, number>();
  const peers: ExpandedPeerLoc[] = [];
  for (const peerPath of [...peerPaths].sort((a, b) => a.localeCompare(b, "ru"))) {
    const existing = locGraph.locNodes.find((n) => n.path === peerPath);
    const id = existing?.id ?? locationPathId(peerPath, used);
    idByPeerPath.set(peerPath, id);
    const segs = locationSegments(peerPath);
    const list = locGraph.devicesByPath.get(peerPath) ?? [];
    peers.push({
      id,
      path: peerPath,
      label: existing?.label ?? segs[segs.length - 1] ?? peerPath,
      device_count: existing?.device_count ?? list.length,
      offline: existing?.offline ?? list.some((d) => !d.virtual && !isDeviceOnline(d)),
    });
  }
  for (const ex of externalEdges) {
    ex.peer_id = idByPeerPath.get(ex.peer_path) ?? 0;
  }

  return {
    path,
    label: loc.label,
    devices,
    internalEdges,
    peers,
    externalEdges,
  };
}

const PEER_CARD_H = 64;
const GROUP_PAD = 28;
const PEER_GAP = 18;
const SIDE_GAP = 72;

export type ExpandedLayout = {
  pos: Map<number, Pos>;
  group: { x: number; y: number; w: number; h: number };
  width: number;
  height: number;
  cardW: number;
  peerCardW: number;
  deviceCardH: number;
  peerCardH: number;
  mode: LayoutMode;
};

/** Layout раскрытой локации: устройства внутри рамки, peer-локации слева/справа. */
export function layoutExpandedLocationView(exp: ExpandedLocationView, layoutMode: LayoutMode): ExpandedLayout {
  const deviceCardH = CARD_H;
  const titles = exp.devices.map((d) => estimateCardWidth(d.name || d.host || "?", d.host || "—"));
  const cardW = Math.max(188, ...(titles.length ? titles : [188]));
  const peerCardW = Math.max(
    200,
    ...exp.peers.map((p) => estimateCardWidth(p.label, `${p.device_count} устройств`)),
    200,
  );

  const inner = layoutTopology(
    layoutMode === "force" || layoutMode === "radial" ? "lr" : layoutMode,
    exp.devices.map((d) => ({
      id: d.id,
      name: d.name || d.host,
      host: d.host,
      kind: d.kind || "switch",
      link_count: d.link_count,
      last_snmp_ok: d.last_snmp_ok,
    })),
    exp.internalEdges.map((e) => ({
      local_device_id: e.local_device_id,
      remote_device_id: e.remote_device_id!,
    })),
    { cardW, cardH: deviceCardH },
  );

  const pos = new Map<number, Pos>();
  let minX = Infinity;
  let minY = Infinity;
  let maxX = -Infinity;
  let maxY = -Infinity;
  if (exp.devices.length === 0) {
    minX = 0;
    minY = 0;
    maxX = 280;
    maxY = 120;
  } else {
    for (const d of exp.devices) {
      const p = inner.pos.get(d.id);
      if (!p || !Number.isFinite(p.x) || !Number.isFinite(p.y)) continue;
      pos.set(d.id, p);
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + cardW);
      maxY = Math.max(maxY, p.y + deviceCardH);
    }
    // layoutTopology мог не разместить узлы (пустой root) — запасная сетка
    if (!Number.isFinite(minX) || pos.size === 0) {
      pos.clear();
      const cols = Math.max(1, Math.ceil(Math.sqrt(exp.devices.length)));
      exp.devices.forEach((d, i) => {
        const col = i % cols;
        const row = Math.floor(i / cols);
        const p = { x: col * (cardW + 36), y: row * (deviceCardH + 28) };
        pos.set(d.id, p);
        minX = Math.min(minX === Infinity ? p.x : minX, p.x);
        minY = Math.min(minY === Infinity ? p.y : minY, p.y);
        maxX = Math.max(maxX === -Infinity ? p.x + cardW : maxX, p.x + cardW);
        maxY = Math.max(maxY === -Infinity ? p.y + deviceCardH : maxY, p.y + deviceCardH);
      });
    }
  }

  // Сдвиг: место слева под peer-карточки
  const leftPeers: typeof exp.peers = [];
  const rightPeers: typeof exp.peers = [];
  const midX = (minX + maxX) / 2;
  for (const peer of exp.peers) {
    const connected = exp.externalEdges.filter((e) => e.peer_path === peer.path);
    let avg = midX;
    if (connected.length) {
      let sx = 0;
      let n = 0;
      for (const c of connected) {
        const p = pos.get(c.device_id);
        if (p) {
          sx += p.x + cardW / 2;
          n++;
        }
      }
      if (n) avg = sx / n;
    }
    if (avg <= midX) leftPeers.push(peer);
    else rightPeers.push(peer);
  }
  // если все уехали в одну сторону — чередуем
  if (leftPeers.length === 0 && rightPeers.length > 1) {
    const half = Math.ceil(rightPeers.length / 2);
    leftPeers.push(...rightPeers.splice(0, half));
  }
  if (rightPeers.length === 0 && leftPeers.length > 1) {
    const half = Math.ceil(leftPeers.length / 2);
    rightPeers.push(...leftPeers.splice(leftPeers.length - half, half));
  }

  const leftColW = leftPeers.length ? peerCardW + SIDE_GAP : 0;
  const shiftX = GROUP_PAD + leftColW;
  const shiftY = GROUP_PAD + 36; // место под заголовок рамки

  for (const [id, p] of [...pos.entries()]) {
    pos.set(id, { x: p.x - minX + shiftX, y: p.y - minY + shiftY });
  }
  const group = {
    x: GROUP_PAD + leftColW - 16,
    y: GROUP_PAD,
    w: Math.max(320, maxX - minX + 32),
    h: Math.max(160, maxY - minY + 36 + 32),
  };

  const stackPeers = (list: typeof exp.peers, x: number) => {
    const totalH = list.length * PEER_CARD_H + Math.max(0, list.length - 1) * PEER_GAP;
    let y = group.y + Math.max(0, (group.h - totalH) / 2);
    for (const peer of list) {
      pos.set(peer.id, { x, y });
      y += PEER_CARD_H + PEER_GAP;
    }
  };
  if (leftPeers.length) {
    stackPeers(leftPeers, GROUP_PAD);
  }
  if (rightPeers.length) {
    stackPeers(rightPeers, group.x + group.w + SIDE_GAP - 16);
  }

  let width = group.x + group.w + GROUP_PAD;
  let height = group.y + group.h + GROUP_PAD;
  if (rightPeers.length) {
    width = Math.max(width, group.x + group.w + SIDE_GAP + peerCardW + GROUP_PAD);
  }
  for (const p of pos.values()) {
    if (!Number.isFinite(p.x) || !Number.isFinite(p.y)) continue;
    width = Math.max(width, p.x + Math.max(cardW, peerCardW) + GROUP_PAD);
    height = Math.max(height, p.y + Math.max(deviceCardH, PEER_CARD_H) + GROUP_PAD);
  }
  if (!Number.isFinite(width) || width <= 0) width = 480;
  if (!Number.isFinite(height) || height <= 0) height = 240;

  return {
    pos,
    group,
    width,
    height,
    cardW,
    peerCardW,
    deviceCardH,
    peerCardH: PEER_CARD_H,
    mode: layoutMode === "tb" ? "tb" : "lr",
  };
}

