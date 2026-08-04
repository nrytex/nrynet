import { Alert, Button, Descriptions, Form, Input, message, Space, Switch, Tag, Typography } from "antd";
import { useCallback, useEffect, useState } from "react";
import { api } from "../api/client";
import { toMessage } from "../hooks/useAsync";
import type { TransportStatus } from "../types";
import { autoSubdomainStatusText } from "./autoSubdomain";
import { CertificateError } from "./CertificateError";
import { certificatePending, certificateStateLabel, endpointRows, nextPlainState, nextTLSState } from "./transportDisplay";

const POLL_MS = 2000;

export function TransportSettings() {
  const [status, setStatus] = useState<TransportStatus>();
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState<string>();
  const pending = certificatePending(status);

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true);
    try {
      setStatus(await api.transport());
    } catch (error) {
      message.error(toMessage(error));
    } finally {
      if (!silent) setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!pending) return;
    const timer = window.setInterval(() => void load(true), POLL_MS);
    return () => window.clearInterval(timer);
  }, [load, pending]);

  async function toggleTLS() {
    const enabled = nextTLSState(status);
    await run("tls", () => api.setTransportTLS(enabled), `TLS 已${enabled ? "开启" : "关闭"}，HTTPS/WSS 配置已热更新`);
  }

  async function togglePlain() {
    const enabled = nextPlainState(status);
    await run("plain", () => api.setTransportPlain(enabled), `兼容明文访问已${enabled ? "开启" : "关闭"}，配置已热更新`);
  }

  async function run(key: string, action: () => Promise<TransportStatus>, success: string) {
    setBusy(key);
    try {
      setStatus(await action());
      message.success(success);
    } catch (error) {
      message.error(toMessage(error));
    } finally {
      setBusy(undefined);
    }
  }

  return (
    <section className="transport-settings">
      <Typography.Title level={4}>访问与证书</Typography.Title>
      <Alert
        showIcon
        type="info"
        message="默认提供 HTTP 控制台、WS Agent 通道和明文数据通道。绑定域名并启用 TLS 后，会同时提供 HTTPS、WSS 和 TLS 数据通道。申请证书前请确认 DNS 已指向本机，公网 TCP 80 端口可访问。"
      />
      <EndpointSummary status={status} loading={loading} />
      <TLSControls status={status} busy={busy} onToggleTLS={toggleTLS} />
      <CertificateForm status={status} busy={busy} onSaved={(next) => setStatus(next)} />
      <AutoSubdomainControl status={status} busy={busy} onSaved={(next) => setStatus(next)} />
      <PlainCompatControl status={status} busy={busy} onTogglePlain={togglePlain} />
    </section>
  );
}

function EndpointSummary({ status, loading }: { status?: TransportStatus; loading: boolean }) {
  const rows = loading && !status ? [{ key: "loading", label: "加载中", value: "正在读取访问地址..." }] : endpointRows(status);
  return (
    <Descriptions
      bordered
      size="small"
      column={1}
      className="transport-endpoints"
      title="当前访问地址"
      items={rows.map((row) => ({ key: row.key, label: row.label, children: row.value }))}
    />
  );
}

function TLSControls({ status, busy, onToggleTLS }: { status?: TransportStatus; busy?: string; onToggleTLS: () => void }) {
  const certReady = Boolean(status?.certificate?.domain);
  return (
    <div className="transport-row">
      <Space direction="vertical" size={4}>
        <Typography.Text strong>TLS / HTTPS / WSS</Typography.Text>
        <Typography.Text type="secondary">启用后立即热更新监听与访问方式；如未绑定域名，请先申请证书。</Typography.Text>
      </Space>
      <Switch
        checked={Boolean(status?.tls?.enabled)}
        checkedChildren="开启"
        unCheckedChildren="关闭"
        loading={busy === "tls"}
        disabled={!certReady && !status?.tls?.enabled}
        onChange={onToggleTLS}
      />
    </div>
  );
}

