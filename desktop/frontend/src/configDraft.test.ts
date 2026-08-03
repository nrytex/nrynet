import { describe, expect, it } from "vitest";
import { completeConfigForSave } from "./configDraft";

describe("completeConfigForSave", () => {
  it("preserves settings from other tabs during a partial save", () => {
    const current = completeConfigForSave(undefined, {
      serverUrl: "wss://server.example/agent/connect",
      dataAddress: "server.example:7001",
    });
    const result = completeConfigForSave(current, { token: "agent-token" });
    expect(result.serverUrl).toBe(current.serverUrl);
    expect(result.dataAddress).toBe(current.dataAddress);
    expect(result.token).toBe("agent-token");
  });
});
