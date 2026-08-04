package storage

import "testing"

func TestNormalizeDomain(t *testing.T) {
	domain, err := NormalizeDomain(" Tunnels.Example.COM. ")
	if err != nil {
		t.Fatal(err)
	}
	if domain != "tunnels.example.com" {
		t.Fatalf("domain=%q", domain)
	}
}

func TestValidateDomainRejectsUnsafeNames(t *testing.T) {
	for _, domain := range []string{"*.example.com", "127.0.0.1", "bad_name.example.com", "-bad.example.com", "localhost"} {
		if err := ValidateDomain(domain); err == nil {
			t.Fatalf("unsafe domain accepted: %s", domain)
		}
	}
}

func TestSlugFromNameFallsBackForNonASCIIName(t *testing.T) {
	slug, err := SlugFromName("服务")
	if err != nil {
		t.Fatal(err)
	}
	if slug != "tunnel" {
		t.Fatalf("slug=%q", slug)
	}
	domain, err := JoinDomain(slug, "tunnels.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if domain != "tunnel.tunnels.example.com" {
		t.Fatalf("domain=%q", domain)
	}
}
