/** Иерархический L→R layout топологии (лес BFS от корня: роутер → свитчи по степени). */

export type LayoutNode = {
  id: number;
  name: string;
  host: string;
  kind: string;
  virtual?: boolean;
  link_count?: number;
  last_snmp_ok?: boolean | null;
};

export type LayoutEdge = {
  local_device_id: number;
  remote_device_id: number;
};

export type Pos = { x: number; y: number };

export const CARD_W = 168;
export const CARD_H = 52;
export const COL_GAP = 48;
export const ROW_GAP = 14;
export const PAD = 28;

export function estimateCardWidth(title: string | null | undefined, subtitle: string | null | undefined): number {
  const t = (title ?? "").length;
  const s = (subtitle ?? "").length;
  const n = Math.max(t, s, 14);
  // ~7.2px на символ + отступ под иконку
  return Math.min(340, Math.max(188, Math.ceil(32 + n * 7.2)));
}

export type LayoutOptions = {
  cardW?: number;
  cardH?: number;
  /** lr: depth → X, siblings → Y; tb: depth → Y, siblings → X */
  orient?: "lr" | "tb";
  /** Явный корень дерева (если узел есть в графе). */
  preferredRootId?: number | null;
};

/** Оценка кандидата в корень: preferred → router → switch → степень. */
export function rootCandidateScore(
  n: LayoutNode | undefined,
  degree: number,
  preferredRootId?: number | null,
): number {
  if (!n) return -1;
  if (preferredRootId != null && n.id === preferredRootId) return 1_000_000;
  let kindBonus = 0;
  if (!n.virtual) {
    if (n.kind === "router") kindBonus = 5000;
    else if (n.kind === "switch") kindBonus = 1000;
  }
  return kindBonus + degree * 10 + (n.link_count ?? 0);
}

export function pickTopologyRoot(
  pool: Iterable<number>,
  byId: Map<number, LayoutNode>,
  degreeOf: (id: number) => number,
  preferredRootId?: number | null,
): number | null {
  let best: number | null = null;
  let bestScore = -1;
  for (const id of pool) {
    const score = rootCandidateScore(byId.get(id), degreeOf(id), preferredRootId);
    if (score > bestScore) {
      bestScore = score;
      best = id;
    }
  }
  return best;
}

