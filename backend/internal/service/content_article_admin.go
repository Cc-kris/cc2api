package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/contentmd"
	"github.com/Wei-Shaw/sub2api/internal/pkg/ctxkey"
	"github.com/gin-gonic/gin"
)

var (
	ErrContentArticleNotFound            = errors.New("content article not found")
	ErrContentArticleInvalid             = errors.New("invalid content article input")
	ErrContentArticleNotPublishable      = errors.New("content article is not publishable")
	ErrContentArticleConflict            = errors.New("content article version conflict")
	ErrContentTranslationNotFound        = errors.New("content translation not found")
	ErrContentWritingTaskNotFound        = errors.New("content writing task not found")
	ErrContentWritingExecutorMissing     = errors.New("content writing executor unavailable")
	ErrContentTranslationExecutorMissing = errors.New("content translation executor unavailable")
	ErrContentAIModelNotConfigured       = errors.New("model_not_configured")
	ErrContentAIModelUnavailable         = errors.New("model_not_available")
	ErrContentAIUnavailable              = errors.New("content AI provider unavailable")
	ErrContentSEOResultInvalid           = errors.New("generated SEO result is outside the allowed boundaries")
)

type ContentArticle struct {
	ID             int64      `json:"id"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	DeletedAt      *time.Time `json:"deletedAt,omitempty"`
	Locale         string     `json:"locale"`
	Slug           string     `json:"slug"`
	Title          string     `json:"title"`
	Excerpt        *string    `json:"excerpt,omitempty"`
	BodyMarkdown   *string    `json:"bodyMarkdown,omitempty"`
	BodyHTML       *string    `json:"bodyHtml,omitempty"`
	CoverImage     *string    `json:"coverImage,omitempty"`
	CoverAlt       *string    `json:"coverAlt,omitempty"`
	Category       *string    `json:"category,omitempty"`
	Tags           []string   `json:"tags,omitempty"`
	Status         string     `json:"status"`
	AuthorID       *int64     `json:"authorId,omitempty"`
	Model          *string    `json:"model,omitempty"`
	SeoTitle       *string    `json:"seoTitle,omitempty"`
	SeoDescription *string    `json:"seoDescription,omitempty"`
	CanonicalURL   *string    `json:"canonicalUrl,omitempty"`
	OgImage        *string    `json:"ogImage,omitempty"`
	PublishedAt    *time.Time `json:"publishedAt,omitempty"`
	FeaturedAt     *time.Time `json:"featuredAt,omitempty"`
	ReadCount      int        `json:"readCount"`
	Version        int        `json:"version"`
	// IsFeatured is derived from FeaturedAt, the database source of truth.
	IsFeatured bool `json:"isFeatured"`
}

type ContentArticleFilters struct {
	Status, Locale, Category, Search, Tags, Model string
	AuthorID                                      *int64
	From, To                                      *time.Time
	ReadMin, ReadMax                              *int
	IncludeDeleted                                bool
}

type ContentTranslation struct {
	ID               int64      `json:"id"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	DeletedAt        *time.Time `json:"deletedAt,omitempty"`
	ContentType      string     `json:"contentType"`
	ContentID        int64      `json:"contentId"`
	SourceVersion    int        `json:"sourceVersion"`
	SourceLocale     string     `json:"sourceLocale"`
	Locale           string     `json:"locale"`
	Slug             string     `json:"slug"`
	Title            string     `json:"title"`
	Excerpt          *string    `json:"excerpt,omitempty"`
	BodyMarkdown     *string    `json:"bodyMarkdown,omitempty"`
	BodyHTML         *string    `json:"bodyHtml,omitempty"`
	SeoTitle         *string    `json:"seoTitle,omitempty"`
	SeoDescription   *string    `json:"seoDescription,omitempty"`
	CanonicalURL     *string    `json:"canonicalUrl,omitempty"`
	TranslationModel *string    `json:"translationModel,omitempty"`
	FailureCode      *string    `json:"failureCode,omitempty"`
	Status           string     `json:"status"`
	ReviewedBy       *int64     `json:"reviewedBy,omitempty"`
	ReviewedAt       *time.Time `json:"reviewedAt,omitempty"`
	PublishedAt      *time.Time `json:"publishedAt,omitempty"`
}

type ContentTranslationInput struct {
	ContentType                                                                                            string
	ContentID                                                                                              int64
	SourceVersion                                                                                          int
	SourceLocale, Locale, Slug, Title                                                                      string
	Excerpt, BodyMarkdown, BodyHTML, SeoTitle, SeoDescription, CanonicalURL, TranslationModel, FailureCode *string
	Status                                                                                                 string
}

type ContentArticleRepository interface {
	ListArticles(context.Context, ContentArticleFilters, int, int) ([]ContentArticle, int, error)
	GetArticle(context.Context, int64, bool) (*ContentArticle, error)
	CreateArticle(context.Context, *ContentArticle) (*ContentArticle, error)
	UpdateArticle(context.Context, *ContentArticle, int) (*ContentArticle, error)
	SetArticleStatus(context.Context, int64, string, *time.Time) (*ContentArticle, error)
	SoftDeleteArticle(context.Context, int64, time.Time) error
	RestoreArticle(context.Context, int64) (*ContentArticle, error)
	ListTranslations(context.Context, string, int64, bool) ([]ContentTranslation, error)
	GetTranslation(context.Context, int64, bool) (*ContentTranslation, error)
	CreateTranslation(context.Context, *ContentTranslation) (*ContentTranslation, error)
	UpdateTranslation(context.Context, *ContentTranslation) (*ContentTranslation, error)
	SoftDeleteTranslation(context.Context, int64, time.Time) error
}

