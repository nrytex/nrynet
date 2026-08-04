import { describe, expect, it } from "vitest";
import { PLAIN_WS_SETTING_KEY, plainWsSaveMessage, splitSettingsRows } from "../settings/settingsRows";
import type { SettingItem } from "../types";

describe("settings page helpers", () => {
  it("extracts the plain WS switch from the raw server settings table", () => {
    const settings: SettingItem[] = [
      { key: "server.listen", value: "0.0.0.0:7001", mutable: true },
      { key: PLAIN_WS_SETTING_KEY, value: false, mutable: true },
      { key: "server.plain_listen", value: "0.0.0.0:7004", mutable: true },
    ];

    const result = splitSettingsRows(settings);

    expect(result.plainWsSetting?.key).toBe(PLAIN_WS_SETTING_KEY);
    expect(result.rows.map((item) => item.key)).toEqual(["server.listen", "server.plain_listen"]);
  });

  it("describes the restart requirement after toggling plain WS access", () => {
    expect(plainWsSaveMessage(true)).toBe("明文 WS 访问已开启，重启 Nrynet 后生效");
    expect(plainWsSaveMessage(false)).toBe("明文 WS 访问已关闭，重启 Nrynet 后生效");
  });
});
