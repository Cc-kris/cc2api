package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/pkg/richtext"
)

// 内容类型常量，与 content_translations.content_type 的既有取值保持一致。
const (
	contentTranslationTypeNews    = "news"
	contentTranslationTypeArticle = "article"
)

// ErrContentTranslationLocaleUnsupported 表示 syncLocales 里出现了未支持的目标语言。
var ErrContentTranslationLocaleUnsupported = errors.New("unsupported content translation target locale")

// contentTranslationTargetFamilies 是可同步的目标语言家族（中文是源语言，不在此列），
// 与前端 ADMIN_TARGET_LOCALES 的 en-US/ru-RU/fr-FR/de-DE 一一对应。
var contentTranslationTargetFamilies = map[string]struct{}{
	"en": {},
	"ru": {},
	"fr": {},
	"de": {},
}

// ContentTranslationDraftStore 是同步翻译记录所需的最小仓储能力。
type ContentTranslationDraftStore interface {
	UpsertDraft(context.Context, *ContentTranslation) (*ContentTranslation, error)
	// ListUnfinished 返回某条内容所有「未完成」的翻译记录（不限版本），
	// 用于避免同一 locale 反复保存时堆积重复草稿。
	ListUnfinished(ctx context.Context, contentType string, contentID int64) ([]ContentTranslation, error)
}

// ContentTranslationSyncer 在内容保存时按 syncLocales 建立待翻译记录。
type ContentTranslationSyncer struct {
	store     ContentTranslationDraftStore
	lifecycle *ContentTranslationLifecycle
}

// NewContentTranslationSyncer 构造同步器；store 为 nil 时所有调用都是空操作。
func NewContentTranslationSyncer(store ContentTranslationDraftStore) *ContentTranslationSyncer {
	return &ContentTranslationSyncer{store: store}
}

// NewContentTranslationSyncerWithLifecycle 复用既有翻译生命周期建草稿：由
// ContentTranslationLifecycle.CreateDraft 负责状态流转与模型解析；模型未配置时
// 降级为直接落一条 pending 草稿，避免影响内容保存。
func NewContentTranslationSyncerWithLifecycle(store ContentTranslationDraftStore, lifecycle *ContentTranslationLifecycle) *ContentTranslationSyncer {
	return &ContentTranslationSyncer{store: store, lifecycle: lifecycle}
}

// TranslationDraftRequest 描述一次同步翻译请求。
type TranslationDraftRequest struct {
	ContentType   string
	ContentID     int64
	SourceVersion int
	SourceLocale  string
	Slug          string
	Title         string
	Locales       []string
}

// EnsureDrafts 为每个目标 locale 建立（或复用）一条待翻译记录。
//
// 已存在同一内容同一 locale 的未完成记录时直接跳过，不重复创建；与源语言
// 相同的 locale 也忽略。单个 locale 失败不影响其他 locale，返回的错误切片
// 按 locale 对齐（成功为 nil）。
func (s *ContentTranslationSyncer) EnsureDrafts(ctx context.Context, req TranslationDraftRequest) ([]*ContentTranslation, []error) {
	if s == nil || s.store == nil || req.ContentID <= 0 || len(req.Locales) == 0 {
		return nil, nil
	}
	existing, err := s.store.ListUnfinished(ctx, req.ContentType, req.ContentID)
	if err != nil {
		return nil, []error{err}
	}
	byLocale := make(map[string]*ContentTranslation, len(existing))
	for i := range existing {
		row := &existing[i]
		byLocale[strings.ToLower(domain.NormalizeContentLocale(row.Locale))] = row
	}
	sourceLocale := strings.ToLower(domain.NormalizeContentLocale(req.SourceLocale))
	// 复用文章服务既有的 locale 归一化/去重逻辑，避免同一批里重复建记录。
	locales := normalizeTranslationLocales(req.Locales)
	out := make([]*ContentTranslation, 0, len(locales))
	errs := make([]error, 0, len(locales))
	for _, locale := range locales {
		key := strings.ToLower(locale)
		if sourceLocale != "" && key == sourceLocale {
			out = append(out, nil)
			errs = append(errs, nil)
			continue
		}
		if _, ok := contentTranslationTargetFamilies[domain.ContentLocaleFamily(locale)]; !ok {
			out = append(out, nil)
			errs = append(errs, fmt.Errorf("%w: %s", ErrContentTranslationLocaleUnsupported, locale))
			continue
		}
		if row, ok := byLocale[key]; ok && row != nil {
			out = append(out, row)
			errs = append(errs, nil)
			continue
		}
		draft := &ContentTranslation{
			ContentType:   req.ContentType,
			ContentID:     req.ContentID,
			SourceVersion: req.SourceVersion,
			SourceLocale:  domain.NormalizeContentLocale(req.SourceLocale),
			Locale:        locale,
			Slug:          translationDraftSlug(req.Slug, locale),
			Title:         req.Title,
			Status:        domain.ContentTranslationDraft,
		}
		saved, err := s.createDraft(ctx, draft)
		out = append(out, saved)
		errs = append(errs, err)
		if err == nil && saved != nil {
			byLocale[key] = saved
		}
	}
	return out, errs
}

