package crypto_test

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/google/uuid"

	"github.com/Ahmed20011994/anton/internal/crypto"
)

func TestEncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}

	tenantID := uuid.New()
	otherTenantID := uuid.New()

	tests := []struct {
		name       string
		plaintext  []byte
		sourceType string
	}{
		{name: "short plaintext", plaintext: []byte(`{"a":1}`), sourceType: "jira"},
		{name: "empty plaintext", plaintext: []byte(``), sourceType: "jira"},
		{name: "long plaintext", plaintext: bytes.Repeat([]byte("x"), 4096), sourceType: "zendesk"},
		{name: "binary plaintext", plaintext: []byte{0x00, 0xff, 0x10, 0x20}, sourceType: "jira"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			aad := crypto.BuildAAD(tenantID, tc.sourceType)
			ct, err := crypto.Encrypt(key, tc.plaintext, aad)
			if err != nil {
				t.Fatalf("Encrypt: %v", err)
			}
			pt, err := crypto.Decrypt(key, ct, aad)
			if err != nil {
				t.Fatalf("Decrypt: %v", err)
			}
			if !bytes.Equal(pt, tc.plaintext) {
				t.Fatalf("plaintext mismatch: got %q, want %q", pt, tc.plaintext)
			}

			// AAD binding: ciphertext must not decrypt under a different tenant_id.
			wrongAAD := crypto.BuildAAD(otherTenantID, tc.sourceType)
			if _, err := crypto.Decrypt(key, ct, wrongAAD); err == nil {
				t.Fatalf("Decrypt with wrong tenant AAD must fail")
			}

			// AAD binding: ciphertext must not decrypt under a different source_type.
			wrongSourceAAD := crypto.BuildAAD(tenantID, "intercom")
			if _, err := crypto.Decrypt(key, ct, wrongSourceAAD); err == nil {
				t.Fatalf("Decrypt with wrong source_type AAD must fail")
			}
		})
	}
}

func TestEncryptRejectsBadKey(t *testing.T) {
	tests := []struct {
		name    string
		keySize int
	}{
		{name: "16 bytes", keySize: 16},
		{name: "31 bytes", keySize: 31},
		{name: "33 bytes", keySize: 33},
		{name: "0 bytes", keySize: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := make([]byte, tc.keySize)
			aad := crypto.BuildAAD(uuid.New(), "jira")
			if _, err := crypto.Encrypt(key, []byte("x"), aad); err == nil {
				t.Fatalf("expected error for key size %d", tc.keySize)
			}
		})
	}
}

func TestDecryptRejectsTampered(t *testing.T) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("rand: %v", err)
	}
	aad := crypto.BuildAAD(uuid.New(), "jira")
	ct, err := crypto.Encrypt(key, []byte("secret"), aad)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	tests := []struct {
		name   string
		mutate func([]byte) []byte
	}{
		{name: "flip last byte", mutate: func(b []byte) []byte { b[len(b)-1] ^= 0x01; return b }},
		{name: "flip nonce byte", mutate: func(b []byte) []byte { b[2] ^= 0x01; return b }},
		{name: "wrong version", mutate: func(b []byte) []byte { b[0] = 0x02; return b }},
		{name: "truncated", mutate: func(b []byte) []byte { return b[:5] }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cp := append([]byte(nil), ct...)
			tampered := tc.mutate(cp)
			if _, err := crypto.Decrypt(key, tampered, aad); err == nil {
				t.Fatalf("Decrypt of tampered ciphertext must fail")
			}
		})
	}
}

func TestZero(t *testing.T) {
	b := []byte{1, 2, 3, 4}
	crypto.Zero(b)
	for i, v := range b {
		if v != 0 {
			t.Errorf("byte %d not zeroed: %d", i, v)
		}
	}
}
