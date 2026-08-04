package certbothelper

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	return command.CombinedOutput()
}

func RunHelper(ctx context.Context, runner CommandRunner) error {
	return RunHelperWithOptions(ctx, runner, DefaultOptions())
}

func RunHelperWithOptions(ctx context.Context, runner CommandRunner, options Options) error {
	if runner == nil {
		runner = ExecRunner{}
	}
	unlock, err := lockRequest(options.LockPath)
	if err != nil {
		return err
	}
	defer unlock()
	request, err := readRequest(options.RequestPath)
	if err != nil {
		_ = writePrivilegedStatus(options, Status{State: "failed", Message: err.Error(), Updated: time.Now()})
		return err
	}
	if err := ValidateRequest(request); err != nil {
		_ = writePrivilegedStatus(options, statusFromRequest(request, "failed", err.Error(), ""))
		return err
	}
	if request.Action == "renew" {
		request, err = renewRequestFromManagedState(options)
		if err != nil {
			_ = writePrivilegedStatus(options, Status{Action: "renew", State: "failed", Message: err.Error(), Updated: time.Now()})
			return err
		}
	}
	_ = writePrivilegedStatus(options, statusFromRequest(request, "running", "Certbot is running", ""))
	output, err := runCertbot(ctx, runner, request, options)
	if err != nil {
		err = fmt.Errorf("certbot failed: %w: %s", err, truncate(output))
	}
	if err == nil {
		err = installCertificate(request.Domain, options)
	}
	if err != nil {
		_ = writePrivilegedStatus(options, statusFromRequest(request, "failed", err.Error(), truncate(output)))
		return err
	}
	if request.Action == "issue" || request.Action == "renew" {
		if err := writeManagedState(options, request); err != nil {
			_ = writePrivilegedStatus(options, statusFromRequest(request, "failed", err.Error(), truncate(output)))
			return err
		}
	}
	return writePrivilegedStatus(options, Status{
		Action: request.Action, Domain: request.Domain, Email: request.Email, State: "success",
		Output: truncate(output), Updated: time.Now(),
		CertFile: options.FullchainPath, KeyFile: options.PrivateKeyPath,
	})
}

func RunRenewWithOptions(ctx context.Context, runner CommandRunner, options Options) error {
	if runner == nil {
		runner = ExecRunner{}
	}
	request, err := renewRequestFromManagedState(options)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	unlock, err := lockRequest(options.LockPath)
	if err != nil {
		return err
	}
	defer unlock()
	output, err := runCertbot(ctx, runner, request, options)
	if err != nil {
		err = fmt.Errorf("certbot renewal failed: %w: %s", err, truncate(output))
	}
	if err == nil {
		err = installCertificate(request.Domain, options)
	}
	if err != nil {
		_ = writePrivilegedStatus(options, statusFromRequest(request, "failed", err.Error(), truncate(output)))
		return err
	}
	return writePrivilegedStatus(options, Status{
		Action: "renew", Domain: request.Domain, Email: request.Email, State: "success",
		Output: truncate(output), Updated: time.Now(),
		CertFile: options.FullchainPath, KeyFile: options.PrivateKeyPath,
	})
}

func writePrivilegedStatus(options Options, status Status) error {
	if err := writeStatus(options.StatusPath, status); err != nil {
		return err
	}
	return chownStatus(options)
}

func runCertbot(ctx context.Context, runner CommandRunner, request Request, options Options) ([]byte, error) {
	args := []string{"certonly", "--standalone", "--non-interactive", "--agree-tos", "--reuse-key",
		"--preferred-challenges", "http", "--http-01-port", "80",
		"--config-dir", options.LetsEncryptDir, "--work-dir", options.WorkDir, "--logs-dir", options.LogsDir}
	if request.Staging {
		args = append(args, "--staging")
	}
	if request.Action == "renew" {
		args[0] = "renew"
		if request.Domain != "" {
			args = append(args, "--cert-name", request.Domain)
		}
		return runner.Run(ctx, "certbot", args...)
	}
	args = append(args, "--cert-name", request.Domain, "-d", request.Domain, "-m", request.Email)
	return runner.Run(ctx, "certbot", args...)
}

func installCertificate(domain string, options Options) error {
	lineage := filepath.Join(options.LetsEncryptDir, "live", domain)
	sourceCert := filepath.Join(lineage, "fullchain.pem")
	sourceKey := filepath.Join(lineage, "privkey.pem")
	if err := copyAtomic(sourceCert, options.FullchainPath, 0o644); err != nil {
		return fmt.Errorf("copy fullchain: %w", err)
	}
	if err := copyAtomic(sourceKey, options.PrivateKeyPath, 0o640); err != nil {
		return fmt.Errorf("copy private key: %w", err)
	}
	return chownTargets(options)
}

func readRequest(path string) (Request, error) {
	file, err := openRequestFile(path)
	if err != nil {
		return Request{}, fmt.Errorf("read request: %w", err)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 65537))
	if err != nil {
		return Request{}, fmt.Errorf("read request: %w", err)
	}
	if len(data) > 65536 {
		return Request{}, fmt.Errorf("read request: file is too large")
	}
	var request Request
	if err := json.Unmarshal(data, &request); err != nil {
		return Request{}, fmt.Errorf("parse request: %w", err)
	}
	request.Domain = strings.TrimSpace(strings.TrimSuffix(strings.ToLower(request.Domain), "."))
	request.Email = strings.TrimSpace(request.Email)
	return request, nil
}

func writeStatus(path string, status Status) error {
	if status.Updated.IsZero() {
		status.Updated = time.Now()
	}
	data, err := json.MarshalIndent(status, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	return writeAtomic(path, append(data, '\n'), defaultStatusMode)
}

func statusFromRequest(request Request, state, message, output string) Status {
	return Status{Action: request.Action, Domain: request.Domain, Email: request.Email, State: state, Message: message, Output: output, Updated: time.Now()}
}

func copyAtomic(source, target string, mode os.FileMode) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(target), ".nrynet-cert-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer func() { _ = os.Remove(tempName) }()
	if _, err := temp.Write(data); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Chmod(mode); err != nil {
		_ = temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempName, target); err != nil {
		return err
	}
	return syncDir(filepath.Dir(target))
}

func truncate(output []byte) string {
	if len(output) <= maxOutputBytes {
		return string(bytes.TrimSpace(output))
	}
	return string(bytes.TrimSpace(output[len(output)-maxOutputBytes:]))
}
