# Nrynet 安装部署指南

本文说明 Nrynet Server、命令行 Agent、桌面客户端和可选 Relay 节点的安装与生产部署。示例以源码仓库根目录为起点，覆盖 Linux、Windows 和 macOS；根据项目要求，本文不使用 Docker。

## 1. 组件与部署位置

| 组件 | 推荐部署位置 | 作用 |
| --- | --- | --- |
| `nrynet-server` | 具有公网地址的 Linux/Windows 主机 | Dashboard、管理 API、控制通道、数据转发和 SQLite |
| `nrynet-client` | 需要暴露内网服务的主机 | 命令行 Agent，连接内网本地服务 |
| `nrynet-desktop` | Windows/macOS 用户桌面 | 带图形界面的 Agent |
| `nrynet-relay` | 可选的边缘公网节点 | 分布式承载公网隧道端口 |

Server 和 Agent 可以安装在同一台机器用于本地验证，但生产环境通常分别部署。

## 2. 环境要求

- Go 1.25 或项目 `go.mod` 指定的兼容版本
- Node.js 和 npm，仅构建 Dashboard/桌面前端时需要
- Wails v3，仅构建桌面客户端时需要
- 公网 IP；生产环境推荐使用解析到 Server 的域名
- Linux 安装器会安装受限 Certbot helper，Dashboard 可申请 Let's Encrypt 证书
- Linux 使用 systemd；Windows 使用系统服务保持后台运行

新安装默认提供 `7000` 上的 HTTP/WS 和 `7001` 上的明文数据通道，不默认启用 TLS。管理员在 Dashboard 的“访问与证书”中绑定域名并申请证书后，相同端口会热开启 HTTPS/WSS 和 TLS 数据；原 HTTP/WS 仍可同时使用。`7004/7005` 仅作为旧部署的额外兼容端口，默认关闭。

## 3. 一键安装 Server

Release 提供 Linux 和 Windows 一键安装脚本。脚本会下载对应架构的软件包、校验 SHA-256、写入 `0.0.0.0` 监听配置并注册系统服务。Linux 还会安装 Certbot 及受限的 root helper；Nrynet 主服务仍以非 root 用户运行。整个流程不使用 Docker。

### 3.1 Linux

适用于使用 systemd 的 Linux amd64/arm64 发行版：

```bash
curl -fLO https://github.com/nrytex/nrynet/releases/latest/download/install-server.sh
chmod +x install-server.sh
sudo ./install-server.sh --public-host nat.example.com
```

安装后打开 `http://<server-ip>:7000`，进入“设置 > 访问与证书”，填写域名和邮箱即可申请 Let's Encrypt。域名必须已解析到本机，公网 TCP `80` 必须可访问。也保留无人值守安装方式：

```bash
sudo ./install-server.sh --certbot-domain nat.example.com --certbot-email admin@example.com
```

证书签发成功后，域名用户使用 `wss://nat.example.com:7000/agent/connect`，IP 用户仍可使用 `ws://<server-ip>:7000/agent/connect`；数据地址都使用 `7001`，服务会自动识别明文或 TLS。整个切换与续期均不需要重启 Nrynet。`--enable-ws` 只用于启用额外兼容端口 `7004/7005`，普通部署不需要。

脚本会自动安装缺少的 `curl`、`openssl`、`certbot`、`tar` 和校验工具，安装目录默认为 `/opt/nrynet`，服务名为 `nrynet-server`。指定版本或明确生成自签名证书：

```bash
sudo ./install-server.sh --version 1.0.0 --public-host nat.example.com --renew-cert
```

通过 HTTP(S) 或 SOCKS5h 代理下载安装依赖和 GitHub Release：

```bash
sudo ./install-server.sh --public-host nat.example.com --proxy http://127.0.0.1:7890
```

SOCKS5h 会让代理端完成 DNS 解析，避免本机 DNS 泄漏：

```bash
sudo ./install-server.sh --public-host nat.example.com --proxy socks5h://127.0.0.1:1080
```

再次执行同一个脚本即可升级。安装器会读取已安装 Server 的版本，和 Release 软件包内的 `VERSION` 比较：相同版本不会重复替换二进制，只允许正常升级，并保留现有数据库、配置和证书。需要明确回滚时才使用 `--allow-downgrade`；Windows 对应参数为 `-AllowDowngrade`。

早期错误使用 `NAT-Link` 名称的一键安装会被自动迁移到 Nrynet：默认安装目录、数据库、配置、证书和服务会保留并改用新的名称。

