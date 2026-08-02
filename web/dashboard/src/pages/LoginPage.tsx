import { Alert, Button, Card, Form, Input, Typography } from "antd";
import { LockKeyhole, UserRound } from "lucide-react";
import { useState } from "react";
import { api } from "../api/client";
import { toMessage } from "../hooks/useAsync";

interface LoginPageProps {
  onSuccess: () => void;
}

export function LoginPage({ onSuccess }: LoginPageProps) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string>();

  async function submit(values: { username: string; password: string }) {
    setLoading(true);
    setError(undefined);
    try {
      await api.login(values.username, values.password);
      onSuccess();
    } catch (err) {
      setError(toMessage(err));
    } finally {
      setLoading(false);
    }
  }

  return (
    <main className="login-screen">
      <Card className="login-card">
        <Typography.Title level={2}>NAT-Link</Typography.Title>
        <Typography.Text type="secondary">登录本地管理员账号</Typography.Text>
        {error && <Alert showIcon type="error" message={error} />}
        <Form layout="vertical" onFinish={submit} requiredMark={false}>
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: "请输入用户名" }]}>
            <Input prefix={<UserRound size={16} />} autoComplete="username" />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: "请输入密码" }]}>
            <Input.Password prefix={<LockKeyhole size={16} />} autoComplete="current-password" />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={loading} block>
            登录
          </Button>
        </Form>
      </Card>
    </main>
  );
}
