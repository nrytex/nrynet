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
  const [updateLoading, setUpdateLoading] = useState(false);
  const [form] = Form.useForm<AppConfig>();
  const previewTick = useRef(0);
  const snapshotRefreshInFlight = useRef(false);
  const statusRefreshInFlight = useRef(false);

  const showError = (error: unknown, action: FeedbackAction) => {
    message.error({ content: userErrorMessage(error, action), duration: 6 });
  };

  const refreshSnapshot = async () => {
    if (snapshotRefreshInFlight.current) return;
    snapshotRefreshInFlight.current = true;
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
      snapshotRefreshInFlight.current = false;
    }
  };

  const refreshStatus = async () => {
    if (statusRefreshInFlight.current) return;
    statusRefreshInFlight.current = true;
    try {
      try {
        const status = await DesktopService.Status();
        setSnapshot((current) => current ? { ...current, status } : current);
      } catch (error) {
        if (!import.meta.env.DEV) throw error;
        const preview = makePreviewSnapshot(previewTick.current++);
        setSnapshot((current) => current ? { ...current, status: preview.status } : preview);
      }
    } finally {
      statusRefreshInFlight.current = false;
    }
  };

  useEffect(() => {
    refreshSnapshot().catch((error) => showError(error, "load"));
	const snapshotID = window.setInterval(() => refreshSnapshot().catch(() => undefined), 3000);
	const statusID = window.setInterval(() => refreshStatus().catch(() => undefined), 1000);
    return () => {
      window.clearInterval(snapshotID);
      window.clearInterval(statusID);
    };
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
      await refreshSnapshot();
    } catch (error) {
      if (import.meta.env.DEV) {
        setSnapshot((current) => current ? {
          ...current,
          status: { ...current.status, connected: action === "connect", state: action === "connect" ? "connected" : "disconnected" },
        } : current);
      } else {
        showError(error, "connect");
        await refreshSnapshot().catch(() => undefined);
      }
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
    setUpdateLoading(true);
    try {
      const result = await DesktopService.CheckForUpdate();
      if (result.ready) {
        message.success(result.message);
      } else if (result.available) {
        message.info(result.message);
      } else {
        message.success(result.message);
      }
      await refreshSnapshot();
    } catch (error) {
      if (import.meta.env.DEV) message.info("当前已是最新版本");
      else showError(error, "update");
    } finally {
      setUpdateLoading(false);
    }
  };

  const applyUpdate = async () => {
    setLoading(true);
    try {
      await DesktopService.ApplyUpdate();
    } catch (error) {
      showError(error, "update");
      setLoading(false);
    }
  };

  const config = snapshot?.config ?? emptyConfig;
  const status = snapshot?.status;
  const tunnels = snapshot?.tunnels ?? [];
  const selectedTunnel = view.name === "tunnel" ? tunnels.find((item) => item.id === view.tunnelId) : undefined;

  if (view.name === "settings") {
    return <SettingsView
      form={form} config={config} status={status} logs={snapshot?.logs ?? []}
      loading={loading} update={snapshot?.update ?? undefined} updateLoading={updateLoading} initialSection={view.section} onBack={() => setView({ name: "home" })}
      onSave={saveConfig} onCheckUpdate={checkUpdate} onApplyUpdate={applyUpdate}
    />;
  }
  if (view.name === "tunnel" && selectedTunnel) {
    return <TunnelDetailView tunnel={selectedTunnel} status={status} path={snapshot?.tunnelPaths?.[selectedTunnel.id]} publicHost={tunnelPublicHost(config)} serverUrl={config.serverUrl} onBack={() => setView({ name: "home" })} />;
  }
  return <HomeView
    snapshot={snapshot} loading={loading}
    updateNotice={snapshot?.update ?? undefined}
    onApplyUpdate={applyUpdate}
    onConnect={() => runConnectionAction("connect")} onDisconnect={() => runConnectionAction("disconnect")}
    onSettings={(section = "general") => setView({ name: "settings", section })}
    onTunnel={(tunnelId) => setView({ name: "tunnel", tunnelId })}
    serverUrl={config.serverUrl}
  />;
}