type ContentWritingTask struct {
	ID             string    `json:"id"`
	Kind           string    `json:"kind,omitempty"`
	ArticleID      int64     `json:"articleId,omitempty"`
	TranslationID  int64     `json:"translationId,omitempty"`
	SourceVersion  int       `json:"sourceVersion,omitempty"`
	SourceLocale   string    `json:"sourceLocale,omitempty"`
	TargetLocale   string    `json:"targetLocale,omitempty"`
	Topic          string    `json:"topic"`
	Audience       string    `json:"audience,omitempty"`
	Language       string    `json:"language,omitempty"`
	ContentType    string    `json:"contentType,omitempty"`
	Keywords       []string  `json:"keywords,omitempty"`
	WordCount      int       `json:"wordCount,omitempty"`
	Tone           string    `json:"tone,omitempty"`
	Facts          []string  `json:"facts,omitempty"`
	InternalLinks  []string  `json:"internalLinks,omitempty"`
	Model          string    `json:"model"`
	ProviderKeyRef string    `json:"providerKeyRef,omitempty"`
	Status         string    `json:"status"`
	Result         *string   `json:"result,omitempty"`
	ErrorCode      *string   `json:"errorCode,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Version        int64     `json:"version"`
}

const (
	ContentWritingQueued    = "queued"
	ContentWritingRunning   = "running"
	ContentWritingSucceeded = "succeeded"
	ContentWritingFailed    = "failed"
	ContentWritingCancelled = "cancelled"
	ContentWritingKind      = "writing"
	ContentTranslationKind  = "translation"
)

// ContentWritingInput is the structured brief used to build an article. It is
// deliberately separate from a rendered prompt so that prompts are never
// persisted or emitted to logs.
type ContentWritingInput struct {
	Topic          string
	Audience       string
	Language       string
	ContentType    string
	Keywords       []string
	WordCount      int
	Tone           string
	Facts          []string
	InternalLinks  []string
	Model          string
	ProviderKeyRef string
	ArticleID      int64
}

// ContentWritingResult is returned by an injected, real CCAI gateway adapter.
// The worker only persists these editorial fields into a draft article.
type ContentWritingResult struct {
	Slug           string `json:"slug"`
	Title          string `json:"title"`
	Excerpt        string `json:"excerpt"`
	BodyMarkdown   string `json:"body_markdown"`
	BodyHTML       string `json:"body_html"`
	SeoTitle       string `json:"seo_title"`
	SeoDescription string `json:"seo_description"`
}

// ContentWritingExecutor is implemented by the existing gateway integration.
// Keeping the boundary here prevents the content worker from inventing a
// provider call or storing credentials in the content module.
type ContentWritingExecutor interface {
	Generate(context.Context, *ContentWritingTask) (*ContentWritingResult, error)
}

type ContentSEOInput struct {
	Title        string
	BodyMarkdown string
	BodyHTML     string
}

type ContentSEOResult struct {
	Description string   `json:"description"`
	Keywords    []string `json:"keywords"`
	Model       string   `json:"model,omitempty"`
}

type ContentSEOExecutor interface {
	GenerateSEO(context.Context, string, ContentSEOInput) (*ContentSEOResult, error)
}

type ContentTranslationExecutor interface {
	Translate(context.Context, *ContentWritingTask, *ContentArticle) (*ContentTranslation, error)
}

// GatewayContentWritingExecutor bridges the content worker to the existing
// account scheduler and gateway. It intentionally accepts an account reference
// (not a raw API key); an empty reference uses normal model scheduling.

type ContentAIProviderConfig struct {
	ID           string   `json:"id,omitempty"`
	Label        string   `json:"label,omitempty"`
	Provider     string   `json:"provider,omitempty"`
	IsDefault    bool     `json:"isDefault,omitempty"`
	BaseURL      string   `json:"baseUrl"`
	APIKey       string   `json:"apiKey,omitempty"`
	EncryptedKey string   `json:"-"`
	APIKeyMasked string   `json:"apiKeyMasked,omitempty"`
	HasAPIKey    bool     `json:"hasApiKey"`
	DefaultModel string   `json:"defaultModel"`
	Models       []string `json:"models"`
	UpdatedAt    string   `json:"updatedAt,omitempty"`
}

var (
	contentAIMu         sync.RWMutex
	contentAIConfigPath = filepath.Join("data", "content_ai_provider.json")
)

func maskAPIKey(key string) string {
	k := strings.TrimSpace(key)
	if len(k) <= 8 {
		if k == "" {
			return ""
		}
		return "sk-****"
	}
	return k[:4] + "****" + k[len(k)-4:]
}

func contentAIStore() *ContentAIConfigStore {
	return NewContentAIConfigStore(contentAIConfigPath, contentAIEncryptorFromEnv())
}

func loadContentAIProviderConfigWithError() (ContentAIProviderConfig, error) {
	configs, defaultID, err := contentAIStore().LoadAll()
	if err != nil {
		return ContentAIProviderConfig{}, err
	}
	if len(configs) == 0 && defaultID == "" {
		return ContentAIProviderConfig{}, nil
	}
	for _, cfg := range configs {
		if cfg.ID == defaultID {
			cfg.IsDefault = true
			cfg.APIKeyMasked = maskAPIKey(cfg.APIKey)
			cfg.HasAPIKey = strings.TrimSpace(cfg.APIKey) != ""
			return cfg, nil
		}
	}
	return ContentAIProviderConfig{}, errors.New("default AI config not found")
}

func saveContentAIProviderConfig(cfg ContentAIProviderConfig) error {
	contentAIMu.Lock()
	defer contentAIMu.Unlock()
	store := contentAIStore()
	configs, defaultID, err := store.LoadAll()
	if err != nil {
		return err
	}
	if cfg.ID == "" {
		cfg.ID = defaultID
		if cfg.ID == "" {
			cfg.ID = fmt.Sprintf("config-%d", time.Now().UnixNano())
		}
	}
	found := false
	for i, existing := range configs {
		if existing.ID != cfg.ID {
			continue
		}
		found = true
		if strings.TrimSpace(cfg.APIKey) == "" || strings.Contains(cfg.APIKey, "****") {
			cfg.APIKey = existing.APIKey
		}
		if strings.TrimSpace(cfg.BaseURL) == "" {
			cfg.BaseURL = existing.BaseURL
		}
		if len(cfg.Models) == 0 {
			cfg.Models = existing.Models
		}
		if strings.TrimSpace(cfg.DefaultModel) == "" {
			cfg.DefaultModel = existing.DefaultModel
		}
		if cfg.Label == "" {
			cfg.Label = existing.Label
		}
		if cfg.Provider == "" {
			cfg.Provider = existing.Provider
		}
		configs[i] = cfg
		break
	}
	if !found {
		if cfg.Label == "" {
			cfg.Label = cfg.ID
		}
		if cfg.Provider == "" {
			cfg.Provider = "custom"
		}
		configs = append(configs, cfg)
	}
	cfg.BaseURL = strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultContentAIConfig().BaseURL
	}
	if cfg.DefaultModel == "" && len(cfg.Models) > 0 {
		cfg.DefaultModel = cfg.Models[0]
	}
	cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	for i := range configs {
		if configs[i].ID == cfg.ID {
			configs[i] = cfg
			break
		}
	}
	if cfg.IsDefault || defaultID == "" {
		defaultID = cfg.ID
	}
	if err := store.SaveAll(configs, defaultID); err != nil {
		return err
	}
	return nil
}

func (s *ContentArticleAdminService) GetAIProviderConfig(ctx context.Context) (ContentAIProviderConfig, error) {
	cfg, err := loadContentAIProviderConfigWithError()
	if err != nil {
		return ContentAIProviderConfig{}, err
	}
	// Auto-hydrate API key from active database account if empty
	if strings.TrimSpace(cfg.APIKey) == "" && s.executor != nil {
		if gwExec, ok := s.executor.(*GatewayContentWritingExecutor); ok && gwExec.gateway != nil && gwExec.gateway.accountRepo != nil {
			if accounts, err := gwExec.gateway.accountRepo.ListSchedulable(ctx); err == nil {
				for _, acc := range accounts {
					k := strings.TrimSpace(acc.GetCredential("api_key"))
					if k != "" {
						cfg.APIKey = k
						cfg.APIKeyMasked = maskAPIKey(k)
						cfg.HasAPIKey = true
						if u := strings.TrimRight(strings.TrimSpace(acc.GetOpenAIBaseURL()), "/"); u != "" {
							cfg.BaseURL = u
						}
						if err := saveContentAIProviderConfig(cfg); err != nil {
							return ContentAIProviderConfig{}, err
						}
						break
					}
				}
			}
		}
	}
	return cfg, nil
}

func (s *ContentArticleAdminService) UpdateAIProviderConfig(ctx context.Context, input ContentAIProviderConfig) (ContentAIProviderConfig, error) {
	if err := saveContentAIProviderConfig(input); err != nil {
		return ContentAIProviderConfig{}, err
	}
	return s.GetAIProviderConfig(ctx)
}

// AIConfigRevision returns a stable optimistic concurrency token for the current config set.
func aiConfigHash(configs []ContentAIProviderConfig, def string) string {
	for i := range configs {
		configs[i].APIKey = ""
		configs[i].APIKeyMasked = ""
		configs[i].HasAPIKey = false
	}
	b, _ := json.Marshal(struct {
		Configs []ContentAIProviderConfig `json:"configs"`
		Default string                    `json:"default"`
	}{configs, def})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func ListAIProviderConfigs() ([]ContentAIProviderConfig, string, string, error) {
	configs, def, err := contentAIStore().LoadAll()
	if err != nil {
		return nil, "", "", err
	}
	for i := range configs {
		configs[i].IsDefault = configs[i].ID == def
		configs[i].APIKey = ""
	}
	return configs, def, aiConfigHash(configs, def), nil
}
func (s *ContentArticleAdminService) MutateAIProviderConfig(ctx context.Context, action, id, revision string, input ContentAIProviderConfig) error {
	contentAIMu.Lock()
	defer contentAIMu.Unlock()
	configs, def, err := contentAIStore().LoadAll()
	if err != nil {
		return err
	}
	if revision == "" || revision != aiConfigHash(configs, def) {
		return fmt.Errorf("AI config revision conflict")
	}
	switch action {
	case "save":
		if strings.TrimSpace(input.Label) == "" || len([]rune(input.Label)) > 50 {
			return fmt.Errorf("invalid configuration label")
		}
		if input.ID == "" {
			input.ID = fmt.Sprintf("cfg-%d", time.Now().UnixNano())
		}
		found := false
		for i, c := range configs {
			if c.ID == input.ID {
				if input.APIKey == "" {
					input.APIKey = c.APIKey
				}
				configs[i] = input
				found = true
			} else if strings.EqualFold(strings.TrimSpace(c.Label), strings.TrimSpace(input.Label)) {
				return fmt.Errorf("duplicate configuration label")
			}
		}
		if !found {
			configs = append(configs, input)
		}
		if input.IsDefault || def == "" {
			def = input.ID
		}
	case "default":
		found := false
		for _, c := range configs {
			if c.ID == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("configuration not found")
		}
		def = id
	case "delete":
		if id == def {
			return fmt.Errorf("model_in_use: default configuration cannot be deleted")
		}
		for _, c := range configs {
			if c.ID == id {
				for _, t := range []string{"writing", "translation"} {
					tasks, e := s.ListWritingTasks(ctx, t, 200)
					if e != nil {
						return e
					}
					for _, task := range tasks {
						if task.ProviderKeyRef == id && (task.Status == ContentWritingQueued || task.Status == ContentWritingRunning) {
							return fmt.Errorf("model_in_use")
						}
					}
				}
				filtered := configs[:0]
				for _, cfg := range configs {
					if cfg.ID != id {
						filtered = append(filtered, cfg)
					}
				}
				configs = filtered
				goto deleted
			}
		}
		return fmt.Errorf("configuration not found")
	deleted:
	}
	return contentAIStore().SaveAll(configs, def)
}

// TestAIProviderConnection performs a non-persistent minimal models request.
func TestAIProviderConnection(ctx context.Context, baseURL, key string) (map[string]any, error) {
	start := time.Now()
	u := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if u == "" {
		return nil, fmt.Errorf("baseUrl required")
	}
	req, e := http.NewRequestWithContext(ctx, http.MethodGet, u+"/models", nil)
	if e != nil {
		return nil, e
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	client := http.Client{Timeout: 8 * time.Second}
	resp, e := client.Do(req)
	elapsed := time.Since(start).Milliseconds()
	if e != nil {
		return map[string]any{"ok": false, "latencyMs": elapsed, "error": e.Error()}, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return map[string]any{"ok": false, "latencyMs": elapsed, "error": resp.Status}, nil
	}
	return map[string]any{"ok": true, "latencyMs": elapsed}, nil
}
func (s *ContentArticleAdminService) FetchAIProviderModels(ctx context.Context, reqBaseURL, reqAPIKey string) ([]string, ContentAIProviderConfig, error) {
	current, err := loadContentAIProviderConfigWithError()
	if err != nil {
		return nil, ContentAIProviderConfig{}, err
	}
	baseURL := strings.TrimRight(strings.TrimSpace(reqBaseURL), "/")
	if baseURL == "" {
		baseURL = current.BaseURL
	}
	if baseURL == "" {
		baseURL = "https://cc-ai.xyz/v1"
	}
	apiKey := strings.TrimSpace(reqAPIKey)
	if apiKey == "" || strings.Contains(apiKey, "****") {
		apiKey = current.APIKey
	}

	// 1. Try standard /v1/models endpoint
	modelsURL := baseURL + "/models"
	if !strings.HasSuffix(baseURL, "/v1") && !strings.Contains(baseURL, "/v1/") {
		modelsURL = baseURL + "/v1/models"
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, modelsURL, nil)
	var fetchedModels []string
	if err == nil {
		if apiKey != "" {
			httpReq.Header.Set("Authorization", "Bearer "+apiKey)
		}
		httpReq.Header.Set("User-Agent", "CCAI-Content-Admin/1.0")
		client := &http.Client{Timeout: 12 * time.Second}
		resp, reqErr := client.Do(httpReq)
		if reqErr == nil && resp != nil {
			defer func() { _ = resp.Body.Close() }()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
				var parsed struct {
					Data []struct {
						ID string `json:"id"`
					} `json:"data"`
					Models []string `json:"models"`
				}
				if json.Unmarshal(body, &parsed) == nil {
					for _, m := range parsed.Data {
						id := strings.TrimSpace(m.ID)
						if id != "" {
							fetchedModels = append(fetchedModels, id)
						}
					}
					for _, m := range parsed.Models {
						id := strings.TrimSpace(m)
						if id != "" && !slicesContains(fetchedModels, id) {
							fetchedModels = append(fetchedModels, id)
						}
					}
				}
			}
		}
	}

	// Fallback/enrich with standard high-value models if remote returned none
	if len(fetchedModels) == 0 {
		fetchedModels = []string{
			"claude-opus-5.5",
			"claude-sonnet-4.6",
			"claude-haiku-4.5",
			"gpt-5.4",
			"gpt-5.3-codex",
			"grok-4.7",
			"deepseek-reasoner",
			"deepseek-chat",
		}
	}
	sort.Strings(fetchedModels)

	// Save configuration with new models
	updated := current
	updated.BaseURL = baseURL
	if apiKey != "" {
		updated.APIKey = apiKey
	}
	updated.Models = fetchedModels
	if !slicesContains(fetchedModels, updated.DefaultModel) && len(fetchedModels) > 0 {
		updated.DefaultModel = fetchedModels[0]
	}
	if err := saveContentAIProviderConfig(updated); err != nil {
		return nil, ContentAIProviderConfig{}, err
	}

	resultCfg, err := loadContentAIProviderConfigWithError()
	if err != nil {
		return nil, ContentAIProviderConfig{}, err
	}
	return fetchedModels, resultCfg, nil
}

type ContentTaxonomyConfig struct {
	ArticleCategories []string `json:"articleCategories"`
	NewsCategories    []string `json:"newsCategories"`
	Tags              []string `json:"tags"`
	UpdatedAt         string   `json:"updatedAt,omitempty"`
}

var (
	contentTaxonomyMu   sync.RWMutex
	contentTaxonomyPath = filepath.Join("data", "content_taxonomy.json")
)

func loadContentTaxonomyConfig() ContentTaxonomyConfig {
	contentTaxonomyMu.Lock()
	defer contentTaxonomyMu.Unlock()

	data, err := os.ReadFile(contentTaxonomyPath)
	if err == nil && len(data) > 0 {
		var loaded ContentTaxonomyConfig
		if json.Unmarshal(data, &loaded) == nil {
			if loaded.ArticleCategories == nil {
				loaded.ArticleCategories = []string{}
			}
			if loaded.NewsCategories == nil {
				loaded.NewsCategories = []string{}
			}
			if loaded.Tags == nil {
				loaded.Tags = []string{}
			}
			return loaded
		}
	}

	def := ContentTaxonomyConfig{
		ArticleCategories: []string{"架构与网关", "客户端接入", "模型评测与路由", "成本与高可用", "安全与合规"},
		NewsCategories:    []string{"模型发布", "工程架构", "产品与生态", "开发者工具", "行业动态"},
		Tags:              []string{"CCAI", "API网关", "Claude Code", "Codex CLI", "Prompt Caching", "高可用", "OpenAI", "多模型路由"},
		UpdatedAt:         time.Now().UTC().Format(time.RFC3339),
	}
	_ = os.MkdirAll(filepath.Dir(contentTaxonomyPath), 0755)
	if raw, mErr := json.MarshalIndent(def, "", "  "); mErr == nil {
		_ = os.WriteFile(contentTaxonomyPath, raw, 0600)
	}
	return def
}

func saveContentTaxonomyConfig(cfg ContentTaxonomyConfig) error {
	contentTaxonomyMu.Lock()
	defer contentTaxonomyMu.Unlock()

	cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if cfg.ArticleCategories == nil {
		cfg.ArticleCategories = []string{}
	}
	if cfg.NewsCategories == nil {
		cfg.NewsCategories = []string{}
	}
	if cfg.Tags == nil {
		cfg.Tags = []string{}
	}
	_ = os.MkdirAll(filepath.Dir(contentTaxonomyPath), 0755)
	raw, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(contentTaxonomyPath, raw, 0600)
}

func (s *ContentArticleAdminService) GetTaxonomy(ctx context.Context) (ContentTaxonomyConfig, error) {
	return loadContentTaxonomyConfig(), nil
}

const contentTaxonomyArticlePageSize = 200

// listAllArticlesForTaxonomyMutation snapshots every article before any
// category/tag update changes UpdatedAt and therefore the repository order.
func (s *ContentArticleAdminService) listAllArticlesForTaxonomyMutation(ctx context.Context) ([]ContentArticle, error) {
	if s.repo == nil {
		return nil, nil
	}
	all := make([]ContentArticle, 0)
	page := 1
	total := 0
	for {
		items, pageTotal, err := s.repo.ListArticles(ctx, ContentArticleFilters{IncludeDeleted: true}, page, contentTaxonomyArticlePageSize)
		if err != nil {
			return nil, err
		}
		if page == 1 || pageTotal > total {
			total = pageTotal
		}
		all = append(all, items...)
		if len(items) == 0 || len(items) < contentTaxonomyArticlePageSize || (total > 0 && len(all) >= total) {
			break
		}
		page++
	}
	return all, nil
}

func isArticleFeatured(a *ContentArticle, _ []int64) bool {
	return a != nil && a.FeaturedAt != nil
}

// decorateFeatured exposes the persisted featured state on admin payloads.
func (s *ContentArticleAdminService) decorateFeatured(a *ContentArticle) {
	if a == nil {
		return
	}
	a.IsFeatured = isArticleFeatured(a, nil)
}

func (s *ContentArticleAdminService) MutateTaxonomy(ctx context.Context, scope, action, name, newName string) (ContentTaxonomyConfig, error) {
	cfg := loadContentTaxonomyConfig()
	cleanScope := strings.TrimSpace(scope)
	cleanAction := strings.TrimSpace(action)
	cleanName := strings.TrimSpace(name)
	cleanNew := strings.TrimSpace(newName)
	if cleanName == "" {
		return cfg, errors.New("name is required")
	}
	if cleanScope == "featured_article" {
		if cleanAction != "set" && cleanAction != "unset" {
			return cfg, errors.New("invalid featured_article action: must be set or unset")
		}
	} else {
		if cleanAction != "add" && cleanAction != "rename" && cleanAction != "delete" {
			return cfg, errors.New("invalid taxonomy action")
		}
		if cleanAction == "rename" && cleanNew == "" {
			return cfg, errors.New("newName is required for rename")
		}
	}

	mutateSlice := func(list []string) []string {
		switch cleanAction {
		case "add":
			for _, item := range list {
				if item == cleanName {
					return list
				}
			}
			return append(list, cleanName)
		case "rename":
			if cleanNew == "" {
				return list
			}
			out := make([]string, 0, len(list))
			found := false
			for _, item := range list {
				if item == cleanName {
					if !slicesContains(out, cleanNew) {
						out = append(out, cleanNew)
					}
					found = true
				} else if !slicesContains(out, item) {
					out = append(out, item)
				}
			}
			if !found && !slicesContains(out, cleanNew) {
				out = append(out, cleanNew)
			}
			return out
		case "delete":
			out := make([]string, 0, len(list))
			for _, item := range list {
				if item != cleanName {
					out = append(out, item)
				}
			}
			return out
		}
		return list
	}

	switch cleanScope {
	case "featured_article":
		artID, parseErr := strconv.ParseInt(cleanName, 10, 64)
		if parseErr != nil || artID <= 0 {
			return cfg, errors.New("invalid article id for featured_article")
		}
		if s.repo == nil {
			return cfg, errors.New("article repository unavailable")
		}
		art, getErr := s.repo.GetArticle(ctx, artID, false)
		if getErr != nil {
			return cfg, fmt.Errorf("get article for featured mutation: %w", getErr)
		}
		if art == nil {
			return cfg, ErrContentArticleNotFound
		}
		if cleanAction == "set" && art.Status != domain.ContentStatusPublished {
			return cfg, errors.New("only published articles can be featured")
		}
		artCopy := *art
		if cleanAction == "set" {
			now := time.Now().UTC()
			artCopy.FeaturedAt = &now
		} else {
			artCopy.FeaturedAt = nil
		}
		// FeaturedAt is the only source of truth. Remove the legacy marker
		// from the article and taxonomy snapshot whenever its featured state is
		// explicitly reconciled.
		artCopy.Tags = make([]string, 0, len(art.Tags))
		for _, tag := range art.Tags {
			if strings.TrimSpace(tag) != "精选" {
				artCopy.Tags = append(artCopy.Tags, tag)
			}
		}
		if _, updateErr := s.repo.UpdateArticle(ctx, &artCopy, art.Version); updateErr != nil {
			return cfg, fmt.Errorf("update article featured state: %w", updateErr)
		}
		InvalidatePublicContentCache()
	case "article_category":
		cfg.ArticleCategories = mutateSlice(cfg.ArticleCategories)
		if (cleanAction == "rename" || cleanAction == "delete") && s.repo != nil {
			replacement := cleanNew
			if cleanAction == "delete" {
				if len(cfg.ArticleCategories) > 0 {
					replacement = cfg.ArticleCategories[0]
				} else {
					replacement = "未分类"
				}
			}
			items, err := s.listAllArticlesForTaxonomyMutation(ctx)
			if err != nil {
				return cfg, fmt.Errorf("list articles for category mutation: %w", err)
			}
			for _, art := range items {
				catVal := ""
				if art.Category != nil {
					catVal = strings.TrimSpace(*art.Category)
				}
				if catVal == cleanName {
					artCopy := art
					repCopy := replacement
					artCopy.Category = &repCopy
					if _, updateErr := s.repo.UpdateArticle(ctx, &artCopy, art.Version); updateErr != nil {
						return cfg, fmt.Errorf("update article category: %w", updateErr)
					}
				}
			}
		}
	case "news_category":
		cfg.NewsCategories = mutateSlice(cfg.NewsCategories)
	case "tag":
		cfg.Tags = mutateSlice(cfg.Tags)
		if (cleanAction == "rename" || cleanAction == "delete") && s.repo != nil {
			items, err := s.listAllArticlesForTaxonomyMutation(ctx)
			if err != nil {
				return cfg, fmt.Errorf("list articles for tag mutation: %w", err)
			}
			for _, art := range items {
				changed := false
				newTags := make([]string, 0, len(art.Tags))
				for _, tg := range art.Tags {
					if strings.TrimSpace(tg) == cleanName {
						changed = true
						if cleanAction == "rename" && !slicesContains(newTags, cleanNew) {
							newTags = append(newTags, cleanNew)
						}
					} else if !slicesContains(newTags, tg) {
						newTags = append(newTags, tg)
					}
				}
				if changed {
					artCopy := art
					artCopy.Tags = newTags
					if _, updateErr := s.repo.UpdateArticle(ctx, &artCopy, art.Version); updateErr != nil {
						return cfg, fmt.Errorf("update article tags: %w", updateErr)
					}
				}
			}
		}
	default:
		return cfg, errors.New("invalid taxonomy scope")
	}

	if err := saveContentTaxonomyConfig(cfg); err != nil {
		return cfg, err
	}
	InvalidatePublicContentCache()
	return cfg, nil
}

func slicesContains(list []string, item string) bool {
	for _, v := range list {
		if v == item {
			return true
		}
	}
	return false
}

type GatewayContentWritingExecutor struct {
	gateway *GatewayService
	groupID *int64
}

func NewGatewayContentWritingExecutor(gateway *GatewayService, groupID *int64) *GatewayContentWritingExecutor {
	return &GatewayContentWritingExecutor{gateway: gateway, groupID: groupID}
}

func (e *GatewayContentWritingExecutor) GenerateSEO(ctx context.Context, model string, input ContentSEOInput) (*ContentSEOResult, error) {
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, ErrContentAIModelNotConfigured
	}
	cfg, err := loadContentAIProviderConfigWithError()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.DefaultModel) == "" || !strings.EqualFold(strings.TrimSpace(cfg.DefaultModel), model) {
		return nil, ErrContentAIModelNotConfigured
	}
	if strings.TrimSpace(cfg.APIKey) == "" || strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, ErrContentAIUnavailable
	}
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.BaseURL), "/")
	if strings.HasSuffix(endpoint, "/v1") {
		endpoint += "/chat/completions"
	} else if !strings.HasSuffix(endpoint, "/chat/completions") {
		endpoint += "/v1/chat/completions"
	}
	prompt := buildContentSEOPrompt(input)
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		raw, err := doContentAIChatRequest(ctx, endpoint, cfg.APIKey, model, prompt, contentSEOSystemPrompt)
		if err != nil {
			return nil, err
		}
		result, parseErr := parseContentSEOResult(raw)
		if parseErr == nil && validateContentSEOResult(result) == nil {
			result.Model = model
			return result, nil
		}
		if attempt == 0 {
			prompt += "\n上次结果未通过校验，请重新生成；描述必须严格为 120-160 个 Unicode 字符，关键词必须为 5-8 个，并按 JSON 格式输出。"
		}
	}
	return nil, ErrContentSEOResultInvalid
}

func buildContentSEOPrompt(input ContentSEOInput) string {
	return fmt.Sprintf("为以下文章生成 SEO 描述与关键词。只允许输出 JSON：{\\\"description\\\":\\\"...\\\",\\\"keywords\\\":[\\\"...\\\"]}。描述控制在 120-160 个 Unicode 字符，关键词 5-8 个。标题：%s\\n正文：%s\\n%s", strings.TrimSpace(input.Title), strings.TrimSpace(input.BodyMarkdown), strings.TrimSpace(input.BodyHTML))
}

func parseContentSEOResult(raw []byte) (*ContentSEOResult, error) {
	content, err := extractContentAIResponse(raw)
	if err != nil {
		return nil, err
	}
	var result ContentSEOResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return nil, errors.New("invalid generated SEO JSON")
	}
	return &result, nil
}

func validateContentSEOResult(result *ContentSEOResult) error {
	if result == nil {
		return ErrContentSEOResultInvalid
	}
	description := strings.TrimSpace(result.Description)
	if length := len([]rune(description)); length < 120 || length > 160 {
		return ErrContentSEOResultInvalid
	}
	keywords := make([]string, 0, len(result.Keywords))
	seen := make(map[string]struct{}, len(result.Keywords))
	for _, keyword := range result.Keywords {
		keyword = strings.TrimSpace(keyword)
		key := strings.ToLower(keyword)
		if keyword == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keywords = append(keywords, keyword)
	}
	if len(keywords) < 5 || len(keywords) > 8 {
		return ErrContentSEOResultInvalid
	}
	result.Description = description
	result.Keywords = keywords
	return nil
}

func doContentAIChatRequest(ctx context.Context, endpoint, apiKey, model, prompt, systemPrompt string) ([]byte, error) {
	payload := map[string]any{"model": model, "max_tokens": 1200, "messages": []map[string]string{{"role": "system", "content": systemPrompt}, {"role": "user", "content": prompt}}, "stream": false}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := (&http.Client{Timeout: 65 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream AI status %d", resp.StatusCode)
	}
	return raw, nil
}

func extractContentAIResponse(raw []byte) (string, error) {
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return "", err
	}
	var content string
	if len(envelope.Choices) > 0 {
		content = strings.TrimSpace(envelope.Choices[0].Message.Content)
	}
	if content == "" && len(envelope.Content) > 0 {
		content = strings.TrimSpace(envelope.Content[0].Text)
	}
	content = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(content, "```json"), "```"))
	if start, end := strings.IndexByte(content, '{'), strings.LastIndexByte(content, '}'); start >= 0 && end > start {
		content = content[start : end+1]
	}
	if content == "" {
		return "", errors.New("empty AI response")
	}
	return content, nil
}

func (e *GatewayContentWritingExecutor) Generate(ctx context.Context, task *ContentWritingTask) (*ContentWritingResult, error) {
	if e == nil || task == nil {
		return nil, ErrContentWritingExecutorMissing
	}

	prompt := buildContentWritingPrompt(task)

	// Direct priority: if standalone AI Provider config is present, use it directly!
	cfg, err := loadContentAIProviderConfigWithError()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.BaseURL) != "" && strings.TrimSpace(cfg.APIKey) != "" {
		endpoint := strings.TrimRight(cfg.BaseURL, "/")
		if strings.HasSuffix(endpoint, "/v1") {
			endpoint = endpoint + "/chat/completions"
		} else if !strings.HasSuffix(endpoint, "/chat/completions") {
			endpoint = endpoint + "/v1/chat/completions"
		}
		rawResp, reqErr := e.doOpenAIChatRequest(ctx, endpoint, cfg.APIKey, task.Model, prompt)
		if reqErr == nil {
			if res, parseErr := parseContentWritingResult(rawResp); parseErr == nil {
				return res, nil
			}
		}
	}

	account, err := e.selectAccount(ctx, task)
	if err != nil {
		return nil, err
	}
	prompt = buildContentWritingPrompt(task)
	if account != nil && strings.EqualFold(strings.TrimSpace(account.Platform), PlatformOpenAI) && strings.TrimSpace(account.GetCredential("api_key")) != "" {
		rawResp, err := e.forwardOpenAICompatible(ctx, account, task.Model, prompt)
		if err == nil {
			return parseContentWritingResult(rawResp)
		}
	}
	payload := map[string]any{
		"model":      task.Model,
		"max_tokens": 4096,
		"system":     contentWritingSystemPrompt,
		"messages":   []map[string]string{{"role": "user", "content": prompt}},
		"stream":     false,
	}
	if account == nil || strings.EqualFold(strings.TrimSpace(account.Platform), PlatformOpenAI) {
		payload = map[string]any{
			"model":    task.Model,
			"messages": []map[string]string{{"role": "system", "content": contentWritingSystemPrompt}, {"role": "user", "content": prompt}},
			"stream":   false,
		}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	parsed := &ParsedRequest{Body: body, Model: task.Model, Stream: false, GroupID: e.groupID}
	if _, err = e.gateway.Forward(ctx, c, account, parsed); err != nil {
		return nil, err
	}
	return parseContentWritingResult(recorder.Body.Bytes())
}

func (e *GatewayContentWritingExecutor) forwardOpenAICompatible(ctx context.Context, account *Account, model, prompt string) ([]byte, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(account.GetOpenAIBaseURL()), "/")
	if baseURL == "" || strings.Contains(baseURL, "api.cc-ai.xyz") {
		baseURL = "https://cc-ai.xyz/v1"
	}
	endpoint := baseURL + "/v1/chat/completions"
	if strings.HasSuffix(baseURL, "/chat/completions") {
		endpoint = baseURL
	} else if strings.HasSuffix(baseURL, "/v1") {
		endpoint = baseURL + "/chat/completions"
	}
	mappedModel := strings.TrimSpace(account.GetMappedModel(model))
	if mappedModel == "" {
		mappedModel = strings.TrimSpace(model)
	}
	candidates := []string{mappedModel}
	if strings.Contains(strings.ToLower(account.Name), "grok") && !strings.HasPrefix(strings.ToLower(mappedModel), "grok") {
		candidates = []string{"grok-4.7", mappedModel}
	} else if mappedModel != "grok-4.7" {
		candidates = append(candidates, "grok-4.7")
	}
	var lastErr error
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		raw, err := e.doOpenAIChatRequest(ctx, endpoint, strings.TrimSpace(account.GetOpenAIApiKey()), candidate, prompt)
		if err == nil {
			return raw, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (e *GatewayContentWritingExecutor) doOpenAIChatRequest(ctx context.Context, endpoint, apiKey, model, prompt string) ([]byte, error) {
	payload := map[string]any{
		"model":      model,
		"max_tokens": 2000,
		"messages":   []map[string]string{{"role": "system", "content": contentWritingSystemPrompt}, {"role": "user", "content": prompt}},
		"stream":     false,
	}
	if strings.HasPrefix(strings.ToLower(model), "grok") {
		payload["reasoning_effort"] = "low"
	}
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 65 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("upstream openai status %d", resp.StatusCode)
	}
	return raw, nil
}

func (e *GatewayContentWritingExecutor) Translate(ctx context.Context, task *ContentWritingTask, article *ContentArticle) (*ContentTranslation, error) {
	if task == nil || article == nil {
		return nil, ErrContentTranslationExecutorMissing
	}
	copyTask := *task
	copyTask.Topic = "将以下 CCAI 文章翻译为 " + task.TargetLocale + "，保留 Markdown 结构和代码块：\n标题：" + article.Title + "\n摘要：" + derefContentString(article.Excerpt) + "\n正文：" + derefContentString(article.BodyMarkdown)
	copyTask.Language = task.TargetLocale
	result, err := e.Generate(ctx, &copyTask)
	if err != nil {
		return nil, err
	}
	body := result.BodyMarkdown
	if strings.TrimSpace(body) == "" && strings.TrimSpace(result.BodyHTML) != "" {
		body = result.BodyHTML
	}
	return &ContentTranslation{ContentType: "article", ContentID: article.ID, SourceVersion: article.Version, SourceLocale: article.Locale, Locale: task.TargetLocale, Slug: result.Slug, Title: result.Title, Excerpt: stringPtr(result.Excerpt), BodyMarkdown: stringPtr(body), SeoTitle: stringPtr(result.SeoTitle), SeoDescription: stringPtr(result.SeoDescription), TranslationModel: stringPtr(task.Model), Status: domain.ContentTranslationDraft}, nil
}

func derefContentString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func (e *GatewayContentWritingExecutor) selectAccount(ctx context.Context, task *ContentWritingTask) (*Account, error) {
	if task != nil {
		keyRef := strings.TrimSpace(task.ProviderKeyRef)
		if keyRef != "" {
			id, parseErr := strconv.ParseInt(keyRef, 10, 64)
			if parseErr != nil || id <= 0 {
				return nil, errors.New("provider key reference must be a valid account id")
			}
			// The gateway itself owns account hydration and credential handling.
			// A numeric reference is passed through the group selector only when
			// the scheduler has no explicit account API; never treat it as a key.
			if e.gateway.accountRepo != nil {
				if account, getErr := e.gateway.accountRepo.GetByID(ctx, id); getErr != nil {
					return nil, getErr
				} else if account != nil {
					return account, nil
				}
				return nil, errors.New("provider account not found")
			}
			return nil, errors.New("provider account repository unavailable")
		}
	}
	account, err := e.gateway.SelectAccountForModel(ctx, e.groupID, "ccai-content", task.Model)
	if err == nil && account != nil {
		return account, nil
	}
	if e.gateway.accountRepo != nil {
		if accounts, listErr := e.gateway.accountRepo.ListSchedulable(ctx); listErr == nil {
			for i := range accounts {
				if accounts[i].IsActive() && strings.TrimSpace(accounts[i].GetCredential("api_key")) != "" {
					return &accounts[i], nil
				}
			}
			if len(accounts) > 0 {
				return &accounts[0], nil
			}
		}
		if fallbackAcc, getErr := e.gateway.accountRepo.GetByID(ctx, 1); getErr == nil && fallbackAcc != nil {
			return fallbackAcc, nil
		}
	}
	return account, err
}

func buildContentWritingPrompt(task *ContentWritingTask) string {
	var b strings.Builder
	targetWords := task.WordCount
	if targetWords <= 0 || targetWords > 650 {
		targetWords = 550
	}
	fmt.Fprintf(&b, "题材：%s\n", task.Topic)
	fmt.Fprintf(&b, "受众：%s\n语言：%s\n分类：%s\n", task.Audience, defaultContentLanguage(task.Language), task.ContentType)
	fmt.Fprintf(&b, "关键词：%s\n精炼篇幅：%d字左右\n语气：%s\n", strings.Join(task.Keywords, ", "), targetWords, task.Tone)
	if len(task.Facts) > 0 || len(task.InternalLinks) > 0 {
		fmt.Fprintf(&b, "已确认事实：%s\n内部链接候选：%s\n", strings.Join(task.Facts, "；"), strings.Join(task.InternalLinks, "；"))
	}
	b.WriteString("请按照最佳技术 SEO 内容架构撰写（包含：3-4个 H2 章节、1段实用配置或代码示例、常见问题 FAQ），并自动生成符合 Google SEO 规范的英文短横线 slug、50-60 字符 seo_title 和 120-155 字符 seo_description。\n")
	b.WriteString("请直接输出合法 JSON（不要代码块包裹）：{\"slug\":\"...\",\"title\":\"...\",\"excerpt\":\"...\",\"body_markdown\":\"...\",\"seo_title\":\"...\",\"seo_description\":\"...\"}。")
	return b.String()
}

const contentWritingSystemPrompt = "你是 CCAI 的资深技术主编与 SEO 架构专家。不要长篇推理思考，立即直接输出合法的 JSON 对象（包含 slug, title, excerpt, body_markdown, seo_title, seo_description），不要输出 JSON 以外的任何文字。"
const contentSEOSystemPrompt = "你是 SEO 编辑助手。只输出 JSON 对象，包含 description 字符串和 keywords 字符串数组。description 为 120-160 个 Unicode 字符，keywords 为 5-8 个不重复关键词。"

func parseContentWritingResult(raw []byte) (*ContentWritingResult, error) {
	var envelope struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, errors.New("invalid gateway content response")
	}
	var content string
	if len(envelope.Choices) > 0 {
		content = strings.TrimSpace(envelope.Choices[0].Message.Content)
	} else if len(envelope.Content) > 0 {
		for _, block := range envelope.Content {
			if strings.TrimSpace(block.Text) != "" {
				content = strings.TrimSpace(block.Text)
				break
			}
		}
	}
	if content == "" {
		return nil, errors.New("invalid gateway content response")
	}
	content = strings.TrimPrefix(content, "```json")
	content = strings.TrimPrefix(content, "```JSON")
	content = strings.TrimPrefix(content, "```")
	content = strings.TrimSuffix(strings.TrimSpace(content), "```")
	content = strings.TrimSpace(content)
	if start := strings.IndexByte(content, '{'); start >= 0 {
		if end := strings.LastIndexByte(content, '}'); end > start {
			content = content[start : end+1]
		}
	}
	var result ContentWritingResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return salvageContentWritingResult(content)
	}
	var camelFallback struct {
		BodyMarkdown   string `json:"bodyMarkdown"`
		BodyHTML       string `json:"bodyHtml"`
		SeoTitle       string `json:"seoTitle"`
		SeoDescription string `json:"seoDescription"`
	}
	_ = json.Unmarshal([]byte(content), &camelFallback)
	if strings.TrimSpace(result.BodyMarkdown) == "" && strings.TrimSpace(camelFallback.BodyMarkdown) != "" {
		result.BodyMarkdown = camelFallback.BodyMarkdown
	}
	if strings.TrimSpace(result.BodyHTML) == "" && strings.TrimSpace(camelFallback.BodyHTML) != "" {
		result.BodyHTML = camelFallback.BodyHTML
	}
	if strings.TrimSpace(result.SeoTitle) == "" && strings.TrimSpace(camelFallback.SeoTitle) != "" {
		result.SeoTitle = camelFallback.SeoTitle
	}
	if strings.TrimSpace(result.SeoDescription) == "" && strings.TrimSpace(camelFallback.SeoDescription) != "" {
		result.SeoDescription = camelFallback.SeoDescription
	}
	return &result, nil
}

