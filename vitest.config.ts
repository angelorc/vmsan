import { defineConfig } from "vitest/config";

export default defineConfig({
  test: {
    include: ["tests/unit/**/*.test.ts"],
    testTimeout: 180_000,
    hookTimeout: 180_000,
    pool: "forks",
    forks: {
      singleFork: true,
    },
    reporters: ["verbose"],
  },
});
