import { describe, expect, it } from "vitest";
import { parseVmsanToml, isMultiService, normalizeToml } from "../../src/lib/toml/parser.ts";

// ---------- Single-service TOML ----------

describe("parseVmsanToml – single-service", () => {
  it("parses flat runtime/build/start", () => {
    const toml = `
runtime = "node22"
build = "npm install && npm run build"
start = "node dist/server.js"
`;
    const config = parseVmsanToml(toml);
    expect(config.runtime).toBe("node22");
    expect(config.build).toBe("npm install && npm run build");
    expect(config.start).toBe("node dist/server.js");
  });

  it("parses minimal config with only runtime", () => {
    const config = parseVmsanToml('runtime = "node22"');
    expect(config.runtime).toBe("node22");
    expect(config.build).toBeUndefined();
    expect(config.start).toBeUndefined();
  });

  it("parses empty TOML as valid", () => {
    const config = parseVmsanToml("");
    expect(config).toEqual({});
  });
});

// ---------- Multi-service TOML ----------

describe("parseVmsanToml – multi-service", () => {
  it("parses services with depends_on and connect_to", () => {
    const toml = `
[services.web]
runtime = "node22"
build = "npm install && npm run build"
start = "node dist/server.js"
depends_on = ["db"]
connect_to = ["db:5432"]

[services.worker]
runtime = "node22"
start = "node dist/worker.js"
depends_on = ["db", "redis"]
`;
    const config = parseVmsanToml(toml);
    expect(config.services).toBeDefined();
    expect(config.services!.web.runtime).toBe("node22");
    expect(config.services!.web.depends_on).toEqual(["db"]);
    expect(config.services!.web.connect_to).toEqual(["db:5432"]);
    expect(config.services!.worker.depends_on).toEqual(["db", "redis"]);
  });

  it("parses service with all optional fields", () => {
    const toml = `
[services.api]
runtime = "node22"
build = "npm run build"
start = "node server.js"
service = "web"
publish_ports = ["3000:3000", "3001:3001"]
memory = 512
vcpus = 2
disk = "4gb"
network_policy = "custom"
allowed_domains = ["api.example.com", "cdn.example.com"]

[services.api.env]
NODE_ENV = "production"
PORT = "3000"
`;
    const config = parseVmsanToml(toml);
    const api = config.services!.api;
    expect(api.service).toBe("web");
    expect(api.publish_ports).toEqual(["3000:3000", "3001:3001"]);
    expect(api.memory).toBe(512);
    expect(api.vcpus).toBe(2);
    expect(api.disk).toBe("4gb");
    expect(api.network_policy).toBe("custom");
    expect(api.allowed_domains).toEqual(["api.example.com", "cdn.example.com"]);
    expect(api.env).toEqual({ NODE_ENV: "production", PORT: "3000" });
  });
});

// ---------- Accessories ----------

describe("parseVmsanToml – accessories", () => {
  it("parses accessories with type and version", () => {
    const toml = `
[accessories.db]
type = "postgres"
version = "16"

[accessories.redis]
type = "redis"
`;
    const config = parseVmsanToml(toml);
    expect(config.accessories).toBeDefined();
    expect(config.accessories!.db.type).toBe("postgres");
    expect(config.accessories!.db.version).toBe("16");
    expect(config.accessories!.redis.type).toBe("redis");
  });

  it("parses accessory with env vars", () => {
    const toml = `
[accessories.db]
type = "postgres"

[accessories.db.env]
POSTGRES_PASSWORD = "secret"
POSTGRES_DB = "myapp"
`;
    const config = parseVmsanToml(toml);
    expect(config.accessories!.db.env).toEqual({
      POSTGRES_PASSWORD: "secret",
      POSTGRES_DB: "myapp",
    });
  });

  it("rejects accessory without type", () => {
    const toml = `
[accessories.db]
version = "16"
`;
    expect(() => parseVmsanToml(toml)).toThrow('Missing required field "type" in accessories.db');
  });
});

// ---------- Deploy config ----------

describe("parseVmsanToml – deploy", () => {
  it("parses deploy.release", () => {
    const toml = `
[deploy]
release = "npx prisma migrate deploy"
`;
    const config = parseVmsanToml(toml);
    expect(config.deploy?.release).toBe("npx prisma migrate deploy");
  });
});

// ---------- Tunnel config ----------

describe("parseVmsanToml – tunnel", () => {
  it("parses tunnel with single hostname", () => {
    const toml = `
[tunnel]
hostname = "app.example.com"
`;
    const config = parseVmsanToml(toml);
    expect(config.tunnel?.hostname).toBe("app.example.com");
  });

  it("parses tunnel with multiple hostnames", () => {
    const toml = `
[tunnel]
hostnames = ["app.example.com", "api.example.com"]
`;
    const config = parseVmsanToml(toml);
    expect(config.tunnel?.hostnames).toEqual(["app.example.com", "api.example.com"]);
  });
});

// ---------- Health checks ----------

