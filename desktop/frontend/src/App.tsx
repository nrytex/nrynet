import { App as AntApp, ConfigProvider, Form } from "antd";
import { useEffect, useRef, useState } from "react";
import "antd/dist/reset.css";
import { DesktopService } from "../bindings/github.com/nrytex/nrynet/desktop";
import type { AppConfig, DesktopSnapshot } from "../bindings/github.com/nrytex/nrynet/desktop";
import { completeConfigForSave, emptyConfig } from "./configDraft";
import { HomeView } from "./HomeView";
import { SettingsView, type SettingsSection } from "./SettingsView";
import { TunnelDetailView } from "./TunnelDetailView";
import { tunnelPublicHost } from "./tunnelEndpoint";
import { makePreviewSnapshot } from "./previewData";
import { connectionConfigIssue, userErrorMessage, type FeedbackAction } from "./userFeedback";
import "./styles.css";

type View =
  | { name: "home" }
  | { name: "settings"; section: SettingsSection }
  | { name: "tunnel"; tunnelId: string };

export default function App() {
  return (
    <ConfigProvider theme={{ token: {
      colorPrimary: "#13ad68", colorInfo: "#13ad68", borderRadius: 8,
      colorText: "#152033", colorBorder: "#e5ebe8", fontFamily: "Segoe UI, PingFang SC, Microsoft YaHei, system-ui, sans-serif",
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
  const refreshInFlight = useRef(false);

  const showError = (error: unknown, action: FeedbackAction) => {
    message.error({ content: userErrorMessage(error, action), duration: 6 });
  };

  const refresh = async () => {
    if (refreshInFlight.current) return;
    refreshInFlight.current = true;
    try {
      let next: DesktopSnapshot;
      try {
        next = await DesktopService.Snapshot();
      } catch (error) {
        if (!import.meta.env.DEV) throw error;
        next = makePreviewSnapshot(previewTick.current++);
      }
      setSnapshot(next);
    } finally {
      refreshInFlight.current = false;
    }
  };

  useEffect(() => {
    refresh().catch((error) => showError(error, "load"));
    const id = window.setInterval(() => refresh().catch(() => undefined), 1000);
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

  const saveConfig = async (values: Partial<AppConfig>) => {
    const completeConfig = completeConfigForSave(snapshot?.config, values);
    setLoading(true);
    try {
      const next = await DesktopService.SaveConfig(completeConfig);
      setSnapshot(next);
      form.setFieldsValue(next.config);
      message.success("设置已保存");
    } catch (error) {
      if (import.meta.env.DEV) {
        setSnapshot((current) => current ? { ...current, config: completeConfig } : current);
        message.success("预览设置已保存");
      } else showError(error, "save");
    } finally {
      setLoading(false);
    }
  };

  const checkUpdate = async () => {
    try {
      const result = await DesktopService.CheckForUpdate();
      if (result.available && result.downloadURL) {
        message.info(result.message);
      } else {
        message.success(result.message);
      }
      await refresh();
    } catch (error) {
      if (import.meta.env.DEV) message.info("当前已是最新版本");
      else showError(error, "update");
    }
  };

  const openUpdateDownload = async () => {
    const url = snapshot?.update?.downloadURL;
    if (!url) return;
    try {
      await DesktopService.OpenURL(url);
    } catch (error) {
      showError(error, "update");
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
    return <TunnelDetailView tunnel={selectedTunnel} status={status} path={snapshot?.tunnelPaths?.[selectedTunnel.id]} publicHost={tunnelPublicHost(config)} serverUrl={config.serverUrl} onBack={() => setView({ name: "home" })} />;
  }
  return <HomeView
    snapshot={snapshot} loading={loading}
    updateNotice={snapshot?.update ?? undefined}
    onOpenUpdate={openUpdateDownload}
    onConnect={() => runConnectionAction("connect")} onDisconnect={() => runConnectionAction("disconnect")}
    onSettings={(section = "general") => setView({ name: "settings", section })}
    onTunnel={(tunnelId) => setView({ name: "tunnel", tunnelId })}
    serverUrl={config.serverUrl}
  />;
}
