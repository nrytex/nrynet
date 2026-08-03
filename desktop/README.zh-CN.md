# Nrynet 桌面客户端说明

Nrynet Desktop 是基于 Wails v3、Go 和 React 构建的 Windows/macOS 客户端。桌面端复用项目中的真实 `client/agent` 运行时，支持连接管理、隧道查看、实时流量、运行日志、托盘运行、开机启动和安全自动更新。

[English](README.md)

服务端、命令行 Agent 和桌面端的完整安装步骤见[中文安装部署指南](../docs/deployment.zh-CN.md)。

## 界面说明

### 首页

- **连接状态**：显示当前是否已连接、连接时长和客户端状态。
- **实时指标**：显示已分配隧道数、上传速率、下载速率和累计流量。
- **流量趋势**：根据客户端实际传输字节增量绘制实时上传趋势。
- **我的隧道**：展示服务端分配给当前设备的隧道。点击隧道名称可进入详情页，复制按钮用于复制外部访问地址。
- **开机启动**：显示当前开机启动状态；点击后进入常规设置进行修改。

隧道由服务端管理员在 Dashboard 中创建、启停和分配，桌面客户端不会在本地创建未经授权的隧道。

### 隧道详情

详情页提供以下信息：

- 本地目标地址和端口
- TCP、UDP、HTTP 或 HTTPS 协议类型
- 远程端口或域名
- 当前运行状态
- 创建时间和运行时长
- 实时上传、下载速率趋势
- IP 白名单等只读配置

### 设置

设置页分为五个区域：

| 区域 | 作用 |
| --- | --- |
| 常规设置 | 开机启动和 GitHub 自动更新状态 |
| 网络设置 | 控制服务器、数据通道、传输协议和 QUIC |
| 连接设置 | 设备名称、设备 ID 和 Agent Token |
| 运行日志 | 查看桌面客户端最近的运行记录 |
| 关于 Nrynet | 查看版本、连接状态并手动检查更新 |

修改设置后必须点击“保存设置”。连接期间修改网络或身份配置后，建议先断开再重新连接。

## 首次连接

1. 在 Nrynet Dashboard 的 Token 页面创建 Agent Token。
2. 打开桌面客户端，进入“设置 -> 网络设置”。
3. 填写控制服务器和数据通道地址。
4. 进入“连接设置”，填写设备名称并粘贴 Agent Token。
5. 保存设置，返回首页并点击“立即连接”。
6. 连接成功后，服务端分配的隧道会自动同步到首页。

公网部署必须使用加密连接。控制地址应使用 `wss://`，数据通道和 QUIC 地址必须与服务端保持一致。使用安装器生成的自签名证书时，新版 Agent Token 会携带服务器证书公钥指纹，桌面端自动校验；使用受信任证书时由系统证书库校验。两种方式都不需要单独下载或配置 CA 文件。

## 配置字段

| 字段 | 示例 | 说明 |
| --- | --- | --- |
| 控制服务器 | `wss://nat.example.com:7000/agent/connect` | Agent WebSocket 控制通道 |
| 数据通道 | `nat.example.com:7001` | TCP/HTTP 数据连接地址 |
| 传输协议 | `WebSocket` 或 `QUIC` | 控制通道传输方式 |
| QUIC 地址 | `nat.example.com:7002` | QUIC 控制和数据地址 |
| 设备名称 | `Office-PC` | Dashboard 中显示的客户端名称 |
| 设备 ID | 自动生成 | 设备稳定身份，不应复制给其他设备 |
| Agent Token | 服务端生成 | Agent 鉴权凭证，禁止公开或提交到仓库 |

## 托盘和开机启动

点击主窗口右上角关闭按钮时，客户端会隐藏到系统托盘并继续运行，不会退出或主动断开连接。需要彻底退出时，请使用托盘菜单中的“退出”。系统托盘菜单支持：

- 显示主窗口
- 连接或断开
- 退出应用

开机启动的实现方式：

- Windows：当前用户的 `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`
- macOS：当前用户 LaunchAgent

## 自动更新

桌面端使用 Wails Updater 直接检查 `nrytex/nrynet` 的 GitHub Release，不需要用户填写更新地址或密钥。更新包必须通过：

- 版本比较和防降级检查
- SHA-256 摘要校验
- Release 中 `SHA256SUMS` 的文件摘要校验

客户端每 6 小时自动检查一次最新正式版本；“关于 Nrynet”页面仍可手动检查。生产构建必须通过 `APP_VERSION` 注入真实版本号，否则更新比较将不可靠。

早期误用 `NAT-Link` 名称发布的桌面端只识别旧资产名称，需要从 GitHub Release 手动安装一次 `nrynet-desktop-*`。完成这次名称迁移后，后续更新可继续自动完成，原有配置和开机启动设置会被兼容读取。

## 本地开发

```powershell
cd desktop
go mod tidy
cd frontend
npm install
npm run build
cd ..
wails3 dev
```

仅预览 React 界面：

```powershell
cd desktop/frontend
npm run dev
```

浏览器开发模式使用本地预览数据；正式 Wails 构建始终调用真实桌面服务。

## 测试与构建

```powershell
cd desktop
go test ./...
cd frontend
npm test -- --run
npm run build
cd ..
$env:APP_VERSION = "1.0.0"
wails3 build
```

Windows 输出文件默认为 `desktop/bin/nrynet-desktop.exe`。macOS 正式包应在 macOS 主机上使用相同 `APP_VERSION` 构建，并同步维护 `build/config.yml` 中的原生包版本。

## 常见问题

### 一直显示“未连接”

检查控制服务器地址、Agent Token、系统时间、TLS 证书和防火墙。服务端日志中会记录鉴权失败或传输连接错误。

### 已连接但没有隧道

桌面端只显示服务端已分配给该 Client 的隧道。请在 Dashboard 中确认隧道的 Client、状态和协议配置。

### 证书连接失败

确认桌面端使用的是新版 Dashboard 生成的完整 Agent Token，并确保证书中的域名或 IP 与控制服务器地址一致。升级旧 Server 后需要重新生成一次 Token。若 Server 刚执行过证书续期，请确认服务端安全后重新生成 Token；桌面端不需要配置 CA 文件。

### 重置 Token 后无法连接

在 Dashboard 重置 Client Token 后，旧会话会被断开。必须将新 Token 保存到桌面客户端，再重新连接。
