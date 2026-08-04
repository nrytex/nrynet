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

export function plainWsSaveMessage(enabled: boolean) {
  return `兼容明文访问已${enabled ? "开启" : "关闭"}，配置已热更新`;
}
