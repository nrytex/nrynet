import { describe, expect, it } from "vitest";
import { certificatePending, certificateStateLabel, endpointRows, nextPlainState, nextTLSState } from "../settings/transportDisplay";
import type { TransportStatus } from "../types";

const baseStatus: TransportStatus = {
  plain: {
    enabled: true,
    listen: "0.0.0.0:7000",
    data_listen: "0.0.0.0:7002",
  },
  compatibility_plain: {
    enabled: false,
    listen: "0.0.0.0:7004",
    data_listen: "0.0.0.0:7005",
  },
  tls: {
    enabled: false,
    listen: "0.0.0.0:7001",
    data_listen: "0.0.0.0:7003",
  },
  certbot: { available: true },
};

describe("transport display helpers", () => {
  it("shows HTTP, WS and plaintext data endpoints by default", () => {
    expect(endpointRows(baseStatus).map((row) => row.label)).toEqual(["控制台 HTTP", "Agent WS", "明文数据通道"]);
  });

  it("adds HTTPS, WSS and TLS data endpoints when TLS is enabled", () => {
    const rows = endpointRows({ ...baseStatus, tls: { ...baseStatus.tls, enabled: true } });

    expect(rows.map((row) => row.label)).toContain("控制台 HTTPS");
    expect(rows.map((row) => row.label)).toContain("Agent WSS");
    expect(rows.map((row) => row.label)).toContain("TLS 数据通道");
  });

  it("labels certificate issuing states", () => {
    expect(certificateStateLabel("issuing")).toBe("签发中");
    expect(certificateStateLabel("valid")).toBe("已签发");
    expect(certificateStateLabel("failed")).toBe("签发失败");
    expect(certificatePending({ ...baseStatus, certificate: { status: "pending" } })).toBe(true);
  });

  it("computes next toggle states", () => {
    expect(nextTLSState(baseStatus)).toBe(true);
    expect(nextPlainState(baseStatus)).toBe(true);
  });
});
