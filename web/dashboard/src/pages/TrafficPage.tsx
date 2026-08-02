import { Col, Row, Segmented, Statistic, Table } from "antd";
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
  const rows = state.data?.tunnels ?? [];

  return (
    <Page title="Traffic" loading={state.loading} error={state.error} onReload={state.reload}
      extra={<Segmented value={range} options={[{ label: "Today", value: "today" }, { label: "Month", value: "month" }]} onChange={setRange} />}>
      <Row gutter={[16, 16]} className="metrics">
        <Col xs={12} md={8}><div className="metric"><Statistic title="Upload" value={formatBytes(summary.upload)} /></div></Col>
        <Col xs={12} md={8}><div className="metric"><Statistic title="Download" value={formatBytes(summary.download)} /></div></Col>
        <Col xs={12} md={8}><div className="metric"><Statistic title="Total" value={formatBytes(summary.upload + summary.download)} /></div></Col>
      </Row>
      <Table<TrafficTarget>
        rowKey="id"
        dataSource={rows}
        locale={{ emptyText: "No tunnel traffic in this range" }}
        columns={[
          { title: "Tunnel", dataIndex: "name" },
          { title: "Upload", dataIndex: "upload", render: formatBytes },
          { title: "Download", dataIndex: "download", render: formatBytes },
          { title: "Total", render: (_, row) => formatBytes(row.upload + row.download) },
        ]}
      />
    </Page>
  );
}
