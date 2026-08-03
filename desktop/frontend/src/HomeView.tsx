import { App, Button, Empty, Switch, Tooltip, Typography } from "antd";
import { CircleAlert, Copy, Database, MoreHorizontal, Power, Settings, SlidersHorizontal } from "lucide-react";
import type { MouseEvent } from "react";
import type { DesktopSnapshot } from "../bindings/github.com/nat-link/nat-link/desktop";
import type { Tunnel } from "../bindings/github.com/nat-link/nat-link/internal/model";
import { formatBytes } from "./format";
import { TrafficSparkline } from "./TrafficChart";
import { useTrafficHistory } from "./useTrafficHistory";
import { connectionStatusMessage } from "./userFeedback";
import type { SettingsSection } from "./SettingsView";
import brandMark from "./assets/nat-link-mark.png";

interface HomeViewProps {
  snapshot?: DesktopSnapshot;
  loading: boolean;
  onConnect: () => void;
  onDisconnect: () => void;
  onSettings: (section?: SettingsSection) => void;
  onTunnel: (tunnelId: string) => void;
}

export function HomeView(props: HomeViewProps) {
  const status = props.snapshot?.status;
  const config = props.snapshot?.config;
  const tunnels = props.snapshot?.tunnels ?? [];
  const connected = Boolean(status?.connected);
  const statusMessage = connectionStatusMessage(status);
  const { points, rates } = useTrafficHistory(status);
  return (
    <main className="desktop-frame home-view">
      <header className="brand-header">
        <Brand />
        <div className="header-actions">
          <Tooltip title="运行日志"><Button aria-label="运行日志" type="text" icon={<Database size={19} />} onClick={() => props.onSettings("logs")} /></Tooltip>
          <Tooltip title="设置"><Button aria-label="设置" type="text" icon={<Settings size={19} />} onClick={() => props.onSettings("general")} /></Tooltip>
        </div>
      </header>

      <section className="connection-panel">
        <div className="connection-heading">
          <div className="connection-state">
            <span className={`status-dot ${connected ? "online" : "offline"}`} />
            <strong>{connected ? "已连接" : "未连接"}</strong>
            <span className="secondary">连接时长 {connectedDuration(status?.lastStartedAt, connected)}</span>
          </div>
          <Button
            className="connection-button" loading={props.loading}
            onClick={connected ? props.onDisconnect : props.onConnect}
            icon={<Power size={15} />}
          >{connected ? "断开连接" : "立即连接"}</Button>
        </div>
        {statusMessage && <div className="connection-feedback" role="status"><CircleAlert size={15} /><span>{statusMessage}</span></div>}
        <div className="metric-strip">
          <Metric label="隧道数" value={String(tunnels.length)} />
          <Metric label="上传速率" value={formatRate(rates.upload)} tone="green" />
          <Metric label="下载速率" value={formatRate(rates.download)} tone="blue" />
          <Metric label="累计流量" value={formatBytes((status?.uploadBytes ?? 0) + (status?.downloadBytes ?? 0))} compact />
        </div>
        <TrafficSparkline points={points} />
      </section>

      <section className="tunnel-section">
        <div className="section-heading">
          <div><Typography.Title level={4}>我的隧道</Typography.Title><span>{tunnels.length} 条已分配</span></div>
          <Tooltip title="隧道由服务端管理员分配"><Button aria-label="隧道分配说明" type="text" icon={<SlidersHorizontal size={18} />} /></Tooltip>
        </div>
        {tunnels.length === 0 ? <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无已分配隧道" /> : (
          <div className="tunnel-list">
            {tunnels.map((tunnel) => <TunnelRow key={tunnel.id} tunnel={tunnel} onOpen={() => props.onTunnel(tunnel.id)} />)}
          </div>
        )}
      </section>

      <footer className="home-footer">
        <div className="footer-control"><Power size={16} /><span>开机启动</span><Switch size="small" checked={config?.autoStart} onChange={() => props.onSettings("general")} /></div>
        <button className="footer-link" onClick={() => props.onSettings("network")}>{config?.transport === "quic" ? "QUIC" : "WebSocket"} <span>·</span> {serverName(config?.serverUrl)}</button>
      </footer>
    </main>
  );
}

export function Brand() {
  return <div className="brand-lockup"><img src={brandMark} alt="" /><span>NAT-Link</span></div>;
}

function Metric({ label, value, tone, compact }: { label: string; value: string; tone?: string; compact?: boolean }) {
  return <div className={`metric-item ${tone ?? ""}`}><span>{label}</span><strong className={compact ? "compact" : ""}>{value}</strong></div>;
}

function TunnelRow({ tunnel, onOpen }: { tunnel: Tunnel; onOpen: () => void }) {
  const { message } = App.useApp();
  const endpoint = tunnel.domain ? `${tunnel.domain}${tunnel.remote_port ? `:${tunnel.remote_port}` : ""}` : `:${tunnel.remote_port}`;
  const copy = async (event: MouseEvent) => {
    event.stopPropagation();
    await navigator.clipboard.writeText(endpoint);
    message.success("访问地址已复制");
  };
  return (
    <div className="tunnel-row">
      <span className={`status-dot ${tunnel.status === "running" ? "online" : "offline"}`} />
      <button className="tunnel-open" onClick={onOpen}><span className="tunnel-name"><strong>{tunnel.name}</strong><small>{endpoint}</small></span></button>
      <span className="protocol-badge">{tunnel.protocol.toUpperCase()}</span>
      <Tooltip title="复制访问地址"><Button aria-label="复制访问地址" icon={<Copy size={16} />} onClick={copy} /></Tooltip>
      <MoreHorizontal size={18} />
    </div>
  );
}

function formatRate(value: number) {
  if (value >= 1024) return `${(value / 1024).toFixed(1)} MB/s`;
  return `${value.toFixed(1)} KB/s`;
}

function connectedDuration(start?: string, connected = false) {
  if (!start || !connected) return "--:--:--";
  const seconds = Math.max(0, Math.floor((Date.now() - new Date(start).getTime()) / 1000));
  const hours = String(Math.floor(seconds / 3600)).padStart(2, "0");
  const minutes = String(Math.floor(seconds / 60) % 60).padStart(2, "0");
  return `${hours}:${minutes}:${String(seconds % 60).padStart(2, "0")}`;
}

function serverName(value?: string) {
  if (!value) return "未配置服务器";
  try { return new URL(value).host; } catch { return value; }
}