安装脚本不只是复制文件。它会创建 `/etc/systemd/system/nrynet-server.service`，执行 `daemon-reload`，设置开机启动并立即启动 Server。安装后可直接管理服务：

```bash
sudo systemctl status nrynet-server
sudo systemctl restart nrynet-server
sudo journalctl -u nrynet-server -f
sudo systemctl status nrynet-certbot.path nrynet-certbot-renew.timer
sudo journalctl -u nrynet-certbot.service -n 50 --no-pager
```

### 3.2 Windows

使用管理员身份打开 PowerShell：

```powershell
Invoke-WebRequest https://github.com/nrytex/nrynet/releases/latest/download/install-server.ps1 -OutFile install-server.ps1
Set-ExecutionPolicy -Scope Process Bypass
.\install-server.ps1 -PublicHost nat.example.com
```

Windows 默认通过主端口提供 HTTP/WS；确实需要额外的 `7004/7005` 兼容端口时，可在安装时预设开启：

```powershell
.\install-server.ps1 -PublicHost nat.example.com -EnableWS
```

需要代理时使用：

```powershell
.\install-server.ps1 -PublicHost nat.example.com --proxy http://127.0.0.1:7890
```

PowerShell 原生写法 `-Proxy http://127.0.0.1:7890` 同样支持。

SOCKS5h 使用 Windows 自带的 `curl.exe` 下载 Release，并将同一代理传给 `winget`：

```powershell
.\install-server.ps1 -PublicHost nat.example.com --proxy socks5h://127.0.0.1:1080
```

脚本会在缺少 OpenSSL 时通过 `winget` 安装，注册 `NrynetServer` Windows 服务，并开放 TCP `7000/7001/7004/7005/8080` 和 UDP `7002/7003`。`7004/7005` 会预先加入防火墙规则，但只有在兼容开关开启时才监听。Dashboard 中 TLS 与兼容端口开关均热生效。使用 `-SkipFirewall` 可跳过防火墙规则。

不使用 `-EnableWS` 时，也可以之后在 Dashboard 开启额外兼容访问，不需要重启服务。若安装时使用了 `-SkipFirewall`，需自行放行 TCP `7004/7005`。

首次安装完成后，终端会显示一次性管理员密码，随后从配置文件移除。Windows 会准备一份默认关闭的自签名证书；手动开启自签名 TLS 时浏览器仍会提示不受信任。Windows Server 当前不支持从 Dashboard 自动运行 Certbot，可使用已有证书或反向代理终止 TLS。

Windows 服务同样会设置为自动启动并立即运行：

```powershell
Get-Service NrynetServer
Restart-Service NrynetServer
Get-WinEvent -LogName Application -MaxEvents 50
```

### 3.3 安装完成后开始使用

1. 首次在浏览器打开 `http://<public-host>:7000`。
2. 使用安装器终端输出的管理员用户名和一次性密码登录 Dashboard。
3. 在“访问令牌”页面创建 Agent Token。不要把管理员密码当作 Agent Token 使用。
4. Agent 默认填写 `ws://<public-host>:7000/agent/connect` 和数据地址 `<public-host>:7001`。绑定域名后也可使用 `wss://<domain>:7000/agent/connect`，无需修改服务端端口。
5. Agent 上线后，在 Dashboard 创建 TCP、UDP、HTTP 或 HTTPS 隧道，选择对应设备和本地服务地址，再启动隧道。

### 自动分配隧道子域名

进入“设置 > 访问与证书”，填写隧道根域名（例如 `tunnels.example.com`）并开启“自动子域名分配”。然后在 DNS 服务商处只需配置一次：

```text
*.tunnels.example.com  A/AAAA  <Nrynet Server 公网地址>
```

此后新建 HTTP/HTTPS 隧道时可以把“域名”留空。Server 会根据隧道名称生成唯一地址，例如 `dashboard.tunnels.example.com`；重名时自动追加数字后缀。手动填写完整域名时始终优先使用手动值。关闭功能只会停止后续自动分配，不会删除或改变已有隧道的域名，配置保存后立即生效，无需重启。

HTTP 网关根据 `Host` 分流，HTTPS 网关根据 SNI 分流，默认监听 TCP `8080`。公网应直接开放该端口，或把公网 `80/443` 转发到 `8080`。HTTPS 隧道目前是 SNI 透传，因此客户端侧的目标 HTTPS 服务仍需提供与自动分配域名匹配的证书；控制台通过 HTTP-01 申请的单域名证书不会自动成为通配符隧道证书。

