import { randomBytes, createCipheriv, createDecipheriv } from "node:crypto";
import { existsSync, readFileSync, unlinkSync } from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";
import { mkdirSecure, writeSecure } from "../utils.ts";

interface EncryptedValue {
  iv: string;
  tag: string;
  data: string;
}

type SecretsFile = Record<string, EncryptedValue>;

const ALGORITHM = "aes-256-gcm";
const IV_LENGTH = 12;

export class SecretsStore {
  private keysDir: string;
  private secretsDir: string;

  constructor() {
    const base = join(homedir(), ".vmsan");
    this.keysDir = join(base, "keys");
    this.secretsDir = join(base, "secrets");
  }

  /** Set a secret value for a project. */
  set(project: string, key: string, value: string): void {
    const encKey = this.ensureKey(project);
    const secrets = this.loadSecrets(project);

    const iv = randomBytes(IV_LENGTH);
    const cipher = createCipheriv(ALGORITHM, encKey, iv);
    const encrypted = Buffer.concat([cipher.update(value, "utf8"), cipher.final()]);
    const tag = cipher.getAuthTag();

    secrets[key] = {
      iv: iv.toString("hex"),
      tag: tag.toString("hex"),
      data: encrypted.toString("hex"),
    };

    this.saveSecrets(project, secrets);
  }

  /** Get a single secret value. Returns null if the key doesn't exist. */
  get(project: string, key: string): string | null {
    const secrets = this.loadSecrets(project);
    const entry = secrets[key];
    if (!entry) return null;

    const encKey = this.loadKey(project);
    if (!encKey) return null;

    return this.decrypt(encKey, entry);
  }

  /** List secret key names (not values) for a project. */
  list(project: string): string[] {
    const secrets = this.loadSecrets(project);
    return Object.keys(secrets).sort();
  }

  /** Remove a secret by key name. */
  unset(project: string, key: string): boolean {
    const secrets = this.loadSecrets(project);
    if (!(key in secrets)) return false;

    delete secrets[key];

    if (Object.keys(secrets).length === 0) {
      // Clean up empty files
      const secretsPath = this.secretsPath(project);
      if (existsSync(secretsPath)) {
        unlinkSync(secretsPath);
      }
    } else {
      this.saveSecrets(project, secrets);
    }

    return true;
  }

  /** Get all secrets as a key-value map (for injection into VM env). */
  getAll(project: string): Record<string, string> {
    const secrets = this.loadSecrets(project);
    const encKey = this.loadKey(project);
    if (!encKey) return {};

    const result: Record<string, string> = {};
    for (const [key, entry] of Object.entries(secrets)) {
      result[key] = this.decrypt(encKey, entry);
    }
    return result;
  }

  // --- Private helpers ---

  private keyPath(project: string): string {
    return join(this.keysDir, `${project}.key`);
  }

  private secretsPath(project: string): string {
    return join(this.secretsDir, `${project}.enc`);
  }

  private ensureKey(project: string): Buffer {
    const existing = this.loadKey(project);
    if (existing) return existing;

    mkdirSecure(this.keysDir);
    const key = randomBytes(32);
    writeSecure(this.keyPath(project), key.toString("hex"));
    return key;
  }

  private loadKey(project: string): Buffer | null {
    const path = this.keyPath(project);
    if (!existsSync(path)) return null;
    const hex = readFileSync(path, "utf8").trim();
    return Buffer.from(hex, "hex");
  }

  private loadSecrets(project: string): SecretsFile {
    const path = this.secretsPath(project);
    if (!existsSync(path)) return {};
    try {
      return JSON.parse(readFileSync(path, "utf8")) as SecretsFile;
    } catch {
      return {};
    }
  }

  private saveSecrets(project: string, secrets: SecretsFile): void {
    mkdirSecure(this.secretsDir);
    writeSecure(this.secretsPath(project), JSON.stringify(secrets, null, 2));
  }

  private decrypt(key: Buffer, entry: EncryptedValue): string {
    const iv = Buffer.from(entry.iv, "hex");
    const tag = Buffer.from(entry.tag, "hex");
    const data = Buffer.from(entry.data, "hex");

    const decipher = createDecipheriv(ALGORITHM, key, iv);
    decipher.setAuthTag(tag);
    return Buffer.concat([decipher.update(data), decipher.final()]).toString("utf8");
  }
}
