package certbothelper

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Enqueue(request Request) error {
	return EnqueueWithOptions(request, DefaultOptions())
}

func EnqueueWithOptions(request Request, options Options) error {
	request.Domain = normalizeDomain(request.Domain)
	if err := ValidateRequest(request); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(options.RequestPath), 0o750); err != nil {
		return err
	}
	data, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(options.RequestPath, append(data, '\n'), 0o640)
}

func ReadStatus() (Status, error) {
	return ReadStatusWithOptions(DefaultOptions())
}

func ReadStatusWithOptions(options Options) (Status, error) {
	data, err := os.ReadFile(options.StatusPath)
	if err != nil {
		return Status{}, fmt.Errorf("read certbot status: %w", err)
	}
	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		return Status{}, fmt.Errorf("parse certbot status: %w", err)
	}
	return status, nil
}

func normalizeDomain(domain string) string {
	return strings.TrimSpace(strings.TrimSuffix(strings.ToLower(domain), "."))
}

func PendingStatus(domain string) Status {
	return Status{Action: "issue", Domain: domain, State: "pending", Message: "\u8bc1\u4e66\u4efb\u52a1\u5df2\u63d0\u4ea4", Updated: time.Now()}
}