Server 是后台常驻服务，不需要每次手动运行二进制。Linux 命令行 Agent 的完整配置见第 9 节，Windows/macOS 桌面客户端见第 11 节。

## 4. 从源码构建

### 4.1 构建当前平台

```bash
go build -trimpath -o nrynet-server ./server
go build -trimpath -ldflags "-X main.version=1.0.0" -o nrynet-client ./client
go build -trimpath -o nrynet-relay ./relay
```

Windows 输出文件名可加 `.exe`。

### 4.2 批量交叉构建

PowerShell：

```powershell
.\scripts\build.ps1 -Version 1.0.0
```

Linux/macOS：

```bash
VERSION=1.0.0 ./scripts/build.sh
```

产物位于 `bin/<版本>-<系统>-<架构>/`。每个目标目录包含 Server、Client、Relay，以及面向网络部署和本地验证的两份配置示例。

## 5. 本地验证部署

本地验证只监听 `127.0.0.1`，不需要 TLS：

```powershell
Copy-Item config.local.example.yaml config.yaml
go run ./server -config config.yaml
```

首次启动会在终端输出：

- 管理员用户名
- 一次性管理员密码
- Server Secret

打开 `http://127.0.0.1:7000` 登录 Dashboard。创建 Agent Token 后，将 Token 填入 `config.yaml` 的 `client.token`，再启动 Agent：

```powershell
go run ./client -config config.yaml
```

本地验证配置不能直接用于公网部署。

## 6. 生产配置

以下示例是新安装的默认 HTTP/WS 配置。域名证书签发后，Dashboard 会把 TLS 配置作为运行时覆盖保存并热加载：

```yaml
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
  plain_enabled: false
  plain_listen: "0.0.0.0:7004"
  plain_data_listen: "0.0.0.0:7005"
  public_data_address: "nat.example.com:7001"
  quic_listen: "0.0.0.0:7002"
  public_quic_address: "nat.example.com:7002"
  rendezvous_listen: "0.0.0.0:7003"
  public_rendezvous_address: "nat.example.com:7003"
  http_listen: "0.0.0.0:8080"
  database: "/opt/nrynet/data/nrynet.db"
  log_directory: "/opt/nrynet/logs"
  jwt_ttl: "12h"
  heartbeat_timeout: "45s"
  relay_api_token: ""
  tls:
    enabled: false
    cert_file: "/opt/nrynet/tls/fullchain.pem"
    key_file: "/opt/nrynet/tls/privkey.pem"
  bootstrap:
    admin_username: "admin"
    admin_password: ""
client:
  server_url: "ws://nat.example.com:7000/agent/connect"
  data_address: "nat.example.com:7001"
  transport: "websocket"
  quic_address: "nat.example.com:7002"
  rendezvous_address: "nat.example.com:7003"
  ca_file: ""
  token: ""
  name: ""
  device_id: ""
  insecure_skip_verify: false
```

使用受信任 CA 证书时，证书的 DNS SAN 或 IP SAN 必须包含客户端使用的地址。使用安装器生成的自签名证书时，Dashboard 新生成的 Agent Token 会自动携带证书公钥指纹，客户端直接校验公钥，不要求访问地址出现在证书 SAN 中，也无需配置 `ca_file`。旧版两段式 Token 仍可配合 `ca_file` 使用；升级后建议重新生成 Token。公网部署不要启用 `insecure_skip_verify`。

如果通过 `--renew-cert` 或 `-RenewCertificate` 更换了证书密钥，已有 Token 中的旧指纹会拒绝新证书，这是正常的防劫持保护。请在确认 Server 安全后，从 Dashboard 重新生成 Token 并更新各 Agent。使用受信任 CA 证书时，系统证书链仍会正常验证。

Linux Dashboard 的证书申请使用 standalone HTTP-01 挑战，运行前必须确保域名已解析到本机，且公网入站 TCP/80 能到达 Server。普通 `nrynet-server` 进程不会获得 root 权限；它只向 `/opt/nrynet/data/certbot/inbox` 写入经过校验的任务，由 root systemd helper 使用固定参数调用 Certbot。已批准的续期目标和工作状态保存在 root 管理的 `/var/lib/nrynet/certbot`，不会信任普通服务可写的状态。证书以原子替换方式安装到 `/opt/nrynet/tls`，服务每两秒检查任务和证书变化并热加载。`nrynet-certbot-renew.timer` 每日检查续期，不会重启主服务。