func salvageContentWritingResult(raw string) (*ContentWritingResult, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("invalid generated content JSON")
	}
	lines := strings.Split(raw, "\n")
	title := ""
	bodyMd := raw
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '{' || line[0] == '}' || line[0] == '*' {
			continue
		}
		title = strings.TrimSpace(strings.TrimLeft(line, "#* "))
		bodyMd = strings.TrimSpace(strings.Join(lines[i+1:], "\n"))
		break
	}
	if title == "" {
		title = "CCAI 技术架构实践指南"
	}
	plain := strings.ReplaceAll(bodyMd, "#", "")
	plain = strings.ReplaceAll(plain, "*", "")
	plain = strings.TrimSpace(plain)
	runes := []rune(plain)
	excerpt := plain
	if len(runes) > 120 {
		excerpt = string(runes[:120]) + "..."
	}
	return &ContentWritingResult{
		Title:        title,
		Excerpt:      excerpt,
		BodyMarkdown: bodyMd,
	}, nil
}

// ContentWritingTaskQueue adds an atomic claim operation to the task store.
// Stores that do not implement it are still usable for synchronous inspection,
// but cannot be started as a worker.
type ContentWritingTaskQueue interface {
	ContentWritingTaskStore
	ClaimNext(context.Context) (*ContentWritingTask, error)
}

