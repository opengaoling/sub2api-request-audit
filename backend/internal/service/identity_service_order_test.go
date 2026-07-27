package service

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestIdentityService_CaptureClientFingerprintStoresOpenAIHeadersWithoutAnthropicDefaults(t *testing.T) {
	cache := &identityCacheStub{fingerprints: make(map[int64]*Fingerprint)}
	repository := &clientFingerprintRepositoryStub{}
	svc := NewIdentityService(cache)
	svc.SetClientFingerprintRepository(repository)
	headers := http.Header{
		"User-Agent":      {"codex_cli_rs/0.144.1 (Ubuntu 22.4.0; x86_64) xterm-256color"},
		"Originator":      {"codex_cli_rs"},
		"Openai-Beta":     {"responses=experimental"},
		"Version":         {"0.144.1"},
		"Accept":          {"text/event-stream"},
		"Accept-Language": {"en-US,en;q=0.9"},
	}

	require.NoError(t, svc.CaptureClientFingerprint(context.Background(), string(PlatformOpenAI), headers))
	candidates, _, err := svc.ListCapturedFingerprintCandidates(context.Background(), string(PlatformOpenAI))
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	require.Equal(t, "codex_cli_rs", candidates[0].Originator)
	require.Equal(t, "responses=experimental", candidates[0].OpenAIBeta)
	require.Equal(t, "0.144.1", candidates[0].ClientVersion)
	require.Equal(t, "text/event-stream", candidates[0].Accept)
	require.Equal(t, "en-US,en;q=0.9", candidates[0].AcceptLanguage)
	require.Empty(t, candidates[0].StainlessLang)
}

func TestIdentityService_CapturedFingerprintCandidatesAreSeparatedByPlatform(t *testing.T) {
	repository := &clientFingerprintRepositoryStub{}
	svc := NewIdentityService(&identityCacheStub{})
	svc.SetClientFingerprintRepository(repository)
	require.NoError(t, svc.CaptureClientFingerprint(context.Background(), string(PlatformOpenAI), http.Header{
		"User-Agent": {"codex_cli_rs/0.144.1"}, "Originator": {"codex_cli_rs"},
	}))
	require.NoError(t, svc.CaptureClientFingerprint(context.Background(), string(PlatformAnthropic), http.Header{
		"User-Agent": {"claude-cli/2.1.210"}, "X-Stainless-Lang": {"js"},
	}))

	openAI, _, err := svc.ListCapturedFingerprintCandidates(context.Background(), string(PlatformOpenAI))
	require.NoError(t, err)
	require.Len(t, openAI, 1)
	require.Equal(t, string(PlatformOpenAI), openAI[0].Platform)
	require.Equal(t, "codex_cli_rs", openAI[0].Headers["originator"])
	require.NotContains(t, openAI[0].Headers, "x-stainless-lang")

	anthropic, _, err := svc.ListCapturedFingerprintCandidates(context.Background(), string(PlatformAnthropic))
	require.NoError(t, err)
	require.Len(t, anthropic, 1)
	require.Equal(t, string(PlatformAnthropic), anthropic[0].Platform)
	require.Equal(t, "js", anthropic[0].Headers["x-stainless-lang"])
	require.NotContains(t, anthropic[0].Headers, "originator")
}

type clientFingerprintRepositoryStub struct {
	fingerprints map[string]CapturedFingerprint
}

func (s *clientFingerprintRepositoryStub) Upsert(_ context.Context, fingerprint CapturedFingerprint) error {
	if s.fingerprints == nil {
		s.fingerprints = make(map[string]CapturedFingerprint)
	}
	fingerprint.CaptureCount++
	fingerprint.FirstSeenAt = time.Now()
	fingerprint.LastSeenAt = time.Now()
	s.fingerprints[fingerprint.Platform+":"+fingerprint.ID] = fingerprint
	return nil
}

func (s *clientFingerprintRepositoryStub) List(_ context.Context, platform string, _ int) ([]CapturedFingerprint, error) {
	result := make([]CapturedFingerprint, 0)
	for _, fingerprint := range s.fingerprints {
		if fingerprint.Platform == platform {
			result = append(result, fingerprint)
		}
	}
	return result, nil
}

func (s *clientFingerprintRepositoryStub) Get(_ context.Context, platform, id string) (*CapturedFingerprint, error) {
	fingerprint, ok := s.fingerprints[platform+":"+id]
	if !ok {
		return nil, nil
	}
	return &fingerprint, nil
}

