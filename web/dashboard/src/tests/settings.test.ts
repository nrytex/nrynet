import { describe, expect, it } from "vitest";
import { PLAIN_WS_SETTING_KEY, plainWsSaveMessage, splitSettingsRows } from "../settings/settingsRows";
import type { SettingItem } from "../types";

describe("settings page helpers", () => {
  it("extracts the plain WS switch from the raw server settings table", () => {
    const settings: SettingItem[] = [
      { key: "server.listen", value: "0.0.0.0:7001", mutable: true },
      { key: PLAIN_WS_SETTING_KEY, value: false, mutable: true },
      { key: "server.plain_listen", value: "0.0.0.0:7004", mutable: true },
      { key: "server.tls.enabled", value: false, mutable: true },
      { key: "auth.jwt_secret", value: "secret", mutable: true },
    ];

    const result = splitSettingsRows(settings);

    expect(result.plainWsSetting?.key).toBe(PLAIN_WS_SETTING_KEY);
    expect(result.rows.map((item) => item.key)).toEqual(["auth.jwt_secret"]);
  });

  it("describes hot updates after toggling compatibility plain access", () => {
    expect(plainWsSaveMessage(true)).toBe("兼容明文访问已开启，配置已热更新");
    expect(plainWsSaveMessage(false)).toBe("兼容明文访问已关闭，配置已热更新");
  });
});
