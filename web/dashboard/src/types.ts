export type Status = "online" | "offline" | "running" | "stopped" | string;

export interface SessionUser {
  sub?: string;
  username?: string;
  exp?: number;
  [key: string]: unknown;
}

export interface Token {
  id: string;
  name: string;
  prefix: string;
  disabled: boolean;
  last_used?: string;
  created_at: string;
}

export interface Client {
  id: string;
  name: string;
  device_id: string;
  token_id: string;
  status: Status;
  disabled: boolean;
  ip: string;
  os: string;
  version: string;
  last_online: string;
  created_at: string;
}

export interface TrafficSummary {
  upload: number;
  download: number;
}

export interface ClientDetail {
  client: Client;
  tunnels: Tunnel[];
  connected_at?: string;
  connected_seconds: number;
  traffic: {
    today: TrafficSummary;
    month: TrafficSummary;
  };
}

export interface Tunnel {
  id: string;
  client_id: string;
  name: string;
  protocol: "tcp" | "p2p" | "http" | "https" | "udp" | "visitor_webrtc" | string;
  visitor_token?: string;
  local_host: string;
  local_port: number;
  remote_port: number;
  domain: string;
  status: Status;
  ip_allowlist: string[];
  created_at: string;
  updated_at: string;
}

export interface TrafficPoint {
  tunnel_id?: string;
  upload: number;
  download: number;
  created_at?: string;
  at?: string;
}

export interface TrafficTarget {
  id: string;
  name: string;
  upload: number;
  download: number;
}

export interface TrafficResponse {
  summary: TrafficSummary;
  clients: TrafficTarget[];
  tunnels: TrafficTarget[];
  since: string;
}

export interface Overview {
  status: string;
  uptime_seconds: number;
  online_clients: number;
  total_clients: number;
  active_tunnels: number;
  total_tunnels: number;
  connections: number;
  bandwidth_bps: number;
  today_upload: number;
  today_download: number;
}

export interface LogEntry {
  id?: string | number;
  level: string;
  event?: string;
  message: string;
  source?: string;
  created_at?: string;
}

export interface SettingItem {
  key: string;
  value: string | number | boolean;
  description?: string;
  mutable?: boolean;
}

export interface TransportEndpoint {
  enabled: boolean;
  listen?: string;
  data_listen?: string;
  control_url?: string;
  websocket_url?: string;
  data_address?: string;
}

export interface TransportCertificate {
  domain?: string;
  issuer?: string;
  not_after?: string;
  status?: string;
  error?: string;
  details?: string;
}

export interface TransportCertbot {
  available: boolean;
  message?: string;
  version?: string;
}

export interface TransportAutoSubdomain {
  enabled: boolean;
  base_domain?: string;
  suffix_example?: string;
}

export interface TransportStatus {
  plain: TransportEndpoint;
  compatibility_plain: TransportEndpoint;
  tls: TransportEndpoint;
  certbot: TransportCertbot;
  certificate?: TransportCertificate;
  auto_subdomain?: TransportAutoSubdomain;
}

export interface RelayNode {
	id: string;
	address: string;
	control_address?: string;
	connections: number;
	healthy: boolean;
	last_seen: string;
}

export interface RelayAssignment {
	tunnel_id: string;
	node_id: string;
	address: string;
}