## 7. 端口与防火墙

| 端口 | 协议 | 用途 |
| --- | --- | --- |
| 7000 | HTTP/HTTPS、WS/WSS | Dashboard、管理 API、Agent 控制通道；TLS 开启后同端口并存 |
| 7001 | TCP/TLS | 明文与 TLS 数据通道；按握手自动识别 |
| 7002 | UDP/QUIC | QUIC 控制和数据流 |
| 7003 | UDP | P2P Rendezvous 和打洞 |
| 7004 | TCP/WS | 可选旧版兼容 Dashboard、管理 API、Agent 控制通道；后台热启停 |
| 7005 | TCP | 可选旧版兼容明文数据通道；后台热启停 |
| 8080 | TCP | HTTP Host 与 HTTPS SNI 网关 |
| 隧道远程端口 | TCP 或 UDP | 访客访问端口 |

仅向管理网段开放 Dashboard 更安全。隧道端口应按业务需要开放，并在 Dashboard 配置 IP 白名单。

## 8. Linux Server 手动安装

```bash
sudo useradd --system --home /opt/nrynet --shell /usr/sbin/nologin nrynet
sudo install -d -o root -g nrynet -m 0750 /opt/nrynet
sudo install -d -o nrynet -g nrynet -m 0750 /opt/nrynet/data /opt/nrynet/logs
sudo install -d -o root -g nrynet -m 0750 /opt/nrynet/tls
sudo install -o nrynet -g nrynet -m 0755 nrynet-server /opt/nrynet/
sudo install -o nrynet -g nrynet -m 0600 config.yaml /opt/nrynet/
sudo install -o nrynet -g nrynet -m 0644 fullchain.pem /opt/nrynet/tls/
sudo install -o nrynet -g nrynet -m 0600 privkey.pem /opt/nrynet/tls/
sudo install -m 0644 deploy/nrynet-server.service /etc/systemd/system/nrynet-server.service
sudo systemctl daemon-reload
sudo systemctl enable --now nrynet-server
```

查看首次密码和运行日志：

```bash
sudo systemctl status nrynet-server
sudo journalctl -u nrynet-server -n 100 --no-pager
sudo journalctl -u nrynet-server -f
```

首次登录后立即修改管理员密码，并妥善保存 Server Secret。

## 9. Linux 命令行 Agent 安装

在 Dashboard 为每台设备创建独立 Token。准备只包含 `client` 段的配置文件：

```yaml
client:
  server_url: "wss://nat.example.com:7000/agent/connect"
  data_address: "nat.example.com:7001"
  transport: "websocket"
  quic_address: "nat.example.com:7002"
  rendezvous_address: "nat.example.com:7003"
  token: "在此填写 Agent Token"
  name: "office-linux"
  device_id: ""
  ca_file: ""
  insecure_skip_verify: false
```

安装并启动：

```bash
sudo useradd --system --home /opt/nrynet-client --shell /usr/sbin/nologin nrynet 2>/dev/null || true
sudo install -d -o nrynet -g nrynet /opt/nrynet-client/logs
sudo install -o nrynet -g nrynet -m 0755 nrynet-client /opt/nrynet-client/
sudo install -o nrynet -g nrynet -m 0600 config.yaml /opt/nrynet-client/
sudo install -m 0644 deploy/nrynet-client.service /etc/systemd/system/nrynet-client.service
sudo systemctl daemon-reload
sudo systemctl enable --now nrynet-client
sudo journalctl -u nrynet-client -f
```

Agent 首次连接后会生成稳定设备 ID。不要将同一个设备 ID 复制给其他机器。

## 10. Windows Server 手动安装

将文件放在 `C:\ProgramData\Nrynet`：

```powershell
$install = "C:\ProgramData\Nrynet"
New-Item -ItemType Directory -Force "$install\data", "$install\logs", "$install\tls"
Copy-Item .\nrynet-server.exe $install
Copy-Item .\config.yaml $install
Copy-Item .\fullchain.pem, .\privkey.pem "$install\tls"
```

配置中的数据库、日志和证书路径可使用 YAML 单引号，例如 `'C:\ProgramData\Nrynet\data\nrynet.db'`。

使用 Windows 任务计划程序在开机时启动：

