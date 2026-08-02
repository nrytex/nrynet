package tunnel

import (
	"net"
	"testing"
)

func TestVisitorAllowed(t *testing.T) {
	address := &net.TCPAddr{IP: net.ParseIP("192.168.1.20"), Port: 1234}
	if !visitorAllowed(address, []string{"192.168.1.0/24"}) {
		t.Fatal("CIDR should allow visitor")
	}
	if visitorAllowed(address, []string{"10.0.0.1"}) {
		t.Fatal("unlisted visitor should be denied")
	}
	if !visitorAllowed(address, nil) {
		t.Fatal("empty allowlist should permit visitor")
	}
}
