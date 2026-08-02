import { Button, Input, message, Modal, Select, Space, Table, Tag } from "antd";
import { Download, Trash2 } from "lucide-react";
import { useMemo, useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { LogEntry } from "../types";
import { formatDate } from "../utils/format";

export function LogsPage() {
  const state = useAsync(api.logs, []);
  const [query, setQuery] = useState("");
  const [level, setLevel] = useState<string>();
  const rows = useMemo(() => filterLogs(state.data ?? [], query, level), [state.data, query, level]);
  const clear = () => Modal.confirm({
    title: "Clear all server logs?",
    content: "Downloaded copies are not affected.",
    okButtonProps: { danger: true },
    onOk: async () => {
      const result = await api.clearLogs();
      message.success(`Deleted ${result.deleted} log entries`);
      state.reload();
    },
  });

  return (
    <Page title="Logs" loading={state.loading} error={state.error} empty={!rows.length} onReload={state.reload}
      extra={<Space wrap><Input.Search allowClear placeholder="查询日志" onSearch={setQuery} onChange={(e) => setQuery(e.target.value)} /><Select allowClear placeholder="Level" options={["debug", "info", "warn", "error"].map((v) => ({ value: v, label: v }))} onChange={setLevel} /><Button icon={<Download size={16} />} onClick={() => api.downloadLogs()}>Download</Button><Button danger icon={<Trash2 size={16} />} onClick={clear}>Clear</Button></Space>}>
      <Table<LogEntry>
        rowKey={(row, index) => row.id || `${row.created_at}-${index}`}
        dataSource={rows}
        columns={[
          { title: "Level", dataIndex: "level", render: (v) => <Tag color={color(v)}>{v}</Tag> },
          { title: "Event", dataIndex: "event", render: (v) => v || "-" },
          { title: "Message", dataIndex: "message" },
          { title: "Time", dataIndex: "created_at", render: formatDate },
        ]}
      />
    </Page>
  );
}

function filterLogs(rows: LogEntry[], query: string, level?: string) {
  const needle = query.trim().toLowerCase();
  return rows.filter((row) => {
    const levelMatched = !level || row.level === level;
    const textMatched = !needle || [row.message, row.event, row.level].some((v) => v?.toLowerCase().includes(needle));
    return levelMatched && textMatched;
  });
}

function color(level: string) {
  if (level === "error") return "red";
  if (level === "warn") return "orange";
  if (level === "info") return "blue";
  return "default";
}
