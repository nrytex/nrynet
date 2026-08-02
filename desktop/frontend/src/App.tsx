import { useEffect, useMemo, useState } from "react";
import {
  Alert, Button, Card, Col, Divider, Form, Input, Layout, List, Row,
  Select, Space, Switch, Table, Tabs, Tag, Typography, message,
} from "antd";
import {
  Activity, Download, EyeOff, Power, PowerOff, RefreshCw, Save, Settings,
} from "lucide-react";
import "antd/dist/reset.css";
import "./styles.css";
import { DesktopService } from "../bindings/github.com/nat-link/nat-link/desktop";
import type {
  AppConfig, DesktopSnapshot, LogEntry,
} from "../bindings/github.com/nat-link/nat-link/desktop";
import type { Tunnel } from "../bindings/github.com/nat-link/nat-link/internal/model";
import { formatBytes, formatTime, logLine, redact } from "./format";

const emptyConfig: AppConfig = {
  serverUrl: "", dataAddress: "", token: "", name: "", deviceId: "",
  transport: "websocket", quicAddress: "", insecureSkipVerify: false,
  updateManifestUrl: "", updatePublicKey: "", updateChannel: "stable",
  autoStart: false,
};

export default function App() {
  const [snapshot, setSnapshot] = useState<DesktopSnapshot>();
  const [loading, setLoading] = useState(false);
  const [form] = Form.useForm<AppConfig>();

  const refresh = async () => {
    const next = await DesktopService.Snapshot();
    setSnapshot(next);
    form.setFieldsValue({ ...emptyConfig, ...next.config });
  };

  useEffect(() => {
    refresh().catch((err) => message.error(String(err)));
    const id = window.setInterval(() => refresh().catch(() => undefined), 2000);
    return () => window.clearInterval(id);
  }, []);

  const status = snapshot?.status;
  const cfg = snapshot?.config ?? emptyConfig;
  const isConnected = Boolean(status?.connected);
  const tunnels = snapshot?.tunnels ?? [];
  const logs = snapshot?.logs ?? [];

  const saveConfig = async (values: AppConfig) => {
    setLoading(true);
    try {
      const next = await DesktopService.SaveConfig(values);
      setSnapshot(next);
      message.success("Saved");
    } finally {
      setLoading(false);
    }
  };

  const connect = async () => {
    setLoading(true);
    try {
      await DesktopService.Connect();
      await refresh();
    } finally {
      setLoading(false);
    }
  };

  const disconnect = async () => {
    await DesktopService.Disconnect();
    await refresh();
  };

  const checkUpdate = async () => {
    try {
      const result = await DesktopService.CheckForUpdate();
      message.success(result.message);
    } catch (err) {
      message.error(String(err));
    }
  };

  const items = useMemo(() => [
    { key: "config", label: "Config", children: <ConfigForm form={form} cfg={cfg} loading={loading} onSave={saveConfig} /> },
    { key: "tunnels", label: "Tunnels", children: <TunnelTable tunnels={tunnels} /> },
    { key: "logs", label: "Logs", children: <LogList logs={logs} /> },
  ], [cfg, form, loading, logs, tunnels]);

  return (
    <Layout className="shell">
      <Layout.Sider className="side" width={244}>
        <div className="brand">NAT-Link</div>
        <StatusBlock status={status} config={cfg} />
        <Space direction="vertical" className="actions">
          <Button icon={<Power size={16} />} type="primary" disabled={isConnected} loading={loading} onClick={connect}>Connect</Button>
          <Button icon={<PowerOff size={16} />} disabled={!isConnected} onClick={disconnect}>Disconnect</Button>
          <Button icon={<Download size={16} />} onClick={checkUpdate}>Update</Button>
          <Button icon={<EyeOff size={16} />} onClick={() => DesktopService.HideWindow()}>Hide</Button>
        </Space>
      </Layout.Sider>
      <Layout.Content className="content">
        <Row gutter={12} className="metrics">
          <Metric title="State" value={status?.state ?? "disconnected"} active={isConnected} />
          <Metric title="Tunnels" value={String(tunnels.length)} />
          <Metric title="Upload" value={formatBytes(status?.uploadBytes ?? 0)} />
          <Metric title="Download" value={formatBytes(status?.downloadBytes ?? 0)} />
        </Row>
        <Alert className="notice" type="info" showIcon message={`Token: ${redact(cfg.token)} | Version: ${status?.version ?? "0.1.0"}`} />
        <Tabs items={items} />
      </Layout.Content>
    </Layout>
  );
}