export function layoutHierarchy(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  opts: LayoutOptions = {},
): { pos: Map<number, Pos>; width: number; height: number; cardW: number; cardH: number } {
  const cardW = opts.cardW ?? CARD_W;
  const cardH = opts.cardH ?? CARD_H;
  const tb = opts.orient === "tb";
  const pos = new Map<number, Pos>();
  if (nodes.length === 0) {
    return { pos, width: 480, height: 160, cardW, cardH };
  }

  const ids = new Set(nodes.map((n) => n.id));
  const adj = new Map<number, number[]>();
  for (const id of ids) adj.set(id, []);
  for (const e of edges) {
    if (!ids.has(e.local_device_id) || !ids.has(e.remote_device_id)) continue;
    if (e.local_device_id === e.remote_device_id) continue;
    adj.get(e.local_device_id)!.push(e.remote_device_id);
    adj.get(e.remote_device_id)!.push(e.local_device_id);
  }
  for (const [k, list] of adj) {
    adj.set(k, [...new Set(list)]);
  }

  const degree = (id: number) => (adj.get(id) ?? []).length;
  const byId = new Map(nodes.map((n) => [n.id, n]));

  const visited = new Set<number>();
  type Placed = { id: number; depth: number; parent: number | null };
  const components: Placed[][] = [];

  const remaining = () => {
    const s = new Set<number>();
    for (const id of ids) if (!visited.has(id)) s.add(id);
    return s;
  };

  while (visited.size < ids.size) {
    const pool = remaining();
    // preferredRoot только для первого (основного) компонента
    const preferred = components.length === 0 ? opts.preferredRootId : null;
    const root = pickTopologyRoot(pool, byId, degree, preferred);
    if (root == null || !pool.has(root)) break;
    const order: Placed[] = [];
    const q: number[] = [root];
    const depth = new Map<number, number>([[root, 0]]);
    const parent = new Map<number, number | null>([[root, null]]);
    visited.add(root);
    while (q.length) {
      const u = q.shift()!;
      order.push({ id: u, depth: depth.get(u)!, parent: parent.get(u) ?? null });
      const kids = (adj.get(u) ?? [])
        .filter((v) => !visited.has(v) && pool.has(v))
        .sort((a, b) => {
          const na = byId.get(a)?.name ?? "";
          const nb = byId.get(b)?.name ?? "";
          return na.localeCompare(nb, "ru");
        });
      for (const v of kids) {
        visited.add(v);
        depth.set(v, depth.get(u)! + 1);
        parent.set(v, u);
        q.push(v);
      }
    }
    components.push(order);
  }

  // Depth axis step / sibling axis step — зависят от ориентации, чтобы карточки не перекрывались.
  const depthStep = tb ? cardH + COL_GAP : cardW + COL_GAP;
  const siblingStep = tb ? cardW + ROW_GAP : cardH + ROW_GAP;
  let stackOffset = PAD;
  let maxX = 0;
  let maxY = 0;

  for (const order of components) {
    const children = new Map<number, number[]>();
    for (const p of order) {
      if (p.parent != null) {
        const list = children.get(p.parent) ?? [];
        list.push(p.id);
        children.set(p.parent, list);
      }
    }
    const leafWeight = new Map<number, number>();
    const calc = (id: number): number => {
      const kids = children.get(id) ?? [];
      if (kids.length === 0) {
        leafWeight.set(id, 1);
        return 1;
      }
      let w = 0;
      for (const k of kids) w += calc(k);
      leafWeight.set(id, Math.max(1, w));
      return leafWeight.get(id)!;
    };
    const roots = order.filter((p) => p.parent == null).map((p) => p.id);
    for (const r of roots) calc(r);

    const setPos = (id: number, depth: number, sibling: number) => {
      if (tb) {
        pos.set(id, { x: stackOffset + sibling, y: PAD + depth * depthStep });
        maxX = Math.max(maxX, stackOffset + sibling + cardW);
        maxY = Math.max(maxY, PAD + depth * depthStep + cardH);
      } else {
        pos.set(id, { x: PAD + depth * depthStep, y: stackOffset + sibling });
        maxX = Math.max(maxX, PAD + depth * depthStep + cardW);
        maxY = Math.max(maxY, stackOffset + sibling + cardH);
      }
    };

    const place = (id: number, depth: number, siblingStart: number): number => {
      const kids = children.get(id) ?? [];
      const w = leafWeight.get(id) ?? 1;
      if (kids.length === 0) {
        setPos(id, depth, siblingStart);
        return siblingStart + siblingStep;
      }
      let cursor = siblingStart;
      for (const k of kids) {
        cursor = place(k, depth + 1, cursor);
      }
      const sFirst = tb ? pos.get(kids[0])!.x - stackOffset : pos.get(kids[0])!.y - stackOffset;
      const sLast = tb ? pos.get(kids[kids.length - 1])!.x - stackOffset : pos.get(kids[kids.length - 1])!.y - stackOffset;
      const sibling = (sFirst + sLast) / 2;
      setPos(id, depth, sibling);
      return Math.max(cursor, siblingStart + w * siblingStep);
    };

    let localSibling = 0;
    for (const r of roots) {
      localSibling = place(r, 0, localSibling);
    }
    for (const p of order) {
      if (!pos.has(p.id)) {
        setPos(p.id, 0, localSibling);
        localSibling += siblingStep;
      }
    }
    stackOffset = (tb ? maxX : maxY) + PAD * 1.5;
  }

  return {
    pos,
    width: Math.max(480, maxX + PAD),
    height: Math.max(160, maxY + PAD),
    cardW,
    cardH,
  };
}

export type LayoutMode = "lr" | "tb" | "radial" | "force";

export type LayoutResult = {
  pos: Map<number, Pos>;
  width: number;
  height: number;
  cardW: number;
  cardH: number;
  mode: LayoutMode;
};

/** Tree L→R (по умолчанию), T→B, radial, force-directed. */
export function layoutTopology(
  mode: LayoutMode,
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  opts: LayoutOptions = {},
): LayoutResult {
  if (mode === "tb") {
    const base = layoutHierarchy(nodes, edges, { ...opts, orient: "tb" });
    return { ...base, mode: "tb" };
  }
  if (mode === "radial") {
    return layoutRadial(nodes, edges, opts);
  }
  if (mode === "force") {
    return layoutForce(nodes, edges, opts);
  }
  const base = layoutHierarchy(nodes, edges, { ...opts, orient: "lr" });
  return { ...base, mode: "lr" };
}

