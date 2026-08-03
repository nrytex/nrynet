import type { AppConfig } from "../bindings/github.com/nrytex/nrynet/desktop";

export const emptyConfig: AppConfig = {
  serverUrl: "",
  dataAddress: "",
  transport: "websocket",
  quicAddress: "",
  caFile: "",
  token: "",
  name: "",
  deviceId: "",
  insecureSkipVerify: false,
  autoStart: false,
};

export function completeConfigForSave(
  current: AppConfig | undefined,
  draft: Partial<AppConfig>,
): AppConfig {
  return { ...emptyConfig, ...current, ...draft };
}
