package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestCryptoVerify(t *testing.T) {
	// Read private key generated earlier
	privBytes, err := os.ReadFile("../../cmd/keygen/private.key")
	if err != nil {
		t.Skip("No private.key present, skipping test")
	}

	privHex := strings.TrimSpace(string(privBytes))
	privKeyRaw, _ := hex.DecodeString(privHex)
	privKey := ed25519.PrivateKey(privKeyRaw)

	testMID := GetMachineID()

	// 1. Generate valid permanent license
	payload := Payload{
		MachineID: testMID,
		Customer:  "Unit Tester",
		ExpiresAt: 0,
		IssuedAt:  time.Now().Unix(),
		MaxStores: 10,
	}
	payloadBytes, _ := json.Marshal(payload)
	sig := ed25519.Sign(privKey, payloadBytes)
	validKey := "TKACT-" + base64.RawURLEncoding.EncodeToString(payloadBytes) + "." + base64.RawURLEncoding.EncodeToString(sig)

	// Test ParseAndVerify with matching MID
	p, err := ParseAndVerify(validKey, testMID)
	if err != nil {
		t.Fatalf("Valid license failed verification: %v", err)
	}
	if p.Customer != "Unit Tester" {
		t.Errorf("Expected customer 'Unit Tester', got '%s'", p.Customer)
	}

	// Test ParseAndVerify with wrong MID
	_, err = ParseAndVerify(validKey, "TK-WRONG-0000-0000-0000")
	if err == nil || !strings.Contains(err.Error(), "不匹配") {
		t.Fatalf("Expected machine mismatch error, got: %v", err)
	}

	// Test Expired License
	expiredPayload := Payload{
		MachineID: testMID,
		Customer:  "Expired User",
		ExpiresAt: time.Now().Unix() - 3600, // 1 hour ago
		IssuedAt:  time.Now().Unix() - 7200,
	}
	expBytes, _ := json.Marshal(expiredPayload)
	expSig := ed25519.Sign(privKey, expBytes)
	expiredKey := "TKACT-" + base64.RawURLEncoding.EncodeToString(expBytes) + "." + base64.RawURLEncoding.EncodeToString(expSig)

	_, err = ParseAndVerify(expiredKey, testMID)
	if err == nil || !strings.Contains(err.Error(), "已过期") {
		t.Fatalf("Expected expired error, got: %v", err)
	}

	t.Logf("All crypto tests passed! Valid key: %s", validKey)
}
