import { App, Button, Form, Input, Modal, Space, Switch, Table, Typography } from "antd";
import { Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { Token } from "../types";
import { formatDate } from "../utils/format";

export function TokensPage() {
  const { message, modal } = App.useApp();
  const state = useAsync(api.listTokens, []);
  const [open, setOpen] = useState(false);
  const [created, setCreated] = useState<string>();
  const [form] = Form.useForm<{ name: string }>();

  async function create() {
    const values = await form.validateFields();
    const result = await api.createToken(values.name);
    setCreated(result.value);
    setOpen(false);
    form.resetFields();
    await state.reload();
  }

  async function remove(token: Token) {
    modal.confirm({
      title: `删除 ${token.name}?`,
      content: "这会断开并删除使用该令牌的客户端、隧道和关联流量记录。",
      okButtonProps: { danger: true },
      onOk: async () => {
        await api.deleteToken(token.id);
        message.success("令牌已删除");
        await state.reload();
      },
    });
  }

  return (
    <Page title="令牌" loading={state.loading} error={state.error} empty={!state.data?.length} onReload={state.reload}
      extra={<Button type="primary" icon={<Plus size={16} />} onClick={() => setOpen(true)}>创建</Button>}>
      <Table<Token>
        rowKey="id"
        dataSource={state.data ?? []}
        columns={[
          { title: "名称", dataIndex: "name" },
          { title: "前缀", dataIndex: "prefix" },
          { title: "禁用", render: (_, t) => <Switch checked={t.disabled} onChange={(v) => api.setTokenDisabled(t.id, v).then(state.reload)} /> },
          { title: "最后使用", dataIndex: "last_used", render: formatDate },
          { title: "创建时间", dataIndex: "created_at", render: formatDate },
          { title: "操作", render: (_, t) => <Button danger icon={<Trash2 size={16} />} onClick={() => remove(t)} /> },
        ]}
      />
      <Modal title="创建令牌" open={open} onOk={create} onCancel={() => setOpen(false)} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true, message: "请输入名称" }]}>
            <Input />
          </Form.Item>
        </Form>
      </Modal>
      <Modal title="令牌明文" open={!!created} onCancel={() => setCreated(undefined)} footer={null}>
        <Space direction="vertical" className="full-width">
          <Typography.Text copyable code>{created}</Typography.Text>
        </Space>
      </Modal>
    </Page>
  );
}