function buildAdj(nodes: LayoutNode[], edges: LayoutEdge[]) {
  const ids = new Set(nodes.map((n) => n.id));
  const adj = new Map<number, number[]>();
  for (const id of ids) adj.set(id, []);
  for (const e of edges) {
    if (!ids.has(e.local_device_id) || !ids.has(e.remote_device_id)) continue;
    if (e.local_device_id === e.remote_device_id) continue;
    adj.get(e.local_device_id)!.push(e.remote_device_id);
    adj.get(e.remote_device_id)!.push(e.local_device_id);
  }
  for (const [k, list] of adj) adj.set(k, [...new Set(list)]);
  return { ids, adj };
}

function layoutRadial(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  opts: LayoutOptions,
): LayoutResult {
  const cardW = opts.cardW ?? CARD_W;
  const cardH = opts.cardH ?? CARD_H;
  const pos = new Map<number, Pos>();
  if (nodes.length === 0) {
    return { pos, width: 480, height: 160, cardW, cardH, mode: "radial" };
  }
  const { ids, adj } = buildAdj(nodes, edges);
  const byId = new Map(nodes.map((n) => [n.id, n]));
  const root = pickTopologyRoot(ids, byId, (id) => adj.get(id)?.length ?? 0, opts.preferredRootId);
  if (root == null) {
    return { pos, width: 480, height: 160, cardW, cardH, mode: "radial" };
  }
  const depth = new Map<number, number>([[root, 0]]);
  const q = [root];
  const visited = new Set([root]);
  while (q.length) {
    const u = q.shift()!;
    for (const v of adj.get(u) ?? []) {
      if (visited.has(v)) continue;
      visited.add(v);
      depth.set(v, (depth.get(u) ?? 0) + 1);
      q.push(v);
    }
  }
  let maxConnected = 0;
  for (const d of depth.values()) {
    if (d > maxConnected) maxConnected = d;
  }
  const orphanDepth = maxConnected + 1;
  for (const id of ids) {
    if (!depth.has(id)) depth.set(id, orphanDepth);
  }
  const byDepth = new Map<number, number[]>();
  for (const [id, d] of depth) {
    const list = byDepth.get(d) ?? [];
    list.push(id);
    byDepth.set(d, list);
  }
  const cx = 420;
  const cy = 420;
  const ring = Math.max(cardW, cardH) + 56;
  for (const [d, list] of byDepth) {
    list.sort((a, b) => (byId.get(a)?.name ?? "").localeCompare(byId.get(b)?.name ?? "", "ru"));
    const r = d === 0 ? 0 : d * ring;
    list.forEach((id, i) => {
      if (d === 0) {
        pos.set(id, { x: cx - cardW / 2, y: cy - cardH / 2 });
        return;
      }
      const ang = (i / list.length) * Math.PI * 2 - Math.PI / 2;
      pos.set(id, {
        x: cx + Math.cos(ang) * r - cardW / 2,
        y: cy + Math.sin(ang) * r - cardH / 2,
      });
    });
  }
  let maxX = 0;
  let maxY = 0;
  let minX = Infinity;
  let minY = Infinity;
  for (const p of pos.values()) {
    minX = Math.min(minX, p.x);
    minY = Math.min(minY, p.y);
    maxX = Math.max(maxX, p.x + cardW);
    maxY = Math.max(maxY, p.y + cardH);
  }
  const shiftX = PAD - minX;
  const shiftY = PAD - minY;
  for (const [id, p] of pos) {
    pos.set(id, { x: p.x + shiftX, y: p.y + shiftY });
  }
  return {
    pos,
    width: Math.max(480, maxX - minX + PAD * 2),
    height: Math.max(160, maxY - minY + PAD * 2),
    cardW,
    cardH,
    mode: "radial",
  };
}

