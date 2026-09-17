package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// MasterPublicKey is the official Ed25519 public key embedded into the software.
// Only licenses signed by the corresponding private key can pass verification.
const MasterPublicKey = "41d6114faa5cb8069ad7e7e450ad73c6346ad9df792749d7ed8fa3dd0eb936da"

// Payload contains the decoded data stored inside the license token.
type Payload struct {
	MachineID string `json:"mid"`
	Customer  string `json:"cus,omitempty"`
	ExpiresAt int64  `json:"exp"` // 0 = permanent
	IssuedAt  int64  `json:"iat"`
	MaxStores int    `json:"stores,omitempty"`
}

var (
	ErrInvalidFormat    = errors.New("激活码格式错误")
	ErrSignatureInvalid = errors.New("激活码签名无效或已被篡改")
	ErrMachineMismatch  = errors.New("激活码与当前机器识别码不匹配")
	ErrExpired          = errors.New("软件授权已过期，请续费或联系管理员")
)

// ParseAndVerify verifies the license token using the embedded public key,
// and checks whether it matches the expected machine ID.
func ParseAndVerify(licenseKey string, expectedMachineID string) (*Payload, error) {
	cleanKey := strings.TrimSpace(licenseKey)
	if !strings.HasPrefix(cleanKey, "TKACT-") {
		return nil, ErrInvalidFormat
	}

	body := strings.TrimPrefix(cleanKey, "TKACT-")
	parts := strings.Split(body, ".")
	if len(parts) != 2 {
		return nil, ErrInvalidFormat
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("%w: payload 解码失败", ErrInvalidFormat)
	}

	sigBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sigBytes) != ed25519.SignatureSize {
		return nil, fmt.Errorf("%w: 签名长度非法", ErrInvalidFormat)
	}

	pubKeyBytes, err := hex.DecodeString(MasterPublicKey)
	if err != nil || len(pubKeyBytes) != ed25519.PublicKeySize {
		return nil, errors.New("内置公钥解析失败")
	}

	pubKey := ed25519.PublicKey(pubKeyBytes)
	if !ed25519.Verify(pubKey, payloadBytes, sigBytes) {
		return nil, ErrSignatureInvalid
	}

	var payload Payload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return nil, fmt.Errorf("%w: payload 结构损坏", ErrInvalidFormat)
	}

	if expectedMachineID != "" && !strings.EqualFold(payload.MachineID, expectedMachineID) {
		return nil, fmt.Errorf("%w (授权机器码: %s, 本机识别码: %s)", ErrMachineMismatch, payload.MachineID, expectedMachineID)
	}

	if payload.ExpiresAt > 0 && time.Now().Unix() > payload.ExpiresAt {
		expTime := time.Unix(payload.ExpiresAt, 0).Format("2006-01-02 15:04:05")
		return nil, fmt.Errorf("%w (到期时间: %s)", ErrExpired, expTime)
	}

	return &payload, nil
}
