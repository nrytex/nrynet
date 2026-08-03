import { describe, expect, it } from "vitest";
import { formatElapsedTime } from "./elapsedTime";

describe("formatElapsedTime", () => {
  it("formats elapsed time at one-second precision", () => {
    const start = "2026-08-03T12:00:00.000Z";
    expect(formatElapsedTime(start, true, Date.parse(start) + 3_661_000)).toBe("01:01:01");
  });

  it("hides invalid and inactive durations", () => {
    expect(formatElapsedTime(undefined, true, Date.now())).toBe("--:--:--");
    expect(formatElapsedTime("invalid", true, Date.now())).toBe("--:--:--");
    expect(formatElapsedTime("2026-08-03T12:00:00Z", false, Date.now())).toBe("--:--:--");
  });
});