function StatusBlock({ status, config }: { status?: DesktopSnapshot["status"]; config: AppConfig }) {
  const color = status?.connected ? "green" : "default";
  return (
    <div className="status">
      <Tag color={color}>{status?.state ?? "disconnected"}</Tag>
      <Typography.Text className="muted">{config.serverUrl || "No server"}</Typography.Text>
      <Typography.Text className="muted">{status?.message || "Ready"}</Typography.Text>
      <Typography.Text className="muted">Started {formatTime(status?.lastStartedAt)}</Typography.Text>
    </div>
  );
}

function Metric({ title, value, active = false }: { title: string; value: string; active?: boolean }) {
  return (
    <Col span={6}>
      <Card className="metric">
        <Space><Activity size={16} color={active ? "#16a34a" : "#64748b"} /><span>{title}</span></Space>
        <Typography.Title level={3}>{value}</Typography.Title>
      </Card>
    </Col>
  );
}

function ConfigForm({ form, cfg, loading, onSave }: {
  form: ReturnType<typeof Form.useForm<AppConfig>>[0]; cfg: AppConfig; loading: boolean;
  onSave: (values: AppConfig) => void;
}) {
  return (
    <Card>
      <Form form={form} layout="vertical" initialValues={cfg} onFinish={onSave}>
        <Row gutter={16}>
          <Col span={12}><Form.Item label="Control WebSocket" name="serverUrl" rules={[{ required: true }]}><Input /></Form.Item></Col>
          <Col span={12}><Form.Item label="Data Address" name="dataAddress" rules={[{ required: true }]}><Input /></Form.Item></Col>
          <Col span={12}><Form.Item label="Transport" name="transport"><Select options={[
            { value: "websocket", label: "WebSocket" },
            { value: "quic", label: "QUIC" },
          ]} /></Form.Item></Col>
          <Col span={12}><Form.Item label="QUIC Address" name="quicAddress"><Input /></Form.Item></Col>
          <Col span={12}><Form.Item label="Device Name" name="name"><Input /></Form.Item></Col>
          <Col span={12}><Form.Item label="Device ID" name="deviceId"><Input /></Form.Item></Col>
          <Col span={24}><Form.Item label="Token" name="token" rules={[{ required: true }]}><Input.Password /></Form.Item></Col>
        </Row>
        <Divider />
        <Row gutter={16}>
          <Col span={12}><Form.Item label="Manifest URL" name="updateManifestUrl"><Input /></Form.Item></Col>
          <Col span={12}><Form.Item label="Channel" name="updateChannel"><Input /></Form.Item></Col>
          <Col span={24}><Form.Item label="Updater Public Key" name="updatePublicKey"><Input.TextArea rows={3} /></Form.Item></Col>
          <Col span={8}><Form.Item label="Skip TLS Verify" name="insecureSkipVerify" valuePropName="checked"><Switch /></Form.Item></Col>
          <Col span={8}><Form.Item label="Open at Login" name="autoStart" valuePropName="checked"><Switch /></Form.Item></Col>
        </Row>
        <Button icon={<Save size={16} />} type="primary" htmlType="submit" loading={loading}>Save</Button>
      </Form>
    </Card>
  );
}

function TunnelTable({ tunnels }: { tunnels: Tunnel[] }) {
  return <Table rowKey="id" dataSource={tunnels} pagination={false} columns={[
    { title: "Name", dataIndex: "name" },
    { title: "Protocol", dataIndex: "protocol" },
    { title: "Local", render: (_: unknown, r: Tunnel) => `${r.local_host}:${r.local_port}` },
    { title: "Remote", dataIndex: "remote_port" },
    { title: "Domain", dataIndex: "domain" },
    { title: "Status", dataIndex: "status", render: (v: string) => <Tag>{v}</Tag> },
  ]} />;
}

function LogList({ logs }: { logs: LogEntry[] }) {
  return (
    <Card>
      <Space className="log-title"><Settings size={16} /><Typography.Text>Runtime Log</Typography.Text><RefreshCw size={16} /></Space>
      <List className="logs" dataSource={[...logs].reverse()} renderItem={(entry) => (
        <List.Item><code>{logLine(entry)}</code></List.Item>
      )} />
    </Card>
  );
}
