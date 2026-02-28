import { request } from "node:http";
import { join } from "node:path";
import type { paths } from "../generated/firecracker-api.d.ts";
import { firecrackerApiError } from "../errors/index.ts";

// ---------------------------------------------------------------------------
// Type-level helpers to extract request/response types from the generated spec
// ---------------------------------------------------------------------------

/** Extracts the JSON request-body type for a given path + method.
 *  Handles both required (`requestBody:`) and optional (`requestBody?:`) bodies. */
type RequestBody<P extends keyof paths, M extends string> = M extends keyof paths[P]
  ? paths[P][M] extends { requestBody?: { content: { "application/json": infer B } } }
    ? B
    : never
  : never;

/** Extracts the 200 JSON response type for a given path + method. */
type ResponseBody<P extends keyof paths, M extends string> = M extends keyof paths[P]
  ? paths[P][M] extends { responses: { 200: { content: { "application/json": infer R } } } }
    ? R
    : void
  : void;

// ---------------------------------------------------------------------------
// Low-level transport (unchanged from original)
// ---------------------------------------------------------------------------

export function firecrackerRequest(
  socketPath: string,
  method: string,
  path: string,
  body?: unknown,
): Promise<{ statusCode: number; body: string }> {
  return new Promise((resolve, reject) => {
    const data = body ? JSON.stringify(body) : undefined;
    const req = request(
      {
        socketPath,
        method,
        path,
        headers: {
          "Content-Type": "application/json",
          ...(data ? { "Content-Length": String(Buffer.byteLength(data)) } : {}),
        },
      },
      (res) => {
        const chunks: Buffer[] = [];
        res.on("data", (chunk: Buffer) => chunks.push(chunk));
        res.on("end", () => {
          resolve({
            statusCode: res.statusCode || 0,
            body: Buffer.concat(chunks).toString(),
          });
        });
      },
    );

    req.on("error", reject);
    if (data) req.write(data);
    req.end();
  });
}

// ---------------------------------------------------------------------------
// Typed public function — validates path, method, and body at compile time
// ---------------------------------------------------------------------------

/**
 * Send a type-safe request to the Firecracker API over a Unix socket.
 *
 * The generic parameters ensure the compiler validates:
 * - `path` is a valid Firecracker API path
 * - `method` is a valid HTTP method for that path
 * - `body` matches the expected request body schema
 *
 * Returns the parsed JSON response for 200 responses, `void` for 204,
 * and throws on any error status code.
 */
export async function firecrackerFetch<
  P extends keyof paths,
  M extends Uppercase<string & keyof paths[P]>,
>(
  socketPath: string,
  method: M,
  path: P,
  ...args: RequestBody<P, Lowercase<M>> extends never ? [] : [body: RequestBody<P, Lowercase<M>>]
): Promise<ResponseBody<P, Lowercase<M>>> {
  const res = await firecrackerRequest(socketPath, method, path as string, args[0]);

  if (res.statusCode >= 400) {
    throw firecrackerApiError(method, path as string, res.statusCode, res.body);
  }

  // 204 No Content → return void (cast is safe, ResponseBody resolves to void)
  if (res.statusCode === 204 || !res.body) {
    return undefined as ResponseBody<P, Lowercase<M>>;
  }

  return JSON.parse(res.body) as ResponseBody<P, Lowercase<M>>;
}

// ---------------------------------------------------------------------------
// FirecrackerClient — convenience class with the same public API as before
// ---------------------------------------------------------------------------

export class FirecrackerClient {
  constructor(private readonly socketPath: string) {}

  async boot(kernelPath: string, bootArgs: string): Promise<void> {
    await firecrackerFetch(this.socketPath, "PUT", "/boot-source", {
      kernel_image_path: kernelPath,
      boot_args: bootArgs,
    });
  }

  async addDrive(
    driveId: string,
    pathOnHost: string,
    isRoot: boolean,
    isReadOnly: boolean,
  ): Promise<void> {
    await firecrackerFetch(this.socketPath, "PUT", `/drives/${driveId}` as "/drives/{drive_id}", {
      drive_id: driveId,
      path_on_host: pathOnHost,
      is_root_device: isRoot,
      is_read_only: isReadOnly,
      cache_type: "Unsafe",
      io_engine: "Sync",
    });
  }

  async configure(vcpus: number, memMib: number): Promise<void> {
    await firecrackerFetch(this.socketPath, "PUT", "/machine-config", {
      vcpu_count: vcpus,
      mem_size_mib: memMib,
      smt: false,
      track_dirty_pages: false,
    });
  }

  async addNetwork(ifaceId: string, tapDev: string, macAddress: string): Promise<void> {
    await firecrackerFetch(
      this.socketPath,
      "PUT",
      `/network-interfaces/${ifaceId}` as "/network-interfaces/{iface_id}",
      {
        iface_id: ifaceId,
        host_dev_name: tapDev,
        guest_mac: macAddress,
      },
    );
  }

  async start(): Promise<void> {
    await firecrackerFetch(this.socketPath, "PUT", "/actions", {
      action_type: "InstanceStart",
    });
  }

  async loadSnapshot(snapshotPath: string, memPath: string): Promise<void> {
    await firecrackerFetch(this.socketPath, "PUT", "/snapshot/load", {
      snapshot_path: snapshotPath,
      mem_file_path: memPath,
    });
  }

  async resume(): Promise<void> {
    await firecrackerFetch(this.socketPath, "PATCH", "/vm", {
      state: "Resumed",
    });
  }

  static async getVersion(baseDir: string): Promise<string | undefined> {
    const fcPath = join(baseDir, "bin", "firecracker");
    try {
      const { execSync } = await import("node:child_process");
      return execSync(`"${fcPath}" --version`, { encoding: "utf-8" }).trim();
    } catch {
      return undefined;
    }
  }
}
