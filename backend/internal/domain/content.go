package domain

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const (
	HomeModeStructured   = "structured"
	HomeModeLegacyCustom = "legacy_custom"
)

func IsHomeMode(value string) bool {
	return value == HomeModeStructured || value == HomeModeLegacyCustom
}

const (
	ContentStatusDraft         = "draft"
	ContentStatusPendingReview = "pending_review"
	ContentStatusPublished     = "published"
	ContentStatusUnpublished   = "unpublished"
	ContentStatusDeleted       = "deleted"
)

const (
	ContentNewsSourceRSS        = "rss"
	ContentNewsSourceAtom       = "atom"
	ContentNewsSourcePublicJSON = "public_json"
	ContentNewsSourceManual     = "manual"
)

const (
	ContentNewsBodySourceFeedContent = "feed_content"
	ContentNewsBodySourcePublicAPI   = "public_api"
	ContentNewsBodySourceHTMLExtract = "html_extract"
)

func IsContentNewsBodySourceType(value string) bool {
	switch value {
	case ContentNewsBodySourceFeedContent, ContentNewsBodySourcePublicAPI, ContentNewsBodySourceHTMLExtract:
		return true
	default:
		return false
	}
}

func ValidateContentNewsBodySourceType(value string) error {
	if value == "" || IsContentNewsBodySourceType(value) {
		return nil
	}
	return fmt.Errorf("invalid content news body source type %q", value)
}

func ValidateContentNewsSourceURL(value string) error {
	if value == "" {
		return nil
	}
	u, err := url.ParseRequestURI(value)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return fmt.Errorf("content news source URL must be an absolute http(s) URL")
	}
	return nil
}

const (
	ContentNewsPublishManual = "manual"
	ContentNewsPublishAuto   = "auto"
)

const (
	ContentAutomationManualReview = "manual_review"
	ContentAutomationAutoPublish  = "auto_publish"
)

const (
	ContentFetchQueued      = "queued"
	ContentFetchRunning     = "running"
	ContentFetchSucceeded   = "succeeded"
	ContentFetchPartial     = "partial"
	ContentFetchFailed      = "failed"
	ContentFetchCancelled   = "cancelled"
	ContentFetchInterrupted = "interrupted"
)

const (
	ContentFetchTriggerCron   = "cron"
	ContentFetchTriggerManual = "manual"
)

const (
	ContentTranslationDraft       = "translation_draft"
	ContentTranslationReview      = "translation_review"
	ContentTranslationPublished   = "published"
	ContentTranslationUnpublished = "unpublished"
	ContentTranslationDeleted     = "deleted"
)

func IsContentStatus(value string) bool {
	switch value {
	case ContentStatusDraft, ContentStatusPendingReview, ContentStatusPublished, ContentStatusUnpublished, ContentStatusDeleted:
		return true
	default:
		return false
	}
}

func IsContentNewsSourceType(value string) bool {
	switch value {
	case ContentNewsSourceRSS, ContentNewsSourceAtom, ContentNewsSourcePublicJSON, ContentNewsSourceManual:
		return true
	default:
		return false
	}
}

func IsContentNewsPublishMode(value string) bool {
	return value == ContentNewsPublishManual || value == ContentNewsPublishAuto
}

func IsContentAutomationPublishMode(value string) bool {
	return value == ContentAutomationManualReview || value == ContentAutomationAutoPublish
}

func IsContentFetchStatus(value string) bool {
	switch value {
	case ContentFetchQueued, ContentFetchRunning, ContentFetchSucceeded, ContentFetchPartial, ContentFetchFailed, ContentFetchCancelled, ContentFetchInterrupted:
		return true
	default:
		return false
	}
}

func IsContentFetchTrigger(value string) bool {
	return value == ContentFetchTriggerCron || value == ContentFetchTriggerManual
}

func IsContentTranslationStatus(value string) bool {
	switch value {
	case ContentTranslationDraft, ContentTranslationReview, ContentTranslationPublished, ContentTranslationUnpublished, ContentTranslationDeleted:
		return true
	default:
		return false
	}
}

func ValidateContentStatus(value string) error {
	if !IsContentStatus(value) {
		return fmt.Errorf("invalid content status %q", value)
	}
	return nil
}

func ValidateContentNewsSourceType(value string) error {
	if !IsContentNewsSourceType(value) {
		return fmt.Errorf("invalid content news source type %q", value)
	}
	return nil
}

func ValidateContentNewsPublishMode(value string) error {
	if !IsContentNewsPublishMode(value) {
		return fmt.Errorf("invalid content news publish mode %q", value)
	}
	return nil
}

func ValidateContentAutomationPublishMode(value string) error {
	if !IsContentAutomationPublishMode(value) {
		return fmt.Errorf("invalid content automation publish mode %q", value)
	}
	return nil
}

