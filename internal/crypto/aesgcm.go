package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"fmt"
	"io"

	"github.com/google/uuid"
)

const (
	keyVersionV1 byte = 0x01
	nonceSize         = 12
)

// Envelope wire format: [version_byte][nonce_12_bytes][ciphertext_with_gcm_tag]
//
// AAD is bound to (tenant_id, source_type) so a ciphertext copied
// to a different row fails to decrypt.

func BuildAAD(tenantID uuid.UUID, sourceType string) []byte {
	out := make([]byte, 0, 16+len(sourceType))
	out = append(out, tenantID[:]...)
	out = append(out, []byte(sourceType)...)
	return out
}

func Encrypt(key []byte, plaintext []byte, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("Encrypt: key must be 32 bytes, got %d", len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("Encrypt: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("Encrypt: gcm: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("Encrypt: nonce: %w", err)
	}

	ct := gcm.Seal(nil, nonce, plaintext, aad)

	out := make([]byte, 0, 1+nonceSize+len(ct))
	out = append(out, keyVersionV1)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func Decrypt(key []byte, envelope []byte, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("Decrypt: key must be 32 bytes, got %d", len(key))
	}
	if len(envelope) < 1+nonceSize {
		return nil, fmt.Errorf("Decrypt: envelope too short: %d bytes", len(envelope))
	}

	version := envelope[0]
	if version != keyVersionV1 {
		return nil, fmt.Errorf("Decrypt: unsupported key version 0x%02x", version)
	}

	nonce := envelope[1 : 1+nonceSize]
	ct := envelope[1+nonceSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("Decrypt: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("Decrypt: gcm: %w", err)
	}

	pt, err := gcm.Open(nil, nonce, ct, aad)
	if err != nil {
		return nil, fmt.Errorf("Decrypt: open: %w", err)
	}
	return pt, nil
}

// Zero overwrites b with zeros. Callers should defer Zero(plaintext)
// after decrypting credentials.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
