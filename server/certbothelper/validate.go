package certbothelper

import (
	"errors"
	"net"
	"net/mail"
	"regexp"
	"strings"
)

var labelPattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

func ValidateRequest(request Request) error {
	switch request.Action {
	case "issue":
		if err := ValidateDomain(request.Domain); err != nil {
			return err
		}
		if err := ValidateEmail(request.Email); err != nil {
			return err
		}
		return nil
	case "renew":
		if strings.TrimSpace(request.Domain) != "" || strings.TrimSpace(request.Email) != "" {
			return errors.New("renew request must not include domain or email")
		}
		return nil
	default:
		return errors.New("invalid certbot action")
	}
}

func ValidateDomain(domain string) error {
	value := strings.TrimSpace(strings.TrimSuffix(strings.ToLower(domain), "."))
	if value == "" {
		return errors.New("domain is required")
	}
	if strings.HasPrefix(value, "-") || strings.ContainsAny(value, " /*\\'\"`$;&|<>()[]{}") {
		return errors.New("domain contains unsafe characters")
	}
	if strings.HasPrefix(value, "*.") || net.ParseIP(value) != nil {
		return errors.New("domain must be a DNS name, not an IP or wildcard")
	}
	labels := strings.Split(value, ".")
	if len(labels) < 2 || len(value) > 253 {
		return errors.New("domain must be a fully qualified DNS name")
	}
	for _, label := range labels {
		if !labelPattern.MatchString(label) {
			return errors.New("domain contains an invalid label")
		}
	}
	return nil
}

func ValidateEmail(email string) error {
	value := strings.TrimSpace(email)
	if value == "" {
		return errors.New("email is required")
	}
	if strings.ContainsAny(value, " \t\r\n'\"`$;&|<>()[]{}") {
		return errors.New("email contains unsafe characters")
	}
	address, err := mail.ParseAddress(value)
	if err != nil || address.Address != value {
		return errors.New("email is invalid")
	}
	return nil
}
