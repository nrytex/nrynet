import { Tag } from "antd";
import { statusText } from "../display/status";

export function StatusTag({ value }: { value: string }) {
  const normalized = value.toLowerCase();
  const color = normalized === "online" || normalized === "running" ? "green" : normalized === "disabled" ? "red" : "default";
  return <Tag color={color}>{statusText(value)}</Tag>;
}
