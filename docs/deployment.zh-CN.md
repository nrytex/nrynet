# NAT-Link 安装部署指南

本文说明 NAT-Link Server、命令行 Agent、桌面客户端和可选 Relay 节点的安装与生产部署。示例以源码仓库根目录为起点，覆盖 Linux、Windows 和 macOS；根据项目要求，本文不使用 Docker。

## 1. 组件与部署位置

| 组件 | 推荐部署位置 | 作用 |
| --- | --- | --- |
| `nat-link-server` | 具有公网地址的 Linux/Windows 主机 | Dashboard、管理 API、控制通道、数据转发和 SQLite |
| `nat-link-client` | 需要暴露内网服务的主机 | 命令行 Agent，连接内网本地服务 |
| `nat-linkdesktop` | Windows/macOS 用户桌面 | 带图形界面的 Agent |
| `nat-link-relay` | 可选的边缘公网节点 | 分布式承载公网隧道端口 |

Server 和 Agent 可以安装在同一台机器用于本地验证，但生产环境通常分别部署。

## 2. 环境要求

- Go 1.25 或项目 `go.mod` 指定的兼容版本
- Node.js 和 npm，仅构建 Dashboard/桌面前端时需要
- Wails v3，仅构建桌面客户端时需要
- 一个解析到 Server 公网地址的域名
- 受信任的 TLS 证书和私钥
- Linux 使用 systemd；Windows 使用系统服务保持后台运行

生产环境不要公开使用明文 `ws://`。NAT-Link 会拒绝绑定到非回环地址的明文控制和数据监听器。

## 3. 一键安装 Server

Release 提供 Linux 和 Windows 一键安装脚本。脚本会下载对应架构的软件包、校验 SHA-256、调用 OpenSSL 生成自签名 TLS 证书、写入 `0.0.0.0` 监听配置，并注册系统服务。整个流程不使用 Docker。

### 3.1 Linux

适用于使用 systemd 的 Linux amd64/arm64 发行版：

```bash
curl -fLO https://github.com/nrytex/nrynet/releases/latest/download/install-server.sh
chmod +x install-server.sh
sudo ./install-server.sh --public-host nat.example.com
```

脚本会自动安装缺少的 `curl`、`openssl`、`tar` 和校验工具，安装目录默认为 `/opt/nat-link`，服务名为 `nat-link-server`。指定版本或重新生成证书：

```bash
sudo ./install-server.sh --version 1.0.0 --public-host nat.example.com --renew-cert
```

再次执行同一个脚本即可升级。安装器会读取已安装 Server 的版本，和 Release 软件包内的 `VERSION` 比较：相同版本不会重复替换二进制，只允许正常升级，并保留现有数据库、配置和证书。需要明确回滚时才使用 `--allow-downgrade`；Windows 对应参数为 `-AllowDowngrade`。

安装脚本不只是复制文件。它会创建 `/etc/systemd/system/nat-link-server.service`，执行 `daemon-reload`，设置开机启动并立即启动 Server。安装后可直接管理服务：

```bash
sudo systemctl status nat-link-server
sudo systemctl restart nat-link-server
sudo journalctl -u nat-link-server -f
```

### 3.2 Windows

使用管理员身份打开 PowerShell：

```powershell
Invoke-WebRequest https://github.com/nrytex/nrynet/releases/latest/download/install-server.ps1 -OutFile install-server.ps1
Set-ExecutionPolicy -Scope Process Bypass
.\install-server.ps1 -PublicHost nat.example.com
```

脚本会在缺少 OpenSSL 时通过 `winget` 安装，注册 `NATLinkServer` Windows 服务，并开放 TCP `7000/7001/8080` 和 UDP `7002/7003`。使用 `-SkipFirewall` 可跳过防火墙规则。

首次安装完成后，终端会显示一次性管理员密码。脚本随后会从配置文件中移除明文密码。自动生成的是自签名证书，必须将输出位置的 `fullchain.pem` 安全复制到每台 Agent，并在命令行或桌面客户端中配置为私有 CA。浏览器访问 Dashboard 时也需要信任该证书；正式公网服务可替换为受信任 CA 签发的同名证书。

Windows 服务同样会设置为自动启动并立即运行：

```powershell
Get-Service NATLinkServer
Restart-Service NATLinkServer
Get-WinEvent -LogName Application -MaxEvents 50
```

### 3.3 安装完成后开始使用

