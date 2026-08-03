import type { Status } from "../types";

const statusLabels: Record<string, string> = {
  active: "活跃",
  disabled: "已禁用",
  error: "错误",
  healthy: "健康",
  offline: "离线",
  online: "在线",
  running: "运行中",
  stopped: "已停止",
  unknown: "未知",
  warn: "警告",
};

export function statusText(value?: Status) {
  if (!value) return statusLabels.unknown;
  const normalized = value.toLowerCase();
  return statusLabels[normalized] ?? statusLabels.unknown;
}
