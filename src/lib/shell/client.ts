import WebSocket from "ws";
import {
  MsgData,
  parse,
  serializeData,
  serializeReady,
  serializeResize,
  parseSessionMetadata,
} from "./protocol.ts";

export interface ShellSessionOptions {
  host: string;
  port: number;
  token: string;
  shell?: string;
  sessionId?: string;
}

export class ShellSession {
  private _sessionId: string | null;
  private ws: WebSocket | null = null;
  private stdinRaw = false;

  constructor(private opts: ShellSessionOptions) {
    this._sessionId = opts.sessionId ?? null;
  }

  get sessionId(): string | null {
    return this._sessionId;
  }

  /** Connect to the shell. Resolves when WebSocket closes. */
  connect(): Promise<void> {
    return new Promise((resolve, reject) => {
      const url = this.buildUrl();
      this.ws = new WebSocket(url);

      const restoreStdin = () => {
        if (this.stdinRaw && process.stdin.isTTY) {
          process.stdin.setRawMode(false);
          this.stdinRaw = false;
        }
      };

      const exitHandler = () => restoreStdin();
      process.on("exit", exitHandler);

      const onStdinData = (chunk: Buffer) => {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
          this.ws.send(serializeData(chunk));
        }
      };

      const onResize = () => {
        if (
          this.ws &&
          this.ws.readyState === WebSocket.OPEN &&
          process.stdout.columns &&
          process.stdout.rows
        ) {
          this.ws.send(serializeResize(process.stdout.columns, process.stdout.rows));
        }
      };

      const cleanup = () => {
        restoreStdin();
        process.stdin.removeListener("data", onStdinData);
        process.stdout.removeListener("resize", onResize);
        process.removeListener("exit", exitHandler);
        process.stdin.pause();
        process.stdin.unref();
      };

      this.ws.on("open", () => {
        if (process.stdin.isTTY) {
          process.stdin.setRawMode(true);
          this.stdinRaw = true;
        }
        process.stdin.resume();

        this.ws!.send(serializeReady());
        if (process.stdout.columns && process.stdout.rows) {
          this.ws!.send(serializeResize(process.stdout.columns, process.stdout.rows));
        }

        process.stdin.on("data", onStdinData);
        process.stdout.on("resize", onResize);
      });

      this.ws.on("message", (data: WebSocket.RawData, isBinary: boolean) => {
        if (!isBinary) {
          // Text frame: session metadata from server.
          const text = typeof data === "string" ? data : data.toString();
          const meta = parseSessionMetadata(text);
          if (meta) {
            this._sessionId = meta.sessionId;
          }
          return;
        }

        // Binary frame: PTY data.
        const buf = Buffer.isBuffer(data) ? data : Buffer.from(data as ArrayBuffer);
        const msg = parse(buf);
        if (msg && msg.type === MsgData) {
          process.stdout.write(msg.data);
        }
      });

      this.ws.on("close", () => {
        cleanup();
        resolve();
      });

      this.ws.on("error", (err) => {
        cleanup();
        reject(err);
      });
    });
  }

  /** Force-close the connection. */
  close(): void {
    if (this.ws) {
      this.ws.close();
    }
  }

  private buildUrl(): string {
    const proto = "ws";
    if (this._sessionId) {
      const url = new URL(`${proto}://${this.opts.host}:${this.opts.port}/ws/shell/${encodeURIComponent(this._sessionId)}`);
      url.searchParams.set("token", this.opts.token);
      return url.toString();
    }
    const url = new URL(`${proto}://${this.opts.host}:${this.opts.port}/ws/shell`);
    url.searchParams.set("token", this.opts.token);
    if (this.opts.shell) {
      url.searchParams.set("shell", this.opts.shell);
    }
    return url.toString();
  }
}

/** Convenience wrapper for backward compat. */
export function connectShell(opts: {
  wsUrl: string;
  token: string;
  shell?: string;
}): Promise<void> {
  const parsed = new URL(opts.wsUrl);
  const session = new ShellSession({
    host: parsed.hostname,
    port: Number(parsed.port),
    token: opts.token,
    shell: opts.shell,
  });
  return session.connect();
}
