import type { AgentClient, RunEvent } from "../services/agent.ts";

/**
 * Lightweight async iterable queue for pushing events and yielding them to consumers.
 */
class AsyncQueue<T> {
  private _buffer: T[] = [];
  private _waiting: Array<(r: IteratorResult<T>) => void> = [];
  private _closed = false;

  push(item: T): void {
    if (this._closed) return;
    if (this._waiting.length > 0) {
      const resolve = this._waiting.shift()!;
      resolve({ value: item, done: false });
    } else {
      this._buffer.push(item);
    }
  }

  close(): void {
    this._closed = true;
    for (const resolve of this._waiting) {
      resolve({ value: undefined as unknown as T, done: true });
    }
    this._waiting.length = 0;
  }

  async *[Symbol.asyncIterator](): AsyncGenerator<T> {
    while (true) {
      if (this._buffer.length > 0) {
        yield this._buffer.shift()!;
      } else if (this._closed) {
        return;
      } else {
        const item = await new Promise<IteratorResult<T>>((resolve) => {
          this._waiting.push(resolve);
        });
        if (item.done) return;
        yield item.value;
      }
    }
  }
}

export interface LogEntry {
  stream: "stdout" | "stderr";
  data: string;
}

export interface CommandInit {
  agent: AgentClient;
  cmdId: string;
  startedAt: Date;
  stream: AsyncIterable<RunEvent>;
  signal?: AbortSignal;
  onStdout?: (line: string) => void;
  onStderr?: (line: string) => void;
}

export class CommandFinished {
  readonly cmdId: string;
  readonly exitCode: number;
  readonly stdout: string;
  readonly stderr: string;
  readonly output: string;
  readonly timedOut: boolean;
  readonly startedAt: Date;

  constructor(opts: {
    cmdId: string;
    exitCode: number;
    stdout: string;
    stderr: string;
    output: string;
    timedOut: boolean;
    startedAt: Date;
  }) {
    this.cmdId = opts.cmdId;
    this.exitCode = opts.exitCode;
    this.stdout = opts.stdout;
    this.stderr = opts.stderr;
    this.output = opts.output;
    this.timedOut = opts.timedOut;
    this.startedAt = opts.startedAt;
  }

  get ok(): boolean {
    return this.exitCode === 0;
  }
}

const MAX_LOG_ENTRIES = 100_000;

export class Command {
  readonly cmdId: string;

  private _startedAt: Date;
  private _exitCode: number | null = null;
  private _timedOut = false;
  private _logEntries: LogEntry[] = [];
  private _logTruncated = false;
  private _eventQueue = new AsyncQueue<RunEvent>();
  private _completion: Promise<CommandFinished>;

  private _stdoutPromise: Promise<string> | null = null;
  private _stderrPromise: Promise<string> | null = null;
  private _outputPromise: Promise<string> | null = null;

  private _agent: AgentClient;

