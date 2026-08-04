package app

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/nrytex/nrynet/server/certbothelper"
)

const maxCertificateFailureRunes = 1200

func certificateFailure(job certbothelper.Status) (string, string) {
	details := certificateFailureDetails(job)
	search := strings.ToLower(job.Message + "\n" + job.Output)
	switch {
	case containsAny(search, "executable file not found", "certbot: not found", "no such file or directory: certbot"):
		return "Certbot helper 找不到 certbot 命令，请重新运行最新版安装脚本", details
	case containsAny(search, "address already in use", "could not bind tcp port 80", "problem binding to port 80"):
		return "TCP 80 已被其他进程占用，请停止占用端口的服务后重试", details
	case containsAny(search, "read-only file system", "permission denied", "operation not permitted"):
		return "Certbot helper 权限或 systemd 沙箱配置异常，请重新运行最新版安装脚本", details
	case containsAny(search, "too many certificates", "rate limit", "too many requests"):
		return "Let's Encrypt 已触发签发频率限制，请按详细信息提示稍后重试", details
	case containsAny(search, "unauthorized", "invalid response", "challenge failed"):
		return "Let's Encrypt 未能完成 HTTP-01 验证，请检查详细信息中的访问地址和响应", details
	case details != "":
		return "证书申请失败，Certbot helper 已返回详细诊断", details
	default:
		return "证书申请失败，请检查 Certbot helper 服务日志", "sudo journalctl -u nrynet-certbot.service -n 100 --no-pager"
	}
}

func certificateFailureDetails(job certbothelper.Status) string {
	details := strings.TrimSpace(job.Output)
	if details == "" {
		details = strings.TrimSpace(job.Message)
	}
	details = strings.Map(func(char rune) rune {
		if char == '\n' || char == '\t' || !unicode.IsControl(char) {
			return char
		}
		return -1
	}, details)
	if utf8.RuneCountInString(details) <= maxCertificateFailureRunes {
		return details
	}
	runes := []rune(details)
	return "..." + string(runes[len(runes)-maxCertificateFailureRunes:])
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
