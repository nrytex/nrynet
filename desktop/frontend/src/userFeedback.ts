import type { AppConfig, RuntimeStatus } from "../bindings/github.com/nat-link/nat-link/desktop";
import type { SettingsSection } from "./SettingsView";

export type FeedbackAction = "load" | "connect" | "save" | "update";

export interface ConfigIssue {
  message: string;
  section: SettingsSection;
}

export function connectionConfigIssue(config?: Partial<AppConfig>): ConfigIssue | undefined {
  if (!config?.serverUrl?.trim()) {
    return { message: "请先在“网络设置”中填写控制服务器地址。", section: "network" };
  }
  if (config.transport === "quic" && !config.quicAddress?.trim()) {
    return { message: "已选择 QUIC 传输，请先填写 QUIC 地址。", section: "network" };
  }
  if (config.transport !== "quic" && !config.dataAddress?.trim()) {
    return { message: "请先在“网络设置”中填写数据通道地址。", section: "network" };
  }
  if (!config.token?.trim()) {
    return { message: "请先在“连接设置”中填写 Agent Token。", section: "connection" };
  }
  return undefined;
}

export function userErrorMessage(error: unknown, action: FeedbackAction): string {
  const raw = errorText(error);
  if (containsChinese(raw)) return raw.replace(/^error:\s*/i, "");
  for (const rule of errorRules) {
    if (rule.pattern.test(raw)) return rule.message;
  }
  return fallbackMessages[action];
}

export function connectionStatusMessage(status?: RuntimeStatus): string | undefined {
  if (!status || status.connected) return undefined;
  if (status.state === "connecting") return "正在连接服务器，请稍候...";
  if (!status.message || status.message.includes("已由用户断开")) return undefined;
  return userErrorMessage(status.message, "connect");
}

const errorRules = [
  { pattern: /server_url is required/i, message: "尚未配置控制服务器，请前往“设置 > 网络设置”填写后重试。" },
  { pattern: /data_address is required/i, message: "尚未配置数据通道，请前往“设置 > 网络设置”填写后重试。" },
  { pattern: /quic_address is required/i, message: "已选择 QUIC 传输，但尚未填写 QUIC 地址。" },
  { pattern: /token is required/i, message: "尚未配置 Agent Token，请前往“设置 > 连接设置”填写后重试。" },
  { pattern: /invalid or disabled|invalid agent token|bad handshake/i, message: "服务器拒绝连接，请检查 Agent Token 是否正确且仍然有效。" },
  { pattern: /unknown authority|certificate/i, message: "服务器证书校验失败，请检查服务器域名和私有 CA 配置。" },
  { pattern: /no such host/i, message: "找不到服务器域名，请检查地址拼写和 DNS 网络设置。" },
  { pattern: /connection refused|actively refused/i, message: "服务器拒绝连接，请确认服务端已启动且端口可访问。" },
  { pattern: /timeout|deadline exceeded/i, message: "连接服务器超时，请检查网络、防火墙和服务器端口。" },
  { pattern: /update check is already running/i, message: "正在检查更新，请稍候。" },
  { pattern: /update settings changed/i, message: "更新设置已变更，请重启 NAT-Link 后再检查更新。" },
];

const fallbackMessages: Record<FeedbackAction, string> = {
  load: "客户端信息加载失败，请重启 NAT-Link；若问题仍然存在，请查看运行日志。",
  connect: "连接失败，请检查服务器地址、网络和服务端状态后重试。",
  save: "设置保存失败，请检查填写内容和系统权限后重试。",
  update: "检查更新失败，请检查网络连接和 GitHub Release 状态后重试。",
};

function errorText(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === "string") return error;
  if (error && typeof error === "object" && "message" in error) return String(error.message);
  return String(error ?? "");
}

function containsChinese(value: string): boolean {
  return /[\u3400-\u9fff]/.test(value);
}
