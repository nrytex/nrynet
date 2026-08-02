import { Button, Input, message, Modal, Select, Space, Table, Tag } from "antd";
import { Download, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { LogEntry } from "../types";
import { formatDate } from "../utils/format";

export function LogsPage() {
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [level, setLevel] = useState<string>();
  const [page, setPage] = useState(1);
  const state = useAsync(() => api.logs({ keyword: query, level, page }), [query, level, page]);
  const rows = state.data?.items ?? [];
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
  const applySearch = (value: string) => {
    setPage(1);
    setQuery(value.trim());
  };

  return (
    <Page title="Logs" loading={state.loading} error={state.error} empty={!rows.length} onReload={state.reload}
      extra={<Space wrap><Input.Search allowClear placeholder="查询日志" value={search} onSearch={applySearch} onChange={(e) => { setSearch(e.target.value); if (!e.target.value) applySearch(""); }} /><Select allowClear placeholder="Level" options={["debug", "info", "warn", "error"].map((v) => ({ value: v, label: v }))} onChange={(value) => { setPage(1); setLevel(value); }} /><Button icon={<Download size={16} />} onClick={() => api.downloadLogs({ keyword: query, level })}>Download</Button><Button danger icon={<Trash2 size={16} />} onClick={clear}>Clear</Button></Space>}>
      <Table<LogEntry>
        rowKey={(row, index) => row.id || `${row.created_at}-${index}`}
        dataSource={rows}
        pagination={{ current: page, pageSize: 100, total: state.data?.total ?? 0, showSizeChanger: false, onChange: setPage }}
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

function color(level: string) {
  if (level === "error") return "red";
  if (level === "warn") return "orange";
  if (level === "info") return "blue";
  return "default";
}
