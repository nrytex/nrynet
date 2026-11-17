import { App } from "antd";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { api } from "../api/client";
import { ClientsPage } from "../pages/ClientsPage";
import { SettingsPage } from "../pages/SettingsPage";
import { TunnelsPage } from "../pages/TunnelsPage";
import { TokensPage } from "../pages/TokensPage";
import type { Client, SettingItem, Tunnel } from "../types";

const tunnels = [
  { id: "first", name: "first tunnel", protocol: "tcp", client_id: "client-1", local_host: "192.168.3.10", local_port: 8080, remote_port: 6001, domain: "", status: "stopped", ip_allowlist: ["10.0.0.1"] },
  { id: "second", name: "second tunnel", protocol: "http", client_id: "client-1", local_host: "192.168.3.20", local_port: 9000, remote_port: 0, domain: "second.example.com", status: "stopped", ip_allowlist: [] },
] as Tunnel[];

let root: Root;
let container: HTMLDivElement;

beforeEach(() => {
  vi.stubGlobal("IS_REACT_ACT_ENVIRONMENT", true);
  vi.stubGlobal("ResizeObserver", class { observe() {} unobserve() {} disconnect() {} });
  const getComputedStyle = window.getComputedStyle;
  vi.spyOn(window, "getComputedStyle").mockImplementation((element) => getComputedStyle.call(window, element));
  window.matchMedia = vi.fn().mockImplementation(() => ({
    matches: false, addListener() {}, removeListener() {}, addEventListener() {}, removeEventListener() {}, dispatchEvent: () => false,
  }));
  container = document.createElement("div");
  document.body.appendChild(container);
  root = createRoot(container);
});

afterEach(async () => {
  await act(async () => root.unmount());
  container.remove();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function click(element: Element | null) {
  expect(element).not.toBeNull();
  await act(async () => { (element as HTMLElement).click(); });
}

function row(name: string) {
  return [...document.querySelectorAll("tr")].find((element) => element.textContent?.includes(name));
}

function button(parent: ParentNode, label: string) {
  return [...parent.querySelectorAll("button")].find((element) => element.textContent?.replace(/\s/g, "") === label) ?? null;
}

function input(name: string) {
  return document.querySelector<HTMLInputElement>(`.ant-modal input[id="${name}"]`)?.value;
}

describe("dashboard form state", () => {
  it("loads the selected tunnel and clears the previous tunnel when creating", async () => {
    vi.spyOn(api, "listTunnels").mockResolvedValue(tunnels);
    vi.spyOn(api, "listClients").mockResolvedValue([{ id: "client-1", name: "client" }] as Awaited<ReturnType<typeof api.listClients>>);
    vi.spyOn(api, "transport").mockResolvedValue({} as Awaited<ReturnType<typeof api.transport>>);

    await act(async () => root.render(<App><TunnelsPage /></App>));
    await click(button(row("first tunnel")!, "编辑"));
    expect(input("name")).toBe("first tunnel");
    expect(input("local_host")).toBe("192.168.3.10");

    await click(document.querySelector(".ant-modal-close"));
    await click(button(row("second tunnel")!, "编辑"));
    expect(input("name")).toBe("second tunnel");
    expect(input("local_host")).toBe("192.168.3.20");
    expect(input("remote_port")).toBe("0");
    expect(input("domain")).toBe("second.example.com");
    expect(input("ip_allowlist_text")).toBe("");

    await click(document.querySelector(".ant-modal-close"));
    await click(button(container, "创建"));
    expect(input("name")).toBe("");
    expect(input("local_host")).toBe("127.0.0.1");
    expect(input("domain")).toBe("");
  });

  it("discards a cancelled token draft", async () => {
    vi.spyOn(api, "listTokens").mockResolvedValue([]);
    await act(async () => root.render(<App><TokensPage /></App>));
    await click(button(container, "创建"));
    const name = document.querySelector<HTMLInputElement>(".ant-modal input[id='name']")!;
    await act(async () => {
      Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")!.set!.call(name, "old draft");
      name.dispatchEvent(new Event("input", { bubbles: true }));
    });
    expect(name.value).toBe("old draft");
    await click(document.querySelector(".ant-modal-close"));
    await click(button(container, "创建"));
    expect(input("name")).toBe("");
  });

  it("shows each client's own details when editing", async () => {
    vi.spyOn(api, "listClients").mockResolvedValue([
      { id: "first", name: "first client", disabled: true, status: "offline" },
      { id: "second", name: "second client", disabled: false, status: "online" },
    ] as Client[]);

    await act(async () => root.render(<App><ClientsPage /></App>));
    await click(button(row("first client")!, "编辑"));
    expect(input("name")).toBe("first client");
    expect(document.querySelector(".ant-modal button[role='switch']")?.getAttribute("aria-checked")).toBe("true");
    await click(document.querySelector(".ant-modal-close"));
    await click(button(row("second client")!, "编辑"));
    expect(input("name")).toBe("second client");
    expect(document.querySelector(".ant-modal button[role='switch']")?.getAttribute("aria-checked")).toBe("false");
  });

  it("refreshes a changed setting value", async () => {
    const settings = vi.spyOn(api, "settings")
      .mockResolvedValueOnce([{ key: "server.public_data_address", value: "old.example.com:7001", mutable: true }] as SettingItem[])
      .mockResolvedValueOnce([{ key: "server.public_data_address", value: "new.example.com:7001", mutable: true }] as SettingItem[]);
    vi.spyOn(api, "transport").mockResolvedValue({} as Awaited<ReturnType<typeof api.transport>>);

    await act(async () => root.render(<App><SettingsPage /></App>));
    expect(settings).toHaveBeenCalledTimes(1);
    expect(container.querySelector<HTMLInputElement>("tr input[id='value']")?.value).toBe("old.example.com:7001");
    await click(button(container, "刷新"));
    expect(container.querySelector<HTMLInputElement>("tr input[id='value']")?.value).toBe("new.example.com:7001");
  });
});
