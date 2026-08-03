package agenttoken

import (
	"encoding/base64"
	"errors"
	"strings"
)

const certificatePinPrefix = "spki-sha256-"

type Parts struct {
	ID             string
	Secret         string
	CertificatePin string
}

func Parse(value string) (Parts, error) {
	segments := strings.Split(value, ".")
	if len(segments) < 2 || len(segments) > 3 || segments[0] == "" || segments[1] == "" {
		return Parts{}, errors.New("invalid agent token")
	}
	parts := Parts{ID: segments[0], Secret: segments[1]}
	if len(segments) == 2 {
		return parts, nil
	}
	if !strings.HasPrefix(segments[2], certificatePinPrefix) {
		return Parts{}, errors.New("invalid agent token certificate pin")
	}
	parts.CertificatePin = strings.TrimPrefix(segments[2], certificatePinPrefix)
	if !ValidCertificatePin(parts.CertificatePin) {
		return Parts{}, errors.New("invalid agent token certificate pin")
	}
	return parts, nil
}

func WithCertificatePin(value, pin string) (string, error) {
	parts, err := Parse(value)
	if err != nil {
		return "", err
	}
	if !ValidCertificatePin(pin) {
		return "", errors.New("invalid certificate pin")
	}
	return parts.ID + "." + parts.Secret + "." + certificatePinPrefix + pin, nil
}

func ValidCertificatePin(pin string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(pin)
	return err == nil && len(decoded) == 32
}
