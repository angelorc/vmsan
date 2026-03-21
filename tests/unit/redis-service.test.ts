import { describe, it, expect } from "vitest";
import {
  buildRedisConfig,
  getRedisEnvVars,
  getRedisHealthCheck,
  getRedisSetupCommands,
} from "../../src/lib/services/redis.ts";
import type { AccessoryConfig } from "../../src/lib/toml/parser.ts";

// ---------- buildRedisConfig ----------

describe("buildRedisConfig", () => {
  it("defaults to no auth", () => {
    const accessory: AccessoryConfig = { type: "redis" };
    const config = buildRedisConfig("cache", accessory);
    expect(config.authEnabled).toBe(false);
    expect(config.password).toBeNull();
  });

  it("defaults to port 6379", () => {
    const accessory: AccessoryConfig = { type: "redis" };
    const config = buildRedisConfig("cache", accessory);
    expect(config.port).toBe(6379);
  });

  it("enables auth and generates password when auth=true", () => {
    const accessory: AccessoryConfig = {
      type: "redis",
      env: { auth: "true" },
    };
    const config = buildRedisConfig("cache", accessory);
    expect(config.authEnabled).toBe(true);
    expect(config.password).not.toBeNull();
    expect(config.password).toHaveLength(32);
  });

  it("generates unique passwords each time", () => {
    const accessory: AccessoryConfig = {
      type: "redis",
      env: { auth: "true" },
    };
    const passwords = new Set<string>();
    for (let i = 0; i < 20; i++) {
      passwords.add(buildRedisConfig("cache", accessory).password!);
    }
    expect(passwords.size).toBe(20);
  });

  it("respects custom port from accessory.env", () => {
    const accessory: AccessoryConfig = {
      type: "redis",
      env: { port: "6380" },
    };
    const config = buildRedisConfig("cache", accessory);
    expect(config.port).toBe(6380);
  });

  it("does not enable auth when auth is not 'true'", () => {
    const accessory: AccessoryConfig = {
      type: "redis",
      env: { auth: "false" },
    };
    const config = buildRedisConfig("cache", accessory);
    expect(config.authEnabled).toBe(false);
    expect(config.password).toBeNull();
  });
});

// ---------- getRedisEnvVars ----------

describe("getRedisEnvVars", () => {
  it("includes REDIS_URL without auth", () => {
    const config = { password: null, port: 6379, authEnabled: false };
    const vars = getRedisEnvVars(config, "10.0.0.2");
    expect(vars.REDIS_URL).toBe("redis://10.0.0.2:6379");
    expect(vars.REDIS_HOST).toBe("10.0.0.2");
    expect(vars.REDIS_PORT).toBe("6379");
  });

  it("does not include REDIS_PASSWORD when auth disabled", () => {
    const config = { password: null, port: 6379, authEnabled: false };
    const vars = getRedisEnvVars(config, "10.0.0.2");
    expect(vars.REDIS_PASSWORD).toBeUndefined();
  });

  it("includes REDIS_URL with auth credentials", () => {
    const config = { password: "mypassword", port: 6379, authEnabled: true };
    const vars = getRedisEnvVars(config, "10.0.0.2");
    expect(vars.REDIS_URL).toBe("redis://:mypassword@10.0.0.2:6379");
  });

  it("includes REDIS_PASSWORD when auth enabled", () => {
    const config = { password: "mypassword", port: 6379, authEnabled: true };
    const vars = getRedisEnvVars(config, "10.0.0.2");
    expect(vars.REDIS_PASSWORD).toBe("mypassword");
  });

  it("uses custom port in URL", () => {
    const config = { password: null, port: 6380, authEnabled: false };
    const vars = getRedisEnvVars(config, "10.0.0.2");
    expect(vars.REDIS_URL).toBe("redis://10.0.0.2:6380");
    expect(vars.REDIS_PORT).toBe("6380");
  });
});

// ---------- getRedisHealthCheck ----------

describe("getRedisHealthCheck", () => {
  it("returns TCP check on port 6379", () => {
    const check = getRedisHealthCheck();
    expect(check.type).toBe("tcp");
    expect(check.port).toBe(6379);
  });

  it("includes interval, timeout, and retries", () => {
    const check = getRedisHealthCheck();
    expect(check.interval).toBeGreaterThan(0);
    expect(check.timeout).toBeGreaterThan(0);
    expect(check.retries).toBeGreaterThan(0);
  });
});

// ---------- getRedisSetupCommands ----------

describe("getRedisSetupCommands", () => {
  it("returns empty array when auth disabled", () => {
    const config = { password: null, port: 6379, authEnabled: false };
    const cmds = getRedisSetupCommands(config);
    expect(cmds).toEqual([]);
  });

  it("returns requirepass command when auth enabled", () => {
    const config = { password: "mypassword", port: 6379, authEnabled: true };
    const cmds = getRedisSetupCommands(config);
    expect(cmds).toHaveLength(1);
    expect(cmds[0]).toContain("requirepass");
    expect(cmds[0]).toContain("mypassword");
  });

  it("returns empty array when authEnabled is true but password is null", () => {
    const config = { password: null, port: 6379, authEnabled: true };
    const cmds = getRedisSetupCommands(config);
    expect(cmds).toEqual([]);
  });
});
