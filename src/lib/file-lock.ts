import { mkdirSync } from "node:fs";
import { dirname } from "node:path";
import { lockSync, lock, type LockOptions } from "proper-lockfile";
import { lockTimeoutError } from "../errors/index.ts";

const STALE_MS = 300_000;
const RETRY_MS = 50;
const MAX_RETRIES = 600;
const WAIT_ARRAY = new Int32Array(new SharedArrayBuffer(4));

interface FileLockOptions {
  stale?: number;
  retries?: number;
  realpath?: boolean;
}

export class FileLock {
  private readonly stale: number;
  private readonly retries: number;
  private readonly realpath: boolean;

  constructor(
    private readonly path: string,
    private readonly name: string,
    options?: FileLockOptions,
  ) {
    this.stale = options?.stale ?? STALE_MS;
    this.retries = options?.retries ?? MAX_RETRIES;
    this.realpath = options?.realpath ?? false;
  }

  run<T>(fn: () => T): T {
    mkdirSync(dirname(this.path), { recursive: true });
    const syncOpts: LockOptions = { stale: this.stale, realpath: this.realpath };
    let release: (() => void) | undefined;

    for (let attempt = 0; ; attempt++) {
      try {
        release = lockSync(this.path, syncOpts);
        break;
      } catch (error) {
        if ((error as NodeJS.ErrnoException).code !== "ELOCKED") throw error;
        if (attempt >= this.retries) throw lockTimeoutError(this.name);
        Atomics.wait(WAIT_ARRAY, 0, 0, RETRY_MS);
      }
    }
    try {
      return fn();
    } finally {
      release!();
    }
  }

  async runAsync<T>(fn: () => Promise<T>): Promise<T> {
    mkdirSync(dirname(this.path), { recursive: true });
    const asyncOpts: LockOptions = {
      stale: this.stale,
      realpath: this.realpath,
      retries: { retries: this.retries, minTimeout: RETRY_MS, maxTimeout: RETRY_MS, factor: 1 },
    };
    let release: (() => Promise<void>) | undefined;
    try {
      release = await lock(this.path, asyncOpts);
    } catch (error) {
      if ((error as NodeJS.ErrnoException).code === "ELOCKED") {
        throw lockTimeoutError(this.name);
      }
      throw error;
    }
    try {
      return await fn();
    } finally {
      await release();
    }
  }
}
