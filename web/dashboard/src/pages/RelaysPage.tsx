import { Descriptions, Table, Tag } from "antd";
import { useEffect } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
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
    <Page title="Relays" loading={nodes.loading || assignments.loading} error={nodes.error || assignments.error} onReload={reload}>
      <Table<RelayNode>
        rowKey="id"
        pagination={false}
        dataSource={nodes.data ?? []}
        columns={[
          { title: "Node", dataIndex: "id" },
          { title: "Public address", dataIndex: "address" },
          { title: "Health", render: (_, node) => <Tag color={node.healthy ? "green" : "red"}>{node.healthy ? "healthy" : "offline"}</Tag> },
          { title: "Live connections", dataIndex: "connections" },
          { title: "Last heartbeat", dataIndex: "last_seen", render: formatDate },
        ]}
      />
      <Descriptions size="small" column={1} style={{ marginTop: 20 }} title="Tunnel assignments" />
      <Table<RelayAssignment>
        rowKey="tunnel_id"
        pagination={false}
        dataSource={assignments.data ?? []}
        columns={[
          { title: "Tunnel", dataIndex: "tunnel_id" },
          { title: "Relay node", dataIndex: "node_id" },
          { title: "Public address", dataIndex: "address" },
        ]}
      />
    </Page>
  );
}
