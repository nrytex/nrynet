import { describe, expect, it } from "vitest";
import { formatBytes, formatDuration, isOnline, sumBy } from "../utils/format";

describe("format helpers", () => {
  it("formats bytes", () => {
    expect(formatBytes(0)).toBe("0 B");
    expect(formatBytes(1536)).toBe("1.5 KB");
  });

  it("detects online state", () => {
    expect(isOnline("online")).toBe(true);
    expect(isOnline("running")).toBe(true);
    expect(isOnline("online", true)).toBe(false);
  });

  it("formats connection duration", () => {
    expect(formatDuration(65)).toBe("1m");
    expect(formatDuration(90061)).toBe("1d 1h");
  });

  it("sums selected values", () => {
    expect(sumBy([{ n: 2 }, { n: 3 }], (item) => item.n)).toBe(5);
  });
});
