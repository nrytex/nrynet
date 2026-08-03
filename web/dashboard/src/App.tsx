import { App, Button, ConfigProvider, Layout, Menu, Typography, theme } from "antd";
import {
  Activity,
  ChartNoAxesCombined,
  FileText,
  Gauge,
  KeyRound,
  MonitorSmartphone,
  Network,
  RadioTower,
  Settings,
} from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { clearSession, getSession } from "./api/session";
import { ClientsPage } from "./pages/ClientsPage";
import { LogsPage } from "./pages/LogsPage";
import { LoginPage } from "./pages/LoginPage";
import { OverviewPage } from "./pages/OverviewPage";
import { SettingsPage } from "./pages/SettingsPage";
import { TokensPage } from "./pages/TokensPage";
import { TrafficPage } from "./pages/TrafficPage";
import { TunnelsPage } from "./pages/TunnelsPage";
import { RelaysPage } from "./pages/RelaysPage";

const pages = [
  { key: "overview", label: "概览", icon: <Gauge size={17} />, component: <OverviewPage /> },
  { key: "clients", label: "Clients", icon: <MonitorSmartphone size={17} />, component: <ClientsPage /> },
  { key: "tokens", label: "Tokens", icon: <KeyRound size={17} />, component: <TokensPage /> },
  { key: "tunnels", label: "Tunnels", icon: <Network size={17} />, component: <TunnelsPage /> },
  { key: "relays", label: "Relays", icon: <RadioTower size={17} />, component: <RelaysPage /> },
  { key: "traffic", label: "Traffic", icon: <ChartNoAxesCombined size={17} />, component: <TrafficPage /> },
  { key: "logs", label: "Logs", icon: <FileText size={17} />, component: <LogsPage /> },
  { key: "settings", label: "Settings", icon: <Settings size={17} />, component: <SettingsPage /> },
];

export function AppRoot() {
  return (
    <ConfigProvider theme={{ algorithm: theme.defaultAlgorithm, token: { borderRadius: 6, colorPrimary: "#1677ff" } }}>
      <App>
        <DashboardApp />
      </App>
    </ConfigProvider>
  );
}

function DashboardApp() {
  const [authenticated, setAuthenticated] = useState(() => !!getSession());
  const [active, setActive] = useState("overview");
  const activePage = useMemo(() => pages.find((page) => page.key === active) ?? pages[0], [active]);

  useEffect(() => {
    const handler = () => setAuthenticated(false);
    window.addEventListener("nrynet:unauthorized", handler);
    return () => window.removeEventListener("nrynet:unauthorized", handler);
  }, []);

  if (!authenticated) return <LoginPage onSuccess={() => setAuthenticated(true)} />;

  return (
    <Layout className="app-layout">
      <Layout.Sider breakpoint="lg" collapsedWidth="0" width={224} className="sider">
        <div className="brand">
          <Activity size={23} />
          <Typography.Text strong>Nrynet</Typography.Text>
        </div>
        <Menu
          theme="dark"
          mode="inline"
          selectedKeys={[active]}
          items={pages.map(({ key, label, icon }) => ({ key, label, icon }))}
          onClick={({ key }) => setActive(key)}
        />
      </Layout.Sider>
      <Layout>
        <Layout.Header className="topbar">
          <Typography.Text type="secondary">运维控制台</Typography.Text>
          <Button
            onClick={() => {
              clearSession();
              setAuthenticated(false);
            }}
          >
            退出
          </Button>
        </Layout.Header>
        <Layout.Content className="content">{activePage.component}</Layout.Content>
      </Layout>
    </Layout>
  );
}
