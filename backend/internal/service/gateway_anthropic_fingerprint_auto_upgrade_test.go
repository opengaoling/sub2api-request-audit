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

type staticVersionProvider struct {
	major, minor, patch int
}

func (s staticVersionProvider) GetClaudeCodeFingerprintMinVersion(ctx context.Context) (int, int, int) {
	return s.major, s.minor, s.patch
}

func TestApplyAnthropicOAuthFingerprint_DynamicMinVersion(t *testing.T) {
	repository := &clientFingerprintRepositoryStub{
		fingerprints: map[string]CapturedFingerprint{
			"anthropic:fp-290": {
				ID:        "fp-290",
				Platform:  string(PlatformAnthropic),
				UserAgent: "claude-cli/2.1.290 (external, cli)",
				Headers: map[string]string{
					"user-agent": "claude-cli/2.1.290 (external, cli)",
				},
			},
			"anthropic:fp-300": {
				ID:        "fp-300",
				Platform:  string(PlatformAnthropic),
				UserAgent: "claude-cli/2.1.300 (external, cli)",
				Headers: map[string]string{
					"user-agent": "claude-cli/2.1.300 (external, cli)",
					"x-app":      "cli",
				},
			},
		},
	}
	identitySvc := NewIdentityService(&identityCacheStub{})
	identitySvc.SetClientFingerprintRepository(repository)
	// 动态设置最低版本限制为 2.1.300
	identitySvc.SetClaudeCodeFingerprintVersionProvider(staticVersionProvider{major: 2, minor: 1, patch: 300})

	mockRepo := &mockAccountRepoForFingerprint{}
	gw := &GatewayService{
		identityService: identitySvc,
		accountRepo:     mockRepo,
	}

	account := &Account{
		ID:       103,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			AnthropicFingerprintExtraKey: "fp-290", // 2.1.290 低于 2.1.300
		},
	}

	headers := make(http.Header)
	gw.applyAnthropicOAuthFingerprint(context.Background(), nil, account, headers)

	// 低于 2.1.300 的指纹应该被自动升级到 fp-300
	require.Equal(t, "fp-300", account.Extra[AnthropicFingerprintExtraKey])
	require.Equal(t, "claude-cli/2.1.300 (external, cli)", headers.Get("User-Agent"))
	require.Equal(t, "fp-300", mockRepo.updatedExtra[AnthropicFingerprintExtraKey])

	// 测试采集：低于 2.1.300 不采集，>= 2.1.300 采集
	reqHeadersOld := http.Header{"User-Agent": []string{"claude-cli/2.1.290 (external, cli)"}}
	capturedOld, err := identitySvc.CaptureClientFingerprintID(context.Background(), string(PlatformAnthropic), reqHeadersOld)
	require.NoError(t, err)
	require.Empty(t, capturedOld)

	reqHeadersNew := http.Header{"User-Agent": []string{"claude-cli/2.1.305 (external, cli)"}}
	capturedNew, err := identitySvc.CaptureClientFingerprintID(context.Background(), string(PlatformAnthropic), reqHeadersNew)
	require.NoError(t, err)
	require.NotEmpty(t, capturedNew)
}
