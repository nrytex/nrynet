import { describe, expect, it } from "vitest";
import type { Tunnel } from "../bindings/github.com/nrytex/nrynet/internal/model";
import { resolveTunnelEndpoint, tunnelPublicHost } from "./tunnelEndpoint";

const tunnel = (values: Partial<Tunnel>): Tunnel => ({
  id: "tunnel", client_id: "client", name: "test", protocol: "udp",
  local_host: "127.0.0.1", local_port: 4246, remote_port: 6000,
  domain: "", status: "running", ip_allowlist: [],
  created_at: "2026-08-03T12:00:00Z", updated_at: "2026-08-03T12:00:00Z",
  ...values,
});

describe("tunnel endpoints", () => {
  it("uses the public data host for port tunnels", () => {
    const host = tunnelPublicHost({ dataAddress: "nat.nrytex.com:7001", serverUrl: "wss://150.158.46.132:7000/agent/connect" });
    expect(resolveTunnelEndpoint(tunnel({}), host)).toEqual({ label: "nat.nrytex.com:6000", copyValue: "nat.nrytex.com:6000" });
  });

  it("formats domain tunnels as usable URLs", () => {
    expect(resolveTunnelEndpoint(tunnel({ protocol: "https", domain: "predict.example.com", remote_port: 3008 }), "server.example.com"))
      .toEqual({ label: "https://predict.example.com", copyValue: "https://predict.example.com" });
  });

  it("falls back to the control server host", () => {
    expect(tunnelPublicHost({ dataAddress: "", serverUrl: "wss://150.158.46.132:7000/agent/connect" })).toBe("150.158.46.132");
  });

  it("does not expose a port-only address when the public host is unavailable", () => {
    expect(resolveTunnelEndpoint(tunnel({}), "")).toEqual({ label: "未获取到访问地址" });
  });

  it("formats bracketed IPv6 public hosts", () => {
    const host = tunnelPublicHost({ dataAddress: "[2001:db8::1]:7001", serverUrl: "" });
    expect(resolveTunnelEndpoint(tunnel({}), host).copyValue).toBe("[2001:db8::1]:6000");
  });

  it("builds a browser visitor URL from the control server", () => {
    expect(resolveTunnelEndpoint(
      tunnel({ protocol: "visitor_webrtc", remote_port: 0, visitor_token: "visitor-secret" }),
      "",
      "wss://server.example:7000/agent/connect",
    )).toEqual({
      label: "https://server.example:7000/visitor/tunnel/visitor-secret",
      copyValue: "https://server.example:7000/visitor/tunnel/visitor-secret",
    });
  });
});