function layoutForce(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  opts: LayoutOptions,
): LayoutResult {
  const cardW = opts.cardW ?? CARD_W;
  const cardH = opts.cardH ?? CARD_H;
  const pos = new Map<number, Pos>();
  if (nodes.length === 0) {
    return { pos, width: 480, height: 160, cardW, cardH, mode: "force" };
  }
  const { adj } = buildAdj(nodes, edges);
  const ids = nodes.map((n) => n.id);
  const n = ids.length;
  const side = Math.max(480, Math.ceil(Math.sqrt(n)) * (cardW + 40));
  ids.forEach((id, i) => {
    const col = i % Math.ceil(Math.sqrt(n));
    const row = Math.floor(i / Math.ceil(Math.sqrt(n)));
    pos.set(id, { x: PAD + col * (cardW + 36), y: PAD + row * (cardH + 28) });
  });
  const vel = new Map<number, Pos>();
  for (const id of ids) vel.set(id, { x: 0, y: 0 });
  const ideal = cardW + 80;
  for (let iter = 0; iter < 80; iter++) {
    for (const id of ids) {
      let fx = 0;
      let fy = 0;
      const p = pos.get(id)!;
      for (const other of ids) {
        if (other === id) continue;
        const o = pos.get(other)!;
        let dx = p.x - o.x;
        let dy = p.y - o.y;
        let dist = Math.hypot(dx, dy) || 1;
        const rep = 8000 / (dist * dist);
        fx += (dx / dist) * rep;
        fy += (dy / dist) * rep;
      }
      for (const nb of adj.get(id) ?? []) {
        const o = pos.get(nb)!;
        let dx = o.x - p.x;
        let dy = o.y - p.y;
        let dist = Math.hypot(dx, dy) || 1;
        const att = (dist - ideal) * 0.04;
        fx += (dx / dist) * att;
        fy += (dy / dist) * att;
      }
      const v = vel.get(id)!;
      v.x = (v.x + fx) * 0.6;
      v.y = (v.y + fy) * 0.6;
      p.x += v.x;
      p.y += v.y;
    }
  }
  let minX = Infinity;
  let minY = Infinity;
  let maxX = 0;
  let maxY = 0;
  for (const p of pos.values()) {
    minX = Math.min(minX, p.x);
    minY = Math.min(minY, p.y);
    maxX = Math.max(maxX, p.x + cardW);
    maxY = Math.max(maxY, p.y + cardH);
  }
  const shiftX = PAD - minX;
  const shiftY = PAD - minY;
  for (const [id, p] of pos) {
    pos.set(id, { x: p.x + shiftX, y: p.y + shiftY });
  }
  return {
    pos,
    width: Math.max(side, maxX - minX + PAD * 2),
    height: Math.max(160, maxY - minY + PAD * 2),
    cardW,
    cardH,
    mode: "force",
  };
}

/** Минимальная длина ребра (px): короче — не рисуем (артефакты пунктира). */
export const MIN_EDGE_LEN = 18;

function perpOffset(dx: number, dy: number, bend: number): { ox: number; oy: number } {
  const len = Math.hypot(dx, dy) || 1;
  return { ox: (-dy / len) * bend, oy: (dx / len) * bend };
}

function lrSameColumnPath(a: Pos, b: Pos, cardW: number, cardH: number, bend = 0): string {
  const upper = a.y <= b.y ? a : b;
  const lower = a.y <= b.y ? b : a;
  const cx = upper.x + cardW / 2;
  const yStart = upper.y + cardH;
  const yEnd = lower.y;
  if (yEnd > yStart + 6) {
    const dy = Math.max(20, (yEnd - yStart) * 0.45);
    const { ox, oy } = perpOffset(0, yEnd - yStart, bend);
    return `M ${cx} ${yStart} C ${cx + ox} ${yStart + dy + oy}, ${cx + ox} ${yEnd - dy + oy}, ${cx} ${yEnd}`;
  }
  const channelX = upper.x + cardW + 32 + bend;
  const cy = upper.y + cardH / 2;
  const cy2 = lower.y + cardH / 2;
  return `M ${upper.x + cardW} ${cy} C ${channelX} ${cy}, ${channelX} ${cy2}, ${lower.x} ${cy2}`;
}

function tbSameRowPath(a: Pos, b: Pos, cardW: number, cardH: number, bend = 0): string {
  const left = a.x <= b.x ? a : b;
  const right = a.x <= b.x ? b : a;
  const cy = left.y + cardH / 2;
  const xStart = left.x + cardW;
  const xEnd = right.x;
  if (xEnd > xStart + 6) {
    const dx = Math.max(20, (xEnd - xStart) * 0.45);
    const { ox, oy } = perpOffset(xEnd - xStart, 0, bend);
    return `M ${xStart} ${cy} C ${xStart + dx + ox} ${cy + oy}, ${xEnd - dx + ox} ${cy + oy}, ${xEnd} ${cy}`;
  }
  const channelY = left.y + cardH + 32 + bend;
  const cx = left.x + cardW / 2;
  const cx2 = right.x + cardW / 2;
  return `M ${cx} ${left.y + cardH} C ${cx} ${channelY}, ${cx2} ${channelY}, ${cx2} ${right.y + cardH}`;
}

