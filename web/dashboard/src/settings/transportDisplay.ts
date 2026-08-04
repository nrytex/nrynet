import type { TransportEndpoint, TransportStatus } from "../types";

export function certificatePending(status?: TransportStatus) {
  const value = status?.certificate?.status?.toLowerCase() ?? "";
  return value === "pending" || value === "running" || value === "issuing";
}

export function certificateStateLabel(status?: string) {
  switch ((status ?? "").toLowerCase()) {
    case "valid":
    case "issued":
    case "success":
      return "已签发";
    case "pending":
    case "running":
    case "issuing":
      return "签发中";
    case "failed":
    case "error":
      return "签发失败";
    default:
      return "未绑定";
  }
}

export function endpointRows(status?: TransportStatus) {
  const plain = status?.plain;
  const tls = status?.tls;
  const rows = [
    { key: "http", label: "控制台 HTTP", value: valueOrFallback(plain?.control_url, plain?.listen, "http") },
    { key: "ws", label: "Agent WS", value: valueOrFallback(plain?.websocket_url, plain?.listen, "ws", "/agent/connect") },
    { key: "plain-data", label: "明文数据通道", value: plain?.data_address || plain?.data_listen || "-" },
  ];

  if (!tls?.enabled) return rows;
  return rows.concat([
    { key: "https", label: "控制台 HTTPS", value: valueOrFallback(tls.control_url, tls.listen, "https") },
    { key: "wss", label: "Agent WSS", value: valueOrFallback(tls.websocket_url, tls.listen, "wss", "/agent/connect") },
    { key: "tls-data", label: "TLS 数据通道", value: tls.data_address || tls.data_listen || "-" },
  ]);
}

export function nextTLSState(status?: TransportStatus) {
  return !Boolean(status?.tls?.enabled);
}

export function nextPlainState(status?: TransportStatus) {
  return !Boolean(status?.compatibility_plain?.enabled);
}

function valueOrFallback(value: string | undefined, listen: string | undefined, scheme: string, path = "") {
  if (value) return value;
  if (!listen) return "-";
  return `${scheme}://${normalizeListen(listen)}${path}`;
}

function normalizeListen(listen: string) {
  if (!listen.startsWith("0.0.0.0:")) return listen;
  return listen.replace("0.0.0.0:", "服务器IP:");
}
