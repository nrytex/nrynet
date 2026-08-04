import { App, Button, Form, Input, InputNumber, Modal, Select, Space, Table } from "antd";
import { Pause, Play, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { StatusTag } from "../components/StatusTag";
import { statusText } from "../display/status";
import { useAsync } from "../hooks/useAsync";
import { autoSubdomainHint, shouldSuggestAutoDomain } from "../settings/autoSubdomain";
import type { Client, TransportAutoSubdomain, Tunnel } from "../types";

type TunnelForm = Omit<Tunnel, "id" | "created_at" | "updated_at"> & { ip_allowlist_text?: string };

export function TunnelsPage() {
  const { message, modal } = App.useApp();
  const state = useAsync(async () => ({ tunnels: await api.listTunnels(), clients: await api.listClients(), transport: await api.transport() }), []);
  const [editing, setEditing] = useState<Tunnel | null>(null);
  const tunnels = state.data?.tunnels ?? [];
  const clients = state.data?.clients ?? [];
  const autoSubdomain = state.data?.transport.auto_subdomain;

  async function submit(values: TunnelForm) {
    const payload = normalize(values);
    if (editing?.id) await api.updateTunnel(editing.id, payload);
    else await api.createTunnel(payload);
    setEditing(null);
    await state.reload();
  }

  async function toggle(tunnel: Tunnel) {
    if (tunnel.status === "running") await api.stopTunnel(tunnel.id);
    else await api.startTunnel(tunnel.id);
    await state.reload();
  }

  function remove(tunnel: Tunnel) {
    modal.confirm({
      title: `删除 ${tunnel.name}?`,
      okButtonProps: { danger: true },
      onOk: async () => {
        await api.deleteTunnel(tunnel.id);
        message.success("隧道已删除");
        await state.reload();
      },
    });
  }

  return (
    <Page title="隧道" loading={state.loading} error={state.error} empty={!tunnels.length} onReload={state.reload}
      extra={<Button type="primary" icon={<Plus size={16} />} onClick={() => setEditing(emptyTunnel(clients[0]?.id))}>创建</Button>}>
      <Table<Tunnel>
        rowKey="id"
        dataSource={tunnels}
        columns={[
          { title: "名称", dataIndex: "name" },
          { title: "协议", dataIndex: "protocol" },
          { title: "客户端", dataIndex: "client_id" },
          { title: "本地地址", render: (_, t) => `${t.local_host}:${t.local_port}` },
          { title: "远端地址", render: (_, t) => t.domain || t.remote_port || "-" },
          { title: "状态", dataIndex: "status", render: (v) => <StatusTag value={v} /> },
          { title: "操作", render: (_, t) => <Actions tunnel={t} onToggle={toggle} onEdit={setEditing} onRemove={remove} /> },
        ]}
      />
      <TunnelEditor tunnel={editing} clients={clients} autoSubdomain={autoSubdomain} onCancel={() => setEditing(null)} onSave={submit} />
    </Page>
  );
}

function Actions({ tunnel, onToggle, onEdit, onRemove }: { tunnel: Tunnel; onToggle: (t: Tunnel) => void; onEdit: (t: Tunnel) => void; onRemove: (t: Tunnel) => void }) {
  return (
    <Space>
      <Button icon={tunnel.status === "running" ? <Pause size={16} /> : <Play size={16} />} onClick={() => onToggle(tunnel)} />
      <Button onClick={() => onEdit(tunnel)}>编辑</Button>
      <Button danger icon={<Trash2 size={16} />} onClick={() => onRemove(tunnel)} />
    </Space>
  );
}

function TunnelEditor(props: { tunnel: Tunnel | null; clients: Client[]; autoSubdomain?: TransportAutoSubdomain; onCancel: () => void; onSave: (v: TunnelForm) => void }) {
  const [form] = Form.useForm<TunnelForm>();
  const protocol = Form.useWatch("protocol", form) || props.tunnel?.protocol;
  const initial = props.tunnel ? { ...props.tunnel, ip_allowlist_text: props.tunnel.ip_allowlist?.join(",") } : undefined;
  const suggestAutoDomain = !props.tunnel?.id && shouldSuggestAutoDomain(protocol, props.autoSubdomain);
  return (
    <Modal title={props.tunnel?.id ? "编辑隧道" : "创建隧道"} open={!!props.tunnel} onCancel={props.onCancel} onOk={() => document.getElementById("tunnel-save")?.click()} destroyOnClose width={720}>
      <Form form={form} layout="vertical" initialValues={initial} onFinish={props.onSave}>
        <div className="form-grid">
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="client_id" label="客户端" rules={[{ required: true }]}><Select options={props.clients.map((c) => ({ value: c.id, label: c.name }))} /></Form.Item>
          <Form.Item name="protocol" label="协议" rules={[{ required: true }]}><Select options={["tcp", "http", "https", "udp"].map((v) => ({ value: v, label: v.toUpperCase() }))} /></Form.Item>
          <Form.Item name="status" label="状态"><Select options={["stopped", "running"].map((v) => ({ value: v, label: statusText(v) }))} /></Form.Item>
          <Form.Item name="local_host" label="本地地址" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="local_port" label="本地端口" rules={[{ required: true }]}><InputNumber min={1} max={65535} className="full-width" /></Form.Item>
          <Form.Item name="remote_port" label="远端端口"><InputNumber min={0} max={65535} className="full-width" /></Form.Item>
          <Form.Item name="domain" label="域名" help={suggestAutoDomain ? autoSubdomainHint(props.autoSubdomain) : undefined}>
            <Input placeholder={suggestAutoDomain ? "留空自动生成，填写则优先生效" : undefined} />
          </Form.Item>
        </div>
        <Form.Item name="ip_allowlist_text" label="IP 白名单"><Input placeholder="逗号分隔，留空为不限" /></Form.Item>
        <button id="tunnel-save" hidden type="submit" />
      </Form>
    </Modal>
  );
}

function normalize(values: TunnelForm): Partial<Tunnel> {
  const list = values.ip_allowlist_text?.split(",").map((item) => item.trim()).filter(Boolean) ?? [];
  const { ip_allowlist_text, ...payload } = values;
  return { ...payload, ip_allowlist: list, status: payload.status || "stopped" };
}

function emptyTunnel(clientID = ""): Tunnel {
  const now = new Date().toISOString();
  return { id: "", client_id: clientID, name: "", protocol: "tcp", local_host: "127.0.0.1", local_port: 8080, remote_port: 6000, domain: "", status: "stopped", ip_allowlist: [], created_at: now, updated_at: now };
}