1. 在浏览器打开 `https://<public-host>:7000`。使用自签名证书时，先将安装器输出的 `fullchain.pem` 导入受信任根证书，或按浏览器提示确认该证书。
2. 使用安装器终端输出的管理员用户名和一次性密码登录 Dashboard。
3. 在“访问令牌”页面创建 Agent Token。不要把管理员密码当作 Agent Token 使用。
4. 在需要暴露内网服务的设备上安装命令行 Agent 或桌面客户端，填写 `wss://<public-host>:7000/agent/connect`、数据地址 `<public-host>:7001`、Agent Token 和 `fullchain.pem`。
5. Agent 上线后，在 Dashboard 创建 TCP、UDP、HTTP 或 HTTPS 隧道，选择对应设备和本地服务地址，再启动隧道。

Server 是后台常驻服务，不需要每次手动运行二进制。Linux 命令行 Agent 的完整配置见第 9 节，Windows/macOS 桌面客户端见第 11 节。

## 4. 从源码构建

### 4.1 构建当前平台

```bash
go build -trimpath -o nat-link-server ./server
go build -trimpath -ldflags "-X main.version=1.0.0" -o nat-link-client ./client
go build -trimpath -o nat-link-relay ./relay
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

以下示例假设域名为 `nat.example.com`，证书存放在 `/opt/nat-link/tls/`：

```yaml
server:
  listen: "0.0.0.0:7000"
  data_listen: "0.0.0.0:7001"
  public_data_address: "nat.example.com:7001"
  quic_listen: "0.0.0.0:7002"
  public_quic_address: "nat.example.com:7002"
  rendezvous_listen: "0.0.0.0:7003"
  public_rendezvous_address: "nat.example.com:7003"
  http_listen: "0.0.0.0:8080"
  database: "/opt/nat-link/data/nat-link.db"
  log_directory: "/opt/nat-link/logs"
  jwt_ttl: "12h"
  heartbeat_timeout: "45s"
  relay_api_token: ""
  tls:
    enabled: true
    cert_file: "/opt/nat-link/tls/fullchain.pem"
    key_file: "/opt/nat-link/tls/privkey.pem"
  bootstrap:
    admin_username: "admin"
    admin_password: ""
client:
  server_url: "wss://nat.example.com:7000/agent/connect"
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

证书的 DNS SAN 必须包含客户端使用的域名。使用私有 CA 时，在 Agent 中配置 `ca_file`；公网部署不要启用 `insecure_skip_verify`。

## 7. 端口与防火墙

| 端口 | 协议 | 用途 |
| --- | --- | --- |
| 7000 | TCP/TLS | Dashboard、管理 API、Agent 控制通道 |
| 7001 | TCP/TLS | TCP/HTTP 数据通道 |
| 7002 | UDP/QUIC | QUIC 控制和数据流 |
| 7003 | UDP | P2P Rendezvous 和打洞 |
| 8080 | TCP | HTTP Host 与 HTTPS SNI 网关 |
| 隧道远程端口 | TCP 或 UDP | 访客访问端口 |

仅向管理网段开放 Dashboard 更安全。隧道端口应按业务需要开放，并在 Dashboard 配置 IP 白名单。

## 8. Linux Server 手动安装

```bash
sudo useradd --system --home /opt/nat-link --shell /usr/sbin/nologin nat-link
sudo install -d -o nat-link -g nat-link /opt/nat-link/data /opt/nat-link/logs /opt/nat-link/tls
sudo install -o nat-link -g nat-link -m 0755 nat-link-server /opt/nat-link/
sudo install -o nat-link -g nat-link -m 0600 config.yaml /opt/nat-link/
sudo install -o nat-link -g nat-link -m 0644 fullchain.pem /opt/nat-link/tls/
sudo install -o nat-link -g nat-link -m 0600 privkey.pem /opt/nat-link/tls/
sudo install -m 0644 deploy/nat-link-server.service /etc/systemd/system/nat-link-server.service
sudo systemctl daemon-reload
sudo systemctl enable --now nat-link-server
```

查看首次密码和运行日志：

```bash
sudo systemctl status nat-link-server
sudo journalctl -u nat-link-server -n 100 --no-pager
sudo journalctl -u nat-link-server -f
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
sudo useradd --system --home /opt/nat-link-client --shell /usr/sbin/nologin nat-link 2>/dev/null || true
sudo install -d -o nat-link -g nat-link /opt/nat-link-client/logs
sudo install -o nat-link -g nat-link -m 0755 nat-link-client /opt/nat-link-client/
sudo install -o nat-link -g nat-link -m 0600 config.yaml /opt/nat-link-client/
sudo install -m 0644 deploy/nat-link-client.service /etc/systemd/system/nat-link-client.service
sudo systemctl daemon-reload
sudo systemctl enable --now nat-link-client
sudo journalctl -u nat-link-client -f
```

