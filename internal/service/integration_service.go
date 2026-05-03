package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/Ahmed20011994/anton/internal/crypto"
	"github.com/Ahmed20011994/anton/internal/repository"
	"github.com/Ahmed20011994/anton/internal/tenantctx"
)

type IntegrationService struct {
	repo *repository.IntegrationRepo
	key  []byte
}

func NewIntegrationService(repo *repository.IntegrationRepo, key []byte) *IntegrationService {
	return &IntegrationService{repo: repo, key: key}
}

// StoreCredentials encrypts plaintext credential JSON and persists it.
func (s *IntegrationService) StoreCredentials(
	ctx context.Context,
	scope tenantctx.Scope,
	sourceType string,
	plaintext []byte,
	fieldMapping json.RawMessage,
) (repository.Integration, error) {
	aad := crypto.BuildAAD(scope.TenantID, sourceType)
	ct, err := crypto.Encrypt(s.key, plaintext, aad)
	if err != nil {
		return repository.Integration{}, fmt.Errorf("StoreCredentials: encrypt: %w", err)
	}
	out, err := s.repo.Put(ctx, scope, sourceType, ct, fieldMapping)
	if err != nil {
		return repository.Integration{}, fmt.Errorf("StoreCredentials: %w", err)
	}
	return out, nil
}

// LoadCredentials returns the integration row plus decrypted credential bytes.
// The caller MUST defer crypto.Zero on the returned plaintext slice.
func (s *IntegrationService) LoadCredentials(
	ctx context.Context,
	scope tenantctx.Scope,
	sourceType string,
) (repository.Integration, []byte, error) {
	integ, err := s.repo.Get(ctx, scope, sourceType)
	if err != nil {
		return repository.Integration{}, nil, fmt.Errorf("LoadCredentials: %w", err)
	}
	aad := crypto.BuildAAD(scope.TenantID, sourceType)
	pt, err := crypto.Decrypt(s.key, integ.CredentialsEncrypted, aad)
	if err != nil {
		return repository.Integration{}, nil, fmt.Errorf("LoadCredentials: decrypt: %w", err)
	}
	return integ, pt, nil
}
