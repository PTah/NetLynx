/** Кратчайший L2-путь по неориентированному графу рёбер топологии. */

export type PathEdge = {
  local_device_id: number;
  local_if_index: number;
  local_if_name?: string | null;
  remote_device_id?: number | null;
  remote_port_id?: string | null;
  protocol: string;
  protocols?: string[];
};

export type PathHop = {
  fromId: number;
  toId: number;
  fromPort: string;
  toPort: string;
  protocol: string;
};

export function findShortestPath(
  edges: PathEdge[],
  fromId: number,
  toId: number,
): PathHop[] | null {
  if (fromId === toId) return [];
  type Link = { to: number; fromPort: string; toPort: string; protocol: string };
  const adj = new Map<number, Link[]>();
  const add = (a: number, b: number, fromPort: string, toPort: string, protocol: string) => {
    const list = adj.get(a) ?? [];
    list.push({ to: b, fromPort, toPort, protocol });
    adj.set(a, list);
  };
  for (const e of edges) {
    if (e.remote_device_id == null) continue;
    const proto = (e.protocols?.length ? e.protocols.join("+") : e.protocol).toUpperCase();
    const local = e.local_if_name || `if${e.local_if_index}`;
    const remote = e.remote_port_id || "?";
    add(e.local_device_id, e.remote_device_id, local, remote, proto);
    add(e.remote_device_id, e.local_device_id, remote, local, proto);
  }
  const prev = new Map<number, { from: number; link: Link }>();
  const q = [fromId];
  const seen = new Set<number>([fromId]);
  while (q.length) {
    const u = q.shift()!;
    for (const link of adj.get(u) ?? []) {
      if (seen.has(link.to)) continue;
      seen.add(link.to);
      prev.set(link.to, { from: u, link });
      if (link.to === toId) {
        const hops: PathHop[] = [];
        let cur = toId;
        while (cur !== fromId) {
          const step = prev.get(cur)!;
          hops.push({
            fromId: step.from,
            toId: cur,
            fromPort: step.link.fromPort,
            toPort: step.link.toPort,
            protocol: step.link.protocol,
          });
          cur = step.from;
        }
        hops.reverse();
        return hops;
      }
      q.push(link.to);
    }
  }
  return null;
}
