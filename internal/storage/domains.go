package storage

import (
	"errors"
	"net"
	"strings"
)

func NormalizeDomain(value string) (string, error) {
	domain := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(value)), ".")
	if err := ValidateDomain(domain); err != nil {
		return "", err
	}
	return domain, nil
}

func ValidateDomain(domain string) error {
	if domain == "" {
		return errors.New("domain is required")
	}
	if len(domain) > 253 || strings.ContainsAny(domain, "_*:/\\") || net.ParseIP(domain) != nil {
		return errors.New("domain must be a fully qualified DNS name")
	}
	labels := strings.Split(domain, ".")
	if len(labels) < 2 {
		return errors.New("domain must be fully qualified")
	}
	for _, label := range labels {
		if err := ValidateSlug(label); err != nil {
			return errors.New("domain contains an invalid label")
		}
	}
	return nil
}

func ValidateSlug(slug string) error {
	if slug == "" || len(slug) > 63 {
		return errors.New("slug must be 1-63 characters")
	}
	if slug[0] == '-' || slug[len(slug)-1] == '-' {
		return errors.New("slug must start and end with a letter or number")
	}
	for _, char := range slug {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') || char == '-' {
			continue
		}
		return errors.New("slug may contain only lowercase letters, numbers, and hyphens")
	}
	return nil
}

func SlugFromName(name string) (string, error) {
	var builder strings.Builder
	previousHyphen := false
	for _, char := range strings.ToLower(strings.TrimSpace(name)) {
		if (char >= 'a' && char <= 'z') || (char >= '0' && char <= '9') {
			builder.WriteRune(char)
			previousHyphen = false
			continue
		}
		if builder.Len() > 0 && !previousHyphen {
			builder.WriteByte('-')
			previousHyphen = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if len(slug) > 48 {
		slug = strings.TrimRight(slug[:48], "-")
	}
	if slug == "" {
		slug = "tunnel"
	}
	if err := ValidateSlug(slug); err != nil {
		return "", err
	}
	return slug, nil
}

func JoinDomain(slug, baseDomain string) (string, error) {
	if err := ValidateSlug(slug); err != nil {
		return "", err
	}
	base, err := NormalizeDomain(baseDomain)
	if err != nil {
		return "", err
	}
	domain := slug + "." + base
	if len(domain) > 253 {
		return "", errors.New("generated domain is too long")
	}
	return domain, nil
}
