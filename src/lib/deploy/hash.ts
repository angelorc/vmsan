import { existsSync, readFileSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { homedir } from "node:os";
import { mkdirSecure } from "../utils.ts";
import { toError } from "../utils.ts";
import consola from "consola";

const HASH_FILE = join(homedir(), ".vmsan", "deploy-hashes.json");

function readHashFile(): Record<string, string> {
  if (!existsSync(HASH_FILE)) return {};
  try {
    return JSON.parse(readFileSync(HASH_FILE, "utf-8"));
  } catch (err) {
    consola.debug(`Failed to read deploy hash file: ${toError(err).message}`);
    return {};
  }
}

function writeHashFile(data: Record<string, string>): void {
  mkdirSecure(join(homedir(), ".vmsan"));
  writeFileSync(HASH_FILE, JSON.stringify(data, null, 2));
}

export function getDeployHash(vmId: string): string | null {
  const data = readHashFile();
  return data[vmId] ?? null;
}

export function setDeployHash(vmId: string, hash: string): void {
  const data = readHashFile();
  data[vmId] = hash;
  writeHashFile(data);
}

export function removeDeployHash(vmId: string): void {
  const data = readHashFile();
  delete data[vmId];
  writeHashFile(data);
}
