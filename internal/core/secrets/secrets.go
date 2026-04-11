// Package secrets 提供 AES-256-GCM 加密/解密，用于 LLMProfile API Key 等敏感字段。
//
// 存储格式：v1:<base64url(salt)>:<base64url(iv)>:<base64url(ciphertext+tag)>
// 密钥派生：HKDF-SHA256（per-secret random salt）
// 环境变量：SECRETS_MASTER_KEY（hex 编码，≥32 字节）
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"

	"golang.org/x/crypto/hkdf"
)

const (
	version = "v1"
	info    = "we-secrets-v1"
	saltLen = 16
	ivLen   = 12
	keyLen  = 32
)

var b64 = base64.RawURLEncoding

// Encrypt 使用 masterKey 加密 plaintext，返回 "v1:salt:iv:ciphertext" 格式。
func Encrypt(plaintext, masterKey string) (string, error) {
	if masterKey == "" {
		return "", errors.New("secrets: master key is empty")
	}

	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return "", fmt.Errorf("secrets: rand salt: %w", err)
	}

	key, err := deriveKey(masterKey, salt)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("secrets: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("secrets: gcm: %w", err)
	}

	iv := make([]byte, ivLen)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("secrets: rand iv: %w", err)
	}

	sealed := gcm.Seal(nil, iv, []byte(plaintext), nil)

	return version + ":" + b64.EncodeToString(salt) + ":" + b64.EncodeToString(iv) + ":" + b64.EncodeToString(sealed), nil
}

// Decrypt 解密 Encrypt 产出的密文，返回明文。
func Decrypt(ciphertext, masterKey string) (string, error) {
	if masterKey == "" {
		return "", errors.New("secrets: master key is empty")
	}

	parts := strings.SplitN(ciphertext, ":", 4)
	if len(parts) != 4 || parts[0] != version {
		return "", errors.New("secrets: invalid ciphertext format")
	}

	salt, err := b64.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("secrets: decode salt: %w", err)
	}
	iv, err := b64.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("secrets: decode iv: %w", err)
	}
	sealed, err := b64.DecodeString(parts[3])
	if err != nil {
		return "", fmt.Errorf("secrets: decode payload: %w", err)
	}

	key, err := deriveKey(masterKey, salt)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("secrets: aes: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("secrets: gcm: %w", err)
	}

	plain, err := gcm.Open(nil, iv, sealed, nil)
	if err != nil {
		return "", fmt.Errorf("secrets: decrypt failed: %w", err)
	}
	return string(plain), nil
}

// Mask 返回 API Key 的掩码版本（保留后 4 位）。
func Mask(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return strings.Repeat("*", len(key)-4) + key[len(key)-4:]
}

func deriveKey(masterKey string, salt []byte) ([]byte, error) {
	r := hkdf.New(sha256.New, []byte(masterKey), salt, []byte(info))
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("secrets: hkdf: %w", err)
	}
	return key, nil
}