  constructor(init: CommandInit) {
    const { agent, cmdId, startedAt, stream, signal, onStdout, onStderr } = init;
    this._agent = agent;
    this.cmdId = cmdId;
    this._startedAt = startedAt;

    let resolveCompletion: (result: CommandFinished) => void;
    let rejectCompletion: (err: Error) => void;
    this._completion = new Promise<CommandFinished>((resolve, reject) => {
      resolveCompletion = resolve;
      rejectCompletion = reject;
    });

    // Handle abort signal
    let onAbort: (() => void) | undefined;
    if (signal) {
      onAbort = () => {
        this.kill().catch(() => {});
      };
      if (signal.aborted) {
        void this.kill().catch(() => {});
      } else {
        signal.addEventListener("abort", onAbort, { once: true });
      }
    }

    // Fire-and-forget async IIFE to consume the remaining stream
    void (async () => {
      try {
        for await (const event of stream) {
          this._eventQueue.push(event);

          switch (event.type) {
            case "stdout":
            case "stderr": {
              if (event.data !== undefined) {
                if (this._logEntries.length < MAX_LOG_ENTRIES) {
                  this._logEntries.push({ stream: event.type, data: event.data });
                } else {
                  this._logTruncated = true;
                }
                (event.type === "stdout" ? onStdout : onStderr)?.(event.data);
              }
              break;
            }
            case "exit":
              this._exitCode = event.exitCode ?? 1;
              break;
            case "timeout":
              this._timedOut = true;
              this._exitCode = 124;
              break;
            case "error":
              this._exitCode = 1;
              break;
          }
        }

        this._eventQueue.close();

        resolveCompletion!(
          new CommandFinished({
            cmdId,
            exitCode: this._exitCode ?? 1,
            stdout: this._logEntries
              .filter((e) => e.stream === "stdout")
              .map((e) => e.data)
              .join("\n"),
            stderr: this._logEntries
              .filter((e) => e.stream === "stderr")
              .map((e) => e.data)
              .join("\n"),
            output: this._logEntries.map((e) => e.data).join("\n"),
            timedOut: this._timedOut,
            startedAt,
          }),
        );
        this._logEntries.length = 0;
      } catch (err) {
        this._eventQueue.close();
        const error = err instanceof Error ? err : new Error(String(err));
        rejectCompletion!(error);
      } finally {
        if (signal && onAbort) {
          signal.removeEventListener("abort", onAbort);
        }
      }
    })();
  }

  get startedAt(): Date {
    return this._startedAt;
  }

  get exitCode(): number | null {
    return this._exitCode;
  }

  async *logs(opts?: { signal?: AbortSignal }): AsyncGenerator<LogEntry> {
    const signal = opts?.signal;
    if (signal?.aborted) return;
    for await (const event of this._eventQueue) {
      if (signal?.aborted) return;
      if (event.type === "stdout" || event.type === "stderr") {
        yield { stream: event.type, data: event.data ?? "" };
      }
    }
  }

  stdout(opts?: { signal?: AbortSignal }): Promise<string> {
    if (!this._stdoutPromise) {
      this._stdoutPromise = this._completion.then((r) => r.stdout);
    }
    return this._withSignal(this._stdoutPromise, opts?.signal);
  }

  stderr(opts?: { signal?: AbortSignal }): Promise<string> {
    if (!this._stderrPromise) {
      this._stderrPromise = this._completion.then((r) => r.stderr);
    }
    return this._withSignal(this._stderrPromise, opts?.signal);
  }

  output(stream?: "stdout" | "stderr" | "both", opts?: { signal?: AbortSignal }): Promise<string> {
    if (stream === "stdout") return this.stdout(opts);
    if (stream === "stderr") return this.stderr(opts);
    if (!this._outputPromise) {
      this._outputPromise = this._completion.then((r) => r.output);
    }
    return this._withSignal(this._outputPromise, opts?.signal);
  }

  wait(opts?: { signal?: AbortSignal }): Promise<CommandFinished> {
    return this._withSignal(this._completion, opts?.signal);
  }

  async kill(signal?: string, opts?: { abortSignal?: AbortSignal }): Promise<void> {
    await this._agent.killCommand(this.cmdId, signal, opts?.abortSignal);
  }

  private _withSignal<T>(promise: Promise<T>, signal?: AbortSignal): Promise<T> {
    if (!signal) return promise;
    if (signal.aborted) {
      return Promise.reject(new DOMException("The operation was aborted.", "AbortError"));
    }
    return new Promise<T>((resolve, reject) => {
      const onAbort = () => reject(new DOMException("The operation was aborted.", "AbortError"));
      signal.addEventListener("abort", onAbort, { once: true });
      promise.then(
        (value) => {
          signal.removeEventListener("abort", onAbort);
          resolve(value);
        },
        (err) => {
          signal.removeEventListener("abort", onAbort);
          reject(err);
        },
      );
    });
  }
}
