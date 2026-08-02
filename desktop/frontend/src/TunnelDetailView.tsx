import { App, Button, Tabs, Tooltip, Typography } from "antd";
import { ArrowLeft, Copy, MoreHorizontal } from "lucide-react";
import type { RuntimeStatus } from "../bindings/github.com/nat-link/nat-link/desktop";
import type { Tunnel } from "../bindings/github.com/nat-link/nat-link/internal/model";
import { TrafficTrend } from "./TrafficChart";
import { useTrafficHistory } from "./useTrafficHistory";
import "./details.css";

export function TunnelDetailView({ tunnel, status, onBack }: {
  tunnel: Tunnel;
  status?: RuntimeStatus;
  onBack: () => void;
}) {
  const { message } = App.useApp();
  const { points } = useTrafficHistory(status);
  const endpoint = tunnel.domain ? `${tunnel.domain}:${tunnel.remote_port}` : `:${tunnel.remote_port}`;
  const copyEndpoint = async () => {
    await navigator.clipboard.writeText(endpoint);
    message.success("访问地址已复制");
  };
  return (
    <main className="desktop-frame detail-view">
      <header className="view-header">
        <Button type="text" icon={<ArrowLeft size={18} />} onClick={onBack}>返回</Button>
        <strong>隧道详情</strong>
        <Tooltip title="复制访问地址"><Button aria-label="更多操作" type="text" icon={<MoreHorizontal size={20} />} onClick={copyEndpoint} /></Tooltip>
      </header>
      <section className="detail-summary">
        <div className="detail-title-row">
          <div><span className={`status-dot ${tunnel.status === "running" ? "online" : "offline"}`} /><Typography.Title level={4}>{tunnel.name}</Typography.Title></div>
          <span className={`state-pill ${tunnel.status === "running" ? "running" : ""}`}>{statusLabel(tunnel.status)}</span>
        </div>
        <div className="endpoint-row"><span>{endpoint}</span><Button aria-label="复制访问地址" icon={<Copy size={15} />} onClick={copyEndpoint} /></div>
      </section>
      <Tabs className="detail-tabs" defaultActiveKey="overview" items={[
        { key: "overview", label: "概览", children: <Overview tunnel={tunnel} status={status} points={points} /> },
        { key: "traffic", label: "流量统计", children: <TrafficPanel points={points} /> },
        { key: "config", label: "配置", children: <TunnelConfig tunnel={tunnel} /> },
      ]} />
    </main>
  );
}

function Overview({ tunnel, status, points }: { tunnel: Tunnel; status?: RuntimeStatus; points: ReturnType<typeof useTrafficHistory>["points"] }) {
  return <>
    <div className="detail-grid">
      <DetailCell label="本地地址" value={`${tunnel.local_host}:${tunnel.local_port}`} />
      <DetailCell label="隧道类型" value={tunnel.protocol.toUpperCase()} badge />
      <DetailCell label="创建时间" value={new Date(tunnel.created_at).toLocaleString()} />
      <DetailCell label="连接状态" value={status?.connected ? "连接中" : "已断开"} accent={status?.connected} />
      <DetailCell label="访问端口" value={tunnel.remote_port ? String(tunnel.remote_port) : "域名路由"} />
      <DetailCell label="运行时长" value={uptime(status?.lastStartedAt, status?.connected)} />
    </div>
    <TrafficPanel points={points} />
  </>;
}

function TrafficPanel({ points }: { points: ReturnType<typeof useTrafficHistory>["points"] }) {
  return <section className="trend-section">
    <div className="trend-heading"><strong>流量趋势（实时）</strong><span><i className="legend upload" />上传（KB/s）<i className="legend download" />下载（KB/s）</span></div>
    <TrafficTrend points={points} />
  </section>;
}

function TunnelConfig({ tunnel }: { tunnel: Tunnel }) {
  return <div className="config-readonly">
    <DetailCell label="协议" value={tunnel.protocol.toUpperCase()} />
    <DetailCell label="本地目标" value={`${tunnel.local_host}:${tunnel.local_port}`} />
    <DetailCell label="远程端口" value={String(tunnel.remote_port || "-")} />
    <DetailCell label="域名" value={tunnel.domain || "-"} />
    <DetailCell label="IP 白名单" value={tunnel.ip_allowlist?.join(", ") || "允许全部"} />
  </div>;
}

function DetailCell({ label, value, badge, accent }: { label: string; value: string; badge?: boolean; accent?: boolean }) {
  return <div className="detail-cell"><span>{label}</span><strong className={`${badge ? "protocol-value" : ""} ${accent ? "accent" : ""}`}>{value}</strong></div>;
}

function statusLabel(status: string) {
  if (status === "running") return "运行中";
  if (status === "error") return "异常";
  return "已停止";
}

function uptime(start?: string, connected?: boolean) {
  if (!start || !connected) return "--:--:--";
  const total = Math.floor((Date.now() - new Date(start).getTime()) / 1000);
  return `${String(Math.floor(total / 3600)).padStart(2, "0")}:${String(Math.floor(total / 60) % 60).padStart(2, "0")}:${String(total % 60).padStart(2, "0")}`;
}
