package app

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/nrytex/nrynet/server/api"
	"github.com/nrytex/nrynet/server/certbothelper"
)

const certificatePollInterval = 2 * time.Second
const certificateJobTimeout = 5 * time.Minute

func (c *TransportController) RequestCertificate(_ context.Context, request api.CertificateRequest) (api.TransportStatus, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.certbotAvailableLocked() {
		return c.statusLocked(), errors.New("certbot helper is not available")
	}
	if status, err := certbothelper.ReadStatusWithOptions(c.certbot); err == nil &&
		(status.State == "pending" || status.State == "running") && time.Since(status.Updated) < certificateJobTimeout {
		return c.statusLocked(), errors.New("certbot certificate request is already pending")
	}
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(request.Domain)), ".")
	email := strings.TrimSpace(request.Email)
	job := certbothelper.Request{Action: "issue", Domain: domain, Email: email}
	submitted := time.Now()
	if err := certbothelper.EnqueueWithOptions(job, c.certbot); err != nil {
		return c.statusLocked(), err
	}
	c.pendingCertificate = &api.CertificateStatus{Domain: domain, Email: email, Status: "pending"}
	c.pendingSince = submitted
	return c.statusLocked(), nil
}

func (c *TransportController) monitorCertificates(stop <-chan struct{}) {
	ticker := time.NewTicker(certificatePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			c.applyCertificateUpdates()
		}
	}
}

func (c *TransportController) applyCertificateUpdates() {
	c.mu.Lock()
	defer c.mu.Unlock()
	status, err := certbothelper.ReadStatusWithOptions(c.certbot)
	if err == nil && status.State == "success" && status.Updated.After(c.lastJob) {
		activate := status.Action == "issue" || c.tlsStore.Enabled()
		if c.reloadCertificateJobLocked(status.CertFile, status.KeyFile, status.Domain, status.Email, activate) == nil {
			c.lastJob = status.Updated
		}
	}
	c.reloadChangedCertificateLocked()
}

func (c *TransportController) reloadCertificateJobLocked(certFile, keyFile, domain, email string, activate bool) error {
	if err := c.tlsStore.LoadX509KeyPair(certFile, keyFile); err != nil {
		return err
	}
	if err := c.tlsStore.SetEnabled(activate); err != nil {
		return err
	}
	c.app.config.Server.TLS.Enabled = activate
	c.app.config.Server.TLS.CertFile = certFile
	c.app.config.Server.TLS.KeyFile = keyFile
	if domain != "" {
		c.app.config.Server.TLS.Domain = domain
	}
	if email != "" {
		c.app.config.Server.TLS.Email = email
	}
	if info, err := os.Stat(certFile); err == nil {
		c.certTime = info.ModTime()
	}
	settings := map[string]string{
		"server.tls.enabled": boolText(activate), "server.tls.cert_file": certFile,
		"server.tls.key_file": keyFile, "server.tls.domain": c.app.config.Server.TLS.Domain,
		"server.tls.email": c.app.config.Server.TLS.Email,
	}
	for key, value := range settings {
		if err := c.persistSetting(key, value); err != nil {
			return err
		}
	}
	if err := c.persistSetting("server.tls.certbot_applied_at", time.Now().Format(time.RFC3339Nano)); err != nil {
		return err
	}
	return nil
}

func (c *TransportController) reloadChangedCertificateLocked() {
	certFile := c.app.config.Server.TLS.CertFile
	keyFile := c.app.config.Server.TLS.KeyFile
	if certFile == "" || keyFile == "" {
		return
	}
	info, err := os.Stat(certFile)
	if err != nil || !info.ModTime().After(c.certTime) {
		return
	}
	if c.tlsStore.LoadX509KeyPair(certFile, keyFile) == nil {
		c.certTime = info.ModTime()
	}
}

func (c *TransportController) certbotStatusLocked() api.CertbotStatus {
	if runtime.GOOS != "linux" {
		return api.CertbotStatus{Available: false, Message: "当前平台不支持自动 Certbot 证书申请"}
	}
	if !c.certbotAvailableLocked() {
		return api.CertbotStatus{Available: false, Message: "请先使用最新版安装脚本启用 Certbot helper"}
	}
	return api.CertbotStatus{Available: true, Version: "systemd helper"}
}

func (c *TransportController) certbotAvailableLocked() bool {
	info, err := os.Stat(c.certbot.ReadyPath)
	return err == nil && info.Mode().IsRegular()
}

func (c *TransportController) certificateStatusLocked() *api.CertificateStatus {
	job, jobErr := certbothelper.ReadStatusWithOptions(c.certbot)
	if c.pendingCertificate != nil && (jobErr != nil || !job.Updated.After(c.pendingSince) || job.Domain != c.pendingCertificate.Domain) {
		pending := *c.pendingCertificate
		if time.Since(c.pendingSince) >= certificateJobTimeout {
			pending.Status = "failed"
			pending.Error = "证书任务未能启动，请检查 nrynet-certbot.path 和 helper 服务"
		}
		return &pending
	}
	if c.pendingCertificate != nil {
		c.pendingCertificate = nil
		c.pendingSince = time.Time{}
	}
	if jobErr == nil && (job.State == "pending" || job.State == "running" || job.State == "failed") {
		status := &api.CertificateStatus{Domain: job.Domain, Email: job.Email, Status: job.State}
		if (job.State == "pending" || job.State == "running") && time.Since(job.Updated) >= certificateJobTimeout {
			status.Status = "failed"
			status.Error = "证书任务未能启动，请检查 nrynet-certbot.path 和 helper 服务"
			return status
		}
		if job.State == "failed" {
			status.Error, status.Details = certificateFailure(job)
		}
		return status
	}
	tlsConfig := c.app.config.Server.TLS
	return parseCertificateStatus(tlsConfig.CertFile, tlsConfig.Domain, tlsConfig.Email)
}

func parseCertificateStatus(certFile, domain, email string) *api.CertificateStatus {
	data, err := os.ReadFile(certFile)
	if err != nil {
		return nil
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return &api.CertificateStatus{Domain: domain, Email: email, Status: "invalid", Error: "证书 PEM 格式无效"}
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return &api.CertificateStatus{Domain: domain, Email: email, Status: "invalid", Error: "证书无法解析"}
	}
	if domain == "" && len(cert.DNSNames) > 0 {
		domain = cert.DNSNames[0]
	}
	return &api.CertificateStatus{
		Domain: domain, Email: email, Issuer: cert.Issuer.CommonName,
		NotAfter: cert.NotAfter.Format(time.RFC3339), Status: "success",
	}
}

func (c *TransportController) persistSetting(key, value string) error {
	return c.app.store.SetSetting(context.Background(), "config."+key, value)
}

func boolText(value bool) string {
	return strconv.FormatBool(value)
}

func installDirFromDatabase(database string) string {
	clean := filepath.Clean(database)
	parent := filepath.Dir(clean)
	if filepath.Base(parent) == "data" {
		return filepath.Dir(parent)
	}
	return filepath.Dir(clean)
}

func publicAddress(listen, publicData, domain string) string {
	_, port, err := net.SplitHostPort(listen)
	if err != nil {
		return listen
	}
	host, _, err := net.SplitHostPort(publicData)
	if domain != "" {
		host = domain
	} else if err != nil || host == "" {
		host = "127.0.0.1"
	}
	return net.JoinHostPort(host, port)
}
