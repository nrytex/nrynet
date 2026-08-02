import { Alert, Button, Spin } from "antd";
import { RefreshCw } from "lucide-react";
import type { ReactNode } from "react";

interface PageProps {
  title: string;
  extra?: ReactNode;
  loading?: boolean;
  error?: string;
  empty?: boolean;
  onReload?: () => void;
  children: ReactNode;
}

export function Page({ title, extra, loading, error, onReload, children }: PageProps) {
  return (
    <section className="page-shell">
      <div className="page-header">
        <h1>{title}</h1>
        <div className="page-actions">
          {onReload && (
            <Button icon={<RefreshCw size={16} />} onClick={onReload}>
              刷新
            </Button>
          )}
          {extra}
        </div>
      </div>
      {error && <Alert showIcon type="error" message={error} className="page-alert" />}
      <Spin spinning={!!loading}>
        {children}
      </Spin>
    </section>
  );
}
