import { describe, expect, it } from "vitest";
import { autoSubdomainHint, autoSubdomainStatusText, shouldSuggestAutoDomain } from "../settings/autoSubdomain";

describe("auto subdomain helpers", () => {
  it("describes disabled and enabled status in Chinese", () => {
    expect(autoSubdomainStatusText({ enabled: false })).toBe("未开启");
    expect(autoSubdomainStatusText({ enabled: true })).toBe("待配置根域名");
    expect(autoSubdomainStatusText({ enabled: true, base_domain: "tunnels.example.com" })).toBe("已开启：*.tunnels.example.com");
  });

  it("only suggests automatic domains for HTTP and HTTPS tunnels", () => {
    const config = { enabled: true, base_domain: "tunnels.example.com" };

    expect(shouldSuggestAutoDomain("http", config)).toBe(true);
    expect(shouldSuggestAutoDomain("https", config)).toBe(true);
    expect(shouldSuggestAutoDomain("tcp", config)).toBe(false);
    expect(shouldSuggestAutoDomain("http", { ...config, enabled: false })).toBe(false);
  });

  it("explains explicit domain priority", () => {
    expect(autoSubdomainHint({ enabled: true, base_domain: "tunnels.example.com", suffix_example: "app.tunnels.example.com" }))
      .toContain("手动填写域名时优先使用显式域名");
  });
});
