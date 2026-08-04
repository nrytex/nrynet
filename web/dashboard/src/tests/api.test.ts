import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError, api } from "../api/client";
import { getSession, saveSession } from "../api/session";

describe("api client", () => {
  beforeEach(() => {
    localStorage.clear();
    vi.restoreAllMocks();
  });

  it("saves login token", async () => {
    mockFetch({ token: "jwt", token_type: "Bearer" });
    await api.login("admin", "secret");
    expect(getSession()?.token).toBe("jwt");
  });

  it("sends authorization header", async () => {
    const fetchMock = mockFetch({ items: [] });
    saveSession("jwt");
    await api.listTokens();
    const call = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    const headers = call[1].headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer jwt");
  });

  it("clears session on unauthorized response", async () => {
    saveSession("jwt");
    mockFetch({ error: "nope" }, 401);
    await expect(api.listClients()).rejects.toBeInstanceOf(ApiError);
    expect(getSession()).toBeNull();
  });

  it("localizes English error bodies from the server", async () => {
    mockFetch({ error: "invalid credentials" }, 401);
    await expect(api.login("admin", "bad-password")).rejects.toThrow("用户名或密码错误");
  });

  it("uses a Chinese fallback for unknown English error bodies", async () => {
    mockFetch({ error: "relay frobnicator exploded" }, 500);
    await expect(api.listClients()).rejects.toThrow("请求失败，请稍后重试");
  });

  it("explains missing plaintext listener addresses", async () => {
    mockFetch({ error: "server.plain_listen and server.plain_data_listen are required when server.plain_enabled is true" }, 400);
    await expect(api.updateSetting("server.plain_enabled", true)).rejects.toThrow("请先配置明文 WS 控制端口和数据端口");
  });

  it("preserves Chinese error bodies from the server", async () => {
    mockFetch({ error: "隧道端口已被占用" }, 409);
    await expect(api.listTunnels()).rejects.toThrow("隧道端口已被占用");
  });

  it("sends server-side log filters", async () => {
    const fetchMock = mockFetch({ items: [], total: 0, limit: 100, offset: 200 });
    await api.logs({ keyword: "closed", level: "warn", page: 3 });
    const call = fetchMock.mock.calls[0] as unknown as [string, RequestInit];
    const path = String(call[0]);
    expect(path).toContain("keyword=closed");
    expect(path).toContain("level=warn");
    expect(path).toContain("page=3");
  });

  it("localizes common response status fallback messages", async () => {
    mockFetch({}, 500, "Internal Server Error");
    await expect(api.listTokens()).rejects.toThrow("服务器内部错误");
  });

  it("handles download authorization and localized error bodies", async () => {
    saveSession("jwt");
    mockFetch({ error: "unauthorized" }, 401);
    await expect(api.downloadLogs()).rejects.toThrow("未授权，请重新登录");
    expect(getSession()).toBeNull();
  });

  it("calls transport settings endpoints with expected payloads", async () => {
    const fetchMock = mockFetch({
      plain: { enabled: true },
      tls: { enabled: false },
      certbot: { available: true },
    });

    await api.transport();
    await api.requestCertificate("nrynet.example.com", "admin@example.com");
    await api.setTransportTLS(true);
    await api.setTransportPlain(false);
    await api.setAutoSubdomain(true, "tunnels.example.com");

    const calls = fetchMock.mock.calls as unknown as [string, RequestInit][];
    expect(calls[0][0]).toBe("/api/transport");
    expect(calls[1][0]).toBe("/api/transport/certificates");
    expect(JSON.parse(String(calls[1][1].body))).toEqual({
      domain: "nrynet.example.com",
      email: "admin@example.com",
    });
    expect(calls[2][0]).toBe("/api/transport/tls");
    expect(JSON.parse(String(calls[2][1].body))).toEqual({ enabled: true });
    expect(calls[3][0]).toBe("/api/transport/plain");
    expect(JSON.parse(String(calls[3][1].body))).toEqual({ enabled: false });
    expect(calls[4][0]).toBe("/api/transport/auto-subdomain");
    expect(JSON.parse(String(calls[4][1].body))).toEqual({ enabled: true, base_domain: "tunnels.example.com" });
  });
});

function mockFetch(body: unknown, status = 200, statusText = "") {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(body), { status, statusText, headers: { "Content-Type": "application/json" } }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
