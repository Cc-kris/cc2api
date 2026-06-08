package service

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/Calcium-Ion/new-api/internal/config"
)

type semanticEmbeddingHTTPClientFunc func(req *http.Request) (*http.Response, error)

func (f semanticEmbeddingHTTPClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestSemanticEmbeddingClientGenerateEmbeddingSuccess(t *testing.T) {
	svc := newSemanticEmbeddingSettingService(t, 3)
	client := NewSemanticEmbeddingClient(svc)
	client.SetHTTPClient(semanticEmbeddingHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		require.Equal(t, http.MethodPost, req.Method)
		require.Equal(t, "http://8.8.8.8/v1/embeddings", req.URL.String())
		require.Equal(t, "Bearer sk-semantic-secret", req.Header.Get("Authorization"))
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"model":"text-embedding-3-large","data":[{"embedding":[0.1,0.2,0.3]}]}`)),
		}, nil
	}))

	result, err := client.GenerateEmbedding(context.Background(), " hello ")

	require.NoError(t, err)
	require.False(t, result.Skipped)
	require.Equal(t, []float64{0.1, 0.2, 0.3}, result.Vector)
	require.Equal(t, 3, result.Dimension)
	require.Equal(t, "text-embedding-3-large", result.Model)
}

func TestSemanticEmbeddingClientGenerateEmbeddingDimensionMismatchSkips(t *testing.T) {
	svc := newSemanticEmbeddingSettingService(t, 4)
	client := NewSemanticEmbeddingClient(svc)
	client.SetHTTPClient(semanticEmbeddingHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(`{"data":[{"embedding":[0.1,0.2,0.3]}]}`)),
		}, nil
	}))

	result, err := client.GenerateEmbedding(context.Background(), "hello")

	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Equal(t, SemanticEmbeddingSkipDimensionMismatch, result.SkipReason)
	require.Empty(t, result.Vector)
	require.Equal(t, 3, result.Dimension)
}

func TestSemanticEmbeddingClientGenerateEmbeddingHTTPFailureSkips(t *testing.T) {
	svc := newSemanticEmbeddingSettingService(t, 3)
	client := NewSemanticEmbeddingClient(svc)
	client.SetHTTPClient(semanticEmbeddingHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusUnauthorized,
			Body:       io.NopCloser(strings.NewReader(`{"error":"bad key"}`)),
		}, nil
	}))

	result, err := client.GenerateEmbedding(context.Background(), "hello")

	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Equal(t, SemanticEmbeddingSkipHTTPStatus, result.SkipReason)
	require.Equal(t, http.StatusUnauthorized, result.HTTPStatus)
}

func TestSemanticEmbeddingClientGenerateEmbeddingDisabledSkipsWithoutHTTP(t *testing.T) {
	repo := &cacheManagementSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	client := NewSemanticEmbeddingClient(svc)
	client.SetHTTPClient(semanticEmbeddingHTTPClientFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("HTTP client should not be called when semantic cache is disabled")
		return nil, nil
	}))

	result, err := client.GenerateEmbedding(context.Background(), "hello")

	require.NoError(t, err)
	require.True(t, result.Skipped)
	require.Equal(t, SemanticEmbeddingSkipDisabled, result.SkipReason)
}

func newSemanticEmbeddingSettingService(t *testing.T, dimension int) *SettingService {
	t.Helper()
	repo := &cacheManagementSettingRepoStub{}
	svc := NewSettingService(repo, &config.Config{})
	svc.SetSecretEncryptor(semanticCacheEncryptorStub{})
	cfg := validSemanticCacheConfig("sk-semantic-secret")
	cfg.Enabled = true
	cfg.SemanticModelBaseURL = "http://8.8.8.8/v1"
	cfg.EmbeddingDimension = &dimension
	_, err := svc.UpdateSemanticCacheConfig(context.Background(), cfg)
	require.NoError(t, err)
	return svc
}
