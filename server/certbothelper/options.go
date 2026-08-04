package certbothelper

import "path/filepath"

type Options struct {
	RequestPath    string
	StatusPath     string
	ManagedPath    string
	LockPath       string
	ReadyPath      string
	LetsEncryptDir string
	WorkDir        string
	LogsDir        string
	TargetTLSDir   string
	FullchainPath  string
	PrivateKeyPath string
	ServiceGroup   string
}

func DefaultOptions() Options {
	return Options{
		RequestPath: RequestPath, StatusPath: StatusPath, ManagedPath: ManagedStatePath,
		LockPath: LockPath, ReadyPath: ReadyPath,
		LetsEncryptDir: LetsEncryptDir, WorkDir: CertbotWorkDir, LogsDir: CertbotLogsDir,
		TargetTLSDir: TargetTLSDir, FullchainPath: TargetFullchain,
		PrivateKeyPath: TargetPrivateKey, ServiceGroup: ServiceGroup,
	}
}

func OptionsForInstallDir(installDir string) Options {
	options := DefaultOptions()
	options.RequestPath = filepath.Join(installDir, "data", "certbot", "inbox", "request.json")
	options.TargetTLSDir = filepath.Join(installDir, "tls")
	options.FullchainPath = filepath.Join(options.TargetTLSDir, "fullchain.pem")
	options.PrivateKeyPath = filepath.Join(options.TargetTLSDir, "privkey.pem")
	return options
}
