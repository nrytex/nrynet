import { describe, expect, it } from "vitest";
import { formatBytes, logLine, redact } from "./format";

describe("format helpers", () => {
  it("redacts tokens while preserving useful edges", () => {
    expect(redact("abcdef123456")).toBe("abcd...3456");
    expect(redact("short")).toBe("********");
  });

  it("formats log entries with fields", () => {
    const line = logLine({
      time: "2026-08-02T10:00:00Z",
      level: "INFO",
      message: "connected",
      fields: { retry: 1 },
    });
    expect(line).toContain("INFO connected");
    expect(line).toContain('"retry":1');
  });

  it("formats cumulative byte counters", () => {
    expect(formatBytes(12)).toBe("12 B");
    expect(formatBytes(1536)).toBe("1.5 KiB");
  });
});
