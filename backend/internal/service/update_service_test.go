//go:build unit

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceCacheStub struct {
	data string
}

func (s *updateServiceCacheStub) GetUpdateInfo(context.Context) (string, error) {
	if s.data == "" {
		return "", errors.New("cache miss")
	}
	return s.data, nil
}

func (s *updateServiceCacheStub) SetUpdateInfo(_ context.Context, data string, _ time.Duration) error {
	s.data = data
	return nil
}

type updateServiceGitHubClientStub struct {
	release *GitHubRelease
}

func (s *updateServiceGitHubClientStub) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return s.release, nil
}

func (s *updateServiceGitHubClientStub) DownloadFile(context.Context, string, string, int64) error {
	panic("DownloadFile should not be called when no update is available")
}

func (s *updateServiceGitHubClientStub) FetchChecksumFile(context.Context, string) ([]byte, error) {
	panic("FetchChecksumFile should not be called when no update is available")
}

func TestUpdateServicePerformUpdateNoUpdateReturnsSentinel(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceCacheStub{},
		&updateServiceGitHubClientStub{
			release: &GitHubRelease{
				TagName: "v0.1.132",
				Name:    "v0.1.132",
			},
		},
		"0.1.132",
		"release",
	)

	err := svc.PerformUpdate(context.Background())

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrNoUpdateAvailable))
	require.ErrorIs(t, err, ErrNoUpdateAvailable)
}

func TestUpdateServiceCompareVersions(t *testing.T) {
	tests := []struct {
		current  string
		latest   string
		expected int
	}{
		{"0.1.132", "0.1.132", 0},
		{"0.1.132", "0.1.133", -1},
		{"0.1.133", "0.1.132", 1},
		{"v0.2.4", "0.2.4", 0},
		{"0.2.4-20261003T0428Z", "0.2.4-20260910T1433Z", 1},
		{"0.2.4-20260910T1433Z", "0.2.4-20261003T0428Z", -1},
		{"0.2.4-20261003T0428Z", "0.2.4-20261003T0428Z", 0},
		{"0.2.4", "0.2.4-20261003T0428Z", 1},
		{"0.2.4-20261003T0428Z", "0.2.4", -1},
		{"0.2.4-20261003T0428Z", "0.2.5", -1},
		{"0.2.5", "0.2.4-20261003T0428Z", 1},
	}

	for _, tt := range tests {
		t.Run(tt.current+"_vs_"+tt.latest, func(t *testing.T) {
			result := compareVersions(tt.current, tt.latest)
			require.Equal(t, tt.expected, result)
		})
	}
}
