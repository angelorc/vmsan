import { createGzip } from "node:zlib";
import { Readable } from "node:stream";
import { pack, type Pack } from "tar-stream";

export interface RunParams {
  cmd: string;
  args?: string[];
  cwd?: string;
  env?: Record<string, string>;
  timeoutMs?: number;
  detached?: boolean;
}

export type RunEventType = "started" | "stdout" | "stderr" | "exit" | "timeout" | "error";

export interface RunEvent {
  type: RunEventType;
  data?: string;
  id?: string;
  pid?: number;
  exitCode?: number;
  ts: string;
  error?: string;
}

export interface WriteFileEntry {
  path: string;
  content: Buffer;
}

export interface SessionInfo {
  sessionId: string;
  shell: string;
  createdAt: string;
  subscriberCount: number;
}

export class AgentClient {
  constructor(
    private baseUrl: string,
    private token: string,
  ) {}

  async health(): Promise<{ status: string; version: string }> {
    const res = await fetch(`${this.baseUrl}/health`);
    if (!res.ok) {
      throw new Error(`Agent health check failed: ${res.status}`);
    }
    return res.json() as Promise<{ status: string; version: string }>;
  }

  async *run(params: RunParams): AsyncGenerator<RunEvent> {
    const res = await fetch(`${this.baseUrl}/exec`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify(params),
    });

    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Agent exec failed (${res.status}): ${text}`);
    }

    if (!res.body) {
      throw new Error("Agent exec returned no body");
    }

    // Parse NDJSON stream line by line.
    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        const trimmed = line.trim();
        if (trimmed) {
          yield JSON.parse(trimmed) as RunEvent;
        }
      }
    }

    // Flush remaining buffer.
    if (buffer.trim()) {
      yield JSON.parse(buffer.trim()) as RunEvent;
    }
  }

  async killCommand(cmdId: string, signal?: string): Promise<void> {
    const url = `${this.baseUrl}/exec/${cmdId}/kill`;
    const res = await fetch(url, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.token}`,
      },
      body: signal ? JSON.stringify({ signal }) : undefined,
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Agent kill failed (${res.status}): ${text}`);
    }
  }

  async writeFiles(files: WriteFileEntry[], extractDir?: string): Promise<void> {
    const tarPack = pack();
    for (const file of files) {
      tarPack.entry({ name: file.path }, file.content);
    }
    tarPack.finalize();

    // Gzip the tar stream.
    const gzipped = await tarToGzipBuffer(tarPack);

    const headers: Record<string, string> = {
      "Content-Type": "application/gzip",
      Authorization: `Bearer ${this.token}`,
    };
    if (extractDir) {
      headers["X-Extract-Dir"] = extractDir;
    }

    const res = await fetch(`${this.baseUrl}/files/write`, {
      method: "POST",
      headers,
      body: new Uint8Array(gzipped),
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Agent writeFiles failed (${res.status}): ${text}`);
    }
  }

  async listShellSessions(): Promise<SessionInfo[]> {
    const res = await fetch(`${this.baseUrl}/shell/sessions`, {
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Agent listShellSessions failed (${res.status}): ${text}`);
    }
    return res.json() as Promise<SessionInfo[]>;
  }

  async killShellSession(sessionId: string): Promise<void> {
    const res = await fetch(`${this.baseUrl}/shell/sessions/${sessionId}/kill`, {
      method: "POST",
      headers: { Authorization: `Bearer ${this.token}` },
    });
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Agent killShellSession failed (${res.status}): ${text}`);
    }
  }

  async readFile(path: string): Promise<Buffer | null> {
    const res = await fetch(`${this.baseUrl}/files/read`, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${this.token}`,
      },
      body: JSON.stringify({ path }),
    });

    if (res.status === 404) {
      return null;
    }
    if (!res.ok) {
      const text = await res.text();
      throw new Error(`Agent readFile failed (${res.status}): ${text}`);
    }

    return Buffer.from(await res.arrayBuffer());
  }
}

async function tarToGzipBuffer(tarPack: Pack): Promise<Buffer> {
  const gzip = createGzip();
  const readable = Readable.from(tarPack);
  readable.pipe(gzip);

  const chunks: Buffer[] = [];
  for await (const chunk of gzip) {
    chunks.push(Buffer.from(chunk));
  }
  return Buffer.concat(chunks);
}
