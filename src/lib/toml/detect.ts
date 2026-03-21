import { readFileSync, existsSync } from "node:fs";
import { join } from "node:path";

export interface DetectionResult {
  runtime: string;
  build?: string;
  start?: string;
  confidence: "high" | "medium" | "low";
  reason: string;
}

// ---------- Detection rules ----------

function detectNode(dir: string): DetectionResult | null {
  const pkgPath = join(dir, "package.json");
  if (!existsSync(pkgPath)) return null;

  let pkg: {
    scripts?: Record<string, string>;
    main?: string;
    engines?: { node?: string };
  };
  try {
    pkg = JSON.parse(readFileSync(pkgPath, "utf-8"));
  } catch {
    return null;
  }

  // Determine Node.js version from various sources
  let runtime = "node22";
  const nodeVersion = resolveNodeVersion(dir, pkg.engines?.node);
  if (nodeVersion && nodeVersion >= 24) {
    runtime = "node24";
  }

  const build = pkg.scripts?.build ? "npm run build" : undefined;
  let start: string | undefined;
  if (pkg.scripts?.start) {
    start = "npm start";
  } else if (pkg.main) {
    start = `node ${pkg.main}`;
  }

  return {
    runtime,
    build,
    start,
    confidence: "high",
    reason: "Detected package.json",
  };
}

function resolveNodeVersion(dir: string, enginesNode?: string): number | null {
  // Check .node-version first (most specific)
  for (const file of [".node-version", ".nvmrc"]) {
    const filePath = join(dir, file);
    if (existsSync(filePath)) {
      const content = readFileSync(filePath, "utf-8").trim();
      const major = extractMajorVersion(content);
      if (major !== null) return major;
    }
  }

  // Fall back to engines.node from package.json
  if (enginesNode) {
    const major = extractMajorVersion(enginesNode);
    if (major !== null) return major;
  }

  return null;
}

function extractMajorVersion(version: string): number | null {
  // Handle "v22.1.0", "22.1.0", "22", ">=22", "^22", "~22", "lts/*", etc.
  const match = /(\d+)/.exec(version);
  if (match) {
    return Number(match[1]);
  }
  return null;
}

function detectGo(dir: string): DetectionResult | null {
  const goModPath = join(dir, "go.mod");
  if (!existsSync(goModPath)) return null;

  return {
    runtime: "go",
    build: "go build -o app .",
    start: "./app",
    confidence: "high",
    reason: "Detected go.mod",
  };
}

function detectPython(dir: string): DetectionResult | null {
  const hasRequirements = existsSync(join(dir, "requirements.txt"));
  const hasPyproject = existsSync(join(dir, "pyproject.toml"));
  if (!hasRequirements && !hasPyproject) return null;

  const reason = hasRequirements ? "Detected requirements.txt" : "Detected pyproject.toml";

  let start: string | undefined;

  // Try to detect the framework
  const framework = detectPythonFramework(dir);
  if (framework) {
    start = framework;
  }

  return {
    runtime: "python3.13",
    start,
    confidence: hasRequirements ? "high" : "medium",
    reason,
  };
}

function detectPythonFramework(dir: string): string | null {
  // Check requirements.txt for common frameworks
  const reqPath = join(dir, "requirements.txt");
  if (existsSync(reqPath)) {
    const content = readFileSync(reqPath, "utf-8").toLowerCase();
    if (content.includes("fastapi")) {
      return "uvicorn main:app --host 0.0.0.0";
    }
    if (content.includes("flask")) {
      return "flask run --host 0.0.0.0";
    }
    if (content.includes("django")) {
      return "python manage.py runserver 0.0.0.0:8000";
    }
  }

  // Check pyproject.toml for dependencies
  const pyprojectPath = join(dir, "pyproject.toml");
  if (existsSync(pyprojectPath)) {
    const content = readFileSync(pyprojectPath, "utf-8").toLowerCase();
    if (content.includes("fastapi")) {
      return "uvicorn main:app --host 0.0.0.0";
    }
    if (content.includes("flask")) {
      return "flask run --host 0.0.0.0";
    }
    if (content.includes("django")) {
      return "python manage.py runserver 0.0.0.0:8000";
    }
  }

  return null;
}

function detectRust(dir: string): DetectionResult | null {
  const cargoPath = join(dir, "Cargo.toml");
  if (!existsSync(cargoPath)) return null;

  let start: string | undefined;
  try {
    const content = readFileSync(cargoPath, "utf-8");
    // Try to extract [[bin]] name or package name
    const binMatch = /\[\[bin]]\s*\n\s*name\s*=\s*"([^"]+)"/.exec(content);
    if (binMatch) {
      start = `./target/release/${binMatch[1]}`;
    } else {
      const nameMatch = /\[package]\s*\n\s*name\s*=\s*"([^"]+)"/.exec(content);
      if (nameMatch) {
        start = `./target/release/${nameMatch[1]}`;
      }
    }
  } catch {
    // Fall through without a start command
  }

  return {
    runtime: "rust",
    build: "cargo build --release",
    start,
    confidence: "high",
    reason: "Detected Cargo.toml",
  };
}

function detectDocker(dir: string): DetectionResult | null {
  const dockerfilePath = join(dir, "Dockerfile");
  if (!existsSync(dockerfilePath)) return null;

  return {
    runtime: "from-image",
    confidence: "medium",
    reason: "Detected Dockerfile",
  };
}

// ---------- Public API ----------

const detectors = [detectNode, detectGo, detectPython, detectRust, detectDocker];

export function detectProject(dir: string): DetectionResult | null {
  for (const detect of detectors) {
    const result = detect(dir);
    if (result) return result;
  }
  return null;
}