Agent 首次连接后会生成稳定设备 ID。不要将同一个设备 ID 复制给其他机器。

## 10. Windows Server 手动安装

将文件放在 `C:\ProgramData\NAT-Link`：

```powershell
$install = "C:\ProgramData\NAT-Link"
New-Item -ItemType Directory -Force "$install\data", "$install\logs", "$install\tls"
Copy-Item .\nat-link-server.exe $install
Copy-Item .\config.yaml $install
Copy-Item .\fullchain.pem, .\privkey.pem "$install\tls"
```

配置中的数据库、日志和证书路径可使用 YAML 单引号，例如 `'C:\ProgramData\NAT-Link\data\nat-link.db'`。

使用 Windows 任务计划程序在开机时启动：

```powershell
$action = New-ScheduledTaskAction -Execute "$install\nat-link-server.exe" -Argument "-config `"$install\config.yaml`"" -WorkingDirectory $install
$trigger = New-ScheduledTaskTrigger -AtStartup
$principal = New-ScheduledTaskPrincipal -UserId "SYSTEM" -LogonType ServiceAccount -RunLevel Highest
Register-ScheduledTask -TaskName "NAT-Link Server" -Action $action -Trigger $trigger -Principal $principal -Force
Start-ScheduledTask -TaskName "NAT-Link Server"
```

防火墙只开放实际使用的端口。首次初始化建议先在前台运行一次，以便安全记录管理员密码和 Server Secret。

## 11. Windows/macOS 桌面客户端

Windows 构建：

```powershell
cd desktop
$env:APP_VERSION = "1.0.0"
wails3 build
```

输出文件为 `desktop/bin/nat-linkdesktop.exe`。运行后进入“设置 -> 网络设置”和“连接设置”，填写服务器地址及 Agent Token。

macOS 应在 macOS 主机上构建：

```bash
cd desktop
APP_VERSION=1.0.0 wails3 build GOOS=darwin
```

正式分发需要使用团队证书进行代码签名和公证。桌面端详细使用说明见 `desktop/README.zh-CN.md`。

## 12. 可选 Relay 节点

仅需要分布式公网入口时部署 Relay。中央 Server 与所有 Relay 必须配置同一个高强度 `relay_api_token`，且不能复用管理员密码或 Agent Token。

```bash
./nat-link-relay \
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
  -control-cert-file /opt/nat-link-relay/tls/fullchain.pem \
  -control-key-file /opt/nat-link-relay/tls/privkey.pem \
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
sudo systemctl stop nat-link-server
sudo tar -czf nat-link-backup.tgz /opt/nat-link/config.yaml /opt/nat-link/data /opt/nat-link/tls
sudo systemctl start nat-link-server
```

升级二进制：

```bash
sudo systemctl stop nat-link-server
sudo install -o nat-link -g nat-link -m 0755 nat-link-server /opt/nat-link/nat-link-server
sudo systemctl start nat-link-server
sudo journalctl -u nat-link-server -n 100 --no-pager
```

回滚时停止服务，恢复旧二进制和备份数据库，再启动服务。复制 SQLite 数据库前必须停止 Server，或使用 SQLite 在线备份工具。

## 14. 常见部署问题

### 服务启动后立即退出

执行 `journalctl -u nat-link-server -n 100`，重点检查端口冲突、配置语法、数据库目录权限和证书文件权限。

### 公网 Agent 无法连接

确认使用 `wss://`、证书域名匹配、7000/7001 防火墙已开放，且 `public_data_address` 是 Agent 可访问的公网地址。

### Dashboard 可访问但隧道不通

检查隧道状态、Client 在线状态、本地目标地址、访客端口防火墙和 IP 白名单。HTTP/HTTPS 域名还必须正确解析到 Server。

### QUIC 或 P2P 不工作

确认 UDP 7002/7003 双向开放，NAT/安全组没有仅开放 TCP，并检查 `public_quic_address` 和 `public_rendezvous_address`。

### 桌面客户端更新失败

检查 `APP_VERSION`、GitHub Release 是否发布成功，以及对应桌面软件包是否出现在 `SHA256SUMS` 中。客户端不需要填写更新地址或公钥，每 6 小时自动检查一次正式版本。
