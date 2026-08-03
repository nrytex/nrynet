import type { DesktopSnapshot } from "../bindings/github.com/nat-link/nat-link/desktop";

export function makePreviewSnapshot(tick: number): DesktopSnapshot {
  const started = new Date(Date.now() - 83 * 60 * 1000).toISOString();
  return {
    config: {
      serverUrl: "wss://vip1.nat-link.app/agent/connect", dataAddress: "vip1.nat-link.app:7001",
      transport: "websocket", quicAddress: "vip1.nat-link.app:7002", caFile: "",
      token: "ntk_demo_token", name: "Studio Client", deviceId: "studio-desktop",
      insecureSkipVerify: false, autoStart: true,
    },
    status: {
      connected: true, state: "connected", message: "authenticated session started", version: "2.3.4",
      uploadBytes: 42_440_000 + tick * 1_088_000, downloadBytes: 8_290_000 + tick * 310_000,
      lastStartedAt: started,
    },
    tunnels: [
      tunnel("preview-1", "9Router", "tcp", "192.168.1.1", 80, 3007, "vip1.nat-link.app"),
      tunnel("preview-2", "Home NAS", "https", "192.168.1.20", 443, 3008, "nas.nat-link.app"),
      tunnel("preview-3", "Minecraft", "tcp", "192.168.1.30", 25565, 3009, "vip1.nat-link.app"),
      tunnel("preview-4", "Media Lab", "udp", "192.168.1.40", 1900, 3010, "vip1.nat-link.app"),
    ],
    logs: [
      { time: new Date().toISOString(), level: "INFO", message: "authenticated session started", fields: null },
      { time: started, level: "INFO", message: "configuration loaded", fields: null },
    ],
  };
}

function tunnel(id: string, name: string, protocol: string, host: string, localPort: number, remotePort: number, domain: string) {
  return {
    id, name, protocol, client_id: "preview-client", local_host: host, local_port: localPort,
    remote_port: remotePort, domain, status: "running", ip_allowlist: [],
    created_at: "2026-05-20T14:25:36Z", updated_at: "2026-05-20T14:25:36Z",
  };
}
