import { authHeader, clearSession, saveSession } from "./session";
import type { Client, ClientDetail, LogEntry, Overview, RelayAssignment, RelayNode, SessionUser, SettingItem, Token, TrafficResponse, Tunnel } from "../types";

export class ApiError extends Error {
  constructor(public status: number, message: string) {
    super(message);
  }
}

type JsonObject = Record<string, unknown>;

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
  const response = await fetch(path, { ...options, headers: requestHeaders(options) });
  handleUnauthorized(response);
  if (response.status === 204) return undefined as T;
  const payload = await readPayload(response);
  if (!response.ok) throw new ApiError(response.status, errorMessage(payload, response.statusText));
  return payload as T;
}

function requestHeaders(options: RequestInit = {}) {
  const headers = new Headers(options.headers);
  headers.set("Accept", "application/json");
  if (options.body && !headers.has("Content-Type")) headers.set("Content-Type", "application/json");
  Object.entries(authHeader()).forEach(([key, value]) => headers.set(key, value));
  return headers;
}

function handleUnauthorized(response: Response) {
  if (response.status === 401) {
    clearSession();
    window.dispatchEvent(new CustomEvent("nrynet:unauthorized"));
  }
}

async function readPayload(response: Response): Promise<unknown> {
  const text = await response.text();
  if (!text) return {};
  try {
    return JSON.parse(text);
  } catch {
    return { error: text };
  }
}

function errorMessage(payload: unknown, fallback: string) {
  if (payload && typeof payload === "object" && "error" in payload) {
    return localizedErrorMessage(String((payload as JsonObject).error), fallback);
  }
  return fallbackErrorMessage(fallback);
}

function fallbackErrorMessage(fallback: string) {
  return localizedErrorMessage(fallback, "");
}

function localizedErrorMessage(message: string, fallback: string) {
  if (hasChinese(message)) return message;
  const mapped = mappedErrorMessage(message) || mappedErrorMessage(fallback);
  return mapped || "请求失败，请稍后重试";
}

function mappedErrorMessage(message: string) {
  const normalized = message.toLowerCase();
  if (!normalized) return "";
  if (normalized.includes("unauthorized") || normalized.includes("unauthenticated")) return "未授权，请重新登录";
  if (normalized.includes("forbidden") || normalized.includes("permission denied")) return "无权访问该资源";
  if (normalized.includes("not found")) return "资源不存在";
  if (normalized.includes("internal server error")) return "服务器内部错误";
  if (normalized.includes("bad request") || normalized.includes("invalid request")) return "请求参数无效";
  if (normalized.includes("invalid credentials") || normalized.includes("invalid username") || normalized.includes("invalid password")) return "用户名或密码错误";
  if (normalized.includes("token expired") || normalized.includes("expired token")) return "登录已过期，请重新登录";
  if (normalized.includes("network error") || normalized.includes("failed to fetch")) return "网络连接失败";
  return "";
}

function hasChinese(value: string) {
  return /[\u4e00-\u9fff]/.test(value);
}

const items = <T>(payload: { items: T[] }) => payload.items;

export const api = {
  async login(username: string, password: string) {
    const res = await request<{ token: string; token_type?: string }>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password }),
    });
    saveSession(res.token, res.token_type || "Bearer");
  },
  me: () => request<SessionUser>("/api/auth/me"),
  changePassword: (currentPassword: string, newPassword: string) => request<void>("/api/auth/password", {
    method: "POST",
    body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
  }),
  overview: () => request<Overview>("/api/overview"),
  listTokens: () => request<{ items: Token[] }>("/api/tokens").then(items),
  createToken: (name: string) =>
    request<{ token: Token; value: string }>("/api/tokens", { method: "POST", body: JSON.stringify({ name }) }),
  setTokenDisabled: (id: string, disabled: boolean) =>
    request<void>(`/api/tokens/${id}`, { method: "PATCH", body: JSON.stringify({ disabled }) }),
  deleteToken: (id: string) => request<void>(`/api/tokens/${id}`, { method: "DELETE" }),
  listClients: () => request<{ items: Client[] }>("/api/clients").then(items),
  getClient: (id: string) => request<ClientDetail>(`/api/clients/${id}`),
  updateClient: (id: string, body: Partial<Pick<Client, "name" | "disabled">>) =>
    request<void>(`/api/clients/${id}`, { method: "PATCH", body: JSON.stringify(body) }),
  deleteClient: (id: string) => request<void>(`/api/clients/${id}`, { method: "DELETE" }),
  resetClientToken: (id: string) => request<{ token: Token; value: string }>(`/api/clients/${id}/reset-token`, { method: "POST" }),
  listTunnels: () => request<{ items: Tunnel[] }>("/api/tunnels").then(items),
  createTunnel: (body: Partial<Tunnel>) => request<Tunnel>("/api/tunnels", { method: "POST", body: JSON.stringify(body) }),
  updateTunnel: (id: string, body: Partial<Tunnel>) =>
    request<Tunnel>(`/api/tunnels/${id}`, { method: "PUT", body: JSON.stringify(body) }),
  deleteTunnel: (id: string) => request<void>(`/api/tunnels/${id}`, { method: "DELETE" }),
  startTunnel: (id: string) => request<void>(`/api/tunnels/${id}/start`, { method: "POST" }),
  stopTunnel: (id: string) => request<void>(`/api/tunnels/${id}/stop`, { method: "POST" }),
  traffic: (range: "today" | "month" = "today") => request<TrafficResponse>(`/api/traffic/summary?range=${range}`),
  logs: (filter: { keyword?: string; level?: string; page?: number; limit?: number } = {}) => {
    const query = new URLSearchParams();
    if (filter.keyword) query.set("keyword", filter.keyword);
    if (filter.level) query.set("level", filter.level);
    query.set("page", String(filter.page ?? 1));
    query.set("limit", String(filter.limit ?? 100));
    return request<{ items: LogEntry[]; total: number; limit: number; offset: number }>(`/api/logs?${query}`);
  },
  clearLogs: () => request<{ deleted: number }>("/api/logs", { method: "DELETE" }),
  async downloadLogs(filter: { keyword?: string; level?: string } = {}) {
    const query = new URLSearchParams();
    if (filter.keyword) query.set("keyword", filter.keyword);
    if (filter.level) query.set("level", filter.level);
    const response = await fetch(`/api/logs/download?${query}`, { headers: requestHeaders() });
    handleUnauthorized(response);
    if (!response.ok) {
      const payload = await readPayload(response);
      throw new ApiError(response.status, errorMessage(payload, response.statusText));
    }
    const url = URL.createObjectURL(await response.blob());
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "nrynet-logs.jsonl";
    anchor.click();
    URL.revokeObjectURL(url);
  },
  settings: () => request<{ items: SettingItem[] }>("/api/settings").then(items),
  updateSetting: (key: string, value: SettingItem["value"]) =>
    request<SettingItem>(`/api/settings/${encodeURIComponent(key)}`, { method: "PATCH", body: JSON.stringify({ value }) }),
  relays: () => request<{ nodes: RelayNode[] }>("/api/v2/relays").then((payload) => payload.nodes),
  relayAssignments: () => request<{ assignments: RelayAssignment[] }>("/api/v2/relays/assignments").then((payload) => payload.assignments),
};
