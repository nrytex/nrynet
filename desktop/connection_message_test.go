package main

import (
	"errors"
	"strings"
	"testing"
)

func TestConnectionErrorMessageExplainsCommonFailures(t *testing.T) {
	tests := []struct {
		name string
		err  string
		want string
	}{
		{"missing server", "client.server_url is required", "尚未配置控制服务器"},
		{"invalid token", "server error: agent token is invalid or disabled", "Agent Token 无效或已停用"},
		{"certificate", "tls: failed to verify certificate: x509: certificate signed by unknown authority", "无法验证服务器证书"},
		{"refused", "dial tcp 127.0.0.1:7000: connectex: No connection could be made because the target machine actively refused it", "服务器拒绝连接"},
		{"fallback", "unexpected transport failure", "无法连接到服务器"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := connectionErrorMessage(errors.New(test.err))
			if !strings.Contains(got, test.want) {
				t.Fatalf("message %q does not contain %q", got, test.want)
			}
		})
	}
}