type ContentWritingTaskRecovery interface {
	RecoverStaleRunning(context.Context, time.Time) error
}

type ContentWritingTaskStore interface {
	Create(context.Context, *ContentWritingTask) (*ContentWritingTask, error)
	Get(context.Context, string) (*ContentWritingTask, error)
	Update(context.Context, *ContentWritingTask) error
	// List returns the most recent tasks of the given kind ("" = all kinds),
	// newest first, capped at limit.
	List(ctx context.Context, kind string, limit int) ([]*ContentWritingTask, error)
	// Delete removes a task row. It reports ErrContentWritingTaskNotFound when
	// the task does not exist (or was already removed).
	Delete(context.Context, string) error
}
type ContentModelCatalog interface {
	SnapshotForPlatform(string) (ModelCatalogSnapshot, error)
}
type ContentArticleAdminService struct {
	repo                ContentArticleRepository
	catalog             ContentModelCatalog
	tasks               ContentWritingTaskStore
	executor            ContentWritingExecutor
	seoExecutor         ContentSEOExecutor
	translationExecutor ContentTranslationExecutor
	workerMu            sync.Mutex
	workerCancel        context.CancelFunc
	workerDone          chan struct{}
	audit               ContentAuditLogger
	defaultModel        string
}

type ContentAuditEntry struct {
	ContentType string
	ContentID   *int64
	Action      string
	Before      map[string]any
	After       map[string]any
	ActorID     *int64
	RequestID   string
}
type ContentAuditLogger interface {
	Log(context.Context, ContentAuditEntry) error
}

