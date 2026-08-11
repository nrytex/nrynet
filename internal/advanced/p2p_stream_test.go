package advanced

import "testing"

func TestP2PProofBindsSessionRequestAndRole(t *testing.T) {
	key := []byte("01234567890123456789012345678901")
	proof, err := P2PProof(key, "session-1", "request-1", P2PStreamRoleServer)
	if err != nil {
		t.Fatal(err)
	}
	if !VerifyP2PProof(key, "session-1", "request-1", P2PStreamRoleServer, proof) {
		t.Fatal("expected proof to verify")
	}
	for _, invalid := range []struct {
		name      string
		sessionID string
		requestID string
		role      string
	}{
		{name: "session", sessionID: "other", requestID: "request-1", role: P2PStreamRoleServer},
		{name: "request", sessionID: "session-1", requestID: "other", role: P2PStreamRoleServer},
		{name: "role", sessionID: "session-1", requestID: "request-1", role: P2PStreamRoleAgent},
	} {
		t.Run(invalid.name, func(t *testing.T) {
			if VerifyP2PProof(key, invalid.sessionID, invalid.requestID, invalid.role, proof) {
				t.Fatal("invalid proof context was accepted")
			}
		})
	}
}
