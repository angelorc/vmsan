/**
 * Binary protocol for WebSocket PTY shell.
 *
 * Each message is prefixed with a 1-byte type tag:
 *   0x00  Data     [0x00][payload...]
 *   0x01  Resize   [0x01][cols u16BE][rows u16BE]
 *   0x02  Ready    [0x02]
 */

export const MsgData = 0x00;
export const MsgResize = 0x01;
export const MsgReady = 0x02;

export type ShellMessage =
  | { type: typeof MsgData; data: Buffer }
  | { type: typeof MsgResize; cols: number; rows: number }
  | { type: typeof MsgReady };

export interface SessionMetadata {
  sessionId: string;
}

export function parse(buf: Buffer): ShellMessage | null {
  if (buf.length === 0) return null;
  const tag = buf[0];
  switch (tag) {
    case MsgData:
      return { type: MsgData, data: buf.subarray(1) };
    case MsgResize:
      if (buf.length < 5) return null;
      return {
        type: MsgResize,
        cols: buf.readUInt16BE(1),
        rows: buf.readUInt16BE(3),
      };
    case MsgReady:
      return { type: MsgReady };
    default:
      return null;
  }
}

export function serializeData(data: string | Buffer): Buffer {
  const payload = typeof data === "string" ? Buffer.from(data) : data;
  const out = Buffer.allocUnsafe(1 + payload.length);
  out[0] = MsgData;
  payload.copy(out, 1);
  return out;
}

export function serializeResize(cols: number, rows: number): Buffer {
  const out = Buffer.allocUnsafe(5);
  out[0] = MsgResize;
  out.writeUInt16BE(cols, 1);
  out.writeUInt16BE(rows, 3);
  return out;
}

export function serializeReady(): Buffer {
  return Buffer.from([MsgReady]);
}

export function parseSessionMetadata(text: string): SessionMetadata | null {
  try {
    const obj = JSON.parse(text);
    if (typeof obj.sessionId === "string" && obj.sessionId.length > 0) {
      return { sessionId: obj.sessionId };
    }
    return null;
  } catch {
    return null;
  }
}
