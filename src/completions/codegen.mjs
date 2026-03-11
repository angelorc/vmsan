#!/usr/bin/env node
// Reads shell completion source files and writes TypeScript string-constant modules.
// Edit the source files (.sh, .fish, .ps1) then run: bun run build

import { mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const dir = dirname(fileURLToPath(import.meta.url));
const outDir = join(dir, "generated");
mkdirSync(outDir, { recursive: true });

const shells = [
  { src: "bash.sh", out: "bash.ts", exportName: "bashCompletionScript" },
  { src: "zsh.sh", out: "zsh.ts", exportName: "zshCompletionScript" },
  { src: "fish.fish", out: "fish.ts", exportName: "fishCompletionScript" },
  { src: "powershell.ps1", out: "powershell.ts", exportName: "powershellCompletionScript" },
];

for (const { src, out, exportName } of shells) {
  const content = readFileSync(join(dir, src), "utf8");
  const ts = [
    `// Generated from src/completions/${src} — edit that file, then run: bun run build`,
    `export const ${exportName} = ${JSON.stringify(content)};`,
    "",
  ].join("\n");
  writeFileSync(join(outDir, out), ts);
  console.log(`  generated ${out}`);
}