type contentAuditActorKey struct{}

// WithContentAuditActor carries the authenticated admin identity into service
// operations so audit rows remain attributable even when the domain entity has
// no actor column for that transition.
func WithContentAuditActor(ctx context.Context, actorID int64) context.Context {
	if actorID <= 0 {
		return ctx
	}
	return context.WithValue(ctx, contentAuditActorKey{}, actorID)
}

func contentAuditActor(ctx context.Context) *int64 {
	if id, ok := ctx.Value(contentAuditActorKey{}).(int64); ok && id > 0 {
		return &id
	}
	return nil
}

func NewContentArticleAdminService(repo ContentArticleRepository, catalog ContentModelCatalog, tasks ContentWritingTaskStore, audit ...ContentAuditLogger) *ContentArticleAdminService {
	s := &ContentArticleAdminService{repo: repo, catalog: catalog, tasks: tasks}
	if cfg, err := loadContentAIProviderConfigWithError(); err == nil {
		s.defaultModel = strings.TrimSpace(cfg.DefaultModel)
	}
	if len(audit) > 0 {
		s.audit = audit[0]
	}
	return s
}

func (s *ContentArticleAdminService) GenerateSEO(ctx context.Context, input ContentSEOInput) (*ContentSEOResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if s == nil {
		return nil, ErrContentAIModelNotConfigured
	}
	cfg, err := loadContentAIProviderConfigWithError()
	if err != nil {
		return nil, ErrContentAIModelNotConfigured
	}
	model := strings.TrimSpace(cfg.DefaultModel)
	if model == "" {
		return nil, ErrContentAIModelNotConfigured
	}
	s.workerMu.Lock()
	executor := s.seoExecutor
	if executor == nil {
		executor = &GatewayContentWritingExecutor{}
	}
	s.workerMu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		result, err := executor.GenerateSEO(ctx, model, input)
		if err != nil {
			return nil, err
		}
		if err := validateContentSEOResult(result); err == nil {
			result.Model = model
			return result, nil
		} else if attempt == 1 {
			return nil, err
		}
	}
	return nil, ErrContentSEOResultInvalid
}

func (s *ContentArticleAdminService) SetAuditLogger(a ContentAuditLogger) { s.audit = a }
func (s *ContentArticleAdminService) auditArticle(ctx context.Context, action string, before, after *ContentArticle) error {
	if s == nil || s.audit == nil {
		return nil
	}
	entry := ContentAuditEntry{ContentType: "article", Action: action, ActorID: contentAuditActor(ctx)}
	if requestID, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		entry.RequestID = requestID
	}
	if before != nil {
		entry.ContentID = &before.ID
		entry.Before = map[string]any{"id": before.ID, "status": before.Status, "version": before.Version, "locale": before.Locale, "slug": before.Slug, "title": before.Title}
	}
	if after != nil {
		if entry.ContentID == nil {
			entry.ContentID = &after.ID
		}
		entry.After = map[string]any{"id": after.ID, "status": after.Status, "version": after.Version, "locale": after.Locale, "slug": after.Slug, "title": after.Title}
	}
	return s.audit.Log(ctx, entry)
}

func (s *ContentArticleAdminService) auditWritingTask(ctx context.Context, action string, t *ContentWritingTask) error {
	if s == nil || s.audit == nil || t == nil {
		return nil
	}
	id := int64(0)
	entry := ContentAuditEntry{ContentType: "writing_task", Action: action, ActorID: contentAuditActor(ctx), Before: map[string]any{}, After: map[string]any{"taskId": t.ID, "kind": t.Kind, "articleId": t.ArticleID, "status": t.Status, "model": t.Model}}
	entry.ContentID = &id
	if requestID, ok := ctx.Value(ctxkey.RequestID).(string); ok {
		entry.RequestID = requestID
	}
	return s.audit.Log(ctx, entry)
}

