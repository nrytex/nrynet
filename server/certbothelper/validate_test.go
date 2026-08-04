package certbothelper

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestValidateRequestRejectsUnsafeDomainAndEmail(t *testing.T) {
	valid := Request{Action: "issue", Domain: "nrynet.example.com", Email: "admin@example.com"}
	if err := ValidateRequest(valid); err != nil {
		t.Fatalf("valid request rejected: %v", err)
	}
	invalidDomains := []string{"127.0.0.1", "*.example.com", "-bad.example.com", "bad;rm.example.com", "localhost"}
	for _, domain := range invalidDomains {
		request := valid
		request.Domain = domain
		if err := ValidateRequest(request); err == nil {
			t.Fatalf("unsafe domain accepted: %q", domain)
		}
	}
	request := valid
	request.Email = "admin@example.com;touch /tmp/pwned"
	if err := ValidateRequest(request); err == nil {
		t.Fatal("unsafe email accepted")
	}
	if err := ValidateRequest(Request{Action: "renew", Domain: "attacker.example.com"}); err == nil {
		t.Fatal("renew accepted caller supplied domain")
	}
}

func TestCertbotIssueCommandUsesFixedDirectoriesAndNoShell(t *testing.T) {
	runner := &recordingRunner{}
	request := Request{Action: "issue", Domain: "nrynet.example.com", Email: "admin@example.com"}
	if _, err := runCertbot(context.Background(), runner, request, DefaultOptions()); err != nil {
		t.Fatal(err)
	}
	if runner.name != "certbot" {
		t.Fatalf("command=%q", runner.name)
	}
	required := []string{"certonly", "--standalone", "--non-interactive", "--agree-tos", "--reuse-key",
		"--config-dir", LetsEncryptDir, "--work-dir", CertbotWorkDir, "--logs-dir", CertbotLogsDir,
		"-d", "nrynet.example.com", "-m", "admin@example.com"}
	for _, value := range required {
		if !hasArg(runner.args, value) {
			t.Fatalf("missing arg %q in %#v", value, runner.args)
		}
	}
}

func TestQueueWritesAtomicRequestAndRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	options := OptionsForInstallDir(dir)
	request := Request{Action: "issue", Domain: "nrynet.example.com", Email: "admin@example.com"}
	if err := EnqueueWithOptions(request, options); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := os.Stat(options.RequestPath); err != nil {
		t.Fatalf("request not written: %v", err)
	}
	link := filepath.Join(dir, "data", "certbot", "linked-request.json")
	if err := os.Symlink(options.RequestPath, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := readRequest(link); err == nil {
		t.Fatal("symlink request accepted")
	}
}

func TestRenewUsesRootManagedStateInsteadOfStatusOutput(t *testing.T) {
	dir := t.TempDir()
	options := OptionsForInstallDir(dir)
	options.StatusPath = filepath.Join(dir, "root-state", "status.json")
	options.ManagedPath = filepath.Join(dir, "root-state", "managed.json")
	good := Request{Action: "issue", Domain: "good.example.com", Email: "admin@example.com"}
	if err := writeManagedState(options, good); err != nil {
		t.Fatalf("write managed state: %v", err)
	}
	poisoned := Status{Action: "issue", Domain: "evil.example.com", Email: "evil@example.com", State: "success"}
	if err := writeStatus(options.StatusPath, poisoned); err != nil {
		t.Fatalf("write poisoned status: %v", err)
	}
	request, err := renewRequestFromManagedState(options)
	if err != nil {
		t.Fatalf("renew request: %v", err)
	}
	runner := &recordingRunner{}
	if _, err := runCertbot(context.Background(), runner, request, options); err != nil {
		t.Fatal(err)
	}
	if !hasArg(runner.args, "--cert-name") || !hasArg(runner.args, "good.example.com") {
		t.Fatalf("renew did not use managed domain: %#v", runner.args)
	}
	if hasArg(runner.args, "evil.example.com") {
		t.Fatalf("renew trusted poisoned status: %#v", runner.args)
	}
}

type recordingRunner struct {
	name string
	args []string
}

func (r *recordingRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.name = name
	r.args = append([]string(nil), args...)
	return []byte("ok"), nil
}

func hasArg(args []string, value string) bool {
	for _, arg := range args {
		if arg == value {
			return true
		}
	}
	return false
}
