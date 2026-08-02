import type { LogEntry } from "../bindings/github.com/nat-link/nat-link/desktop";

export function redact(value: string): string {
  if (!value) return "";
  if (value.length <= 8) return "********";
  return `${value.slice(0, 4)}...${value.slice(-4)}`;
}

export function formatTime(value?: string): string {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

export function logLine(entry: LogEntry): string {
  const fields = entry.fields ? JSON.stringify(entry.fields) : "";
  return `[${formatTime(entry.time)}] ${entry.level} ${entry.message} ${fields}`.trim();
}

export function formatBytes(value: number): string {
  if (value < 1024) return `${value} B`;
  const units = ["KiB", "MiB", "GiB"];
  let next = value / 1024;
  for (const unit of units) {
    if (next < 1024) return `${next.toFixed(1)} ${unit}`;
    next /= 1024;
  }
  return `${next.toFixed(1)} TiB`;
}
