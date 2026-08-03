import { Col, Row, Statistic, Table } from "antd";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { StatusTag } from "../components/StatusTag";
import { statusText } from "../display/status";
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
        <Metric title="服务状态" value={statusText(overview?.status ?? "unknown")} />
        <Metric title="在线客户端" value={overview?.online_clients ?? 0} />
        <Metric title="运行中隧道" value={overview?.active_tunnels ?? 0} />
        <Metric title="TCP 连接数" value={overview?.connections ?? 0} />
        <Metric title="实时带宽" value={formatRate(overview?.bandwidth_bps ?? 0)} />
        <Metric title="今日流量" value={formatBytes((overview?.today_upload ?? 0) + (overview?.today_download ?? 0))} />
      </Row>
      <Row gutter={[16, 16]}>
        <Col xs={24} xl={12}>
          <Table<Client>
            rowKey="id"
            size="middle"
            title={() => "最近客户端"}
            dataSource={clients.slice(0, 6)}
            pagination={false}
            columns={[
              { title: "名称", dataIndex: "name" },
              { title: "IP", dataIndex: "ip" },
              { title: "状态", render: (_, item) => <StatusTag value={item.disabled ? "disabled" : item.status} /> },
              { title: "最后在线", dataIndex: "last_online", render: formatDate },
            ]}
          />
        </Col>
        <Col xs={24} xl={12}>
          <Table<Tunnel>
            rowKey="id"
            size="middle"
            title={() => "最近隧道"}
            dataSource={tunnels.slice(0, 6)}
            pagination={false}
            columns={[
              { title: "名称", dataIndex: "name" },
              { title: "协议", dataIndex: "protocol" },
              { title: "远端端口", dataIndex: "remote_port" },
              { title: "状态", dataIndex: "status", render: (value) => <StatusTag value={value} /> },
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
