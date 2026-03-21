// ---------- Types ----------

export interface DeployGroup {
  /** Services in this group can deploy in parallel */
  services: string[];
  /** Group index (0 = first to deploy, no dependencies) */
  level: number;
}

export interface DependencyGraph {
  /** Ordered groups for deployment */
  groups: DeployGroup[];
  /** All service names in topological order (flattened) */
  order: string[];
  /** Reverse order for shutdown */
  reverseOrder: string[];
}

// ---------- Cycle detection (DFS with coloring) ----------

/** Node colors for DFS cycle detection: white=unvisited, gray=in-progress, black=done */
const enum Color {
  White = 0,
  Gray = 1,
  Black = 2,
}

/**
 * Detect cycles using DFS with three-color marking.
 * If a cycle is found, throws an error with the full cycle path.
 */
function detectCycles(nodes: Set<string>, adjacency: Map<string, string[]>): void {
  const color = new Map<string, Color>();
  const parent = new Map<string, string | null>();

  for (const node of nodes) {
    color.set(node, Color.White);
  }

  for (const node of nodes) {
    if (color.get(node) === Color.White) {
      dfsVisit(node, adjacency, color, parent);
    }
  }
}

/**
 * Recursive DFS visit. When we encounter a gray node, we've found a cycle.
 * Walk the parent chain to reconstruct the full cycle path.
 */
function dfsVisit(
  node: string,
  adjacency: Map<string, string[]>,
  color: Map<string, Color>,
  parent: Map<string, string | null>,
): void {
  color.set(node, Color.Gray);

  for (const neighbor of adjacency.get(node) ?? []) {
    if (color.get(neighbor) === Color.Gray) {
      // Found a cycle — reconstruct the path
      const cycle = [neighbor];
      let current = node;
      while (current !== neighbor) {
        cycle.push(current);
        current = parent.get(current)!;
      }
      cycle.push(neighbor);
      cycle.reverse();
      throw new Error(`Circular dependency detected: ${cycle.join(" \u2192 ")}`);
    }

    if (color.get(neighbor) === Color.White) {
      parent.set(neighbor, node);
      dfsVisit(neighbor, adjacency, color, parent);
    }
  }

  color.set(node, Color.Black);
}

// ---------- Public API ----------

/**
 * Build a dependency graph from service configurations.
 *
 * Uses Kahn's algorithm (BFS with in-degree counting) to produce a topological
 * sort grouped into deployment levels. Services within the same level have no
 * mutual dependencies and can deploy in parallel.
 *
 * @param services - Map of service name -> { depends_on?: string[] }
 * @param accessories - Map of accessory name -> config (accessories are implicit level 0)
 * @returns DependencyGraph with deployment groups
 * @throws Error if cycle detected or if depends_on references unknown service
 */
export function buildDependencyGraph(
  services: Record<string, { depends_on?: string[] }>,
  accessories?: Record<string, unknown>,
): DependencyGraph {
  const serviceNames = new Set(Object.keys(services));
  const accessoryNames = new Set(Object.keys(accessories ?? {}));
  const allNames = new Set([...serviceNames, ...accessoryNames]);

  // Empty input — return empty graph
  if (allNames.size === 0) {
    return { groups: [], order: [], reverseOrder: [] };
  }

  // ---------- Validate references ----------
  // Every depends_on target must exist in services or accessories
  for (const [name, config] of Object.entries(services)) {
    for (const dep of config.depends_on ?? []) {
      if (!allNames.has(dep)) {
        const available = [...allNames].sort().join(", ");
        throw new Error(
          `Service '${name}' depends on unknown service '${dep}'. Available services: ${available}`,
        );
      }
    }
  }

  // ---------- Build adjacency list ----------
  // Edge direction: dependency -> dependent (dep must deploy before dependent)
  // We also track the reverse direction for cycle detection (dependent -> dependency)
  const adjacency = new Map<string, string[]>();
  const inDegree = new Map<string, number>();

  for (const name of allNames) {
    adjacency.set(name, []);
    inDegree.set(name, 0);
  }

  for (const [name, config] of Object.entries(services)) {
    for (const dep of config.depends_on ?? []) {
      // dep -> name (dep must deploy before name)
      adjacency.get(dep)!.push(name);
      inDegree.set(name, inDegree.get(name)! + 1);
    }
  }

  // Accessories have no dependencies, so their in-degree stays 0

  // ---------- Cycle detection ----------
  // Build the reverse adjacency for DFS: dependent -> dependency
  // (follow edges from node to its dependencies)
  const reverseAdj = new Map<string, string[]>();
  for (const name of allNames) {
    reverseAdj.set(name, []);
  }
  for (const [name, config] of Object.entries(services)) {
    for (const dep of config.depends_on ?? []) {
      reverseAdj.get(name)!.push(dep);
    }
  }
  detectCycles(allNames, reverseAdj);

  // ---------- Kahn's algorithm (BFS topological sort with levels) ----------
  const groups: DeployGroup[] = [];
  const order: string[] = [];
  const remaining = new Map(inDegree);

  // Seed with all nodes that have in-degree 0
  let currentLevel: string[] = [];
  for (const [name, deg] of remaining) {
    if (deg === 0) {
      currentLevel.push(name);
    }
  }

  let level = 0;
  while (currentLevel.length > 0) {
    // Sort for deterministic output
    currentLevel.sort();

    groups.push({ services: currentLevel, level });
    order.push(...currentLevel);

    // Find next level: decrement in-degrees and collect newly-free nodes
    const nextLevel: string[] = [];
    for (const name of currentLevel) {
      remaining.delete(name);
      for (const dependent of adjacency.get(name)!) {
        const newDeg = remaining.get(dependent)! - 1;
        remaining.set(dependent, newDeg);
        if (newDeg === 0) {
          nextLevel.push(dependent);
        }
      }
    }

    currentLevel = nextLevel;
    level++;
  }

  const reverseOrder = [...order].reverse();

  return { groups, order, reverseOrder };
}