// createDraft 统一走既有翻译生命周期创建 pending 草稿；当翻译模型尚未配置时
// 降级为直接落库，保证「保存后立刻能看到待翻译记录」这一行为不依赖模型配置。
func (s *ContentTranslationSyncer) createDraft(ctx context.Context, draft *ContentTranslation) (*ContentTranslation, error) {
	if s.lifecycle != nil && strings.TrimSpace(draft.Title) != "" && strings.TrimSpace(draft.Slug) != "" {
		saved, err := s.lifecycle.CreateDraft(ctx, draft)
		if err == nil {
			return saved, nil
		}
		if !errors.Is(err, ErrContentTranslationModelUnavailable) {
			return nil, err
		}
	}
	draft.Status = domain.ContentTranslationDraft
	return s.store.UpsertDraft(ctx, draft)
}

// SyncTranslationDrafts 供内容服务在保存后调用。
func (s *ContentTranslationSyncer) SyncTranslationDrafts(ctx context.Context, req TranslationDraftRequest) (*ContentTranslation, []error) {
	rows, errs := s.EnsureDrafts(ctx, req)
	var first *ContentTranslation
	for _, row := range rows {
		if row != nil {
			first = row
			break
		}
	}
	return first, errs
}

// translationDraftSlug 给草稿一个稳定的占位 slug：同源 slug + locale 后缀，
// 实际译文生成后会由翻译流程覆盖。
func translationDraftSlug(slug, locale string) string {
	base := strings.TrimSpace(slug)
	if base == "" {
		base = "draft"
	}
	return base + "-" + strings.ToLower(strings.ReplaceAll(locale, "_", "-"))
}

// sanitizeArticleBody 用白名单清洗文章富文本正文（保存链路统一入口）。
func sanitizeArticleBody(a *ContentArticle) {
	if a == nil || a.BodyHTML == nil {
		return
	}
	clean := richtext.Sanitize(*a.BodyHTML)
	a.BodyHTML = &clean
}

// syncArticleTranslationDrafts 按保存请求里的 syncLocales 建立待翻译记录。
// 失败不影响文章本身的保存结果，只记录审计日志。
func (s *ContentArticleAdminService) syncArticleTranslationDrafts(ctx context.Context, a *ContentArticle) {
	if s == nil || s.translat == nil || a == nil || len(a.SyncLocales) == 0 {
		return
	}
	_, errs := s.translat.EnsureDrafts(ctx, TranslationDraftRequest{
		ContentType:   contentTranslationTypeArticle,
		ContentID:     a.ID,
		SourceVersion: a.Version,
		SourceLocale:  a.Locale,
		Slug:          a.Slug,
		Title:         a.Title,
		Locales:       a.SyncLocales,
	})
	for _, err := range errs {
		if err != nil && s.audit != nil {
			id := a.ID
			_ = s.audit.Log(ctx, ContentAuditEntry{
				ContentType: contentTranslationTypeArticle,
				Action:      "translate_sync_failed",
				ContentID:   &id,
				After:       map[string]any{"error": err.Error()},
			})
		}
	}
}