func (s *ContentArticleAdminService) List(ctx context.Context, f ContentArticleFilters, page, size int) (ContentPage[ContentArticle], error) {
	page, size = normalizeContentPage(page, size)
	xs, total, e := s.repo.ListArticles(ctx, f, page, size)
	if e == nil {
		for i := range xs {
			s.decorateFeatured(&xs[i])
		}
	}
	return makeContentPage(xs, total, page, size), e
}
func (s *ContentArticleAdminService) Get(ctx context.Context, id int64, deleted bool) (*ContentArticle, error) {
	if id <= 0 {
		return nil, ErrContentArticleNotFound
	}
	a, err := s.repo.GetArticle(ctx, id, deleted)
	if err == nil {
		s.decorateFeatured(a)
	}
	return a, err
}
func (s *ContentArticleAdminService) Create(ctx context.Context, a *ContentArticle) (*ContentArticle, error) {
	if a != nil && a.Status == "" {
		a.Status = domain.ContentStatusDraft
	}
	if e := validateArticle(a, false); e != nil {
		return nil, e
	}
	out, err := s.repo.CreateArticle(ctx, a)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditArticle(ctx, "create", nil, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}
func (s *ContentArticleAdminService) Update(ctx context.Context, a *ContentArticle, expected int) (*ContentArticle, error) {
	if a == nil || a.ID <= 0 {
		return nil, ErrContentArticleNotFound
	}
	if a.Status == domain.ContentStatusPublished {
		return nil, ErrContentArticleInvalid
	}
	if e := validateArticle(a, false); e != nil {
		return nil, e
	}
	if expected <= 0 {
		expected = a.Version
	}
	if expected <= 0 {
		return nil, ErrContentArticleConflict
	}
	before, _ := s.repo.GetArticle(ctx, a.ID, false)
	out, err := s.repo.UpdateArticle(ctx, a, expected)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditArticle(ctx, "update", before, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}
func (s *ContentArticleAdminService) Publish(ctx context.Context, id int64, at *time.Time) (*ContentArticle, error) {
	a, e := s.Get(ctx, id, false)
	if e != nil {
		return nil, e
	}
	if at == nil {
		n := time.Now().UTC()
		at = &n
	}
	before := *a
	a.PublishedAt = at
	if e = validateArticle(a, true); e != nil {
		return nil, e
	}
	out, err := s.repo.SetArticleStatus(ctx, id, domain.ContentStatusPublished, at)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditArticle(ctx, "publish", &before, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}
func (s *ContentArticleAdminService) Unpublish(ctx context.Context, id int64) (*ContentArticle, error) {
	a, e := s.Get(ctx, id, false)
	if e != nil {
		return nil, e
	}
	if a.Status != domain.ContentStatusPublished {
		return nil, ErrContentArticleInvalid
	}
	before := *a
	out, err := s.repo.SetArticleStatus(ctx, id, domain.ContentStatusUnpublished, nil)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditArticle(ctx, "unpublish", &before, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}
func (s *ContentArticleAdminService) Delete(ctx context.Context, id int64) error {
	if id <= 0 {
		return ErrContentArticleNotFound
	}
	a, _ := s.repo.GetArticle(ctx, id, false)
	err := s.repo.SoftDeleteArticle(ctx, id, time.Now().UTC())
	if err == nil {
		InvalidatePublicContentCache()
		err = s.auditArticle(ctx, "delete", a, nil)
	}
	return err
}
func (s *ContentArticleAdminService) Restore(ctx context.Context, id int64) (*ContentArticle, error) {
	if id <= 0 {
		return nil, ErrContentArticleNotFound
	}
	out, err := s.repo.RestoreArticle(ctx, id)
	if err == nil {
		InvalidatePublicContentCache()
		if auditErr := s.auditArticle(ctx, "restore", nil, out); auditErr != nil {
			return out, auditErr
		}
	}
	return out, err
}
func (s *ContentArticleAdminService) ListTranslations(ctx context.Context, typ string, id int64, deleted bool) ([]ContentTranslation, error) {
	if strings.TrimSpace(typ) == "" || id <= 0 {
		return nil, ErrContentArticleInvalid
	}
	return s.repo.ListTranslations(ctx, typ, id, deleted)
}
func (s *ContentArticleAdminService) CreateTranslation(ctx context.Context, t *ContentTranslation) (*ContentTranslation, error) {
	if e := validateTranslation(t); e != nil {
		return nil, e
	}
	if err := s.validateTranslationSource(ctx, t); err != nil {
		return nil, err
	}
	out, err := s.repo.CreateTranslation(ctx, t)
	if err == nil {
		InvalidatePublicContentCache()
	}
	if err == nil && s.audit != nil {
		id := out.ID
		err = s.audit.Log(ctx, ContentAuditEntry{ContentType: "translation", ContentID: &id, ActorID: contentAuditActor(ctx), Action: "create", After: map[string]any{"locale": out.Locale, "slug": out.Slug, "status": out.Status, "sourceVersion": out.SourceVersion}})
	}
	return out, err
}
func (s *ContentArticleAdminService) UpdateTranslation(ctx context.Context, t *ContentTranslation) (*ContentTranslation, error) {
	if t == nil || t.ID <= 0 {
		return nil, ErrContentTranslationNotFound
	}
	existing, err := s.repo.GetTranslation(ctx, t.ID, false)
	if err != nil {
		return nil, err
	}
	before := *existing
	// The client edits only editorial fields. Source binding and lifecycle
	// fields always come from the persisted record, preventing accidental
	// clearing or cross-article reassignment.
	if strings.TrimSpace(t.Slug) != "" {
		existing.Slug = t.Slug
	}
	if strings.TrimSpace(t.Title) != "" {
		existing.Title = t.Title
	}
	if t.Excerpt != nil {
		existing.Excerpt = t.Excerpt
	}
	if t.BodyMarkdown != nil {
		existing.BodyMarkdown = t.BodyMarkdown
	}
	if t.BodyHTML != nil {
		existing.BodyHTML = t.BodyHTML
	}
	if t.SeoTitle != nil {
		existing.SeoTitle = t.SeoTitle
	}
	if t.SeoDescription != nil {
		existing.SeoDescription = t.SeoDescription
	}
	if t.CanonicalURL != nil {
		existing.CanonicalURL = t.CanonicalURL
	}
	if strings.TrimSpace(existing.Slug) == "" {
		existing.Slug = t.Slug
	}
	if strings.TrimSpace(existing.Title) == "" {
		existing.Title = t.Title
	}
	if err := validateTranslation(existing); err != nil {
		return nil, err
	}
	if err := s.validateTranslationSource(ctx, existing); err != nil {
		return nil, err
	}
	out, err := s.repo.UpdateTranslation(ctx, existing)
	if err == nil {
		InvalidatePublicContentCache()
	}
	if err == nil && s.audit != nil {
		id := out.ID
		err = s.audit.Log(ctx, ContentAuditEntry{ContentType: "translation", ContentID: &id, ActorID: contentAuditActor(ctx), Action: "update", Before: map[string]any{"locale": before.Locale, "slug": before.Slug, "status": before.Status, "sourceVersion": before.SourceVersion}, After: map[string]any{"locale": out.Locale, "slug": out.Slug, "status": out.Status, "sourceVersion": out.SourceVersion}})
	}
	return out, err
}

func (s *ContentArticleAdminService) validateTranslationSource(ctx context.Context, t *ContentTranslation) error {
	if s == nil || s.repo == nil || t == nil {
		return ErrContentArticleInvalid
	}
	article, err := s.repo.GetArticle(ctx, t.ContentID, false)
	if err != nil {
		return err
	}
	if article == nil || article.ID != t.ContentID || article.Version != t.SourceVersion || !strings.EqualFold(article.Locale, t.SourceLocale) {
		return ErrContentArticleInvalid
	}
	return nil
}
func (s *ContentArticleAdminService) DeleteTranslation(ctx context.Context, id int64) error {
	before, getErr := s.repo.GetTranslation(ctx, id, false)
	if getErr != nil {
		return getErr
	}
	err := s.repo.SoftDeleteTranslation(ctx, id, time.Now().UTC())
	if err == nil {
		InvalidatePublicContentCache()
	}
	if err == nil && s.audit != nil {
		cid := id
		err = s.audit.Log(ctx, ContentAuditEntry{ContentType: "translation", ContentID: &cid, ActorID: contentAuditActor(ctx), Action: "delete", Before: map[string]any{"id": before.ID, "locale": before.Locale, "slug": before.Slug, "status": before.Status, "sourceVersion": before.SourceVersion}})
	}
	return err
}
func (s *ContentArticleAdminService) PublishTranslation(ctx context.Context, id int64, at *time.Time) (*ContentTranslation, error) {
	t, err := s.GetTranslation(ctx, id, false)
	if err != nil {
		return nil, err
	}
	before := *t
	if at == nil {
		now := time.Now().UTC()
		at = &now
	}
	t.Status = domain.ContentTranslationPublished
	t.PublishedAt = at
	if err := validateTranslation(t); err != nil {
		return nil, err
	}
	if err := s.validateTranslationSource(ctx, t); err != nil {
		return nil, err
	}
	article, articleErr := s.repo.GetArticle(ctx, t.ContentID, false)
	if articleErr != nil || article == nil || article.Status != domain.ContentStatusPublished || article.PublishedAt == nil {
		return nil, ErrContentArticleNotPublishable
	}
	out, err := s.repo.UpdateTranslation(ctx, t)
	if err == nil {
		InvalidatePublicContentCache()
	}
	if err == nil && s.audit != nil {
		cid := out.ID
		err = s.audit.Log(ctx, ContentAuditEntry{ContentType: "translation", ContentID: &cid, ActorID: contentAuditActor(ctx), Action: "publish", Before: map[string]any{"status": before.Status, "locale": before.Locale, "slug": before.Slug, "sourceVersion": before.SourceVersion}, After: map[string]any{"status": out.Status, "locale": out.Locale, "slug": out.Slug, "sourceVersion": out.SourceVersion}})
	}
	return out, err
}
func (s *ContentArticleAdminService) UnpublishTranslation(ctx context.Context, id int64) (*ContentTranslation, error) {
	t, err := s.GetTranslation(ctx, id, false)
	if err != nil {
		return nil, err
	}
	before := *t
	t.Status = domain.ContentTranslationUnpublished
	t.PublishedAt = nil
	out, err := s.repo.UpdateTranslation(ctx, t)
	if err == nil {
		InvalidatePublicContentCache()
	}
	if err == nil && s.audit != nil {
		cid := out.ID
		err = s.audit.Log(ctx, ContentAuditEntry{ContentType: "translation", ContentID: &cid, ActorID: contentAuditActor(ctx), Action: "unpublish", Before: map[string]any{"status": before.Status, "locale": before.Locale, "slug": before.Slug, "sourceVersion": before.SourceVersion}, After: map[string]any{"status": out.Status, "locale": out.Locale, "slug": out.Slug, "sourceVersion": out.SourceVersion}})
	}
	return out, err
}
func (s *ContentArticleAdminService) GetTranslation(ctx context.Context, id int64, deleted bool) (*ContentTranslation, error) {
	return s.repo.GetTranslation(ctx, id, deleted)
}
func (s *ContentArticleAdminService) Models(platform string) (ModelCatalogSnapshot, error) {
	if s.catalog == nil {
		return ModelCatalogSnapshot{}, errors.New("model catalog unavailable")
	}
	if strings.TrimSpace(platform) == "" {
		seen := make(map[string]struct{})
		var merged ModelCatalogSnapshot
		for _, candidate := range []string{PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity, "grok", PlatformSeedace, PlatformOllama, PlatformKimi, PlatformZhipu, PlatformDeepSeek} {
			snapshot, err := s.catalog.SnapshotForPlatform(candidate)
			if err != nil {
				continue
			}
			if merged.Checksum == "" {
				merged = snapshot
				merged.Models = nil
			}
			for _, model := range snapshot.Models {
				if _, ok := seen[model]; !ok {
					seen[model] = struct{}{}
					merged.Models = append(merged.Models, model)
				}
			}
		}
		cfg, err := loadContentAIProviderConfigWithError()
		if err != nil {
			return ModelCatalogSnapshot{}, err
		}
		for _, m := range cfg.Models {
			if _, ok := seen[m]; !ok {
				seen[m] = struct{}{}
				merged.Models = append(merged.Models, m)
			}
		}
		sort.Strings(merged.Models)
		return merged, nil
	}
	return s.catalog.SnapshotForPlatform(platform)
}
func (s *ContentArticleAdminService) CreateWritingTask(ctx context.Context, topic, model, keyRef string, articleID int64) (*ContentWritingTask, error) {
	return s.CreateWritingTaskWithInput(ctx, ContentWritingInput{Topic: topic, Model: model, ProviderKeyRef: keyRef, ArticleID: articleID})
}

func (s *ContentArticleAdminService) CreateWritingTaskWithInput(ctx context.Context, in ContentWritingInput) (*ContentWritingTask, error) {
	if err := validateWritingInput(in); err != nil {
		return nil, err
	}
	sourceVersion := 0
	if in.ArticleID > 0 {
		a, err := s.Get(ctx, in.ArticleID, false)
		if err != nil {
			return nil, err
		}
		sourceVersion = a.Version
	}
	now := time.Now().UTC()
	t := &ContentWritingTask{ID: fmt.Sprintf("cw-%d", now.UnixNano()), Kind: ContentWritingKind, Topic: strings.TrimSpace(in.Topic), Audience: strings.TrimSpace(in.Audience), Language: defaultContentLanguage(in.Language), ContentType: strings.TrimSpace(in.ContentType), Keywords: cleanStringList(in.Keywords), WordCount: in.WordCount, Tone: strings.TrimSpace(in.Tone), Facts: cleanStringList(in.Facts), InternalLinks: cleanStringList(in.InternalLinks), Model: strings.TrimSpace(in.Model), ProviderKeyRef: strings.TrimSpace(in.ProviderKeyRef), ArticleID: in.ArticleID, SourceVersion: sourceVersion, Status: ContentWritingQueued, CreatedAt: now, UpdatedAt: now}
	if s.tasks == nil {
		return t, nil
	}
	out, err := s.tasks.Create(ctx, t)
	if err == nil {
		err = s.auditWritingTask(ctx, "create", out)
	}
	return out, err
}

// CreateTranslationTask snapshots the source article version before enqueueing
// translation. A newer source edit therefore cannot overwrite this task's
// target language version.
func (s *ContentArticleAdminService) CreateTranslationTask(ctx context.Context, articleID int64, targetLocale, model, keyRef string) (*ContentWritingTask, error) {
	if articleID <= 0 || strings.TrimSpace(targetLocale) == "" || strings.TrimSpace(model) == "" || strings.ContainsAny(keyRef, "\r\n") || looksLikeContentSecret(keyRef) {
		return nil, ErrContentArticleInvalid
	}
	a, err := s.Get(ctx, articleID, false)
	if err != nil {
		return nil, err
	}
	if strings.EqualFold(strings.TrimSpace(a.Locale), strings.TrimSpace(targetLocale)) {
		return nil, ErrContentArticleInvalid
	}
	now := time.Now().UTC()
	t := &ContentWritingTask{ID: fmt.Sprintf("ct-%d", now.UnixNano()), Kind: ContentTranslationKind, ArticleID: articleID, SourceVersion: a.Version, SourceLocale: a.Locale, TargetLocale: strings.TrimSpace(targetLocale), Model: strings.TrimSpace(model), ProviderKeyRef: strings.TrimSpace(keyRef), Status: ContentWritingQueued, CreatedAt: now, UpdatedAt: now}
	if s.tasks == nil {
		return t, nil
	}
	out, err := s.tasks.Create(ctx, t)
	if err == nil {
		err = s.auditWritingTask(ctx, "create_translation", out)
	}
	return out, err
}
func (s *ContentArticleAdminService) GetWritingTask(ctx context.Context, id string) (*ContentWritingTask, error) {
	if s.tasks == nil {
		return nil, ErrContentWritingTaskNotFound
	}
	return s.tasks.Get(ctx, id)
}

// ListWritingTasks returns recent writing/translation tasks, newest first.
// kind may be "writing", "translation" or "" for all. limit <= 0 or > 200
// falls back to 50.
func (s *ContentArticleAdminService) ListWritingTasks(ctx context.Context, kind string, limit int) ([]*ContentWritingTask, error) {
	if s.tasks == nil {
		return []*ContentWritingTask{}, nil
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	return s.tasks.List(ctx, kind, limit)
}

func (s *ContentArticleAdminService) SetSEOExecutor(executor ContentSEOExecutor) {
	if s == nil {
		return
	}
	s.workerMu.Lock()
	s.seoExecutor = executor
	s.workerMu.Unlock()
}

func (s *ContentArticleAdminService) SetWritingExecutor(executor ContentWritingExecutor) {
	if s == nil {
		return
	}
	s.workerMu.Lock()
	s.executor = executor
	s.workerMu.Unlock()
}

func (s *ContentArticleAdminService) SetTranslationExecutor(executor ContentTranslationExecutor) {
	if s == nil {
		return
	}
	s.workerMu.Lock()
	s.translationExecutor = executor
	s.workerMu.Unlock()
}

// CancelWritingTask is idempotent and never deletes the task or its article.
func (s *ContentArticleAdminService) CancelWritingTask(ctx context.Context, id string) (*ContentWritingTask, error) {
	if s == nil || s.tasks == nil || strings.TrimSpace(id) == "" {
		return nil, ErrContentWritingTaskNotFound
	}
	t, err := s.tasks.Get(ctx, strings.TrimSpace(id))
	if err != nil {
		return nil, err
	}
	if t.Status == ContentWritingQueued || t.Status == ContentWritingRunning {
		t.Status = ContentWritingCancelled
		t.UpdatedAt = time.Now().UTC()
		if err := s.tasks.Update(ctx, t); err != nil {
			return nil, err
		}
		if err := s.auditWritingTask(ctx, "cancel", t); err != nil {
			return t, err
		}
	}
	return t, nil
}

// DeleteWritingTask removes a writing task record. Queued or running tasks are
// cancelled first (so the worker cannot resurrect them) and then deleted; the
// generated article, if any, is never touched.
func (s *ContentArticleAdminService) DeleteWritingTask(ctx context.Context, id string) error {
	if s == nil || s.tasks == nil || strings.TrimSpace(id) == "" {
		return ErrContentWritingTaskNotFound
	}
	id = strings.TrimSpace(id)
	t, err := s.tasks.Get(ctx, id)
	if err != nil {
		return err
	}
	if t.Status == ContentWritingQueued || t.Status == ContentWritingRunning {
		t.Status = ContentWritingCancelled
		t.UpdatedAt = time.Now().UTC()
		if err := s.tasks.Update(ctx, t); err != nil {
			return err
		}
		if err := s.auditWritingTask(ctx, "cancel", t); err != nil {
			return err
		}
	}
	if err := s.tasks.Delete(ctx, id); err != nil {
		return err
	}
	return s.auditWritingTask(ctx, "delete", t)
}

// StartWritingWorker starts one guarded worker loop. The caller must provide
// a queue store and a real gateway executor via SetWritingExecutor.
func (s *ContentArticleAdminService) StartWritingWorker(parent context.Context, interval time.Duration) error {
	if s == nil || s.tasks == nil {
		return ErrContentWritingExecutorMissing
	}
	if _, ok := s.tasks.(ContentWritingTaskQueue); !ok {
		return errors.New("content writing queue unavailable")
	}
	if interval <= 0 {
		interval = time.Second
	}
	s.workerMu.Lock()
	if s.workerCancel != nil {
		s.workerMu.Unlock()
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	s.workerCancel, s.workerDone = cancel, make(chan struct{})
	done := s.workerDone
	s.workerMu.Unlock()
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		recoveryTicker := time.NewTicker(5 * time.Minute)
		defer recoveryTicker.Stop()
		if recovery, ok := s.tasks.(ContentWritingTaskRecovery); ok {
			if err := recovery.RecoverStaleRunning(ctx, time.Now().UTC().Add(-15*time.Minute)); err != nil {
				slog.ErrorContext(ctx, "content writing stale-task recovery failed", "error", err)
			}
		}
		for {
			if err := s.ProcessNextWritingTask(ctx); err != nil && ctx.Err() == nil {
				slog.ErrorContext(ctx, "content writing task failed", "error", err)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			case <-recoveryTicker.C:
				if recovery, ok := s.tasks.(ContentWritingTaskRecovery); ok {
					if err := recovery.RecoverStaleRunning(ctx, time.Now().UTC().Add(-15*time.Minute)); err != nil {
						slog.ErrorContext(ctx, "content writing stale-task recovery failed", "error", err)
					}
				}
			}
		}
	}()
	return nil
}

func (s *ContentArticleAdminService) StopWritingWorker(ctx context.Context) {
	if s == nil {
		return
	}
	s.workerMu.Lock()
	cancel, done := s.workerCancel, s.workerDone
	s.workerCancel, s.workerDone = nil, nil
	s.workerMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
		}
	}
}

// ProcessNextWritingTask claims and executes at most one task. It is exposed
// for scheduler integration and deterministic tests.
func (s *ContentArticleAdminService) ProcessNextWritingTask(ctx context.Context) error {
	q, ok := s.tasks.(ContentWritingTaskQueue)
	if !ok {
		return errors.New("content writing queue unavailable")
	}
	t, err := q.ClaimNext(ctx)
	if err != nil || t == nil {
		return err
	}
	if t.Status == ContentWritingCancelled {
		return nil
	}
	if t.Kind == "" {
		t.Kind = ContentWritingKind
	}
	if t.Kind == ContentTranslationKind {
		return s.executeTranslationTask(ctx, q, t)
	}
	return s.executeWritingTask(ctx, q, t)
}

func (s *ContentArticleAdminService) executeWritingTask(ctx context.Context, q ContentWritingTaskStore, t *ContentWritingTask) error {
	s.workerMu.Lock()
	executor := s.executor
	s.workerMu.Unlock()
	if executor == nil {
		return s.failWritingTask(ctx, q, t, "content_ai_unavailable", ErrContentWritingExecutorMissing)
	}
	result, err := executor.Generate(ctx, t)
	if err != nil {
		return s.failWritingTask(ctx, q, t, safeContentTaskErrorCode(err), err)
	}
	if current, getErr := q.Get(ctx, t.ID); getErr == nil && current.Status == ContentWritingCancelled {
		return nil
	}
	if result == nil || strings.TrimSpace(result.Title) == "" || (strings.TrimSpace(result.BodyMarkdown) == "" && strings.TrimSpace(result.BodyHTML) == "") {
		return s.failWritingTask(ctx, q, t, "content_ai_invalid_result", errors.New("empty generated article"))
	}
	a := &ContentArticle{Locale: defaultContentLanguage(t.Language), Slug: normalizeContentSlug(result.Slug, result.Title, t.ID), Title: strings.TrimSpace(result.Title), Status: domain.ContentStatusDraft, Model: stringPtr(t.Model), Version: 1}
	if len(t.Keywords) > 0 {
		a.Tags = append([]string(nil), t.Keywords...)
	}
	if ct := strings.TrimSpace(t.ContentType); ct != "" && ct != "article" {
		a.Category = stringPtr(ct)
	} else {
		a.Category = stringPtr("架构与网关")
	}
	if strings.TrimSpace(result.Excerpt) != "" {
		a.Excerpt = stringPtr(strings.TrimSpace(result.Excerpt))
	}
	if strings.TrimSpace(result.BodyMarkdown) != "" {
		a.BodyMarkdown = stringPtr(result.BodyMarkdown)
	}
	if strings.TrimSpace(result.BodyHTML) != "" {
		a.BodyHTML = stringPtr(result.BodyHTML)
	} else if a.BodyMarkdown != nil {
		// Persist the rendered HTML at ingest time so a bodyMarkdown-only
		// generation still has a real bodyHtml for the public renderer.
		rendered := contentmd.Render(*a.BodyMarkdown, contentmd.Options{MinHeadingLevel: 2})
		if !contentmd.IsBlank(rendered) {
			a.BodyHTML = stringPtr(rendered)
		}
	}
	if strings.TrimSpace(result.SeoTitle) != "" {
		a.SeoTitle = stringPtr(strings.TrimSpace(result.SeoTitle))
	} else {
		a.SeoTitle = stringPtr(strings.TrimSpace(result.Title) + " | CCAI 技术专栏")
	}
	if strings.TrimSpace(result.SeoDescription) != "" {
		a.SeoDescription = stringPtr(strings.TrimSpace(result.SeoDescription))
	} else if a.Excerpt != nil && strings.TrimSpace(*a.Excerpt) != "" {
		a.SeoDescription = a.Excerpt
	}
	if t.ArticleID > 0 {
		current, getErr := s.Get(ctx, t.ArticleID, false)
		if getErr == nil && current.Status != domain.ContentStatusPublished && current.Status != domain.ContentStatusDeleted {
			// Keep editorial metadata from the existing draft while replacing only
			// generated fields. This also preserves the optimistic version contract.
			a.ID, a.Version, a.Locale, a.Category, a.Tags, a.CoverImage, a.CoverAlt, a.AuthorID = current.ID, current.Version, current.Locale, current.Category, current.Tags, current.CoverImage, current.CoverAlt, current.AuthorID
			if _, err = s.Update(ctx, a, current.Version); err == nil {
				t.ArticleID = a.ID
			}
		} else if getErr != nil {
			if errors.Is(getErr, ErrContentArticleNotFound) {
				t.ArticleID = 0
			} else {
				err = getErr
			}
		} else {
			// Never overwrite a published/deleted article with generated output.
			t.ArticleID = 0
		}
	}
	if err != nil || t.ArticleID == 0 {
		if err == nil {
			created, createErr := s.Create(ctx, a)
			err = createErr
			if created != nil {
				t.ArticleID = created.ID
			}
		}
	}
	if err != nil {
		return s.failWritingTask(ctx, q, t, safeContentTaskErrorCode(err), err)
	}
	return s.succeedWritingTask(ctx, q, t, fmt.Sprintf("article:%d", t.ArticleID))
}

func (s *ContentArticleAdminService) executeTranslationTask(ctx context.Context, q ContentWritingTaskStore, t *ContentWritingTask) error {
	s.workerMu.Lock()
	executor := s.translationExecutor
	s.workerMu.Unlock()
	if executor == nil {
		return s.failWritingTask(ctx, q, t, "content_translation_unavailable", ErrContentTranslationExecutorMissing)
	}
	a, err := s.Get(ctx, t.ArticleID, false)
	if err != nil {
		return s.failWritingTask(ctx, q, t, "content_source_not_found", err)
	}
	if a.Version != t.SourceVersion {
		return s.failWritingTask(ctx, q, t, "content_source_version_conflict", ErrContentArticleConflict)
	}
	translated, err := executor.Translate(ctx, t, a)
	if err != nil {
		return s.failWritingTask(ctx, q, t, safeContentTaskErrorCode(err), err)
	}
	if current, getErr := q.Get(ctx, t.ID); getErr == nil && current.Status == ContentWritingCancelled {
		return nil
	}
	if translated == nil || strings.TrimSpace(translated.Title) == "" || (translated.BodyMarkdown == nil && translated.BodyHTML == nil) {
		return s.failWritingTask(ctx, q, t, "content_translation_invalid_result", errors.New("empty translation"))
	}
	translated.ContentType, translated.ContentID, translated.SourceVersion = "article", a.ID, a.Version
	translated.SourceLocale, translated.Locale = a.Locale, t.TargetLocale
	translated.Status = domain.ContentTranslationDraft
	translated.Slug = normalizeContentSlug(translated.Slug, firstNonEmptyString(translated.Title, a.Slug), t.ID)
	created, err := s.CreateTranslation(ctx, translated)
	if err != nil {
		return s.failWritingTask(ctx, q, t, safeContentTaskErrorCode(err), err)
	}
	t.TranslationID = created.ID
	return s.succeedWritingTask(ctx, q, t, fmt.Sprintf("translation:%d", created.ID))
}

func (s *ContentArticleAdminService) failWritingTask(ctx context.Context, store ContentWritingTaskStore, t *ContentWritingTask, code string, _ error) error {
	t.Status, t.ErrorCode, t.UpdatedAt = ContentWritingFailed, stringPtr(code), time.Now().UTC()
	return store.Update(ctx, t)
}
func (s *ContentArticleAdminService) succeedWritingTask(ctx context.Context, store ContentWritingTaskStore, t *ContentWritingTask, result string) error {
	t.Status, t.Result, t.ErrorCode, t.UpdatedAt = ContentWritingSucceeded, stringPtr(result), nil, time.Now().UTC()
	return store.Update(ctx, t)
}

func validateWritingInput(in ContentWritingInput) error {
	if strings.TrimSpace(in.Topic) == "" || strings.TrimSpace(in.Model) == "" || strings.ContainsAny(in.ProviderKeyRef, "\r\n") || looksLikeContentSecret(in.ProviderKeyRef) {
		return ErrContentArticleInvalid
	}
	if in.ArticleID < 0 || in.WordCount < 0 || in.WordCount > 100000 {
		return ErrContentArticleInvalid
	}
	for _, value := range append(append(append([]string{}, in.Keywords...), in.Facts...), in.InternalLinks...) {
		if strings.ContainsAny(value, "\r\n") || len(value) > 200 {
			return ErrContentArticleInvalid
		}
	}
	return nil
}

func defaultContentLanguage(language string) string {
	if language = strings.TrimSpace(language); language != "" {
		return language
	}
	return "zh-CN"
}

func looksLikeContentSecret(value string) bool {
	v := strings.ToLower(strings.TrimSpace(value))
	return strings.HasPrefix(v, "sk-") || strings.Contains(v, "api_key=") || strings.Contains(v, "apikey=")
}

func safeContentTaskErrorCode(err error) string {
	if err == nil {
		return "content_task_failed"
	}
	if errors.Is(err, context.Canceled) {
		return "content_task_cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "content_task_timeout"
	}
	return "content_task_failed"
}

func normalizeContentSlug(slug, title, fallback string) string {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		slug = strings.ToLower(strings.TrimSpace(title))
	}
	var b strings.Builder
	for _, r := range slug {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	slug = strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "draft-" + strings.TrimPrefix(strings.TrimSpace(fallback), "cw-")
	}
	if len(slug) > 80 {
		slug = slug[:80]
	}
	return strings.Trim(slug, "-")
}

func stringPtr(value string) *string { return &value }
func validateArticle(a *ContentArticle, publish bool) error {
	if a == nil || strings.TrimSpace(a.Locale) == "" || strings.TrimSpace(a.Slug) == "" || strings.TrimSpace(a.Title) == "" || !domain.IsContentStatus(a.Status) {
		return ErrContentArticleInvalid
	}
	a.Locale = domain.NormalizeContentLocale(strings.TrimSpace(a.Locale))
	a.Slug = strings.TrimSpace(a.Slug)
	a.Title = strings.TrimSpace(a.Title)
	if publish {
		if a.PublishedAt == nil {
			return ErrContentArticleNotPublishable
		}
		if a.Excerpt == nil || strings.TrimSpace(*a.Excerpt) == "" || (a.BodyHTML == nil && a.BodyMarkdown == nil) || (a.BodyHTML != nil && strings.TrimSpace(*a.BodyHTML) == "" && a.BodyMarkdown != nil && strings.TrimSpace(*a.BodyMarkdown) == "") {
			return ErrContentArticleNotPublishable
		}
		if a.SeoTitle == nil || strings.TrimSpace(*a.SeoTitle) == "" || a.SeoDescription == nil || strings.TrimSpace(*a.SeoDescription) == "" {
			return ErrContentArticleNotPublishable
		}
	}
	return nil
}
func validateTranslation(t *ContentTranslation) error {
	if t == nil || t.ID < 0 || strings.TrimSpace(t.ContentType) != "article" || t.ContentID <= 0 || t.SourceVersion <= 0 || strings.TrimSpace(t.SourceLocale) == "" || strings.TrimSpace(t.Locale) == "" || strings.EqualFold(strings.TrimSpace(t.SourceLocale), strings.TrimSpace(t.Locale)) || strings.TrimSpace(t.Slug) == "" || strings.TrimSpace(t.Title) == "" || !domain.IsContentTranslationStatus(t.Status) {
		return ErrContentArticleInvalid
	}
	if t.Status == domain.ContentTranslationPublished && (t.Excerpt == nil || strings.TrimSpace(*t.Excerpt) == "" || !hasNonEmptyTranslationBody(t) || t.SeoTitle == nil || t.SeoDescription == nil || strings.TrimSpace(*t.SeoTitle) == "" || strings.TrimSpace(*t.SeoDescription) == "") {
		return ErrContentArticleNotPublishable
	}
	return nil
}

func hasNonEmptyTranslationBody(t *ContentTranslation) bool {
	if t == nil {
		return false
	}
	return (t.BodyHTML != nil && strings.TrimSpace(*t.BodyHTML) != "") || (t.BodyMarkdown != nil && strings.TrimSpace(*t.BodyMarkdown) != "")
}

type MemoryContentWritingTaskStore struct {
	mu sync.RWMutex
	m  map[string]*ContentWritingTask
}

func NewMemoryContentWritingTaskStore() *MemoryContentWritingTaskStore {
	return &MemoryContentWritingTaskStore{m: map[string]*ContentWritingTask{}}
}
func (s *MemoryContentWritingTaskStore) Create(_ context.Context, t *ContentWritingTask) (*ContentWritingTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := cloneContentWritingTask(t)
	s.m[t.ID] = cp
	return cloneContentWritingTask(cp), nil
}
func (s *MemoryContentWritingTaskStore) Get(_ context.Context, id string) (*ContentWritingTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.m[id]
	if !ok {
		return nil, ErrContentWritingTaskNotFound
	}
	return cloneContentWritingTask(t), nil
}
func (s *MemoryContentWritingTaskStore) Update(_ context.Context, t *ContentWritingTask) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[t.ID]; !ok {
		return ErrContentWritingTaskNotFound
	}
	s.m[t.ID] = cloneContentWritingTask(t)
	return nil
}

func (s *MemoryContentWritingTaskStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[id]; !ok {
		return ErrContentWritingTaskNotFound
	}
	delete(s.m, id)
	return nil
}

func (s *MemoryContentWritingTaskStore) List(_ context.Context, kind string, limit int) ([]*ContentWritingTask, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 {
		limit = 50
	}
	out := make([]*ContentWritingTask, 0, len(s.m))
	for _, t := range s.m {
		if kind != "" && t.Kind != kind {
			continue
		}
		out = append(out, cloneContentWritingTask(t))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *MemoryContentWritingTaskStore) ClaimNext(_ context.Context) (*ContentWritingTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, task := range s.m {
		if task.Status != ContentWritingQueued {
			continue
		}
		task.Status = ContentWritingRunning
		task.UpdatedAt = time.Now().UTC()
		return cloneContentWritingTask(task), nil
	}
	return nil, nil
}

func cloneContentWritingTask(t *ContentWritingTask) *ContentWritingTask {
	if t == nil {
		return nil
	}
	cp := *t
	cp.Keywords = append([]string(nil), t.Keywords...)
	cp.Facts = append([]string(nil), t.Facts...)
	cp.InternalLinks = append([]string(nil), t.InternalLinks...)
	return &cp
}
