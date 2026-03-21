import { describe, expect, it, beforeEach, afterEach } from "vitest";
import { mkdtempSync, writeFileSync, rmSync } from "node:fs";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { detectProject } from "../../src/lib/toml/detect.ts";

let tempDir: string;

beforeEach(() => {
  tempDir = mkdtempSync(join(tmpdir(), "vmsan-detect-"));
});

afterEach(() => {
  rmSync(tempDir, { recursive: true, force: true });
});

// ---------- Node.js detection ----------

describe("detectProject – Node.js", () => {
  it("detects Node.js project with build and start scripts", () => {
    writeFileSync(
      join(tempDir, "package.json"),
      JSON.stringify({
        scripts: { build: "tsc", start: "node dist/index.js" },
      }),
    );
    const result = detectProject(tempDir);
    expect(result).not.toBeNull();
    expect(result!.runtime).toBe("node22");
    expect(result!.build).toBe("npm run build");
    expect(result!.start).toBe("npm start");
    expect(result!.confidence).toBe("high");
  });

  it("detects node24 from .node-version", () => {
    writeFileSync(
      join(tempDir, "package.json"),
      JSON.stringify({ scripts: { start: "node index.js" } }),
    );
    writeFileSync(join(tempDir, ".node-version"), "24.1.0\n");
    const result = detectProject(tempDir);
    expect(result!.runtime).toBe("node24");
  });

  it("detects node24 from .nvmrc", () => {
    writeFileSync(
      join(tempDir, "package.json"),
      JSON.stringify({ scripts: { start: "node index.js" } }),
    );
    writeFileSync(join(tempDir, ".nvmrc"), "v24\n");
    const result = detectProject(tempDir);
    expect(result!.runtime).toBe("node24");
  });

  it("detects node version from engines.node", () => {
    writeFileSync(
      join(tempDir, "package.json"),
      JSON.stringify({
        engines: { node: ">=24" },
        scripts: { start: "node index.js" },
      }),
    );
    const result = detectProject(tempDir);
    expect(result!.runtime).toBe("node24");
  });

  it("falls back to main field for start command", () => {
    writeFileSync(join(tempDir, "package.json"), JSON.stringify({ main: "lib/index.js" }));
    const result = detectProject(tempDir);
    expect(result!.start).toBe("node lib/index.js");
    expect(result!.build).toBeUndefined();
  });

  it("prefers .node-version over engines.node", () => {
    writeFileSync(
      join(tempDir, "package.json"),
      JSON.stringify({
        engines: { node: ">=22" },
        scripts: { start: "node index.js" },
      }),
    );
    writeFileSync(join(tempDir, ".node-version"), "24.0.0\n");
    const result = detectProject(tempDir);
    expect(result!.runtime).toBe("node24");
  });
});

// ---------- Go detection ----------

describe("detectProject – Go", () => {
  it("detects Go project", () => {
    writeFileSync(join(tempDir, "go.mod"), "module github.com/example/app\n\ngo 1.23\n");
    const result = detectProject(tempDir);
    expect(result).not.toBeNull();
    expect(result!.runtime).toBe("go");
    expect(result!.build).toBe("go build -o app .");
    expect(result!.start).toBe("./app");
    expect(result!.confidence).toBe("high");
  });
});

// ---------- Python detection ----------

describe("detectProject – Python", () => {
  it("detects Python project with requirements.txt", () => {
    writeFileSync(join(tempDir, "requirements.txt"), "flask==3.0.0\n");
    const result = detectProject(tempDir);
    expect(result).not.toBeNull();
    expect(result!.runtime).toBe("python3.13");
    expect(result!.confidence).toBe("high");
    expect(result!.reason).toBe("Detected requirements.txt");
  });

  it("detects Python project with pyproject.toml", () => {
    writeFileSync(join(tempDir, "pyproject.toml"), '[project]\nname = "myapp"\n');
    const result = detectProject(tempDir);
    expect(result!.runtime).toBe("python3.13");
    expect(result!.confidence).toBe("medium");
    expect(result!.reason).toBe("Detected pyproject.toml");
  });

  it("detects FastAPI start command", () => {
    writeFileSync(join(tempDir, "requirements.txt"), "fastapi==0.110.0\nuvicorn\n");
    const result = detectProject(tempDir);
    expect(result!.start).toBe("uvicorn main:app --host 0.0.0.0");
  });

  it("detects Flask start command", () => {
    writeFileSync(join(tempDir, "requirements.txt"), "flask==3.0.0\n");
    const result = detectProject(tempDir);
    expect(result!.start).toBe("flask run --host 0.0.0.0");
  });

  it("detects Django start command", () => {
    writeFileSync(join(tempDir, "requirements.txt"), "django==5.0.0\n");
    const result = detectProject(tempDir);
    expect(result!.start).toBe("python manage.py runserver 0.0.0.0:8000");
  });
});

