package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/config"
)

type openAIAnnouncementTranslator struct {
	endpoint string
	apiKey   string
	model    string
	client   *http.Client
}

func newAnnouncementTranslator(cfg config.AnnouncementTranslationConfig) AnnouncementTranslator {
	if !cfg.Enabled || strings.TrimSpace(cfg.BaseURL) == "" || strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil
	}
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	return &openAIAnnouncementTranslator{
		endpoint: announcementTranslationEndpoint(cfg.BaseURL),
		apiKey:   strings.TrimSpace(cfg.APIKey),
		model:    strings.TrimSpace(cfg.Model),
		client:   &http.Client{Timeout: timeout},
	}
}

func announcementTranslationEndpoint(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

func (t *openAIAnnouncementTranslator) Translate(
	ctx context.Context,
	sourceLocale string,
	targetLocale string,
	title string,
	content string,
) (AnnouncementTranslation, error) {
	payload := map[string]any{
		"model": t.model,
		"messages": []map[string]string{
			{
				"role":    "system",
				"content": "You translate product announcements. Return only a JSON object with string fields title and content. Preserve Markdown, URLs, code, placeholders, product names, numbers, and API identifiers exactly. Do not add commentary.",
			},
			{
				"role": "user",
				"content": fmt.Sprintf(
					"Translate from %s to %s.\n\nTitle:\n%s\n\nContent:\n%s",
					sourceLocale,
					targetLocale,
					title,
					content,
				),
			},
		},
		"temperature": 0.1,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AnnouncementTranslation{}, fmt.Errorf("marshal translation request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return AnnouncementTranslation{}, fmt.Errorf("create translation request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+t.apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return AnnouncementTranslation{}, fmt.Errorf("send translation request: %w", err)
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return AnnouncementTranslation{}, fmt.Errorf("read translation response: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return AnnouncementTranslation{}, fmt.Errorf("translation provider returned status %d", resp.StatusCode)
	}

	var completion struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(responseBody, &completion); err != nil {
		return AnnouncementTranslation{}, fmt.Errorf("decode translation response: %w", err)
	}
	if len(completion.Choices) == 0 {
		return AnnouncementTranslation{}, fmt.Errorf("translation provider returned no choices")
	}

	contentJSON := strings.TrimSpace(completion.Choices[0].Message.Content)
	if first := strings.Index(contentJSON, "{"); first >= 0 {
		if last := strings.LastIndex(contentJSON, "}"); last > first {
			contentJSON = contentJSON[first : last+1]
		}
	}
	var translated AnnouncementTranslation
	if err := json.Unmarshal([]byte(contentJSON), &translated); err != nil {
		return AnnouncementTranslation{}, fmt.Errorf("decode translated announcement: %w", err)
	}
	translated.Title = strings.TrimSpace(translated.Title)
	translated.Content = strings.TrimSpace(translated.Content)
	if translated.Title == "" || translated.Content == "" {
		return AnnouncementTranslation{}, fmt.Errorf("translation provider returned empty title or content")
	}
	return translated, nil
}
