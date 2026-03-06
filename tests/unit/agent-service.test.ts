import { describe, expect, it } from "vitest";
import { generateAgentService } from "../../src/lib/agent-service.ts";

describe("generateAgentService", () => {
  it("does not block agent startup on network.target", () => {
    const unit = generateAgentService();

    expect(unit).not.toContain("After=network.target");
    expect(unit).toContain("ExecStart=/usr/local/bin/vmsan-agent");
    expect(unit).toContain("WantedBy=multi-user.target");
  });
});
