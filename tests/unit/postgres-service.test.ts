import { describe, it, expect } from "vitest";
import {
  buildPostgresConfig,
  getPostgresEnvVars,
  getPostgresHealthCheck,
  getPostgresSetupCommands,
} from "../../src/lib/services/postgres.ts";
import type { AccessoryConfig } from "../../src/lib/toml/parser.ts";

// ---------- buildPostgresConfig ----------

describe("buildPostgresConfig", () => {
  it("uses default port 5432", () => {
    const accessory: AccessoryConfig = { type: "postgres" };
    const config = buildPostgresConfig("mydb", accessory);
    expect(config.port).toBe(5432);
  });

  it("uses default user 'vmsan'", () => {
    const accessory: AccessoryConfig = { type: "postgres" };
    const config = buildPostgresConfig("mydb", accessory);
    expect(config.user).toBe("vmsan");
  });

  it("uses service name as default database name", () => {
    const accessory: AccessoryConfig = { type: "postgres" };
    const config = buildPostgresConfig("mydb", accessory);
    expect(config.database).toBe("mydb");
  });

  it("generates a 32-character password", () => {
    const accessory: AccessoryConfig = { type: "postgres" };
    const config = buildPostgresConfig("mydb", accessory);
    expect(config.password).toHaveLength(32);
  });

  it("generates a random password each time", () => {
    const accessory: AccessoryConfig = { type: "postgres" };
    const passwords = new Set<string>();
    for (let i = 0; i < 20; i++) {
      passwords.add(buildPostgresConfig("mydb", accessory).password);
    }
    expect(passwords.size).toBe(20);
  });

  it("respects overrides from accessory.env", () => {
    const accessory: AccessoryConfig = {
      type: "postgres",
      env: {
        database: "custom_db",
        user: "custom_user",
        port: "5433",
      },
    };
    const config = buildPostgresConfig("mydb", accessory);
    expect(config.database).toBe("custom_db");
    expect(config.user).toBe("custom_user");
    expect(config.port).toBe(5433);
  });

  it("uses service name when accessory.env exists but database is not set", () => {
    const accessory: AccessoryConfig = {
      type: "postgres",
      env: { user: "other" },
    };
    const config = buildPostgresConfig("fallback-name", accessory);
    expect(config.database).toBe("fallback-name");
  });
});

// ---------- getPostgresEnvVars ----------

describe("getPostgresEnvVars", () => {
  it("includes DATABASE_URL with correct format", () => {
    const config = {
      database: "testdb",
      user: "vmsan",
      password: "secret123",
      port: 5432,
    };
    const vars = getPostgresEnvVars(config, "10.0.0.1");
    expect(vars.DATABASE_URL).toBe("postgresql://vmsan:secret123@10.0.0.1:5432/testdb");
  });

  it("includes all PG* environment variables", () => {
    const config = {
      database: "testdb",
      user: "vmsan",
      password: "secret123",
      port: 5432,
    };
    const vars = getPostgresEnvVars(config, "10.0.0.1");
    expect(vars.PGHOST).toBe("10.0.0.1");
    expect(vars.PGPORT).toBe("5432");
    expect(vars.PGUSER).toBe("vmsan");
    expect(vars.PGPASSWORD).toBe("secret123");
    expect(vars.PGDATABASE).toBe("testdb");
  });

  it("uses the provided mesh IP address", () => {
    const config = {
      database: "db",
      user: "u",
      password: "p",
      port: 5433,
    };
    const vars = getPostgresEnvVars(config, "192.168.1.100");
    expect(vars.DATABASE_URL).toContain("192.168.1.100:5433");
    expect(vars.PGHOST).toBe("192.168.1.100");
  });
});

// ---------- getPostgresHealthCheck ----------

describe("getPostgresHealthCheck", () => {
  it("returns TCP check on port 5432", () => {
    const check = getPostgresHealthCheck();
    expect(check.type).toBe("tcp");
    expect(check.port).toBe(5432);
  });

  it("includes interval, timeout, and retries", () => {
    const check = getPostgresHealthCheck();
    expect(check.interval).toBeGreaterThan(0);
    expect(check.timeout).toBeGreaterThan(0);
    expect(check.retries).toBeGreaterThan(0);
  });
});

// ---------- getPostgresSetupCommands ----------

describe("getPostgresSetupCommands", () => {
  it("includes pg_isready check", () => {
    const config = {
      database: "testdb",
      user: "vmsan",
      password: "secret",
      port: 5432,
    };
    const cmds = getPostgresSetupCommands(config);
    expect(cmds.some((c) => c.includes("pg_isready"))).toBe(true);
  });

  it("includes CREATE USER command with correct user and password", () => {
    const config = {
      database: "testdb",
      user: "vmsan",
      password: "secret",
      port: 5432,
    };
    const cmds = getPostgresSetupCommands(config);
    const createUser = cmds.find((c) => c.includes("CREATE USER"));
    expect(createUser).toBeDefined();
    expect(createUser).toContain("vmsan");
    expect(createUser).toContain("secret");
  });

  it("includes CREATE DATABASE command with correct db and owner", () => {
    const config = {
      database: "testdb",
      user: "vmsan",
      password: "secret",
      port: 5432,
    };
    const cmds = getPostgresSetupCommands(config);
    const createDb = cmds.find((c) => c.includes("CREATE DATABASE"));
    expect(createDb).toBeDefined();
    expect(createDb).toContain("testdb");
    expect(createDb).toContain("OWNER vmsan");
  });
});