// ---------- Rust detection ----------

describe("detectProject – Rust", () => {
  it("detects Rust project with package name", () => {
    writeFileSync(join(tempDir, "Cargo.toml"), '[package]\nname = "myapp"\nversion = "0.1.0"\n');
    const result = detectProject(tempDir);
    expect(result).not.toBeNull();
    expect(result!.runtime).toBe("rust");
    expect(result!.build).toBe("cargo build --release");
    expect(result!.start).toBe("./target/release/myapp");
    expect(result!.confidence).toBe("high");
  });

  it("detects Rust project with [[bin]] name", () => {
    writeFileSync(
      join(tempDir, "Cargo.toml"),
      '[package]\nname = "lib"\n\n[[bin]]\nname = "server"\n',
    );
    const result = detectProject(tempDir);
    expect(result!.start).toBe("./target/release/server");
  });
});

// ---------- Dockerfile detection ----------

describe("detectProject – Dockerfile", () => {
  it("detects Dockerfile project", () => {
    writeFileSync(
      join(tempDir, "Dockerfile"),
      'FROM node:22-alpine\nCOPY . .\nCMD ["node", "index.js"]\n',
    );
    const result = detectProject(tempDir);
    expect(result).not.toBeNull();
    expect(result!.runtime).toBe("from-image");
    expect(result!.confidence).toBe("medium");
    expect(result!.reason).toBe("Detected Dockerfile");
  });
});

// ---------- Unknown project ----------

describe("detectProject – unknown", () => {
  it("returns null for empty directory", () => {
    const result = detectProject(tempDir);
    expect(result).toBeNull();
  });

  it("returns null for directory with only text files", () => {
    writeFileSync(join(tempDir, "README.md"), "# Hello");
    const result = detectProject(tempDir);
    expect(result).toBeNull();
  });
});

// ---------- Priority ----------

describe("detectProject – priority", () => {
  it("prefers package.json over Dockerfile when both exist", () => {
    writeFileSync(
      join(tempDir, "package.json"),
      JSON.stringify({ scripts: { start: "node index.js" } }),
    );
    writeFileSync(join(tempDir, "Dockerfile"), "FROM node:22\n");
    const result = detectProject(tempDir);
    expect(result!.runtime).toBe("node22");
    expect(result!.reason).toBe("Detected package.json");
  });
});

// ---------- Confidence levels ----------

describe("detectProject – confidence", () => {
  it("returns high confidence for package.json", () => {
    writeFileSync(
      join(tempDir, "package.json"),
      JSON.stringify({ scripts: { start: "node index.js" } }),
    );
    expect(detectProject(tempDir)!.confidence).toBe("high");
  });

  it("returns high confidence for go.mod", () => {
    writeFileSync(join(tempDir, "go.mod"), "module example.com/app\n");
    expect(detectProject(tempDir)!.confidence).toBe("high");
  });

  it("returns medium confidence for Dockerfile", () => {
    writeFileSync(join(tempDir, "Dockerfile"), "FROM alpine\n");
    expect(detectProject(tempDir)!.confidence).toBe("medium");
  });

  it("returns medium confidence for pyproject.toml only", () => {
    writeFileSync(join(tempDir, "pyproject.toml"), '[project]\nname = "app"\n');
    expect(detectProject(tempDir)!.confidence).toBe("medium");
  });

  it("returns high confidence for requirements.txt", () => {
    writeFileSync(join(tempDir, "requirements.txt"), "requests\n");
    expect(detectProject(tempDir)!.confidence).toBe("high");
  });
});
