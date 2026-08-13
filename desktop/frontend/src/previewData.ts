import type { DesktopSnapshot } from "../bindings/github.com/nrytex/nrynet/desktop";

const previewStartedAt = new Date(Date.now() - 83 * 60 * 1000).toISOString();

export function makePreviewSnapshot(tick: number): DesktopSnapshot {
  return {
    config: {
      serverUrl: "wss://vip1.nrynet.app/agent/connect", dataAddress: "vip1.nrynet.app:7001",
      transport: "websocket", quicAddress: "vip1.nrynet.app:7002", caFile: "",
      token: "ntk_demo_token", name: "Studio Client", deviceId: "studio-desktop",
      insecureSkipVerify: false, autoStart: true,
    },
    status: {
      connected: true, state: "connected", message: "authenticated session started", version: "2.3.4",
      uploadBytes: 42_440_000 + tick * 1_088_000, downloadBytes: 8_290_000 + tick * 310_000,
      lastStartedAt: previewStartedAt,
    },
    tunnels: [
      tunnel("preview-1", "9Router", "tcp", "192.168.1.1", 80, 3007, "vip1.nrynet.app"),
      tunnel("preview-2", "Home NAS", "https", "192.168.1.20", 443, 3008, "nas.nrynet.app"),
      tunnel("preview-3", "Minecraft", "tcp", "192.168.1.30", 25565, 3009, "vip1.nrynet.app"),
      tunnel("preview-4", "Media Lab", "udp", "192.168.1.40", 1900, 3010, "vip1.nrynet.app"),
      tunnel("preview-5", "Visitor API", "visitor_webrtc", "127.0.0.1", 8080, 0, "", "preview-visitor-token"),
    ],
    tunnelPaths: { "preview-1": "p2p", "preview-2": "relay", "preview-3": "p2p", "preview-4": "relay", "preview-5": "visitor_p2p" },
    logs: [
      { time: new Date().toISOString(), level: "INFO", message: "authenticated session started", fields: null },
      { time: previewStartedAt, level: "INFO", message: "configuration loaded", fields: null },
    ],
    update: {
      checked: true, available: true, latestVersion: "2.5.0",
      downloadURL: "https://github.com/nrytex/nrynet/releases/download/v2.5.0/nrynet-desktop-windows-amd64.zip",
      downloadState: "ready", ready: true,
      message: "新版本 2.5.0 已下载，重启应用即可完成更新。",
    },
  };
}

function tunnel(id: string, name: string, protocol: string, host: string, localPort: number, remotePort: number, domain: string, visitor_token = "") {
  return {
    id, name, protocol, client_id: "preview-client", local_host: host, local_port: localPort,
    remote_port: remotePort, domain, visitor_token, status: "running", ip_allowlist: [],
    created_at: "2026-05-20T14:25:36Z", updated_at: "2026-05-20T14:25:36Z",
  };
}
