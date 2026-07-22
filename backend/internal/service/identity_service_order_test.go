package service

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

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
