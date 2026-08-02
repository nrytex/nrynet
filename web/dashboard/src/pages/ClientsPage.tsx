import { App, Button, Descriptions, Drawer, Form, Input, Modal, Space, Switch, Table, Typography } from "antd";
import { Eye, KeyRound, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { Client, Tunnel } from "../types";
import { formatDate } from "../utils/format";
import { StatusTag } from "./OverviewPage";

export function ClientsPage() {
  const { message, modal } = App.useApp();
  const state = useAsync(api.listClients, []);
  const [detail, setDetail] = useState<{ client: Client; tunnels: Tunnel[] }>();
  const [editing, setEditing] = useState<Client>();
  const [tokenValue, setTokenValue] = useState<string>();

  async function loadDetail(client: Client) {
    setDetail(await api.getClient(client.id));
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
      okButtonProps: { danger: true },
      onOk: async () => {
        await api.deleteClient(client.id);
        message.success("Client 已删除");
        await state.reload();
      },
    });
  }

  return (
    <Page title="Clients" loading={state.loading} error={state.error} empty={!state.data?.length} onReload={state.reload}>
      <Table<Client>
        rowKey="id"
        dataSource={state.data ?? []}
        columns={[
          { title: "Name", dataIndex: "name" },
          { title: "Status", render: (_, c) => <StatusTag value={c.disabled ? "disabled" : c.status} /> },
          { title: "IP", dataIndex: "ip" },
          { title: "OS", dataIndex: "os" },
          { title: "Version", dataIndex: "version" },
          { title: "Last Online", dataIndex: "last_online", render: formatDate },
          { title: "操作", render: (_, c) => <RowActions client={c} onView={loadDetail} onEdit={setEditing} onReset={async () => setTokenValue((await api.resetClientToken(c.id)).value)} onRemove={remove} /> },
        ]}
      />
      <ClientDrawer detail={detail} onClose={() => setDetail(undefined)} />
      <ClientEditor client={editing} onCancel={() => setEditing(undefined)} onSave={saveEdit} />
      <Modal title="新 Token" open={!!tokenValue} onCancel={() => setTokenValue(undefined)} footer={null}>
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

function ClientDrawer({ detail, onClose }: { detail?: { client: Client; tunnels: Tunnel[] }; onClose: () => void }) {
  return (
    <Drawer title="Client 详情" open={!!detail} onClose={onClose} width={560}>
      {detail && <Descriptions column={1} bordered items={Object.entries(detail.client).map(([key, value]) => ({ key, label: key, children: String(value) }))} />}
    </Drawer>
  );
}

function ClientEditor({ client, onCancel, onSave }: { client?: Client; onCancel: () => void; onSave: (v: { name: string; disabled: boolean }) => void }) {
  return (
    <Modal title="编辑 Client" open={!!client} onCancel={onCancel} onOk={() => document.getElementById("client-save")?.click()} destroyOnClose>
      <Form layout="vertical" initialValues={client} onFinish={onSave}>
        <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
        <Form.Item name="disabled" label="禁用" valuePropName="checked"><Switch /></Form.Item>
        <button id="client-save" hidden type="submit" />
      </Form>
    </Modal>
  );
}
