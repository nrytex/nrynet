package advanced

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

const (
	P2PStreamRoleServer = "p2p-server"
	P2PStreamRoleAgent  = "p2p-agent"
)

func P2PProof(key []byte, sessionID, requestID, role string) (string, error) {
	if len(key) != 32 {
		return "", errors.New("p2p session key must be 32 bytes")
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(sessionID))
	mac.Write([]byte{0})
	mac.Write([]byte(requestID))
	mac.Write([]byte{0})
	mac.Write([]byte(role))
	return base64.RawStdEncoding.EncodeToString(mac.Sum(nil)), nil
}

func VerifyP2PProof(key []byte, sessionID, requestID, role, proof string) bool {
	expected, err := P2PProof(key, sessionID, requestID, role)
	if err != nil {
		return false
	}
	actual, err := base64.RawStdEncoding.DecodeString(proof)
	if err != nil {
		return false
	}
	expectedBytes, err := base64.RawStdEncoding.DecodeString(expected)
	return err == nil && hmac.Equal(actual, expectedBytes)
}
