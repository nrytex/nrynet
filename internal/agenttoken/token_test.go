package agenttoken

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestParseSupportsLegacyAndPinnedTokens(t *testing.T) {
	pin := base64.RawURLEncoding.EncodeToString(make([]byte, 32))
	for _, test := range []struct {
		value string
		pin   string
	}{
		{value: "id.secret"},
		{value: "id.secret.spki-sha256-" + pin, pin: pin},
	} {
		parts, err := Parse(test.value)
		if err != nil {
			t.Fatalf("parse %q: %v", test.value, err)
		}
		if parts.ID != "id" || parts.Secret != "secret" || parts.CertificatePin != test.pin {
			t.Fatalf("unexpected parts for %q: %+v", test.value, parts)
		}
	}
}

func TestParseRejectsInvalidCertificatePin(t *testing.T) {
	for _, value := range []string{
		"id.secret.extra", "id.secret.spki-sha256-short", "id.secret.extra.segment",
	} {
		if _, err := Parse(value); err == nil {
			t.Fatalf("invalid token was accepted: %q", value)
		}
	}
}

func TestWithCertificatePinReplacesExistingPin(t *testing.T) {
	pin := base64.RawURLEncoding.EncodeToString([]byte(strings.Repeat("a", 32)))
	updated, err := WithCertificatePin("id.secret", pin)
	if err != nil {
		t.Fatal(err)
	}
	replaced, err := WithCertificatePin(updated, pin)
	if err != nil || replaced != updated {
		t.Fatalf("replace pin: value=%q err=%v", replaced, err)
	}
}
