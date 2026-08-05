package service

import (
	"context"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/config"
	"github.com/stretchr/testify/require"
)

type announcementTranslationSettingRepoStub struct {
	values map[string]string
}

func (s *announcementTranslationSettingRepoStub) Get(context.Context, string) (*Setting, error) {
	return nil, ErrSettingNotFound
}

func (s *announcementTranslationSettingRepoStub) GetValue(_ context.Context, key string) (string, error) {
	value, ok := s.values[key]
	if !ok {
		return "", ErrSettingNotFound
	}
	return value, nil
}

func (s *announcementTranslationSettingRepoStub) Set(context.Context, string, string) error {
	return nil
}

func (s *announcementTranslationSettingRepoStub) GetMultiple(_ context.Context, keys []string) (map[string]string, error) {
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := s.values[key]; ok {
			out[key] = value
		}
	}
	return out, nil
}

func (s *announcementTranslationSettingRepoStub) SetMultiple(_ context.Context, values map[string]string) error {
	if s.values == nil {
		s.values = map[string]string{}
	}
	for key, value := range values {
		s.values[key] = value
	}
	return nil
}

func (s *announcementTranslationSettingRepoStub) GetAll(context.Context) (map[string]string, error) {
	out := make(map[string]string, len(s.values))
	for key, value := range s.values {
		out[key] = value
	}
	return out, nil
}

func (s *announcementTranslationSettingRepoStub) Delete(context.Context, string) error {
	return nil
}

func TestAnnouncementTranslationSettingsConfigFallbackAndDatabaseOverride(t *testing.T) {
	repo := &announcementTranslationSettingRepoStub{values: map[string]string{}}
	svc := NewSettingService(repo, &config.Config{AnnouncementTranslation: config.AnnouncementTranslationConfig{
		Enabled:        true,
		BaseURL:        "https://config.example.com/v1",
		APIKey:         "config-secret",
		Model:          "config-model",
		TimeoutSeconds: 90,
	}})

	got, err := svc.GetAnnouncementTranslationConfig(context.Background())
	require.NoError(t, err)
	require.Equal(t, "https://config.example.com/v1", got.BaseURL)
	require.Equal(t, "config-secret", got.APIKey)
	require.True(t, got.Enabled)

	repo.values = map[string]string{
		SettingKeyAnnouncementTranslationEnabled:        "false",
		SettingKeyAnnouncementTranslationBaseURL:        "https://database.example.com/v1",
		SettingKeyAnnouncementTranslationAPIKey:         "database-secret",
		SettingKeyAnnouncementTranslationModel:          "database-model",
		SettingKeyAnnouncementTranslationTimeoutSeconds: "45",
	}
	got, err = svc.GetAnnouncementTranslationConfig(context.Background())
	require.NoError(t, err)
	require.False(t, got.Enabled)
	require.Equal(t, "https://database.example.com/v1", got.BaseURL)
	require.Equal(t, "database-secret", got.APIKey)
	require.Equal(t, "database-model", got.Model)
	require.Equal(t, 45, got.TimeoutSeconds)
}

func TestAnnouncementServiceResolvesLatestTranslationSettingsPerJob(t *testing.T) {
	repo := &announcementTranslationSettingRepoStub{values: map[string]string{
		SettingKeyAnnouncementTranslationEnabled:        "true",
		SettingKeyAnnouncementTranslationBaseURL:        "https://translation.example.com/v1",
		SettingKeyAnnouncementTranslationAPIKey:         "database-secret",
		SettingKeyAnnouncementTranslationModel:          "database-model",
		SettingKeyAnnouncementTranslationTimeoutSeconds: "45",
	}}
	settingService := NewSettingService(repo, &config.Config{})
	announcementService := NewAnnouncementService(nil, nil, nil, nil, nil, settingService)

	require.IsType(t, &openAIAnnouncementTranslator{}, announcementService.resolveAnnouncementTranslator(context.Background()))

	repo.values[SettingKeyAnnouncementTranslationEnabled] = "false"
	require.Nil(t, announcementService.resolveAnnouncementTranslator(context.Background()))
}
