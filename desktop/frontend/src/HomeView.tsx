import { App, Button, Empty, Switch, Tooltip, Typography } from "antd";
import { ArrowUpRight, CircleAlert, Copy, Database, MoreHorizontal, Power, Settings, SlidersHorizontal } from "lucide-react";
import type { MouseEvent } from "react";
import type { DesktopSnapshot } from "../bindings/github.com/nrytex/nrynet/desktop";
import type { UpdateResult } from "../bindings/github.com/nrytex/nrynet/desktop";
import type { Tunnel } from "../bindings/github.com/nrytex/nrynet/internal/model";
import { formatBytes } from "./format";
import { useElapsedTime } from "./elapsedTime";
import { TrafficSparkline } from "./TrafficChart";
import { resolveTunnelEndpoint, tunnelPublicHost } from "./tunnelEndpoint";
import { useTrafficHistory } from "./useTrafficHistory";
import { connectionStatusMessage } from "./userFeedback";
import type { SettingsSection } from "./SettingsView";
import brandMark from "./assets/nrynet-mark.png";

interface HomeViewProps {
  snapshot?: DesktopSnapshot;
  loading: boolean;
  updateNotice?: UpdateResult;
  onOpenUpdate: () => void;
  onConnect: () => void;
  onDisconnect: () => void;
  onSettings: (section?: SettingsSection) => void;
  onTunnel: (tunnelId: string) => void;
  serverUrl: string;
}

export function HomeView(props: HomeViewProps) {
  const status = props.snapshot?.status;
  const config = props.snapshot?.config;
  const tunnels = props.snapshot?.tunnels ?? [];
  const connected = Boolean(status?.connected);
  const connectionDuration = useElapsedTime(status?.lastStartedAt, connected);
  const publicHost = tunnelPublicHost(config);
  const tunnelPaths = props.snapshot?.tunnelPaths ?? {};
  const statusMessage = connectionStatusMessage(status);
  const { points, rates } = useTrafficHistory(status);
  return (
    <main className="desktop-frame home-view">
      <header className="brand-header">
        <Brand />
        <div className={`header-status ${connected ? "connected" : "offline"}`}>
          <span className={`status-dot ${connected ? "online" : "offline"}`} />
          <span>{connected ? "已连接" : "未连接"}</span>
        </div>
        <div className="header-actions">
          <Tooltip title="运行日志"><Button aria-label="运行日志" type="text" className="header-action" icon={<Database size={16} />} onClick={() => props.onSettings("logs")}><span>日志</span></Button></Tooltip>
          <Tooltip title="设置"><Button aria-label="设置" type="text" className="header-action" icon={<Settings size={16} />} onClick={() => props.onSettings("general")}><span>设置</span></Button></Tooltip>
        </div>
      </header>

      {props.updateNotice && <div className="update-banner">
        <div className="update-banner-text"><strong>发现新版本 {props.updateNotice.latestVersion}</strong><span>{props.updateNotice.message}</span></div>
        <Button size="small" type="primary" icon={<ArrowUpRight size={14} />} onClick={props.onOpenUpdate}>去下载</Button>
      </div>}

      <section className="connection-panel">
        <div className="connection-heading">
          <div className="connection-state">
            <span className={`status-dot ${connected ? "online" : "offline"}`} />
            <strong>{connected ? "已连接" : "未连接"}</strong>
            <span className="secondary">连接时长 {connectionDuration}</span>
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
            {tunnels.map((tunnel) => <TunnelRow key={tunnel.id} tunnel={tunnel} path={tunnelPaths[tunnel.id]} publicHost={publicHost} serverUrl={props.serverUrl} onOpen={() => props.onTunnel(tunnel.id)} />)}
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
  return (
    <div className="brand-lockup">
      <span className="brand-mark"><img src={brandMark} alt="" /></span>
      <span className="brand-text">
        <strong>Nrynet</strong>
        <small>私有网络隧道</small>
      </span>
    </div>
  );
}

function Metric({ label, value, tone, compact }: { label: string; value: string; tone?: string; compact?: boolean }) {
  return <div className={`metric-item ${tone ?? ""}`}><span>{label}</span><strong className={compact ? "compact" : ""}>{value}</strong></div>;
}

function TunnelRow({ tunnel, path, publicHost, serverUrl, onOpen }: { tunnel: Tunnel; path?: string; publicHost: string; serverUrl: string; onOpen: () => void }) {
  const { message } = App.useApp();
  const endpoint = resolveTunnelEndpoint(tunnel, publicHost, serverUrl);
  const copy = async (event: MouseEvent) => {
    event.stopPropagation();
    if (!endpoint.copyValue) return;
    await navigator.clipboard.writeText(endpoint.copyValue);
    message.success("访问地址已复制");
  };
  return (
    <div className="tunnel-row">
      <span className={`status-dot ${tunnel.status === "running" ? "online" : "offline"}`} />
      <button className="tunnel-open" onClick={onOpen}><span className="tunnel-name"><strong>{tunnel.name}</strong><small>{endpoint.label}</small></span></button>
      <span className="protocol-badge">{tunnel.protocol.toUpperCase()}</span>
      <span className={`path-badge ${pathClass(path)}`}>{pathLabel(path)}</span>
      <Tooltip title={endpoint.copyValue ? "复制访问地址" : "请检查服务器公开地址配置"}><Button aria-label="复制访问地址" disabled={!endpoint.copyValue} icon={<Copy size={16} />} onClick={copy} /></Tooltip>
      <MoreHorizontal size={18} />
    </div>
  );
}

function pathLabel(path?: string) {
  if (path === "p2p") return "P2P";
  if (path === "relay") return "Relay";
  if (path === "visitor_p2p") return "访客 P2P";
  if (path === "visitor_relay") return "访客 Relay";
  return "未知";
}

function pathClass(path?: string) {
  if (path === "p2p") return "p2p";
  if (path === "relay") return "relay";
  if (path === "visitor_p2p") return "visitor-p2p";
  if (path === "visitor_relay") return "visitor-relay";
  return "unknown";
}

function formatRate(value: number) {
  if (value >= 1024) return `${(value / 1024).toFixed(1)} MB/s`;
  return `${value.toFixed(1)} KB/s`;
}

function serverName(value?: string) {
  if (!value) return "未配置服务器";
  try { return new URL(value).host; } catch { return value; }
}
