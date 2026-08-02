import { Col, Row, Statistic, Table, Tag } from "antd";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { Client, Tunnel } from "../types";
import { formatBytes, formatDate, formatRate } from "../utils/format";

export function OverviewPage() {
  const state = useAsync(async () => {
    const [overview, clients, tunnels] = await Promise.all([api.overview(), api.listClients(), api.listTunnels()]);
    return { overview, clients, tunnels };
  }, []);
  const clients = state.data?.clients ?? [];
  const tunnels = state.data?.tunnels ?? [];
  const overview = state.data?.overview;

  return (
    <Page title="概览" loading={state.loading} error={state.error} onReload={state.reload}>
      <Row gutter={[16, 16]} className="metrics">
        <Metric title="Server Status" value={overview?.status ?? "unknown"} />
        <Metric title="在线 Clients" value={overview?.online_clients ?? 0} />
        <Metric title="运行中 Tunnels" value={overview?.active_tunnels ?? 0} />
        <Metric title="TCP Connections" value={overview?.connections ?? 0} />
        <Metric title="Bandwidth" value={formatRate(overview?.bandwidth_bps ?? 0)} />
        <Metric title="今日流量" value={formatBytes((overview?.today_upload ?? 0) + (overview?.today_download ?? 0))} />
      </Row>
      <Row gutter={[16, 16]}>
        <Col xs={24} xl={12}>
          <Table<Client>
            rowKey="id"
            size="middle"
            title={() => "最近 Clients"}
            dataSource={clients.slice(0, 6)}
            pagination={false}
            columns={[
              { title: "Name", dataIndex: "name" },
              { title: "IP", dataIndex: "ip" },
              { title: "Status", render: (_, item) => <StatusTag value={item.disabled ? "disabled" : item.status} /> },
              { title: "Last Online", dataIndex: "last_online", render: formatDate },
            ]}
          />
        </Col>
        <Col xs={24} xl={12}>
          <Table<Tunnel>
            rowKey="id"
            size="middle"
            title={() => "最近 Tunnels"}
            dataSource={tunnels.slice(0, 6)}
            pagination={false}
            columns={[
              { title: "Name", dataIndex: "name" },
              { title: "Protocol", dataIndex: "protocol" },
              { title: "Remote", dataIndex: "remote_port" },
              { title: "Status", dataIndex: "status", render: (value) => <StatusTag value={value} /> },
            ]}
          />
        </Col>
      </Row>
    </Page>
  );
}

function Metric({ title, value }: { title: string; value: number | string }) {
  return (
    <Col xs={12} lg={6}>
      <div className="metric"><Statistic title={title} value={value} /></div>
    </Col>
  );
}

export function StatusTag({ value }: { value: string }) {
  const normalized = value.toLowerCase();
  const color = normalized === "online" || normalized === "running" ? "green" : normalized === "disabled" ? "red" : "default";
  return <Tag color={color}>{value}</Tag>;
}
