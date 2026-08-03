import { Button, Form, Input, List, Select, Switch, Typography } from "antd";
import type { FormInstance } from "antd";
import { ArrowLeft, CircleUserRound, Info, Network, RadioTower, Save, ScrollText, Settings } from "lucide-react";
import { useState } from "react";
import type { ReactNode } from "react";
import type { AppConfig, LogEntry, RuntimeStatus } from "../bindings/github.com/nat-link/nat-link/desktop";
import { formatTime, logLine } from "./format";
import "./settings.css";
import brandMark from "./assets/nat-link-mark.png";

export type SettingsSection = "general" | "network" | "connection" | "logs" | "about";

export function SettingsView({ form, config, status, logs, loading, initialSection, onBack, onSave, onCheckUpdate }: {
  form: FormInstance<AppConfig>;
  config: AppConfig;
  status?: RuntimeStatus;
  logs: LogEntry[];
  loading: boolean;
  initialSection: SettingsSection;
  onBack: () => void;
  onSave: (values: AppConfig) => void;
  onCheckUpdate: () => void;
}) {
  const [section, setSection] = useState(initialSection);
  return (
    <main className="desktop-frame settings-view">
      <header className="view-header">
        <Button type="text" icon={<ArrowLeft size={18} />} onClick={onBack}>返回</Button>
        <strong>设置</strong>
        <span className="header-spacer" />
      </header>
      <div className="settings-layout">
        <nav className="settings-nav">
          <NavItem icon={<Settings size={17} />} label="常规设置" active={section === "general"} onClick={() => setSection("general")} />
          <NavItem icon={<Network size={17} />} label="网络设置" active={section === "network"} onClick={() => setSection("network")} />
          <NavItem icon={<RadioTower size={17} />} label="连接设置" active={section === "connection"} onClick={() => setSection("connection")} />
          <NavItem icon={<ScrollText size={17} />} label="运行日志" active={section === "logs"} onClick={() => setSection("logs")} />
          <NavItem icon={<Info size={17} />} label="关于 NAT-Link" active={section === "about"} onClick={() => setSection("about")} />
        </nav>
        <section className="settings-content">
          {section === "logs" ? <LogsPanel logs={logs} /> : section === "about" ? (
            <AboutPanel status={status} onCheckUpdate={onCheckUpdate} />
          ) : (
            <Form form={form} layout="vertical" initialValues={config} onFinish={onSave}>
              <Typography.Title level={4}>{sectionTitle(section)}</Typography.Title>
              {section === "general" && <GeneralPanel />}
              {section === "network" && <NetworkPanel />}
              {section === "connection" && <ConnectionPanel />}
              <div className="settings-save"><Button type="primary" htmlType="submit" loading={loading} icon={<Save size={16} />}>保存设置</Button></div>
            </Form>
          )}
        </section>
      </div>
    </main>
  );
}

function NavItem({ icon, label, active, onClick }: { icon: ReactNode; label: string; active: boolean; onClick: () => void }) {
  return <button className={`settings-nav-item ${active ? "active" : ""}`} onClick={onClick}>{icon}<span>{label}</span></button>;
}

function GeneralPanel() {
  return <div className="settings-group">
    <SwitchRow title="开机启动" detail="系统启动时自动运行 NAT-Link" name="autoStart" />
    <div className="switch-row"><div><strong>自动更新</strong><span>每 6 小时检查一次 GitHub 正式版本</span></div><Switch checked disabled /></div>
  </div>;
}

function NetworkPanel() {
  return <div className="settings-group">
    <Form.Item label="控制服务器" name="serverUrl" rules={[{ required: true, message: "请输入控制服务器地址" }]}><Input placeholder="wss://server.example.com/agent/connect" /></Form.Item>
    <Form.Item label="数据通道" name="dataAddress" rules={[{ required: true, message: "请输入数据通道地址" }]}><Input placeholder="server.example.com:7001" /></Form.Item>
    <Form.Item label="传输协议" name="transport"><Select options={[{ value: "websocket", label: "WebSocket" }, { value: "quic", label: "QUIC" }]} /></Form.Item>
    <Form.Item label="QUIC 地址" name="quicAddress"><Input placeholder="server.example.com:7002" /></Form.Item>
    <Form.Item label="私有 CA 文件" name="caFile"><Input placeholder="C:\\certs\\ca.pem" /></Form.Item>
    <SwitchRow title="跳过 TLS 证书校验" detail="仅用于受控测试环境，不建议在生产环境启用" name="insecureSkipVerify" danger />
  </div>;
}

function ConnectionPanel() {
  return <div className="settings-group">
    <Form.Item label="设备名称" name="name"><Input prefix={<CircleUserRound size={15} />} placeholder="例如：工作室电脑" /></Form.Item>
    <Form.Item label="设备 ID" name="deviceId"><Input placeholder="首次连接后自动生成" /></Form.Item>
    <Form.Item label="Agent Token" name="token" rules={[{ required: true, message: "请输入 Agent Token" }]}><Input.Password placeholder="粘贴服务端生成的 Token" /></Form.Item>
  </div>;
}

function SwitchRow({ title, detail, name, danger }: { title: string; detail: string; name: keyof AppConfig; danger?: boolean }) {
  return <div className="switch-row"><div><strong className={danger ? "danger" : ""}>{title}</strong><span>{detail}</span></div><Form.Item name={name} valuePropName="checked" noStyle><Switch /></Form.Item></div>;
}

function LogsPanel({ logs }: { logs: LogEntry[] }) {
  return <>
    <Typography.Title level={4}>运行日志</Typography.Title>
    <div className="logs-summary"><span>最近 {logs.length} 条</span><span>日志中的 Token 已自动脱敏</span></div>
    <List className="settings-logs" locale={{ emptyText: "暂无运行日志" }} dataSource={[...logs].reverse()} renderItem={(entry) => (
      <List.Item><span className={`log-level ${entry.level.toLowerCase()}`}>{entry.level}</span><code>{logLine(entry)}</code></List.Item>
    )} />
  </>;
}

function AboutPanel({ status, onCheckUpdate }: { status?: RuntimeStatus; onCheckUpdate: () => void }) {
  return <>
    <Typography.Title level={4}>关于 NAT-Link</Typography.Title>
    <div className="about-lockup"><img src={brandMark} alt="" /><div><strong>NAT-Link</strong><span>安全、稳定的私有网络隧道客户端</span></div></div>
    <div className="settings-group about-details">
      <div><span>当前版本</span><strong>{status?.version ?? "1.0.0"}</strong></div>
      <div><span>连接状态</span><strong>{status?.connected ? "已连接" : "未连接"}</strong></div>
      <div><span>上次启动</span><strong>{formatTime(status?.lastStartedAt)}</strong></div>
    </div>
    <Button onClick={onCheckUpdate}>检查更新</Button>
  </>;
}

function sectionTitle(section: SettingsSection) {
  if (section === "network") return "网络设置";
  if (section === "connection") return "连接设置";
  return "常规设置";
}
