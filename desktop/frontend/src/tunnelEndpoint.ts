import type { AppConfig } from "../bindings/github.com/nrytex/nrynet/desktop";
import type { Tunnel } from "../bindings/github.com/nrytex/nrynet/internal/model";

export interface TunnelEndpoint {
  label: string;
  copyValue?: string;
}

export function tunnelPublicHost(config?: Pick<AppConfig, "dataAddress" | "serverUrl">): string {
  const dataHost = addressHost(config?.dataAddress);
  if (dataHost) return dataHost;
  if (!config?.serverUrl) return "";
  try {
    return new URL(config.serverUrl).hostname;
  } catch {
    return "";
  }
}

export function resolveTunnelEndpoint(tunnel: Tunnel, publicHost: string): TunnelEndpoint {
  const protocol = tunnel.protocol.toLowerCase();
  if (tunnel.domain) {
    if (protocol === "http" || protocol === "https") {
      return endpoint(`${protocol}://${tunnel.domain}`);
    }
    return endpoint(withPort(tunnel.domain, tunnel.remote_port));
  }
  if (!tunnel.remote_port || !publicHost) return { label: "未获取到访问地址" };
  return endpoint(withPort(publicHost, tunnel.remote_port));
}

function endpoint(value: string): TunnelEndpoint {
  return { label: value, copyValue: value };
}

function addressHost(address?: string): string {
  if (!address) return "";
  try {
    return new URL(`tcp://${address}`).hostname;
  } catch {
    return "";
  }
}

function withPort(host: string, port: number): string {
  const normalized = host.includes(":") && !host.startsWith("[") ? `[${host}]` : host;
  return port ? `${normalized}:${port}` : normalized;
}