/** Достаточно ли длинное ребро для отрисовки (по якорным точкам на карточках). */
export function edgeRenderable(
  a: Pos,
  b: Pos,
  cardW = CARD_W,
  cardH = CARD_H,
  mode: LayoutMode = "lr",
): boolean {
  if (mode === "tb") {
    const top = a.y <= b.y ? a : b;
    const bot = a.y <= b.y ? b : a;
    const x1 = top.x + cardW / 2;
    const y1 = top.y + cardH;
    const x2 = bot.x + cardW / 2;
    const y2 = bot.y;
    return Math.hypot(x2 - x1, y2 - y1) >= MIN_EDGE_LEN;
  }
  const left = a.x <= b.x ? a : b;
  const right = a.x <= b.x ? b : a;
  const x1 = left.x + cardW;
  const y1 = left.y + cardH / 2;
  const x2 = right.x;
  const y2 = right.y + cardH / 2;
  return Math.hypot(x2 - x1, y2 - y1) >= MIN_EDGE_LEN;
}

export function edgePath(
  a: Pos,
  b: Pos,
  cardW = CARD_W,
  cardH = CARD_H,
  mode: LayoutMode = "lr",
  bend = 0,
): string {
  if (mode === "tb") {
    const top = a.y <= b.y ? a : b;
    const bot = a.y <= b.y ? b : a;
    const x1 = top.x + cardW / 2;
    const y1 = top.y + cardH;
    const x2 = bot.x + cardW / 2;
    const y2 = bot.y;
    if (y2 <= y1 + 4) return tbSameRowPath(a, b, cardW, cardH, bend);
    const dy = Math.max(40, (y2 - y1) * 0.45);
    const { ox, oy } = perpOffset(x2 - x1, y2 - y1, bend);
    return `M ${x1} ${y1} C ${x1 + ox} ${y1 + dy + oy}, ${x2 + ox} ${y2 - dy + oy}, ${x2} ${y2}`;
  }
  const left = a.x <= b.x ? a : b;
  const right = a.x <= b.x ? b : a;
  const x1 = left.x + cardW;
  const y1 = left.y + cardH / 2;
  const x2 = right.x;
  const y2 = right.y + cardH / 2;
  if (x2 <= x1 + 4) return lrSameColumnPath(a, b, cardW, cardH, bend);
  const dx = Math.max(40, (x2 - x1) * 0.45);
  const { ox, oy } = perpOffset(x2 - x1, y2 - y1, bend);
  return `M ${x1} ${y1} C ${x1 + dx + ox} ${y1 + oy}, ${x2 - dx + ox} ${y2 + oy}, ${x2} ${y2}`;
}

export type EdgeBendItem = { key: string; a: number; b: number };

function cardCenter(p: Pos, cardW: number, cardH: number): Pos {
  return { x: p.x + cardW / 2, y: p.y + cardH / 2 };
}

function pointSegDist(p: Pos, a: Pos, b: Pos): number {
  const dx = b.x - a.x;
  const dy = b.y - a.y;
  const len2 = dx * dx + dy * dy || 1;
  let t = ((p.x - a.x) * dx + (p.y - a.y) * dy) / len2;
  if (t < 0) t = 0;
  else if (t > 1) t = 1;
  return Math.hypot(p.x - (a.x + t * dx), p.y - (a.y + t * dy));
}

/** Два ребра накладываются: одна пара узлов или почти коллинеарны от общей вершины. */
function edgesOverlay(
  e1: EdgeBendItem,
  e2: EdgeBendItem,
  pos: Map<number, Pos>,
  cardW: number,
  cardH: number,
): boolean {
  const samePair =
    (e1.a === e2.a && e1.b === e2.b) || (e1.a === e2.b && e1.b === e2.a);
  if (samePair) return true;
  let shared: number | null = null;
  let o1 = 0;
  let o2 = 0;
  if (e1.a === e2.a || e1.a === e2.b) {
    shared = e1.a;
    o1 = e1.b;
    o2 = e1.a === e2.a ? e2.b : e2.a;
  } else if (e1.b === e2.a || e1.b === e2.b) {
    shared = e1.b;
    o1 = e1.a;
    o2 = e1.b === e2.a ? e2.b : e2.a;
  }
  if (shared == null) return false;
  const pShared = pos.get(shared);
  const p1 = pos.get(o1);
  const p2 = pos.get(o2);
  if (!pShared || !p1 || !p2) return false;
  const v = cardCenter(pShared, cardW, cardH);
  const a = cardCenter(p1, cardW, cardH);
  const b = cardCenter(p2, cardW, cardH);
  const da = Math.hypot(a.x - v.x, a.y - v.y);
  const db = Math.hypot(b.x - v.x, b.y - v.y);
  const lim = 28;
  return da >= db ? pointSegDist(b, v, a) < lim : pointSegDist(a, v, b) < lim;
}

