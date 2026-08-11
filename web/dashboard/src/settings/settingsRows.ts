import type { SettingItem } from "../types";

export const PLAIN_WS_SETTING_KEY = "server.plain_enabled";
export const TRANSPORT_SETTING_KEYS = new Set([
  PLAIN_WS_SETTING_KEY,
  "server.plain_listen",
  "server.plain_data_listen",
  "server.listen",
  "server.data_listen",
  "server.tls.enabled",
  "server.tls.cert_file",
  "server.tls.key_file",
]);

export function splitSettingsRows(items: SettingItem[]) {
  return {
    plainWsSetting: items.find((item) => item.key === PLAIN_WS_SETTING_KEY),
    rows: items.filter((item) => !TRANSPORT_SETTING_KEYS.has(item.key)),
  };
}

export function settingSaveMessage(key: string) {
  if (key === "server.p2p_enabled") return "P2P 设置已保存并立即生效";
  return "设置已保存，重启 Nrynet 后生效";
}

export function plainWsSaveMessage(enabled: boolean) {
  return `兼容明文访问已${enabled ? "开启" : "关闭"}，配置已热更新`;
}
