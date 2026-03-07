import { describe, expect, it } from "vitest";
import {
  generateVmId,
  parseDuration,
  toError,
  timeAgo,
  timeRemaining,
} from "../../src/lib/utils.ts";

// ---------- generateVmId ----------

describe("generateVmId", () => {
  it('generates an id starting with "vm-"', () => {
    const id = generateVmId();
    expect(id).toMatch(/^vm-/);
  });

  it("generates an 8-character hex suffix", () => {
    const id = generateVmId();
    const suffix = id.slice(3);
    expect(suffix).toMatch(/^[0-9a-f]{8}$/);
  });

  it("generates unique ids", () => {
    const ids = new Set<string>();
    for (let i = 0; i < 100; i++) {
      ids.add(generateVmId());
    }
    expect(ids.size).toBe(100);
  });

  it("has total length of 11 characters", () => {
    expect(generateVmId()).toHaveLength(11);
  });
});

// ---------- parseDuration ----------

describe("parseDuration", () => {
  it("plain number = minutes", () => {
    expect(parseDuration("5")).toBe(300_000);
  });

  it("parses 1h30m", () => {
    expect(parseDuration("1h30m")).toBe(5_400_000);
  });

  it("parses 0 as 0ms", () => {
    expect(parseDuration("0")).toBe(0);
  });

  it("throws on invalid input", () => {
    expect(() => parseDuration("abc")).toThrow();
    expect(() => parseDuration("")).toThrow();
  });

  it("parses single units correctly", () => {
    expect(parseDuration("1d")).toBe(86_400_000);
    expect(parseDuration("1h")).toBe(3_600_000);
    expect(parseDuration("1m")).toBe(60_000);
    expect(parseDuration("1s")).toBe(1_000);
  });
});

// ---------- toError ----------

describe("toError", () => {
  it("returns the same Error if given an Error", () => {
    const err = new Error("test");
    expect(toError(err)).toBe(err);
  });

  it("wraps a string into an Error", () => {
    const err = toError("something went wrong");
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("something went wrong");
  });

  it("wraps a number into an Error", () => {
    const err = toError(42);
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("42");
  });

  it("wraps null into an Error", () => {
    const err = toError(null);
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("null");
  });

  it("wraps undefined into an Error", () => {
    const err = toError(undefined);
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("undefined");
  });

  it("wraps an object into an Error", () => {
    const err = toError({ code: "ERR" });
    expect(err).toBeInstanceOf(Error);
    expect(err.message).toBe("[object Object]");
  });
});

// ---------- timeAgo ----------

describe("timeAgo", () => {
  it('returns "just now" for recent dates', () => {
    expect(timeAgo(new Date())).toBe("just now");
  });

  it("returns minutes ago", () => {
    const twoMinAgo = new Date(Date.now() - 120_000);
    expect(timeAgo(twoMinAgo)).toBe("2 minutes ago");
  });

  it("returns hours ago", () => {
    const threeHoursAgo = new Date(Date.now() - 3 * 3_600_000);
    expect(timeAgo(threeHoursAgo)).toBe("3 hours ago");
  });

  it("returns singular form", () => {
    const oneMinAgo = new Date(Date.now() - 60_000);
    expect(timeAgo(oneMinAgo)).toBe("1 minute ago");
  });

  it("accepts ISO string input", () => {
    const result = timeAgo(new Date(Date.now() - 120_000).toISOString());
    expect(result).toBe("2 minutes ago");
  });
});

// ---------- timeRemaining ----------

describe("timeRemaining", () => {
  it('returns "expired" for past dates', () => {
    const past = new Date(Date.now() - 60_000);
    expect(timeRemaining(past)).toBe("expired");
  });

  it("returns remaining time for future dates", () => {
    const future = new Date(Date.now() + 2 * 3_600_000 + 30_000);
    expect(timeRemaining(future)).toBe("in 2 hours");
  });

  it("returns singular form", () => {
    const future = new Date(Date.now() + 61_000);
    expect(timeRemaining(future)).toBe("in 1 minute");
  });
});
