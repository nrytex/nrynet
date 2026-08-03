import { App, Button, Descriptions, Drawer, Form, Input, Modal, Space, Switch, Table, Typography } from "antd";
import { Eye, KeyRound, Trash2 } from "lucide-react";
import { useRef, useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { StatusTag } from "../components/StatusTag";
import { useAsync } from "../hooks/useAsync";
import type { Client, ClientDetail, Tunnel } from "../types";
import { formatBytes, formatDate, formatDuration } from "../utils/format";

export function ClientsPage() {
  const { message, modal } = App.useApp();
  const state = useAsync(api.listClients, []);
  const [detail, setDetail] = useState<ClientDetail>();
  const [editing, setEditing] = useState<Client>();
  const [tokenValue, setTokenValue] = useState<string>();
  const detailRequest = useRef(0);

  async function loadDetail(client: Client) {
    const request = ++detailRequest.current;
    const next = await api.getClient(client.id);
    if (request === detailRequest.current) setDetail(next);
  }

  function closeDetail() {
    detailRequest.current++;
    setDetail(undefined);
  }

  async function saveEdit(values: { name: string; disabled: boolean }) {
    if (!editing) return;
    await api.updateClient(editing.id, values);
    setEditing(undefined);
    await state.reload();
  }

  function remove(client: Client) {
    modal.confirm({
      title: `删除 ${client.name}?`,
      content: "这会停止并删除该客户端的隧道，同时撤销该设备身份。",
      okButtonProps: { danger: true },
      onOk: async () => {
        await api.deleteClient(client.id);
        message.success("客户端已删除");
        await state.reload();
      },
    });
  }

  return (
    <Page title="客户端" loading={state.loading} error={state.error} empty={!state.data?.length} onReload={state.reload}>
      <Table<Client>
        rowKey="id"
        dataSource={state.data ?? []}
        columns={[
          { title: "名称", dataIndex: "name" },
          { title: "状态", render: (_, c) => <StatusTag value={c.disabled ? "disabled" : c.status} /> },
          { title: "IP", dataIndex: "ip" },
          { title: "系统", dataIndex: "os" },
          { title: "版本", dataIndex: "version" },
          { title: "最后在线", dataIndex: "last_online", render: formatDate },
          { title: "操作", render: (_, c) => <RowActions client={c} onView={loadDetail} onEdit={setEditing} onReset={async () => setTokenValue((await api.resetClientToken(c.id)).value)} onRemove={remove} /> },
        ]}
      />
      <ClientDrawer detail={detail} onClose={closeDetail} />
      <ClientEditor client={editing} onCancel={() => setEditing(undefined)} onSave={saveEdit} />
      <Modal title="新令牌" open={!!tokenValue} onCancel={() => setTokenValue(undefined)} footer={null}>
        <Typography.Text copyable code>{tokenValue}</Typography.Text>
      </Modal>
    </Page>
  );
}

function RowActions(props: { client: Client; onView: (c: Client) => void; onEdit: (c: Client) => void; onReset: () => void; onRemove: (c: Client) => void }) {
  return (
    <Space>
      <Button icon={<Eye size={16} />} onClick={() => props.onView(props.client)} />
      <Button onClick={() => props.onEdit(props.client)}>编辑</Button>
      <Button icon={<KeyRound size={16} />} onClick={props.onReset} />
      <Button danger icon={<Trash2 size={16} />} onClick={() => props.onRemove(props.client)} />
    </Space>
  );
}

function ClientDrawer({ detail, onClose }: { detail?: ClientDetail; onClose: () => void }) {
  const trafficTotal = (value: { upload: number; download: number }) => formatBytes(value.upload + value.download);
  return (
    <Drawer title="客户端详情" open={!!detail} onClose={onClose} width={560}>
      {detail && <>
        <Descriptions column={1} bordered items={[
          { key: "id", label: "ID", children: detail.client.id },
          { key: "device", label: "设备 ID", children: detail.client.device_id },
          { key: "status", label: "状态", children: <StatusTag value={detail.client.disabled ? "disabled" : detail.client.status} /> },
          { key: "system", label: "系统", children: `${detail.client.os || "-"} / ${detail.client.version || "-"}` },
          { key: "ip", label: "IP", children: detail.client.ip || "-" },
          { key: "connected", label: "已连接时长", children: detail.connected_at ? formatDuration(detail.connected_seconds) : "离线" },
          { key: "today", label: "今日流量", children: trafficTotal(detail.traffic.today) },
          { key: "month", label: "本月流量", children: trafficTotal(detail.traffic.month) },
          { key: "last", label: "最后在线", children: formatDate(detail.client.last_online) },
        ]} />
        <Typography.Title level={5} style={{ marginTop: 20 }}>隧道</Typography.Title>
        <Table<Tunnel> size="small" rowKey="id" pagination={false} dataSource={detail.tunnels} columns={[
          { title: "名称", dataIndex: "name" },
          { title: "协议", dataIndex: "protocol" },
          { title: "本地地址", render: (_, tunnel) => `${tunnel.local_host}:${tunnel.local_port}` },
          { title: "状态", render: (_, tunnel) => <StatusTag value={tunnel.status} /> },
        ]} />
      </>}
    </Drawer>
  );
}

function ClientEditor({ client, onCancel, onSave }: { client?: Client; onCancel: () => void; onSave: (v: { name: string; disabled: boolean }) => void }) {
  return (
    <Modal title="编辑客户端" open={!!client} onCancel={onCancel} onOk={() => document.getElementById("client-save")?.click()} destroyOnClose>
      <Form layout="vertical" initialValues={client} onFinish={onSave}>
        <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="disabled" label="禁用" valuePropName="checked"><Switch /></Form.Item>
        <button id="client-save" hidden type="submit" />
      </Form>
    </Modal>
  );
}
