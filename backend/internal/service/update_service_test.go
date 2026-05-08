//go:build unit

package service

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type updateServiceTestCache struct {
	data string
}

func (c *updateServiceTestCache) GetUpdateInfo(context.Context) (string, error) {
	return c.data, nil
}

func (c *updateServiceTestCache) SetUpdateInfo(context.Context, string, time.Duration) error {
	return nil
}

type updateServiceTestGitHubClient struct {
	release *GitHubRelease
}

func (c *updateServiceTestGitHubClient) FetchLatestRelease(context.Context, string) (*GitHubRelease, error) {
	return c.release, nil
}

func (c *updateServiceTestGitHubClient) DownloadFile(context.Context, string, string, int64) error {
	return nil
}

func (c *updateServiceTestGitHubClient) FetchChecksumFile(context.Context, string) ([]byte, error) {
	return nil, nil
}

func TestUpdateServiceCheckUpdateMarksSimpleReleaseAsManualOnly(t *testing.T) {
	svc := NewUpdateService(
		&updateServiceTestCache{},
		&updateServiceTestGitHubClient{release: &GitHubRelease{
			TagName: "v0.3.0",
			Assets: []GitHubAsset{
				{Name: "image-digest.txt", BrowserDownloadURL: "https://github.com/Cc-kris/sub2apis/releases/download/v0.3.0/image-digest.txt"},
			},
		}},
		"0.2.0",
		"release",
	)

	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.False(t, info.AutoUpdateSupported)
	require.Contains(t, info.UpdateDisabledReason, "no installable archive")
}

func TestUpdateServiceCheckUpdateMarksSourceBuildAsManualOnly(t *testing.T) {
	archiveName := "sub2api_0.3.0_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz"
	svc := NewUpdateService(
		&updateServiceTestCache{},
		&updateServiceTestGitHubClient{release: &GitHubRelease{
			TagName: "v0.3.0",
			Assets: []GitHubAsset{
				{Name: archiveName, BrowserDownloadURL: "https://github.com/Cc-kris/sub2apis/releases/download/v0.3.0/" + archiveName},
				{Name: "checksums.txt", BrowserDownloadURL: "https://github.com/Cc-kris/sub2apis/releases/download/v0.3.0/checksums.txt"},
			},
		}},
		"0.2.0",
		"source",
	)

	info, err := svc.CheckUpdate(context.Background(), true)
	require.NoError(t, err)
	require.True(t, info.HasUpdate)
	require.False(t, info.AutoUpdateSupported)
	require.Equal(t, "source builds must be updated manually", info.UpdateDisabledReason)
}
