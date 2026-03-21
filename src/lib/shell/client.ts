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
  initialCommand?: string;
  user?: string;
}

export interface ShellCloseInfo {
  /** true when the shell process exited (e.g. user typed `exit`) */
  sessionDestroyed: boolean;
}

const MAX_RECONNECT_ATTEMPTS = 10;
const INITIAL_BACKOFF_MS = 1000;
const MAX_BACKOFF_MS = 30_000;

/** Close codes that indicate an intentional disconnect. */
function isIntentionalClose(code: number, reason: string): boolean {
  // 1000 = Normal Closure, 1001 = Going Away
  return code === 1000 || code === 1001 || reason === "session destroyed";
}

export class ShellSession {
  private _sessionId: string | null;
  private ws: WebSocket | null = null;
  private stdinRaw = false;

  /** Handlers shared across reconnections. */
  private onStdinData: ((chunk: Buffer) => void) | null = null;
  private onResize: (() => void) | null = null;
  private exitHandler: (() => void) | null = null;

  constructor(private opts: ShellSessionOptions) {
    this._sessionId = opts.sessionId ?? null;
  }

  get sessionId(): string | null {
    return this._sessionId;
  }

  /** Connect to the shell. Resolves when the session ends (after exhausting reconnections). */
  connect(): Promise<ShellCloseInfo> {
    return new Promise((resolve, reject) => {
      let firstConnection = true;
      let reconnecting = false;

      const restoreStdin = () => {
        if (this.stdinRaw && process.stdin.isTTY) {
          process.stdin.setRawMode(false);
          this.stdinRaw = false;
        }
      };

      const setRawMode = () => {
        if (process.stdin.isTTY && !this.stdinRaw) {
          process.stdin.setRawMode(true);
          this.stdinRaw = true;
        }
      };

      this.exitHandler = () => restoreStdin();
      process.on("exit", this.exitHandler);

      this.onStdinData = (chunk: Buffer) => {
        if (this.ws && this.ws.readyState === WebSocket.OPEN) {
          this.ws.send(serializeData(chunk));
        }
      };

      this.onResize = () => {
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
        if (this.onStdinData) {
          process.stdin.removeListener("data", this.onStdinData);
        }
        if (this.onResize) {
          process.stdout.removeListener("resize", this.onResize);
        }
        if (this.exitHandler) {
          process.removeListener("exit", this.exitHandler);
        }
        process.stdin.pause();
        process.stdin.unref();
        if (this.ws) {
          this.ws.terminate();
          this.ws = null;
        }
      };

      const writeStatus = (msg: string) => {
        const dim = "\x1b[2m";
        const reset = "\x1b[0m";
        process.stderr.write(`\r${dim}${msg}${reset}\x1b[K\n`);
      };

      const attemptReconnect = async (attempt: number): Promise<void> => {
        if (attempt > MAX_RECONNECT_ATTEMPTS) {
          writeStatus(
            `Connection lost. Could not reconnect after ${MAX_RECONNECT_ATTEMPTS} attempts.`,
          );
          if (this._sessionId) {
            writeStatus(`Resume manually with: vmsan connect <vm-id> --session ${this._sessionId}`);
          }
          cleanup();
          resolve({ sessionDestroyed: false });
          return;
        }

        const backoff = Math.min(INITIAL_BACKOFF_MS * 2 ** (attempt - 1), MAX_BACKOFF_MS);
        writeStatus(`[reconnecting... attempt ${attempt}/${MAX_RECONNECT_ATTEMPTS}]`);

        await new Promise<void>((r) => setTimeout(r, backoff));

        try {
          await this.openWebSocket(setRawMode);
          // Reconnection succeeded — rewire events
          reconnecting = false;
          writeStatus("[reconnected]");
          wireWsEvents();
        } catch {
          // This attempt failed; try the next one
          await attemptReconnect(attempt + 1);
        }
      };

      const wireWsEvents = () => {
        const ws = this.ws!;

        ws.on("message", (data: WebSocket.RawData, isBinary: boolean) => {
          if (!isBinary) {
            const text = typeof data === "string" ? data : data.toString();
            const meta = parseSessionMetadata(text);
            if (meta) {
              this._sessionId = meta.sessionId;
            }
            return;
          }

          const buf = Buffer.isBuffer(data) ? data : Buffer.from(data as ArrayBuffer);
          const msg = parse(buf);
          if (msg && msg.type === MsgData) {
            process.stdout.write(msg.data);
          }
        });

        ws.on("close", (code: number, reason: Buffer) => {
          const reasonStr = reason.toString();

          if (isIntentionalClose(code, reasonStr)) {
            cleanup();
            resolve({ sessionDestroyed: reasonStr === "session destroyed" });
            return;
          }

          // Unexpected close — attempt reconnection
          if (reconnecting) return; // avoid double reconnect
          reconnecting = true;
          restoreStdin();
          attemptReconnect(1);
        });

        ws.on("error", () => {
          // The "close" event will fire after "error", triggering reconnection.
          // Nothing extra needed here — just prevent unhandled error crash.
        });
      };

      // First connection — use detailed HTTP errors for better diagnostics
      this.openWebSocket(setRawMode, true)
        .then(() => {
          firstConnection = false;
          wireWsEvents();
        })
        .catch((err) => {
          if (firstConnection) {
            cleanup();
            reject(err);
          }
        });
    });
  }

