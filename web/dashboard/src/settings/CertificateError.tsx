import { Alert, Typography } from "antd";

export function CertificateError({ message, details }: { message?: string; details?: string }) {
  if (!message) return null;
  return (
    <Alert
      showIcon
      type="error"
      message={message}
      description={details ? <Typography.Paragraph className="certificate-error-details" copyable>{details}</Typography.Paragraph> : undefined}
    />
  );
}
