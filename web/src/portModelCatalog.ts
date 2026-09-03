import rawCatalog from "./port-model-catalog.json";

export type PortModelCatalogEntryRaw = {
  manufacturer: string;
  model: string;
  portCount: number;
  copperPorts: number[];
  sfpPorts: number[];
  matchPatterns: string[];
  matchFlags?: string[];
  specialRules?: string[];
};

export type PortModelSpec = {
  manufacturer: string;
  model: string;
  portCount: number;
  copperPorts: number[];
  sfpPorts: number[];
  matchPatterns: RegExp[];
  specialRules?: string[];
};

function assertPortList(name: string, modelKey: string, ports: number[], portCount: number): number[] {
  const seen = new Set<number>();
  for (const port of ports) {
    if (!Number.isInteger(port) || port <= 0) {
      throw new Error(`[port-model-catalog] ${modelKey}: ${name} contains invalid port "${port}"`);
    }
    if (port > portCount) {
      throw new Error(`[port-model-catalog] ${modelKey}: ${name} port ${port} exceeds portCount=${portCount}`);
    }
    if (seen.has(port)) {
      throw new Error(`[port-model-catalog] ${modelKey}: ${name} contains duplicate port ${port}`);
    }
    seen.add(port);
  }
  return [...seen].sort((a, b) => a - b);
}

function compilePatterns(modelKey: string, patterns: string[], flags?: string[]): RegExp[] {
  if (!patterns.length) {
    throw new Error(`[port-model-catalog] ${modelKey}: matchPatterns must not be empty`);
  }
  return patterns.map((pattern, idx) => {
    const reFlags = flags?.[idx] ?? "i";
    try {
      return new RegExp(pattern, reFlags);
    } catch (error) {
      const message = error instanceof Error ? error.message : String(error);
      throw new Error(`[port-model-catalog] ${modelKey}: invalid regexp "${pattern}" (${message})`);
    }
  });
}

function normalizeEntry(entry: PortModelCatalogEntryRaw): PortModelSpec {
  const modelKey = `${entry.manufacturer} ${entry.model}`;
  if (!entry.manufacturer.trim() || !entry.model.trim()) {
    throw new Error(`[port-model-catalog] ${modelKey}: manufacturer/model must not be empty`);
  }
  if (!Number.isInteger(entry.portCount) || entry.portCount <= 0) {
    throw new Error(`[port-model-catalog] ${modelKey}: invalid portCount ${entry.portCount}`);
  }

  const copperPorts = assertPortList("copperPorts", modelKey, entry.copperPorts ?? [], entry.portCount);
  const sfpPorts = assertPortList("sfpPorts", modelKey, entry.sfpPorts ?? [], entry.portCount);
  const intersection = copperPorts.filter((port) => sfpPorts.includes(port));
  if (intersection.length) {
    throw new Error(`[port-model-catalog] ${modelKey}: copperPorts and sfpPorts overlap: ${intersection.join(", ")}`);
  }

  return {
    manufacturer: entry.manufacturer.trim(),
    model: entry.model.trim(),
    portCount: entry.portCount,
    copperPorts,
    sfpPorts,
    matchPatterns: compilePatterns(modelKey, entry.matchPatterns ?? [], entry.matchFlags),
    specialRules: entry.specialRules?.filter(Boolean),
  };
}

export const PORT_MODEL_CATALOG: PortModelSpec[] = (rawCatalog as PortModelCatalogEntryRaw[]).map(normalizeEntry);

export function resolvePortModelFromHint(hint: string | null | undefined): PortModelSpec | null {
  const s = hint?.toLowerCase();
  if (!s) return null;
  for (const entry of PORT_MODEL_CATALOG) {
    if (entry.matchPatterns.some((re) => re.test(s))) return entry;
  }
  return null;
}

