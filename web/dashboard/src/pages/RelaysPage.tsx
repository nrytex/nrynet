import { Descriptions, Table, Tag } from "antd";
import { useEffect } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { statusText } from "../display/status";
import { useAsync } from "../hooks/useAsync";
import type { RelayAssignment, RelayNode } from "../types";
import { formatDate } from "../utils/format";

export function RelaysPage() {
  const nodes = useAsync(api.relays, []);
  const assignments = useAsync(api.relayAssignments, []);
  useEffect(() => {
    const timer = window.setInterval(() => {
      void nodes.reload();
      void assignments.reload();
    }, 5000);
    return () => window.clearInterval(timer);
  }, []);

  async function reload() {
    await Promise.all([nodes.reload(), assignments.reload()]);
  }

  return (
    <Page title="中继" loading={nodes.loading || assignments.loading} error={nodes.error || assignments.error} onReload={reload}>
      <Table<RelayNode>
        rowKey="id"
        pagination={false}
        dataSource={nodes.data ?? []}
        columns={[
          { title: "节点", dataIndex: "id" },
          { title: "公网地址", dataIndex: "address" },
          { title: "健康状态", render: (_, node) => <Tag color={node.healthy ? "green" : "red"}>{statusText(node.healthy ? "healthy" : "offline")}</Tag> },
          { title: "实时连接数", dataIndex: "connections" },
          { title: "最后心跳", dataIndex: "last_seen", render: formatDate },
        ]}
      />
      <Descriptions size="small" column={1} style={{ marginTop: 20 }} title="隧道分配" />
      <Table<RelayAssignment>
        rowKey="tunnel_id"
        pagination={false}
        dataSource={assignments.data ?? []}
        columns={[
          { title: "隧道", dataIndex: "tunnel_id" },
          { title: "中继节点", dataIndex: "node_id" },
          { title: "公网地址", dataIndex: "address" },
        ]}
      />
    </Page>
  );
}
