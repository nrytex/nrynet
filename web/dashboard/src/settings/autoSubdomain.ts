import type { TransportAutoSubdomain } from "../types";

export function autoSubdomainStatusText(config?: TransportAutoSubdomain) {
  if (!config?.enabled) return "未开启";
  if (!config.base_domain) return "待配置根域名";
  return `已开启：*.${config.base_domain}`;
}

export function autoSubdomainHint(config?: TransportAutoSubdomain) {
  if (!config?.enabled || !config.base_domain) return "自动子域名未开启，HTTP/HTTPS 隧道仍需手动填写域名。";
  const example = config.suffix_example || `app.${config.base_domain}`;
  return `已开启自动子域名。新建 HTTP/HTTPS 隧道且域名留空时将自动分配，例如 ${example}；手动填写域名时优先使用显式域名。`;
}

export function shouldSuggestAutoDomain(protocol?: string, config?: TransportAutoSubdomain) {
  const normalized = protocol?.toLowerCase();
  return Boolean(config?.enabled && config.base_domain && (normalized === "http" || normalized === "https"));
}
