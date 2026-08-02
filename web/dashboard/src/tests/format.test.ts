import { describe, expect, it } from "vitest";
import { formatBytes, isOnline, sumBy } from "../utils/format";

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

  it("sums selected values", () => {
    expect(sumBy([{ n: 2 }, { n: 3 }], (item) => item.n)).toBe(5);
  });
});
