import { describe, test, expect } from "vitest";
import { getServiceVariables, resolveReferences } from "../../src/lib/toml/references.ts";

describe("getServiceVariables", () => {
  test("postgres generates correct variables", () => {
    const vars = getServiceVariables("mydb", "postgres", "10.0.0.2");
    expect(vars).toEqual({
      DATABASE_URL: "postgres://vmsan:vmsan@10.0.0.2:5432/mydb",
      PGHOST: "10.0.0.2",
      PGPORT: "5432",
      PGUSER: "vmsan",
      PGPASSWORD: "vmsan",
      PGDATABASE: "mydb",
    });
  });

  test("redis generates correct variables", () => {
    const vars = getServiceVariables("cache", "redis", "10.0.0.3");
    expect(vars).toEqual({
      REDIS_URL: "redis://10.0.0.3:6379",
      REDIS_HOST: "10.0.0.3",
      REDIS_PORT: "6379",
    });
  });

  test("mysql generates correct variables", () => {
    const vars = getServiceVariables("mydb", "mysql", "10.0.0.4");
    expect(vars).toEqual({
      DATABASE_URL: "mysql://vmsan:vmsan@10.0.0.4:3306/mydb",
      MYSQL_HOST: "10.0.0.4",
      MYSQL_PORT: "3306",
      MYSQL_USER: "vmsan",
      MYSQL_PASSWORD: "vmsan",
      MYSQL_DATABASE: "mydb",
    });
  });

  test("unknown type generates generic URL and HOST variables", () => {
    const vars = getServiceVariables("api", "unknown", "10.0.0.5");
    expect(vars).toEqual({
      API_URL: "http://10.0.0.5:8080",
      API_HOST: "10.0.0.5",
    });
  });

  test("service name is uppercased for generic variables", () => {
    const vars = getServiceVariables("myService", "custom", "10.0.0.1");
    expect(vars).toHaveProperty("MYSERVICE_URL");
    expect(vars).toHaveProperty("MYSERVICE_HOST");
  });
});

describe("resolveReferences", () => {
  test("resolves ${{db.DATABASE_URL}} reference", () => {
    const env = {
      DB_URL: "${{db.DATABASE_URL}}",
    };
    const availableVars = {
      db: getServiceVariables("mydb", "postgres", "10.0.0.2"),
    };

    const resolved = resolveReferences(env, availableVars);
    expect(resolved.DB_URL).toBe("postgres://vmsan:vmsan@10.0.0.2:5432/mydb");
  });

  test("resolves multiple references in same value", () => {
    const env = {
      CONNECTION: "${{db.PGHOST}}:${{db.PGPORT}}",
    };
    const availableVars = {
      db: getServiceVariables("mydb", "postgres", "10.0.0.2"),
    };

    const resolved = resolveReferences(env, availableVars);
    expect(resolved.CONNECTION).toBe("10.0.0.2:5432");
  });

  test("resolves references across multiple services", () => {
    const env = {
      DB_URL: "${{db.DATABASE_URL}}",
      REDIS_URL: "${{cache.REDIS_URL}}",
    };
    const availableVars = {
      db: getServiceVariables("mydb", "postgres", "10.0.0.2"),
      cache: getServiceVariables("cache", "redis", "10.0.0.3"),
    };

    const resolved = resolveReferences(env, availableVars);
    expect(resolved.DB_URL).toBe("postgres://vmsan:vmsan@10.0.0.2:5432/mydb");
    expect(resolved.REDIS_URL).toBe("redis://10.0.0.3:6379");
  });

  test("passes through values without references unchanged", () => {
    const env = {
      NODE_ENV: "production",
      PORT: "3000",
    };
    const availableVars = {};

    const resolved = resolveReferences(env, availableVars);
    expect(resolved).toEqual({
      NODE_ENV: "production",
      PORT: "3000",
    });
  });

  test("throws on unknown service reference", () => {
    const env = {
      URL: "${{unknown.DATABASE_URL}}",
    };
    const availableVars = {
      db: getServiceVariables("mydb", "postgres", "10.0.0.2"),
    };

    expect(() => resolveReferences(env, availableVars)).toThrow(/Unknown service "unknown"/);
  });

  test("throws on unknown variable reference", () => {
    const env = {
      URL: "${{db.NONEXISTENT}}",
    };
    const availableVars = {
      db: getServiceVariables("mydb", "postgres", "10.0.0.2"),
    };

    expect(() => resolveReferences(env, availableVars)).toThrow(
      /Unknown variable "NONEXISTENT" for service "db"/,
    );
  });

  test("throws on invalid reference format (no dot)", () => {
    const env = {
      URL: "${{nodot}}",
    };
    const availableVars = {};

    expect(() => resolveReferences(env, availableVars)).toThrow(/expected format "service.VAR"/);
  });
});