type identityCacheStub struct {
	maskedSessionID string
	fingerprints    map[int64]*Fingerprint
	global          *Fingerprint
}

func (s *identityCacheStub) GetFingerprint(_ context.Context, accountID int64) (*Fingerprint, error) {
	return s.fingerprints[accountID], nil
}
func (s *identityCacheStub) SetFingerprint(_ context.Context, accountID int64, fp *Fingerprint) error {
	if s.fingerprints == nil {
		s.fingerprints = make(map[int64]*Fingerprint)
	}
	s.fingerprints[accountID] = fp
	return nil
}
func (s *identityCacheStub) ListFingerprints(_ context.Context) ([]*Fingerprint, error) {
	result := make([]*Fingerprint, 0, len(s.fingerprints))
	for _, fp := range s.fingerprints {
		result = append(result, fp)
	}
	return result, nil
}
func (s *identityCacheStub) GetGlobalFingerprint(_ context.Context) (*Fingerprint, error) {
	return s.global, nil
}
func (s *identityCacheStub) SetGlobalFingerprint(_ context.Context, fp *Fingerprint) error {
	s.global = fp
	return nil
}
func (s *identityCacheStub) GetMaskedSessionID(_ context.Context, _ int64) (string, error) {
	return s.maskedSessionID, nil
}

func TestIdentityService_GlobalFingerprintPreservesPerAccountClientID(t *testing.T) {
	cache := &identityCacheStub{fingerprints: map[int64]*Fingerprint{
		1: {ClientID: "client-one", UserAgent: "claude-cli/2.1.100 (external, cli)", UpdatedAt: 1},
		2: {ClientID: "client-two", UserAgent: "claude-cli/2.1.210 (external, cli)", UpdatedAt: 2},
	}}
	svc := NewIdentityService(cache)
	candidates, _, err := svc.ListFingerprintCandidates(context.Background())
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	require.NoError(t, svc.SelectGlobalFingerprint(context.Background(), candidates[0].ID))

	first, err := svc.GetOrCreateFingerprint(context.Background(), 1, nil)
	require.NoError(t, err)
	second, err := svc.GetOrCreateFingerprint(context.Background(), 2, nil)
	require.NoError(t, err)
	require.Equal(t, "client-one", first.ClientID)
	require.Equal(t, "client-two", second.ClientID)
	require.Equal(t, first.UserAgent, second.UserAgent)
}
func (s *identityCacheStub) SetMaskedSessionID(_ context.Context, _ int64, sessionID string) error {
	s.maskedSessionID = sessionID
	return nil
}

func TestIdentityService_RewriteUserID_PreservesTopLevelFieldOrder(t *testing.T) {
	cache := &identityCacheStub{}
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `},"max_tokens":64000,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"stream":true}`)

	result, err := svc.RewriteUserID(body, 123, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.NotContains(t, resultStr, originalUserID)
	require.Contains(t, resultStr, `"metadata":{"user_id":"`)
}

func TestIdentityService_RewriteUserIDWithMasking_PreservesTopLevelFieldOrder(t *testing.T) {
	cache := &identityCacheStub{maskedSessionID: "11111111-2222-4333-8444-555555555555"}
	svc := NewIdentityService(cache)

	originalUserID := FormatMetadataUserID(
		"d61f76d0730d2b920763648949bad5c79742155c27037fc77ac3f9805cb90169",
		"",
		"7578cf37-aaca-46e4-a45c-71285d9dbb83",
		"2.1.78",
	)
	body := []byte(`{"alpha":1,"messages":[],"metadata":{"user_id":` + strconvQuote(originalUserID) + `},"max_tokens":64000,"thinking":{"type":"adaptive"},"output_config":{"effort":"high"},"stream":true}`)

	account := &Account{
		ID:       123,
		Platform: PlatformAnthropic,
		Type:     AccountTypeOAuth,
		Extra: map[string]any{
			"session_id_masking_enabled": true,
		},
	}

	result, err := svc.RewriteUserIDWithMasking(context.Background(), body, account, "acc-uuid", "client-xyz", "claude-cli/2.1.78 (external, cli)")
	require.NoError(t, err)
	resultStr := string(result)

	assertJSONTokenOrder(t, resultStr, `"alpha"`, `"messages"`, `"metadata"`, `"max_tokens"`, `"thinking"`, `"output_config"`, `"stream"`)
	require.Contains(t, resultStr, cache.maskedSessionID)
	require.True(t, strings.Contains(resultStr, `"metadata":{"user_id":"`))
}

func strconvQuote(v string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`) + `"`
}
