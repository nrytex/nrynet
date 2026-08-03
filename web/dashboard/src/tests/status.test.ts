import { describe, expect, it } from "vitest";
import { statusText } from "../display/status";

describe("status labels", () => {
  it("localizes known states", () => {
    expect(statusText("online")).toBe("在线");
    expect(statusText("running")).toBe("运行中");
    expect(statusText("disabled")).toBe("已禁用");
    expect(statusText("healthy")).toBe("健康");
  });

  it("wraps unknown server states in a Chinese label", () => {
    expect(statusText("degraded")).toBe("未知");
  });
});
