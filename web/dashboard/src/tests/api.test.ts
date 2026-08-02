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
});

function mockFetch(body: unknown, status = 200) {
  const fetchMock = vi.fn(async () => new Response(JSON.stringify(body), { status, headers: { "Content-Type": "application/json" } }));
  vi.stubGlobal("fetch", fetchMock);
  return fetchMock;
}
