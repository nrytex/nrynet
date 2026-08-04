import type { SettingItem } from "../types";

export const PLAIN_WS_SETTING_KEY = "server.plain_enabled";

export function splitSettingsRows(items: SettingItem[]) {
  return {
    plainWsSetting: items.find((item) => item.key === PLAIN_WS_SETTING_KEY),
    rows: items.filter((item) => item.key !== PLAIN_WS_SETTING_KEY),
  };
}

export function plainWsSaveMessage(enabled: boolean) {
  return `明文 WS 访问已${enabled ? "开启" : "关闭"}，重启 Nrynet 后生效`;
}
