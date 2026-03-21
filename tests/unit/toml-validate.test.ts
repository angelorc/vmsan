import { describe, test, expect } from "vitest";
import { parseTomlSafe, validateToml } from "../../src/lib/toml/validate.ts";
import type { VmsanToml } from "../../src/lib/toml/types.ts";

describe("parseTomlSafe", () => {
  test("parses valid TOML", () => {
    const toml = `
[services.web]
runtime = "node22"
start = "npm start"
`;
    const { config, errors } = parseTomlSafe(toml);
    expect(errors).toHaveLength(0);
    expect(config).not.toBeNull();
    expect(config!.services!.web.runtime).toBe("node22");
  });

  test("returns syntax errors for invalid TOML", () => {
    const toml = `
[services.web
runtime = "node22"
`;
    const { config, errors } = parseTomlSafe(toml);
    expect(config).toBeNull();
    expect(errors).toHaveLength(1);
    expect(errors[0].field).toBe("syntax");
  });
});

describe("validateToml", () => {
  test("valid single-service config passes", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "npm start",
        },
      },
    };

    const errors = validateToml(config);
    expect(errors).toHaveLength(0);
  });

  test("valid multi-service config passes", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "npm start",
          depends_on: ["db"],
          connect_to: ["db:5432"],
        },
      },
      accessories: {
        db: {
          type: "postgres",
        },
      },
    };

    const errors = validateToml(config);
    expect(errors).toHaveLength(0);
  });

  test("missing dependency detected", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "npm start",
          depends_on: ["redi"],
        },
      },
      accessories: {
        redis: {
          type: "redis",
        },
      },
    };

    const errors = validateToml(config);
    expect(errors.length).toBeGreaterThan(0);
    const depError = errors.find((e) => e.field.includes("depends_on"));
    expect(depError).toBeDefined();
    expect(depError!.message).toContain('"redi"');
    expect(depError!.suggestion).toContain("redis");
  });

  test("circular dependency detected", () => {
    const config: VmsanToml = {
      services: {
        a: {
          runtime: "node22",
          start: "start-a",
          depends_on: ["b"],
        },
        b: {
          runtime: "node22",
          start: "start-b",
          depends_on: ["a"],
        },
      },
    };

    const errors = validateToml(config);
    const circularError = errors.find((e) => e.message.includes("Circular dependency"));
    expect(circularError).toBeDefined();
    expect(circularError!.message).toContain("a");
    expect(circularError!.message).toContain("b");
  });

  test("invalid port detected", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "npm start",
          connect_to: ["db:99999"],
        },
      },
      accessories: {
        db: {
          type: "postgres",
        },
      },
    };

    const errors = validateToml(config);
    const portError = errors.find((e) => e.message.includes("Invalid port"));
    expect(portError).toBeDefined();
    expect(portError!.message).toContain("99999");
  });

  test("non-numeric port detected", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "npm start",
          connect_to: ["db:abc"],
        },
      },
      accessories: {
        db: {
          type: "postgres",
        },
      },
    };

    const errors = validateToml(config);
    const portError = errors.find((e) => e.message.includes("Invalid port"));
    expect(portError).toBeDefined();
    expect(portError!.message).toContain("abc");
  });

  test("unknown runtime detected", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node19",
          start: "npm start",
        },
      },
    };

    const errors = validateToml(config);
    const runtimeError = errors.find((e) => e.field.includes("runtime"));
    expect(runtimeError).toBeDefined();
    expect(runtimeError!.message).toContain("node19");
    expect(runtimeError!.suggestion).toContain("node22");
  });

  test("missing start command detected", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "",
        },
      },
    };

    const errors = validateToml(config);
    const startError = errors.find((e) => e.field.includes("start"));
    expect(startError).toBeDefined();
    expect(startError!.message).toContain("missing");
  });

  test("duplicate service names across services and accessories", () => {
    const config: VmsanToml = {
      services: {
        db: {
          runtime: "node22",
          start: "npm start",
        },
      },
      accessories: {
        db: {
          type: "postgres",
        },
      },
    };

    const errors = validateToml(config);
    const dupError = errors.find((e) => e.message.includes("Duplicate"));
    expect(dupError).toBeDefined();
    expect(dupError!.message).toContain("db");
  });

  test("connect_to without colon format is rejected", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "npm start",
          connect_to: ["dbnoport"],
        },
      },
    };

    const errors = validateToml(config);
    const formatError = errors.find((e) => e.message.includes("expected format"));
    expect(formatError).toBeDefined();
  });

  test("connect_to referencing unknown service is detected", () => {
    const config: VmsanToml = {
      services: {
        web: {
          runtime: "node22",
          start: "npm start",
          connect_to: ["unknown:5432"],
        },
      },
    };

    const errors = validateToml(config);
    const refError = errors.find(
      (e) => e.field.includes("connect_to") && e.message.includes("unknown"),
    );
    expect(refError).toBeDefined();
  });

  test("empty config with no services passes", () => {
    const config: VmsanToml = {};
    const errors = validateToml(config);
    expect(errors).toHaveLength(0);
  });

  test("valid runtimes pass validation", () => {
    for (const runtime of ["base", "node22", "node24", "python3.13"]) {
      const config: VmsanToml = {
        services: {
          web: {
            runtime,
            start: "start",
          },
        },
      };
      const errors = validateToml(config);
      const runtimeErrors = errors.filter((e) => e.field.includes("runtime"));
      expect(runtimeErrors).toHaveLength(0);
    }
  });

  test("three-way circular dependency detected", () => {
    const config: VmsanToml = {
      services: {
        a: { runtime: "node22", start: "a", depends_on: ["b"] },
        b: { runtime: "node22", start: "b", depends_on: ["c"] },
        c: { runtime: "node22", start: "c", depends_on: ["a"] },
      },
    };

    const errors = validateToml(config);
    const circularError = errors.find((e) => e.message.includes("Circular dependency"));
    expect(circularError).toBeDefined();
  });
});
