import { Alert, Button, Divider, Form, Input, message, Switch, Table, Typography } from "antd";
import { api } from "../api/client";
import { Page } from "../components/Page";
import { toMessage, useAsync } from "../hooks/useAsync";
import { plainWsSaveMessage, splitSettingsRows } from "../settings/settingsRows";
import type { SettingItem } from "../types";

export function SettingsPage() {
  const state = useAsync(api.settings, []);
  const { plainWsSetting, rows } = splitSettingsRows(state.data ?? []);

  return (
    <Page title="设置" loading={state.loading} error={state.error} empty={!rows.length && !plainWsSetting} onReload={state.reload}>
      <PasswordForm />
      <Divider />
      {plainWsSetting && (
        <>
          <PlainWsSetting item={plainWsSetting} onSaved={state.reload} />
          <Divider />
        </>
      )}
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
    try {
      await api.changePassword(values.current, values.password);
      message.success("管理员密码已修改");
      form.resetFields();
    } catch (error) {
      message.error(toMessage(error));
    }
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

function PlainWsSetting({ item, onSaved }: { item: SettingItem; onSaved: () => void }) {
  const [form] = Form.useForm<{ enabled: boolean }>();
  const save = async ({ enabled }: { enabled: boolean }) => {
    try {
      await api.updateSetting(item.key, enabled);
      message.success(plainWsSaveMessage(enabled));
      onSaved();
    } catch (error) {
      message.error(toMessage(error));
    }
  };

  return (
    <section className="plain-ws-settings">
      <Typography.Title level={4}>明文 WS 访问</Typography.Title>
      <Alert
        showIcon
        type="warning"
        message="默认关闭，仅在确实需要兼容 ws:// 客户端或 IP 明文访问时开启。保存后需要重启 Nrynet 服务才会生效。"
      />
      <Form form={form} layout="inline" initialValues={{ enabled: Boolean(item.value) }} onFinish={save}>
        <Form.Item name="enabled" valuePropName="checked">
          <Switch checkedChildren="开启" unCheckedChildren="关闭" disabled={!item.mutable} />
        </Form.Item>
        <Button htmlType="submit" disabled={!item.mutable}>保存</Button>
      </Form>
    </section>
  );
}

function ValueEditor({ item, onSaved }: { item: SettingItem; onSaved: () => void }) {
  const isBool = typeof item.value === "boolean";
  const [form] = Form.useForm();
  const save = async ({ value }: { value: SettingItem["value"] }) => {
    try {
      await api.updateSetting(item.key, value);
      message.success("设置已保存，重启 Nrynet 后生效");
      onSaved();
    } catch (error) {
      message.error(toMessage(error));
    }
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
