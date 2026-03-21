import { describe, expect, it } from "vitest";
import { buildDependencyGraph } from "../../src/lib/deploy/graph.ts";

// ---------- Empty & minimal inputs ----------

describe("buildDependencyGraph – empty & minimal", () => {
  it("returns empty graph for no services and no accessories", () => {
    const graph = buildDependencyGraph({});
    expect(graph.groups).toEqual([]);
    expect(graph.order).toEqual([]);
    expect(graph.reverseOrder).toEqual([]);
  });

  it("handles single service with no dependencies", () => {
    const graph = buildDependencyGraph({ app: {} });
    expect(graph.groups).toEqual([{ services: ["app"], level: 0 }]);
    expect(graph.order).toEqual(["app"]);
    expect(graph.reverseOrder).toEqual(["app"]);
  });

  it("handles multiple independent services", () => {
    const graph = buildDependencyGraph({
      web: {},
      worker: {},
      cron: {},
    });
    expect(graph.groups).toEqual([{ services: ["cron", "web", "worker"], level: 0 }]);
    expect(graph.order).toEqual(["cron", "web", "worker"]);
  });
});

// ---------- Linear dependencies ----------

describe("buildDependencyGraph – linear chain", () => {
  it("sorts a simple A → B chain", () => {
    const graph = buildDependencyGraph({
      web: { depends_on: ["db"] },
      db: {},
    });
    expect(graph.groups).toEqual([
      { services: ["db"], level: 0 },
      { services: ["web"], level: 1 },
    ]);
    expect(graph.order).toEqual(["db", "web"]);
    expect(graph.reverseOrder).toEqual(["web", "db"]);
  });

  it("sorts a three-level chain A → B → C", () => {
    const graph = buildDependencyGraph({
      web: { depends_on: ["api"] },
      api: { depends_on: ["db"] },
      db: {},
    });
    expect(graph.groups).toEqual([
      { services: ["db"], level: 0 },
      { services: ["api"], level: 1 },
      { services: ["web"], level: 2 },
    ]);
    expect(graph.order).toEqual(["db", "api", "web"]);
    expect(graph.reverseOrder).toEqual(["web", "api", "db"]);
  });
});

// ---------- Fan-out & fan-in (diamond) ----------

describe("buildDependencyGraph – fan-out & diamond", () => {
  it("groups services that share dependencies (fan-out)", () => {
    const graph = buildDependencyGraph({
      web: { depends_on: ["db", "cache"] },
      worker: { depends_on: ["db"] },
      db: {},
      cache: {},
    });
    expect(graph.groups).toEqual([
      { services: ["cache", "db"], level: 0 },
      { services: ["web", "worker"], level: 1 },
    ]);
    expect(graph.order).toEqual(["cache", "db", "web", "worker"]);
    expect(graph.reverseOrder).toEqual(["worker", "web", "db", "cache"]);
  });

  it("handles diamond dependency (A → B, A → C, B → D, C → D)", () => {
    const graph = buildDependencyGraph({
      a: { depends_on: ["b", "c"] },
      b: { depends_on: ["d"] },
      c: { depends_on: ["d"] },
      d: {},
    });
    expect(graph.groups).toEqual([
      { services: ["d"], level: 0 },
      { services: ["b", "c"], level: 1 },
      { services: ["a"], level: 2 },
    ]);
    expect(graph.order).toEqual(["d", "b", "c", "a"]);
    expect(graph.reverseOrder).toEqual(["a", "c", "b", "d"]);
  });
});

// ---------- Accessories ----------

describe("buildDependencyGraph – accessories", () => {
  it("places accessories at level 0", () => {
    const graph = buildDependencyGraph(
      { web: { depends_on: ["db"] } },
      { db: { type: "postgres" }, cache: { type: "redis" } },
    );
    expect(graph.groups).toEqual([
      { services: ["cache", "db"], level: 0 },
      { services: ["web"], level: 1 },
    ]);
  });

  it("accessories with no dependent services still appear", () => {
    const graph = buildDependencyGraph({ web: {} }, { cache: { type: "redis" } });
    // Both are level 0 since web has no deps and cache is an accessory
    expect(graph.groups).toEqual([{ services: ["cache", "web"], level: 0 }]);
  });

  it("service can depend on accessory", () => {
    const graph = buildDependencyGraph(
      {
        web: { depends_on: ["db"] },
        worker: { depends_on: ["db", "cache"] },
      },
      {
        db: { type: "postgres" },
        cache: { type: "redis" },
      },
    );
    expect(graph.groups).toEqual([
      { services: ["cache", "db"], level: 0 },
      { services: ["web", "worker"], level: 1 },
    ]);
  });
});

// ---------- Cycle detection ----------

describe("buildDependencyGraph – cycle detection", () => {
  it("detects simple two-node cycle", () => {
    expect(() =>
      buildDependencyGraph({
        a: { depends_on: ["b"] },
        b: { depends_on: ["a"] },
      }),
    ).toThrow(/Circular dependency detected/);
  });

  it("detects three-node cycle and shows the path", () => {
    expect(() =>
      buildDependencyGraph({
        web: { depends_on: ["api"] },
        api: { depends_on: ["db"] },
        db: { depends_on: ["web"] },
      }),
    ).toThrow(/Circular dependency detected/);
  });

  it("detects self-dependency", () => {
    expect(() =>
      buildDependencyGraph({
        web: { depends_on: ["web"] },
      }),
    ).toThrow(/Circular dependency detected/);
  });

  it("cycle error message contains the arrow symbol", () => {
    try {
      buildDependencyGraph({
        a: { depends_on: ["b"] },
        b: { depends_on: ["a"] },
      });
      expect.fail("should have thrown");
    } catch (err) {
      expect((err as Error).message).toContain("\u2192");
    }
  });
});

// ---------- Unknown reference validation ----------

describe("buildDependencyGraph – unknown references", () => {
  it("throws when depends_on references unknown service", () => {
    expect(() =>
      buildDependencyGraph({
        web: { depends_on: ["database"] },
        db: {},
      }),
    ).toThrow(/depends on unknown service 'database'/);
  });

  it("error includes available service names", () => {
    try {
      buildDependencyGraph({
        web: { depends_on: ["database"] },
        db: {},
        cache: {},
      });
      expect.fail("should have thrown");
    } catch (err) {
      const msg = (err as Error).message;
      expect(msg).toContain("Available services:");
      expect(msg).toContain("cache");
      expect(msg).toContain("db");
      expect(msg).toContain("web");
    }
  });

  it("accessory names are valid dependency targets", () => {
    // Should NOT throw — db is an accessory
    const graph = buildDependencyGraph(
      { web: { depends_on: ["db"] } },
      { db: { type: "postgres" } },
    );
    expect(graph.order).toEqual(["db", "web"]);
  });
});

// ---------- Determinism ----------

describe("buildDependencyGraph – deterministic output", () => {
  it("services within a group are sorted alphabetically", () => {
    const graph = buildDependencyGraph({
      zebra: {},
      alpha: {},
      middle: {},
    });
    expect(graph.groups[0].services).toEqual(["alpha", "middle", "zebra"]);
  });

  it("produces stable output across multiple calls", () => {
    const input = {
      c: { depends_on: ["a"] },
      b: { depends_on: ["a"] },
      a: {},
    };
    const g1 = buildDependencyGraph(input);
    const g2 = buildDependencyGraph(input);
    expect(g1).toEqual(g2);
  });
});
