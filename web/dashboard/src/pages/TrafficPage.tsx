import { Col, Row, Segmented, Statistic, Table, Tabs } from "antd";
import { useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { TrafficTarget } from "../types";
import { formatBytes } from "../utils/format";

export function TrafficPage() {
  const [range, setRange] = useState<"today" | "month">("today");
  const state = useAsync(() => api.traffic(range), [range]);
  const summary = state.data?.summary ?? { upload: 0, download: 0 };
  const clients = state.data?.clients ?? [];
  const tunnels = state.data?.tunnels ?? [];

  return (
    <Page title="流量" loading={state.loading} error={state.error} onReload={state.reload}
      extra={<Segmented value={range} options={[{ label: "今日", value: "today" }, { label: "本月", value: "month" }]} onChange={setRange} />}>
      <Row gutter={[16, 16]} className="metrics">
        <Col xs={12} md={8}><div className="metric"><Statistic title="上传" value={formatBytes(summary.upload)} /></div></Col>
        <Col xs={12} md={8}><div className="metric"><Statistic title="下载" value={formatBytes(summary.download)} /></div></Col>
        <Col xs={12} md={8}><div className="metric"><Statistic title="总计" value={formatBytes(summary.upload + summary.download)} /></div></Col>
      </Row>
      <Tabs items={[
        { key: "clients", label: "按客户端", children: <TrafficTable label="客户端" rows={clients} /> },
        { key: "tunnels", label: "按隧道", children: <TrafficTable label="隧道" rows={tunnels} /> },
      ]} />
    </Page>
  );
}

function TrafficTable({ label, rows }: { label: string; rows: TrafficTarget[] }) {
  return (
    <Table<TrafficTarget>
      rowKey="id"
      dataSource={rows}
      locale={{ emptyText: `当前范围内没有${label}流量` }}
      columns={[
        { title: label, dataIndex: "name" },
        { title: "上传", dataIndex: "upload", render: formatBytes },
        { title: "下载", dataIndex: "download", render: formatBytes },
        { title: "总计", render: (_, row) => formatBytes(row.upload + row.download) },
      ]}
    />
  );
}
