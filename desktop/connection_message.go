package main

import "strings"

func connectionErrorMessage(err error) string {
	if err == nil {
		return "连接已中断，客户端正在尝试重新连接。"
	}
	message := strings.ToLower(err.Error())
	for _, rule := range connectionErrorRules {
		if strings.Contains(message, rule.match) {
			return rule.message
		}
	}
	return "无法连接到服务器，请检查服务器地址、网络连接和服务端运行状态。"
}

var connectionErrorRules = []struct {
	match   string
	message string
}{
	{"client.server_url is required", "尚未配置控制服务器，请前往“设置 > 网络设置”填写后重试。"},
	{"client.server_url must be a valid", "控制服务器地址格式不正确，请填写完整的 ws:// 或 wss:// 地址。"},
	{"client.data_address is required", "尚未配置数据通道，请前往“设置 > 网络设置”填写后重试。"},
	{"client.quic_address is required", "已选择 QUIC 传输，但尚未填写 QUIC 地址。"},
	{"client.token is required", "尚未配置 Agent Token，请前往“设置 > 连接设置”填写后重试。"},
	{"client.transport must be", "传输协议配置无效，请在网络设置中选择 WebSocket 或 QUIC。"},
	{"remote agent connections require wss", "当前核心仍拒绝该控制地址。Nrynet 桌面端支持 ws:// 和 wss://，请确认服务端与核心配置已更新后重试。"},
	{"insecure_skip_verify", "TLS 证书校验跳过设置无效，请关闭该选项或改用 wss:// 后重试。"},
	{"agent token is invalid or disabled", "Agent Token 无效或已停用，请在服务端重新生成后更新连接设置。"},
	{"invalid agent token", "Agent Token 无效，请检查是否复制完整或在服务端重新生成。"},
	{"bound to another token", "当前设备身份已绑定其他 Token，请联系管理员重置设备凭证。"},
	{"has been revoked", "当前设备已被服务端撤销，请联系管理员重新授权。"},
	{"bad handshake", "服务器拒绝了连接，请检查 Agent Token、控制服务器地址和服务端状态。"},
	{"certificate pin does not match", "服务器证书已发生变化。为确保连接安全，请确认服务端未被替换，然后在 Dashboard 重新生成 Agent Token。"},
	{"unknown authority", "无法验证服务器证书。请在新版 Nrynet Dashboard 重新生成 Agent Token，新 Token 无需配置 CA 文件。"},
	{"expired or not yet valid", "服务器证书已过期或尚未生效，请检查服务端证书和系统时间。"},
	{"certificate has expired", "服务器证书已过期，请联系管理员更新证书。"},
	{"certificate is valid for", "服务器证书与访问地址不匹配，请检查服务器域名或证书配置。"},
	{"no such host", "找不到服务器域名，请检查地址拼写和 DNS 网络设置。"},
	{"connection refused", "服务器拒绝连接，请确认服务端已启动且端口可访问。"},
	{"actively refused it", "服务器拒绝连接，请确认服务端已启动且端口可访问。"},
	{"quic control unavailable", "QUIC 控制通道暂不可用，客户端将自动改用 WebSocket 重连；请确认服务端 TCP 7000 可访问。"},
	{"read quic control frame", "QUIC 控制连接已断开，客户端正在自动改用 WebSocket 重连。"},
	{"write quic control frame", "QUIC 控制连接已断开，客户端正在自动改用 WebSocket 重连。"},
	{"read control message", "控制连接已断开，客户端正在自动重连。"},
	{"send heartbeat", "控制连接心跳失败，客户端正在自动重连。"},
	{"i/o timeout", "连接服务器超时，请检查网络、防火墙和服务器端口。"},
	{"deadline exceeded", "连接服务器超时，请检查网络、防火墙和服务器端口。"},
}