/**
 * Смещение дуги (px) для рёбер, которые иначе рисуются друг на друге.
 * Обычное дерево с веером детей не трогаем.
 */
export function computeEdgeBends(
  items: EdgeBendItem[],
  pos: Map<number, Pos>,
  cardW = CARD_W,
  cardH = CARD_H,
): Map<string, number> {
  const out = new Map<string, number>();
  const usable = items.filter((e) => pos.has(e.a) && pos.has(e.b) && e.a !== e.b);
  const n = usable.length;
  if (n < 2) return out;

  const parent = usable.map((_, i) => i);
  const find = (i: number): number => {
    while (parent[i] !== i) {
      parent[i] = parent[parent[i]];
      i = parent[i];
    }
    return i;
  };
  const unite = (i: number, j: number) => {
    const ri = find(i);
    const rj = find(j);
    if (ri !== rj) parent[rj] = ri;
  };

  const byNode = new Map<number, number[]>();
  usable.forEach((e, i) => {
    let la = byNode.get(e.a);
    if (!la) {
      la = [];
      byNode.set(e.a, la);
    }
    la.push(i);
    let lb = byNode.get(e.b);
    if (!lb) {
      lb = [];
      byNode.set(e.b, lb);
    }
    lb.push(i);
  });

  const seen = new Set<string>();
  for (const idxs of byNode.values()) {
    for (let i = 0; i < idxs.length; i++) {
      for (let j = i + 1; j < idxs.length; j++) {
        const ia = idxs[i];
        const ib = idxs[j];
        const pair = ia < ib ? `${ia}:${ib}` : `${ib}:${ia}`;
        if (seen.has(pair)) continue;
        seen.add(pair);
        if (edgesOverlay(usable[ia], usable[ib], pos, cardW, cardH)) unite(ia, ib);
      }
    }
  }

  const groups = new Map<number, number[]>();
  for (let i = 0; i < n; i++) {
    const r = find(i);
    const list = groups.get(r);
    if (list) list.push(i);
    else groups.set(r, [i]);
  }

  const STEP = 26;
  const MAX = 78;
  for (const idxs of groups.values()) {
    if (idxs.length < 2) continue;
    idxs.sort((i, j) => {
      const ei = usable[i];
      const ej = usable[j];
      const pai = pos.get(ei.a)!;
      const pbi = pos.get(ei.b)!;
      const paj = pos.get(ej.a)!;
      const pbj = pos.get(ej.b)!;
      const myi = (pai.y + pbi.y) / 2;
      const myj = (paj.y + pbj.y) / 2;
      if (myi !== myj) return myi - myj;
      const mxi = (pai.x + pbi.x) / 2;
      const mxj = (paj.x + pbj.x) / 2;
      if (mxi !== mxj) return mxi - mxj;
      return ei.key.localeCompare(ej.key);
    });
    const mid = (idxs.length - 1) / 2;
    idxs.forEach((i, k) => {
      let bend = (k - mid) * STEP;
      if (bend > MAX) bend = MAX;
      if (bend < -MAX) bend = -MAX;
      out.set(usable[i].key, bend);
    });
  }
  return out;
}

/** Сдвиг подписи, чтобы она сидела на отогнутой дуге. */
export function edgeLabelPos(
  a: Pos,
  b: Pos,
  cardW: number,
  cardH: number,
  bend: number,
): { x: number; y: number } {
  const dx = b.x + cardW / 2 - (a.x + cardW / 2);
  const dy = b.y + cardH / 2 - (a.y + cardH / 2);
  const { ox, oy } = perpOffset(dx, dy, bend);
  return {
    x: (a.x + b.x + cardW) / 2 + ox,
    y: (a.y + b.y + cardH) / 2 + oy,
  };
}
