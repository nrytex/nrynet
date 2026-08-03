import { Button, Input, message, Modal, Select, Space, Table, Tag } from "antd";
import { Download, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { toMessage, useAsync } from "../hooks/useAsync";
import type { LogEntry } from "../types";
import { formatDate } from "../utils/format";

const levelOptions = [
  { value: "debug", label: "调试" },
  { value: "info", label: "信息" },
  { value: "warn", label: "警告" },
  { value: "error", label: "错误" },
];

const levelText = new Map(levelOptions.map((option) => [option.value, option.label]));

export function LogsPage() {
  const [query, setQuery] = useState("");
  const [search, setSearch] = useState("");
  const [level, setLevel] = useState<string>();
  const [page, setPage] = useState(1);
  const state = useAsync(() => api.logs({ keyword: query, level, page }), [query, level, page]);
  const rows = state.data?.items ?? [];
  const clear = () => Modal.confirm({
    title: "清空所有服务端日志？",
    content: "已下载的副本不会受影响。",
    okButtonProps: { danger: true },
    onOk: async () => {
      const result = await api.clearLogs();
      message.success(`已删除 ${result.deleted} 条日志`);
      state.reload();
    },
  });
  const applySearch = (value: string) => {
    setPage(1);
    setQuery(value.trim());
  };
  const download = async () => {
    try {
      await api.downloadLogs({ keyword: query, level });
    } catch (error) {
      message.error(toMessage(error));
    }
  };

  return (
    <Page title="日志" loading={state.loading} error={state.error} empty={!rows.length} onReload={state.reload}
      extra={<Space wrap><Input.Search allowClear placeholder="查询日志" value={search} onSearch={applySearch} onChange={(e) => { setSearch(e.target.value); if (!e.target.value) applySearch(""); }} /><Select allowClear placeholder="级别" options={levelOptions} onChange={(value) => { setPage(1); setLevel(value); }} /><Button icon={<Download size={16} />} onClick={download}>下载</Button><Button danger icon={<Trash2 size={16} />} onClick={clear}>清空</Button></Space>}>
      <Table<LogEntry>
        rowKey={(row, index) => row.id || `${row.created_at}-${index}`}
        dataSource={rows}
        pagination={{ current: page, pageSize: 100, total: state.data?.total ?? 0, showSizeChanger: false, onChange: setPage }}
        columns={[
          { title: "级别", dataIndex: "level", render: (v) => <Tag color={color(v)}>{levelText.get(v) ?? v}</Tag> },
          { title: "事件", dataIndex: "event", render: (v) => v || "-" },
          { title: "消息", dataIndex: "message" },
          { title: "时间", dataIndex: "created_at", render: formatDate },
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
