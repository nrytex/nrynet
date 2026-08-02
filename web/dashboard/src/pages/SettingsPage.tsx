import { Button, Form, Input, message, Switch, Table } from "antd";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { useAsync } from "../hooks/useAsync";
import type { SettingItem } from "../types";

export function SettingsPage() {
  const state = useAsync(api.settings, []);
  const rows = state.data ?? [];

  return (
    <Page title="Settings" loading={state.loading} error={state.error} empty={!rows.length} onReload={state.reload}>
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

function ValueEditor({ item, onSaved }: { item: SettingItem; onSaved: () => void }) {
  const isBool = typeof item.value === "boolean";
  const [form] = Form.useForm();
  const save = async ({ value }: { value: SettingItem["value"] }) => {
    await api.updateSetting(item.key, value);
    message.success("Setting saved; restart NAT-Link to apply it");
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
