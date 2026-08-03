import { App as AntApp, ConfigProvider, Form } from "antd";
import { useEffect, useRef, useState } from "react";
import "antd/dist/reset.css";
import { DesktopService } from "../bindings/github.com/nat-link/nat-link/desktop";
import type { AppConfig, DesktopSnapshot } from "../bindings/github.com/nat-link/nat-link/desktop";
import { HomeView } from "./HomeView";
import { SettingsView, type SettingsSection } from "./SettingsView";
import { TunnelDetailView } from "./TunnelDetailView";
import { makePreviewSnapshot } from "./previewData";
import { connectionConfigIssue, userErrorMessage, type FeedbackAction } from "./userFeedback";
import "./styles.css";

const emptyConfig: AppConfig = {
  serverUrl: "", dataAddress: "", token: "", name: "", deviceId: "",
  transport: "websocket", quicAddress: "", caFile: "", insecureSkipVerify: false,
  updateManifestUrl: "", updatePublicKey: "", updateChannel: "stable", autoStart: false,
};

type View =
  | { name: "home" }
  | { name: "settings"; section: SettingsSection }
  | { name: "tunnel"; tunnelId: string };

export default function App() {
  return (
    <ConfigProvider theme={{ token: {
      colorPrimary: "#13ad68", colorInfo: "#13ad68", borderRadius: 8,
      colorText: "#152033", colorBorder: "#e5ebe8", fontFamily: "Inter, PingFang SC, Microsoft YaHei, sans-serif",
    } }}>
      <AntApp><DesktopApp /></AntApp>
    </ConfigProvider>
  );
}

function DesktopApp() {
  const { message } = AntApp.useApp();
  const [snapshot, setSnapshot] = useState<DesktopSnapshot>();
  const [view, setView] = useState<View>({ name: "home" });
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm<AppConfig>();
  const previewTick = useRef(0);

  const showError = (error: unknown, action: FeedbackAction) => {
    message.error({ content: userErrorMessage(error, action), duration: 6 });
  };

  const refresh = async () => {
    let next: DesktopSnapshot;
    try {
      next = await DesktopService.Snapshot();
    } catch (error) {
      if (!import.meta.env.DEV) throw error;
      next = makePreviewSnapshot(previewTick.current++);
    }
    setSnapshot(next);
  };

  useEffect(() => {
    refresh().catch((error) => showError(error, "load"));
    const id = window.setInterval(() => refresh().catch(() => undefined), 2000);
    return () => window.clearInterval(id);
  }, []);

  const runConnectionAction = async (action: "connect" | "disconnect") => {
    const issue = action === "connect" ? connectionConfigIssue(snapshot?.config) : undefined;
    if (issue) {
      message.warning({ content: issue.message, duration: 5 });
      setView({ name: "settings", section: issue.section });
      return;
    }
    setLoading(true);
    try {
      if (action === "connect") await DesktopService.Connect();
      else await DesktopService.Disconnect();
      await refresh();
    } catch (error) {
      if (!import.meta.env.DEV) showError(error, "connect");
      setSnapshot((current) => current ? {
        ...current,
        status: { ...current.status, connected: action === "connect", state: action === "connect" ? "connected" : "disconnected" },
      } : current);
    } finally {
      setLoading(false);
    }
  };

  const saveConfig = async (values: AppConfig) => {
    setLoading(true);
    try {
      const next = await DesktopService.SaveConfig(values);
      setSnapshot(next);
      message.success("设置已保存");
    } catch (error) {
      if (import.meta.env.DEV) {
        setSnapshot((current) => current ? { ...current, config: values } : current);
        message.success("预览设置已保存");
      } else showError(error, "save");
    } finally {
      setLoading(false);
    }
  };

  const checkUpdate = async () => {
    try {
      const result = await DesktopService.CheckForUpdate();
      message.success(result.message);
    } catch (error) {
      if (import.meta.env.DEV) message.info("当前已是最新版本");
      else showError(error, "update");
    }
  };

  const config = snapshot?.config ?? emptyConfig;
  const status = snapshot?.status;
  const tunnels = snapshot?.tunnels ?? [];
  const selectedTunnel = view.name === "tunnel" ? tunnels.find((item) => item.id === view.tunnelId) : undefined;

  if (view.name === "settings") {
    return <SettingsView
      form={form} config={config} status={status} logs={snapshot?.logs ?? []}
      loading={loading} initialSection={view.section} onBack={() => setView({ name: "home" })}
      onSave={saveConfig} onCheckUpdate={checkUpdate}
    />;
  }
  if (view.name === "tunnel" && selectedTunnel) {
    return <TunnelDetailView tunnel={selectedTunnel} status={status} onBack={() => setView({ name: "home" })} />;
  }
  return <HomeView
    snapshot={snapshot} loading={loading}
    onConnect={() => runConnectionAction("connect")} onDisconnect={() => runConnectionAction("disconnect")}
    onSettings={(section = "general") => setView({ name: "settings", section })}
    onTunnel={(tunnelId) => setView({ name: "tunnel", tunnelId })}
  />;
}
