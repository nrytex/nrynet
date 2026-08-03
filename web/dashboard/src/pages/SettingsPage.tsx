import { Button, Divider, Form, Input, message, Switch, Table, Typography } from "antd";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { SettingItem } from "../types";

export function SettingsPage() {
  const state = useAsync(api.settings, []);
  const rows = state.data ?? [];

  return (
    <Page title="设置" loading={state.loading} error={state.error} empty={!rows.length} onReload={state.reload}>
      <PasswordForm />
      <Divider />
      <Typography.Title level={4}>服务端配置</Typography.Title>
      <Table<SettingItem>
        rowKey="key"
        dataSource={rows}
        pagination={false}
        columns={[
          { title: "配置项", dataIndex: "key" },
          { title: "值", render: (_, item) => <ValueEditor item={item} onSaved={state.reload} /> },
          { title: "说明", dataIndex: "description", render: (v) => v || "-" },
        ]}
      />
    </Page>
  );
}

function PasswordForm() {
  const [form] = Form.useForm();
  const submit = async (values: { current: string; password: string }) => {
    await api.changePassword(values.current, values.password);
    message.success("管理员密码已修改");
    form.resetFields();
  };
  return (
    <section className="password-settings">
      <Typography.Title level={4}>管理员密码</Typography.Title>
      <Form form={form} layout="inline" onFinish={submit}>
        <Form.Item name="current" rules={[{ required: true }]}><Input.Password placeholder="当前密码" /></Form.Item>
        <Form.Item name="password" rules={[{ required: true, min: 12 }]}><Input.Password placeholder="新密码" /></Form.Item>
        <Button type="primary" htmlType="submit">修改</Button>
      </Form>
    </section>
  );
}

function ValueEditor({ item, onSaved }: { item: SettingItem; onSaved: () => void }) {
  const isBool = typeof item.value === "boolean";
  const [form] = Form.useForm();
  const save = async ({ value }: { value: SettingItem["value"] }) => {
    await api.updateSetting(item.key, value);
    message.success("设置已保存，重启 Nrynet 后生效");
    onSaved();
  };
  return (
    <Form form={form} layout="inline" initialValues={{ value: item.value }} onFinish={save}>
      <Form.Item name="value" valuePropName={isBool ? "checked" : "value"}>
        {isBool ? <Switch disabled={!item.mutable} /> : <Input disabled={!item.mutable} />}
      </Form.Item>
      <Button htmlType="submit" disabled={!item.mutable}>保存</Button>
    </Form>
  );
}
