package app

import (
	"strings"
	"testing"

	"github.com/nrytex/nrynet/server/certbothelper"
)

func TestCertificateFailureExplainsCommonHelperErrors(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		message string
	}{
		{name: "missing command", output: `exec: "certbot": executable file not found in $PATH`, message: "找不到 certbot"},
		{name: "port conflict", output: "Could not bind TCP port 80 because it is already in use", message: "TCP 80 已被其他进程占用"},
		{name: "challenge", output: "Challenge failed for domain; unauthorized: Invalid response", message: "未能完成 HTTP-01 验证"},
		{name: "permission", output: "Permission denied: /etc/letsencrypt", message: "权限或 systemd 沙箱配置异常"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, details := certificateFailure(certbothelper.Status{State: "failed", Output: test.output})
			if !strings.Contains(message, test.message) {
				t.Fatalf("message=%q, want substring %q", message, test.message)
			}
			if details != test.output {
				t.Fatalf("details=%q, want %q", details, test.output)
			}
		})
	}
}

func TestCertificateFailureDetailsAreBounded(t *testing.T) {
	output := strings.Repeat("错误", maxCertificateFailureRunes)
	_, details := certificateFailure(certbothelper.Status{State: "failed", Output: output})
	if !strings.HasPrefix(details, "...") || len([]rune(details)) != maxCertificateFailureRunes+3 {
		t.Fatalf("unexpected bounded details length: %d", len([]rune(details)))
	}
}
