import { Button, Divider, Form, Input, message, Switch, Table, Typography } from "antd";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { SettingItem } from "../types";

export function SettingsPage() {
  const state = useAsync(api.settings, []);
  const rows = state.data ?? [];

  return (
    <Page title="Settings" loading={state.loading} error={state.error} empty={!rows.length} onReload={state.reload}>
      <PasswordForm />
      <Divider />
      <Typography.Title level={4}>Server configuration</Typography.Title>
      <Table<SettingItem>
        rowKey="key"
        dataSource={rows}
        pagination={false}
        columns={[
          { title: "Key", dataIndex: "key" },
          { title: "Value", render: (_, item) => <ValueEditor item={item} onSaved={state.reload} /> },
          { title: "Description", dataIndex: "description", render: (v) => v || "-" },
        ]}
      />
    </Page>
  );
}

function PasswordForm() {
  const [form] = Form.useForm();
  const submit = async (values: { current: string; password: string }) => {
    await api.changePassword(values.current, values.password);
    message.success("Administrator password changed");
    form.resetFields();
  };
  return (
    <section className="password-settings">
      <Typography.Title level={4}>Administrator password</Typography.Title>
      <Form form={form} layout="inline" onFinish={submit}>
        <Form.Item name="current" rules={[{ required: true }]}><Input.Password placeholder="Current password" /></Form.Item>
        <Form.Item name="password" rules={[{ required: true, min: 12 }]}><Input.Password placeholder="New password" /></Form.Item>
        <Button type="primary" htmlType="submit">Change</Button>
      </Form>
    </section>
  );
}

function ValueEditor({ item, onSaved }: { item: SettingItem; onSaved: () => void }) {
  const isBool = typeof item.value === "boolean";
  const [form] = Form.useForm();
  const save = async ({ value }: { value: SettingItem["value"] }) => {
    await api.updateSetting(item.key, value);
    message.success("Setting saved; restart Nrynet to apply it");
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
