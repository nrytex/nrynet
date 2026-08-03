import { describe, expect, it } from "vitest";
import { connectionConfigIssue, connectionStatusMessage, userErrorMessage } from "./userFeedback";

describe("connectionConfigIssue", () => {
  it("guides an unconfigured user to the relevant settings section", () => {
    expect(connectionConfigIssue({})?.section).toBe("network");
    expect(connectionConfigIssue({ serverUrl: "wss://nat.example", dataAddress: "nat.example:7001" })?.section).toBe("connection");
  });

  it("accepts the required WebSocket connection fields", () => {
    expect(connectionConfigIssue({
      serverUrl: "wss://nat.example/agent/connect",
      dataAddress: "nat.example:7001",
      transport: "websocket",
      token: "secret",
    })).toBeUndefined();
  });
});

describe("userErrorMessage", () => {
  it("translates known backend errors", () => {
    expect(userErrorMessage("client.server_url is required", "connect")).toContain("控制服务器");
    expect(userErrorMessage(new Error("dial tcp: connection refused"), "connect")).toContain("服务器拒绝连接");
  });

  it("guides certificate errors to a new pinned token", () => {
    expect(userErrorMessage("x509: certificate signed by unknown authority", "connect")).toContain("重新生成 Agent Token");
    expect(userErrorMessage("server TLS certificate pin does not match the Agent Token", "connect")).toContain("证书已发生变化");
  });

  it("never exposes an unknown English error in the user prompt", () => {
    expect(userErrorMessage("opaque runtime error", "save")).toBe("设置保存失败，请检查填写内容和系统权限后重试。");
  });

  it("keeps an already localized backend message", () => {
    expect(userErrorMessage("Error: 服务器证书已过期，请联系管理员。", "connect")).toBe("服务器证书已过期，请联系管理员。");
  });
});

describe("connectionStatusMessage", () => {
  it("shows connecting progress and hides intentional disconnects", () => {
    expect(connectionStatusMessage({ connected: false, state: "connecting" } as never)).toContain("正在连接");
    expect(connectionStatusMessage({ connected: false, state: "disconnected", message: "已由用户断开连接。" } as never)).toBeUndefined();
  });
});
