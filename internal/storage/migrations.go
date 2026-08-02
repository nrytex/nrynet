package storage

var migrations = []string{
	`CREATE TABLE IF NOT EXISTS settings (
        key TEXT PRIMARY KEY, value TEXT NOT NULL
    )`,
	`CREATE TABLE IF NOT EXISTS admins (
        id TEXT PRIMARY KEY, username TEXT NOT NULL UNIQUE,
        password_hash TEXT NOT NULL, created_at DATETIME NOT NULL
    )`,
	`CREATE TABLE IF NOT EXISTS tokens (
        id TEXT PRIMARY KEY, name TEXT NOT NULL, prefix TEXT NOT NULL,
        secret_hash TEXT NOT NULL, disabled INTEGER NOT NULL DEFAULT 0,
        last_used DATETIME, created_at DATETIME NOT NULL
    )`,
	`CREATE TABLE IF NOT EXISTS clients (
        id TEXT PRIMARY KEY, name TEXT NOT NULL, device_id TEXT NOT NULL UNIQUE,
        token_id TEXT NOT NULL, status TEXT NOT NULL, disabled INTEGER NOT NULL DEFAULT 0,
        ip TEXT NOT NULL, os TEXT NOT NULL, version TEXT NOT NULL,
        last_online DATETIME NOT NULL, created_at DATETIME NOT NULL,
        FOREIGN KEY(token_id) REFERENCES tokens(id)
    )`,
	`CREATE TABLE IF NOT EXISTS tunnels (
        id TEXT PRIMARY KEY, client_id TEXT NOT NULL, name TEXT NOT NULL,
        protocol TEXT NOT NULL, local_host TEXT NOT NULL, local_port INTEGER NOT NULL,
        remote_port INTEGER NOT NULL DEFAULT 0, domain TEXT NOT NULL DEFAULT '',
        status TEXT NOT NULL, ip_allowlist TEXT NOT NULL DEFAULT '[]',
        created_at DATETIME NOT NULL, updated_at DATETIME NOT NULL,
        FOREIGN KEY(client_id) REFERENCES clients(id)
    )`,
	`CREATE TABLE IF NOT EXISTS traffic_logs (
        id INTEGER PRIMARY KEY AUTOINCREMENT, tunnel_id TEXT NOT NULL,
        upload INTEGER NOT NULL, download INTEGER NOT NULL, created_at DATETIME NOT NULL,
        FOREIGN KEY(tunnel_id) REFERENCES tunnels(id)
    )`,
	`CREATE INDEX IF NOT EXISTS idx_traffic_tunnel_date ON traffic_logs(tunnel_id, created_at)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_tunnels_protocol_domain
        ON tunnels(protocol, domain) WHERE domain <> ''`,
	`CREATE TABLE IF NOT EXISTS event_logs (
        id INTEGER PRIMARY KEY AUTOINCREMENT, level TEXT NOT NULL,
        event TEXT NOT NULL, message TEXT NOT NULL, fields TEXT NOT NULL,
        created_at DATETIME NOT NULL
    )`,
	`CREATE INDEX IF NOT EXISTS idx_events_date ON event_logs(created_at)`,
}