describe("parseVmsanToml – health checks", () => {
  it("parses HTTP health check", () => {
    const toml = `
[services.web]
runtime = "node22"
start = "node server.js"

[services.web.health_check]
type = "http"
path = "/health"
port = 3000
interval = 10
timeout = 5
retries = 3
`;
    const config = parseVmsanToml(toml);
    const hc = config.services!.web.health_check!;
    expect(hc.type).toBe("http");
    expect(hc.path).toBe("/health");
    expect(hc.port).toBe(3000);
    expect(hc.interval).toBe(10);
    expect(hc.timeout).toBe(5);
    expect(hc.retries).toBe(3);
  });

  it("parses TCP health check", () => {
    const toml = `
[services.db]
runtime = "base"
start = "postgres"

[services.db.health_check]
type = "tcp"
port = 5432
`;
    const config = parseVmsanToml(toml);
    expect(config.services!.db.health_check!.type).toBe("tcp");
    expect(config.services!.db.health_check!.port).toBe(5432);
  });

  it("parses exec health check", () => {
    const toml = `
[services.app]
runtime = "base"
start = "./app"

[services.app.health_check]
type = "exec"
command = "curl -f http://localhost:3000/health"
`;
    const config = parseVmsanToml(toml);
    expect(config.services!.app.health_check!.type).toBe("exec");
    expect(config.services!.app.health_check!.command).toBe("curl -f http://localhost:3000/health");
  });
});

// ---------- Error handling ----------

describe("parseVmsanToml – errors", () => {
  it("throws on invalid TOML syntax", () => {
    expect(() => parseVmsanToml("[invalid")).toThrow("Invalid TOML");
  });

  it("throws on unknown top-level field with suggestion", () => {
    expect(() => parseVmsanToml('runtme = "node22"')).toThrow(
      /Unknown field "runtme" in vmsan.toml.*Did you mean "runtime"/,
    );
  });

  it("throws on unknown service field with suggestion", () => {
    const toml = `
[services.web]
runtme = "node22"
`;
    expect(() => parseVmsanToml(toml)).toThrow(
      /Unknown field "runtme" in services.web.*Did you mean "runtime"/,
    );
  });

  it("throws on unknown service field without suggestion for very different name", () => {
    const toml = `
[services.web]
xyzzy = "foo"
`;
    expect(() => parseVmsanToml(toml)).toThrow(/Unknown field "xyzzy" in services.web/);
  });

  it("throws on unknown accessory field", () => {
    const toml = `
[accessories.db]
type = "postgres"
replicas = 3
`;
    expect(() => parseVmsanToml(toml)).toThrow(/Unknown field "replicas" in accessories.db/);
  });

  it("throws on unknown deploy field", () => {
    const toml = `
[deploy]
rollback = "true"
`;
    expect(() => parseVmsanToml(toml)).toThrow(/Unknown field "rollback" in deploy/);
  });

  it("throws on unknown tunnel field with suggestion", () => {
    const toml = `
[tunnel]
hostnam = "example.com"
`;
    expect(() => parseVmsanToml(toml)).toThrow(
      /Unknown field "hostnam" in tunnel.*Did you mean "hostname"/,
    );
  });

  it("throws on unknown tunnel field without suggestion", () => {
    const toml = `
[tunnel]
domain = "example.com"
`;
    expect(() => parseVmsanToml(toml)).toThrow(/Unknown field "domain" in tunnel/);
  });

  it("throws on unknown health_check field", () => {
    const toml = `
[services.web]
runtime = "node22"

[services.web.health_check]
type = "http"
url = "/health"
`;
    expect(() => parseVmsanToml(toml)).toThrow(/Unknown field "url" in services.web.health_check/);
  });

  it("throws on invalid health_check type", () => {
    const toml = `
[services.web]
runtime = "node22"

[services.web.health_check]
type = "grpc"
`;
    expect(() => parseVmsanToml(toml)).toThrow(/Invalid health_check type "grpc"/);
  });
});

// ---------- isMultiService ----------

describe("isMultiService", () => {
  it("returns false for flat config", () => {
    const config = parseVmsanToml('runtime = "node22"');
    expect(isMultiService(config)).toBe(false);
  });

  it("returns true for multi-service config", () => {
    const toml = `
[services.web]
runtime = "node22"
`;
    const config = parseVmsanToml(toml);
    expect(isMultiService(config)).toBe(true);
  });

  it("returns false for empty config", () => {
    expect(isMultiService(parseVmsanToml(""))).toBe(false);
  });
});

// ---------- normalizeToml ----------

describe("normalizeToml", () => {
  it("wraps flat config into services.app", () => {
    const config = parseVmsanToml(`
runtime = "node22"
build = "npm run build"
start = "node server.js"
`);
    const services = normalizeToml(config);
    expect(services).toHaveProperty("app");
    expect(services.app.runtime).toBe("node22");
    expect(services.app.build).toBe("npm run build");
    expect(services.app.start).toBe("node server.js");
  });

  it("returns services as-is for multi-service config", () => {
    const config = parseVmsanToml(`
[services.web]
runtime = "node22"
start = "node server.js"

[services.worker]
runtime = "node22"
start = "node worker.js"
`);
    const services = normalizeToml(config);
    expect(Object.keys(services)).toEqual(["web", "worker"]);
    expect(services.web.start).toBe("node server.js");
    expect(services.worker.start).toBe("node worker.js");
  });

  it("handles empty flat config", () => {
    const services = normalizeToml(parseVmsanToml(""));
    expect(services).toHaveProperty("app");
    expect(services.app).toEqual({});
  });
});