  /** Force-close the connection. */
  close(): void {
    if (this.ws) {
      this.ws.close(1000, "client close");
    }
  }

  /**
   * Open a WebSocket, wait for it to be ready, set up raw mode and send
   * the ready + resize handshake. Rejects if the connection fails.
   *
   * When `detailedHttpErrors` is true (first connection), HTTP upgrade
   * failures include the status code and response body in the error.
   */
  private openWebSocket(setRawMode: () => void, detailedHttpErrors = false): Promise<void> {
    return new Promise<void>((resolve, reject) => {
      const url = this.buildUrl();
      const ws = new WebSocket(url);
      let settled = false;

      ws.on("open", () => {
        settled = true;
        this.ws = ws;

        setRawMode();
        process.stdin.resume();

        ws.send(serializeReady());
        if (process.stdout.columns && process.stdout.rows) {
          ws.send(serializeResize(process.stdout.columns, process.stdout.rows));
        }

        // Only send initial command on the very first connection
        if (this.opts.initialCommand) {
          ws.send(serializeData(Buffer.from(this.opts.initialCommand)));
          // Clear so it doesn't fire on reconnect
          this.opts.initialCommand = undefined;
        }

        // Wire stdin and resize listeners (idempotent — remove first)
        if (this.onStdinData) {
          process.stdin.removeListener("data", this.onStdinData);
          process.stdin.on("data", this.onStdinData);
        }
        if (this.onResize) {
          process.stdout.removeListener("resize", this.onResize);
          process.stdout.on("resize", this.onResize);
        }

        resolve();
      });

      if (detailedHttpErrors) {
        ws.on("unexpected-response", (_req, res) => {
          if (settled) return;
          settled = true;
          const maxBytes = 4096;
          let body = "";
          res.on("data", (chunk: Buffer) => {
            if (body.length < maxBytes) {
              body += chunk.toString().slice(0, maxBytes - body.length);
            }
          });
          res.on("end", () => {
            ws.terminate();
            reject(
              new Error(
                `Shell connection failed (HTTP ${res.statusCode}): ${body.trim() || "no response body"}`,
              ),
            );
          });
        });
      }

      ws.on("error", (err) => {
        if (!settled) {
          settled = true;
          ws.terminate();
          reject(err);
        }
      });

      // If we get a close before open, it's a failed connection
      ws.on("close", () => {
        if (!settled) {
          settled = true;
          reject(new Error("WebSocket closed before opening"));
        }
      });
    });
  }

  private buildUrl(): string {
    const proto = "ws";
    if (this._sessionId) {
      const url = new URL(
        `${proto}://${this.opts.host}:${this.opts.port}/ws/shell/${encodeURIComponent(this._sessionId)}`,
      );
      url.searchParams.set("token", this.opts.token);
      return url.toString();
    }
    const url = new URL(`${proto}://${this.opts.host}:${this.opts.port}/ws/shell`);
    url.searchParams.set("token", this.opts.token);
    if (this.opts.shell) {
      url.searchParams.set("shell", this.opts.shell);
    }
    if (this.opts.user) {
      url.searchParams.set("user", this.opts.user);
    }
    return url.toString();
  }
}

/** Convenience wrapper for backward compat. */
export function connectShell(opts: {
  wsUrl: string;
  token: string;
  shell?: string;
}): Promise<ShellCloseInfo> {
  const parsed = new URL(opts.wsUrl);
  const session = new ShellSession({
    host: parsed.hostname,
    port: Number(parsed.port),
    token: opts.token,
    shell: opts.shell,
  });
  return session.connect();
}