```powershell
$action = New-ScheduledTaskAction -Execute "$install\nrynet-server.exe" -Argument "-config `"$install\config.yaml`"" -WorkingDirectory $install
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
Register-ScheduledTask -TaskName "Nrynet Server" -Action $action -Trigger $trigger -Principal $principal -Force
Start-ScheduledTask -TaskName "Nrynet Server"
```

手动部署时防火墙应按需开放端口；一键安装器会额外预留可选的 `7004/7005`，但明文开关关闭时服务不会监听。首次初始化建议先在前台运行一次，以便安全记录管理员密码和 Server Secret。

## 11. Windows/macOS 桌面客户端

Windows 构建：

```powershell
cd desktop
$env:APP_VERSION = "1.0.0"
wails3 build
```

输出文件为 `desktop/bin/nrynet-desktop.exe`。运行后进入“设置 -> 网络设置”和“连接设置”，填写服务器地址及 Agent Token。

早期误用 `NAT-Link` 名称发布的桌面端只识别旧资产名称，无法自动发现正式的 `nrynet-desktop-*` 安装包。该版本需要从 GitHub Release 手动更新一次；换成 Nrynet 桌面端后，后续版本会直接从 GitHub 检查和安装，不需要填写更新地址或密钥。旧配置和开机启动设置会自动兼容迁移。

macOS 应在 macOS 主机上构建：

```bash
cd desktop
APP_VERSION=1.0.0 wails3 build GOOS=darwin
```

正式分发需要使用团队证书进行代码签名和公证。桌面端详细使用说明见 `desktop/README.zh-CN.md`。

## 12. 可选 Relay 节点

仅需要分布式公网入口时部署 Relay。中央 Server 与所有 Relay 必须配置同一个高强度 `relay_api_token`，且不能复用管理员密码或 Agent Token。

```bash
./nrynet-relay \
  -server https://nat.example.com:7000 \
  -id edge-1 \
  -address 203.0.113.10 \
  -control-listen 0.0.0.0:7100 \
  -control-address https://edge.example.com:7100 \
  -bind-host 0.0.0.0 \
  -broker nat.example.com:7001 \
  -broker-tls \
  -broker-server-name nat.example.com \
  -control-tls \
  -control-cert-file /opt/nrynet-relay/tls/fullchain.pem \
  -control-key-file /opt/nrynet-relay/tls/privkey.pem \
  -token "$RELAY_API_TOKEN"
```

私有 CA 可通过 `-broker-ca-file` 指定。Relay 的 7100 控制端口只应允许中央 Server 访问。

## 13. 升级、备份与回滚

使用一键安装的环境，升级到最新正式版只需重新运行安装命令：

```bash
sudo ./install-server.sh --public-host nat.example.com
```

安装器会校验软件包摘要和版本，在停止服务后替换二进制，并在升级完成后恢复服务。下面的步骤用于手动升级或回滚。

升级前备份：

```bash
sudo systemctl stop nrynet-server
sudo tar -czf nrynet-backup.tgz /opt/nrynet/config.yaml /opt/nrynet/data /opt/nrynet/tls
sudo systemctl start nrynet-server
```

升级二进制：

```bash
sudo systemctl stop nrynet-server
sudo install -o nrynet -g nrynet -m 0755 nrynet-server /opt/nrynet/nrynet-server
sudo systemctl start nrynet-server
sudo journalctl -u nrynet-server -n 100 --no-pager
```

回滚时停止服务，恢复旧二进制和备份数据库，再启动服务。复制 SQLite 数据库前必须停止 Server，或使用 SQLite 在线备份工具。

## 14. 常见部署问题

### 服务启动后立即退出

执行 `journalctl -u nrynet-server -n 100`，重点检查端口冲突、配置语法、数据库目录权限和证书文件权限。

### 公网 Agent 无法连接

确认使用 `wss://`、证书域名匹配、7000/7001 防火墙已开放，且 `public_data_address` 是 Agent 可访问的公网地址。

### Dashboard 可访问但隧道不通

检查隧道状态、Client 在线状态、本地目标地址、访客端口防火墙和 IP 白名单。HTTP/HTTPS 域名还必须正确解析到 Server。

### QUIC 或 P2P 不工作

确认 UDP 7002/7003 双向开放，NAT/安全组没有仅开放 TCP，并检查 `public_quic_address` 和 `public_rendezvous_address`。

### 桌面客户端更新失败

检查 `APP_VERSION`、GitHub Release 是否发布成功，以及对应桌面软件包是否出现在 `SHA256SUMS` 中。客户端不需要填写更新地址或公钥，每 6 小时自动检查一次正式版本。
