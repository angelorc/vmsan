import { resolve, basename } from "node:path";
import { existsSync } from "node:fs";
import { loadVmsanToml, type VmsanToml } from "./toml/parser.ts";

export interface ProjectConfig {
  config: VmsanToml;
  configPath: string;
  sourceDir: string;
  project: string;
}

export function loadProjectConfig(configArg?: string): ProjectConfig {
  const configPath = resolve(configArg || "vmsan.toml");
  if (!existsSync(configPath)) {
    throw new Error(`Configuration file not found: ${configPath}. Run "vmsan init" to create one.`);
  }
  const config = loadVmsanToml(configPath);
  const sourceDir = resolve(configPath, "..");
  const project = config.project || basename(sourceDir);
  return { config, configPath, sourceDir, project };
}