function CertificateForm({ status, busy, onSaved }: { status?: TransportStatus; busy?: string; onSaved: (status: TransportStatus) => void }) {
  const [form] = Form.useForm<{ domain: string; email: string }>();
  const [submitting, setSubmitting] = useState(false);
  const cert = status?.certificate;
  const unavailable = status?.certbot && !status.certbot.available;
  const pending = certificatePending(status);

  async function submit(values: { domain: string; email: string }) {
    setSubmitting(true);
    try {
      onSaved(await api.requestCertificate(values.domain.trim(), values.email.trim()));
      message.success("证书申请已提交，正在等待 Certbot 签发");
    } catch (error) {
      message.error(toMessage(error));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="transport-certificate">
      <Space className="transport-status-line" wrap>
        <Typography.Text strong>域名证书</Typography.Text>
        <Tag color={certificateTagColor(cert?.status)}>{certificateStateLabel(cert?.status)}</Tag>
        <Tag color={status?.certbot?.available ? "green" : "orange"}>{status?.certbot?.available ? "Certbot 可用" : "Certbot 不可用"}</Tag>
      </Space>
      {unavailable && <Alert showIcon type="warning" message={status?.certbot?.message || "服务器未检测到 Certbot，无法自动申请 Let's Encrypt 证书。"} />}
      <CertificateError message={cert?.error} details={cert?.details} />
      <Descriptions
        size="small"
        column={1}
        items={[
          { key: "domain", label: "绑定域名", children: cert?.domain || "-" },
          { key: "issuer", label: "签发者", children: cert?.issuer || "-" },
          { key: "notAfter", label: "到期时间", children: cert?.not_after ? new Date(cert.not_after).toLocaleString("zh-CN") : "-" },
        ]}
      />
      <Form form={form} layout="inline" onFinish={submit} className="transport-cert-form">
        <Form.Item name="domain" rules={[{ required: true, message: "请输入域名" }]}>
          <Input placeholder="example.com" disabled={unavailable || pending} />
        </Form.Item>
        <Form.Item name="email" rules={[{ required: true, type: "email", message: "请输入有效邮箱" }]}>
          <Input placeholder="admin@example.com" disabled={unavailable || pending} />
        </Form.Item>
        <Button type="primary" htmlType="submit" loading={busy === "certificate" || pending || submitting} disabled={unavailable}>
          申请 Let's Encrypt
        </Button>
      </Form>
    </div>
  );
}

function AutoSubdomainControl({ status, busy, onSaved }: { status?: TransportStatus; busy?: string; onSaved: (status: TransportStatus) => void }) {
  const [form] = Form.useForm<{ enabled: boolean; base_domain: string }>();
  const [submitting, setSubmitting] = useState(false);
  const config = status?.auto_subdomain;

  useEffect(() => {
    form.setFieldsValue({ enabled: Boolean(config?.enabled), base_domain: config?.base_domain || "" });
  }, [config?.base_domain, config?.enabled, form]);

  async function submit(values: { enabled: boolean; base_domain: string }) {
    const baseDomain = values.base_domain.trim();
    if (values.enabled && !baseDomain) {
      message.error("请输入隧道根域名");
      return;
    }
    setSubmitting(true);
    try {
      onSaved(await api.setAutoSubdomain(Boolean(values.enabled), baseDomain));
      message.success(`自动子域名已${values.enabled ? "开启" : "关闭"}，配置已热更新`);
    } catch (error) {
      message.error(toMessage(error));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="transport-auto-subdomain">
      <Space className="transport-status-line" wrap>
        <Typography.Text strong>自动子域名分配</Typography.Text>
        <Tag color={config?.enabled ? "green" : "default"}>{autoSubdomainStatusText(config)}</Tag>
      </Space>
      <Alert
        showIcon
        type="info"
        message="只需一次配置通配符 DNS：*.根域名 指向这台服务器。开启后，新建 HTTP/HTTPS 隧道且域名留空时会自动分配；已有域名不会被修改，手动填写的显式域名优先生效。"
      />
      <Form form={form} layout="inline" onFinish={submit} className="transport-cert-form">
        <Form.Item name="enabled" valuePropName="checked">
          <Switch checkedChildren="开启" unCheckedChildren="关闭" loading={busy === "auto-subdomain" || submitting} />
        </Form.Item>
        <Form.Item name="base_domain">
          <Input placeholder="tunnels.example.com" />
        </Form.Item>
        <Button htmlType="submit" type="primary" loading={submitting}>
          保存
        </Button>
      </Form>
      {config?.suffix_example && <Typography.Text type="secondary">示例：{config.suffix_example}</Typography.Text>}
    </div>
  );
}

function PlainCompatControl({ status, busy, onTogglePlain }: { status?: TransportStatus; busy?: string; onTogglePlain: () => void }) {
  return (
    <div className="transport-row">
      <Space direction="vertical" size={4}>
        <Typography.Text strong>兼容明文访问</Typography.Text>
        <Typography.Text type="secondary">主 HTTP 控制台和 WS 通道会保留；此开关用于兼容额外明文监听，适合内网或旧客户端。</Typography.Text>
      </Space>
      <Switch
        checked={Boolean(status?.compatibility_plain?.enabled)}
        checkedChildren="开启"
        unCheckedChildren="关闭"
        loading={busy === "plain"}
        onChange={onTogglePlain}
      />
    </div>
  );
}

function certificateTagColor(status?: string) {
  const value = status?.toLowerCase();
  if (value === "valid" || value === "issued" || value === "success") return "green";
  if (value === "pending" || value === "running" || value === "issuing") return "blue";
  if (value === "failed" || value === "error") return "red";
  return "default";
}
