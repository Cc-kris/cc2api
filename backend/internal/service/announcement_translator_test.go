package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIAnnouncementTranslator(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/v1/chat/completions", r.URL.Path)
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"{\"title\":\"Titre\",\"content\":\"Contenu **Markdown**\"}"}}]}`)
	}))
	defer server.Close()

	translator := newAnnouncementTranslator(config.AnnouncementTranslationConfig{
		Enabled:        true,
		BaseURL:        server.URL + "/v1",
		APIKey:         "secret",
		Model:          "translation-model",
		TimeoutSeconds: 5,
	})
	require.NotNil(t, translator)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	translated, err := translator.Translate(ctx, "zh", "fr", "标题", "正文")
	require.NoError(t, err)
	require.Equal(t, "Titre", translated.Title)
	require.Equal(t, "Contenu **Markdown**", translated.Content)
}

func TestAnnouncementTranslatorDisabledWithoutConfiguration(t *testing.T) {
	require.Nil(t, newAnnouncementTranslator(config.AnnouncementTranslationConfig{}))
}
