package service

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

type mockAccountRepoForFingerprint struct {
	AccountRepository
	updatedExtra map[string]any
}

func (m *mockAccountRepoForFingerprint) UpdateExtra(ctx context.Context, id int64, updates map[string]any) error {
	m.updatedExtra = updates
	return nil
}

func TestApplyAnthropicOAuthFingerprint_UpgradesOlderFingerprint(t *testing.T) {
	repository := &clientFingerprintRepositoryStub{
		fingerprints: map[string]CapturedFingerprint{
			"anthropic:old-fp": {
				ID:        "old-fp",
				Platform:  string(PlatformAnthropic),
				UserAgent: "claude-cli/2.1.220 (external, cli)",
				Headers: map[string]string{
					"user-agent": "claude-cli/2.1.220 (external, cli)",
				},
			},
			"anthropic:new-fp": {
				ID:        "new-fp",
				Platform:  string(PlatformAnthropic),
				UserAgent: "claude-cli/2.1.290 (external, cli)",
				Headers: map[string]string{
					"user-agent": "claude-cli/2.1.290 (external, cli)",
					"x-app":      "cli",
				},
			},
		},
	}
	identitySvc := NewIdentityService(&identityCacheStub{})
	identitySvc.SetClientFingerprintRepository(repository)

	mockRepo := &mockAccountRepoForFingerprint{}
	gw := &GatewayService{
		identityService: identitySvc,
		accountRepo:     mockRepo,
	}

	account := &Account{
		ID:       101,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			AnthropicFingerprintExtraKey: "old-fp",
		},
	}

	headers := make(http.Header)
	gw.applyAnthropicOAuthFingerprint(context.Background(), nil, account, headers)

	// 旧指纹应被自动替换为合规的新指纹 new-fp
	require.Equal(t, "new-fp", account.Extra[AnthropicFingerprintExtraKey])
	require.Equal(t, "claude-cli/2.1.290 (external, cli)", headers.Get("User-Agent"))
	require.Equal(t, "new-fp", mockRepo.updatedExtra[AnthropicFingerprintExtraKey])
}

func TestApplyAnthropicOAuthFingerprint_ClearsOldFingerprintWhenNoCandidates(t *testing.T) {
	repository := &clientFingerprintRepositoryStub{
		fingerprints: map[string]CapturedFingerprint{
			"anthropic:old-fp": {
				ID:        "old-fp",
				Platform:  string(PlatformAnthropic),
				UserAgent: "claude-cli/2.1.220 (external, cli)",
				Headers: map[string]string{
					"user-agent": "claude-cli/2.1.220 (external, cli)",
				},
			},
		},
	}
	identitySvc := NewIdentityService(&identityCacheStub{})
	identitySvc.SetClientFingerprintRepository(repository)

	mockRepo := &mockAccountRepoForFingerprint{}
	gw := &GatewayService{
		identityService: identitySvc,
		accountRepo:     mockRepo,
	}

	account := &Account{
		ID:       102,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			AnthropicFingerprintExtraKey: "old-fp",
		},
	}

	headers := make(http.Header)
	gw.applyAnthropicOAuthFingerprint(context.Background(), nil, account, headers)

	// 无合规指纹时，旧的无效指纹ID应被清理，且不应用旧指纹头
	require.Empty(t, account.Extra[AnthropicFingerprintExtraKey])
	require.Empty(t, headers.Get("User-Agent"))
	require.Equal(t, "", mockRepo.updatedExtra[AnthropicFingerprintExtraKey])
}