func ValidateContentFetchStatus(value string) error {
	if !IsContentFetchStatus(value) {
		return fmt.Errorf("invalid content fetch status %q", value)
	}
	return nil
}

func ValidateContentFetchTrigger(value string) error {
	if !IsContentFetchTrigger(value) {
		return fmt.Errorf("invalid content fetch trigger %q", value)
	}
	return nil
}

func ValidateContentTranslationStatus(value string) error {
	if !IsContentTranslationStatus(value) {
		return fmt.Errorf("invalid content translation status %q", value)
	}
	return nil
}

var allowedAnalyticsPayloadFields = map[string]struct{}{
	"scroll_percent": {},
	"read_seconds":   {},
	"target":         {},
	"position":       {},
	"section":        {},
}

func ValidateAnalyticsPayload(payload map[string]any) error {
	for key, value := range payload {
		if _, ok := allowedAnalyticsPayloadFields[key]; !ok {
			return fmt.Errorf("analytics payload field %q is not allowed", key)
		}
		switch value.(type) {
		case nil, string, bool, float64, float32, int, int32, int64, uint, uint32, uint64:
		default:
			return fmt.Errorf("analytics payload field %q must be scalar", key)
		}
	}
	return nil
}

// contentLocaleAliases maps short or non-canonical locale tags onto the form the
// content tables store, so admin writes and public reads agree on one spelling.
var contentLocaleAliases = map[string]string{
	"zh":         "zh-CN",
	"zh-cn":      "zh-CN",
	"zh_cn":      "zh-CN",
	"zhcn":       "zh-CN",
	"zh-hans":    "zh-CN",
	"zh_hans":    "zh-CN",
	"zh-hans-cn": "zh-CN",
	"zh_cn_hans": "zh-CN",
	"en":         "en-US",
	"en-us":      "en-US",
	"en_us":      "en-US",
	"enus":       "en-US",
}

// NormalizeContentLocale returns the canonical storage spelling of a locale tag.
// Unknown tags are returned trimmed but otherwise untouched.
func NormalizeContentLocale(locale string) string {
	locale = strings.TrimSpace(locale)
	if locale == "" {
		return ""
	}
	if canonical, ok := contentLocaleAliases[strings.ToLower(locale)]; ok {
		return canonical
	}
	return locale
}

// ContentLocaleFamily groups locale tags by language ("zh", "en", ...).
func ContentLocaleFamily(locale string) string {
	family := strings.ToLower(strings.TrimSpace(locale))
	if family == "" {
		return ""
	}
	if i := strings.IndexAny(family, "-_"); i > 0 {
		family = family[:i]
	}
	if family == "zhhans" {
		return "zh"
	}
	return family
}

// ContentLocaleCandidates lists the stored spellings that may satisfy a request
// for the given locale, most specific first. The public detail endpoints use it
// so that /articles/zh/<slug> can still serve a zh-CN record.
func ContentLocaleCandidates(locale string) []string {
	// Canonicalize before building candidates: otherwise a request for "zh-cn"
	// seeds the list with a spelling the tables never store and, because the
	// de-dupe below is case-insensitive, it also suppresses the canonical
	// "zh-CN" candidate and the lookup 404s.
	requested := NormalizeContentLocale(locale)
	if requested == "" {
		return nil
	}
	out := []string{requested}
	add := func(value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		for _, existing := range out {
			if strings.EqualFold(existing, value) {
				return
			}
		}
		out = append(out, value)
	}
	add(NormalizeContentLocale(requested))
	switch ContentLocaleFamily(requested) {
	case "zh":
		add("zh-CN")
		add("zh-Hans")
		add("zh")
	case "en":
		add("en-US")
		add("en")
	}
	return out
}

// PublishWindowReferenceDate anchors clock-only publish window bounds so the
// stored timestamps stay comparable. Only the HH:MM part is meaningful.
var PublishWindowReferenceDate = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// NormalizePublishWindow validates an "HH:MM" clock value. An empty value clears
// the bound.
func NormalizePublishWindow(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse("15:04", value)
	if err != nil {
		return "", fmt.Errorf("invalid publish window %q, want HH:MM", value)
	}
	return parsed.Format("15:04"), nil
}

// PublishWindowTime converts an "HH:MM" clock value into the timestamp shape
// stored by content_automation_settings.
func PublishWindowTime(clock string) (*time.Time, error) {
	normalized, err := NormalizePublishWindow(clock)
	if err != nil {
		return nil, err
	}
	if normalized == "" {
		return nil, nil
	}
	parsed, _ := time.Parse("15:04", normalized)
	value := time.Date(PublishWindowReferenceDate.Year(), PublishWindowReferenceDate.Month(), PublishWindowReferenceDate.Day(), parsed.Hour(), parsed.Minute(), 0, 0, time.UTC)
	return &value, nil
}

// PublishWindowClock renders the "HH:MM" part of a stored publish window bound.
func PublishWindowClock(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("15:04")
}
