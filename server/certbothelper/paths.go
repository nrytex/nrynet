package certbothelper

const (
	RequestPath       = "/opt/nrynet/data/certbot/inbox/request.json"
	StatusPath        = "/var/lib/nrynet/certbot/status.json"
	ManagedStatePath  = "/var/lib/nrynet/certbot/managed.json"
	LockPath          = "/var/lib/nrynet/certbot/request.lock"
	ReadyPath         = "/var/lib/nrynet/certbot/helper-ready"
	LetsEncryptDir    = "/etc/letsencrypt"
	CertbotWorkDir    = "/var/lib/nrynet/certbot/work"
	CertbotLogsDir    = "/var/log/nrynet/certbot"
	TargetTLSDir      = "/opt/nrynet/tls"
	TargetFullchain   = "/opt/nrynet/tls/fullchain.pem"
	TargetPrivateKey  = "/opt/nrynet/tls/privkey.pem"
	ServiceGroup      = "nrynet"
	maxOutputBytes    = 4096
	defaultStatusMode = 0o640
)
