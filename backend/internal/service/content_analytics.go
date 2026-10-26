package service

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/domain"
)

var ErrContentAnalyticsInvalid = errors.New("invalid content analytics event")

type ContentAnalyticsEvent struct {
	EventID      string         `json:"eventId"`
	EventName    string         `json:"eventName"`
	ContentType  *string        `json:"contentType,omitempty"`
	ContentID    *int64         `json:"contentId,omitempty"`
	Locale       *string        `json:"locale,omitempty"`
	Placement    *string        `json:"placement,omitempty"`
	PagePath     string         `json:"pagePath"`
	SessionHash  string         `json:"-"`
	ReferrerHost *string        `json:"referrerHost,omitempty"`
	EventAt      time.Time      `json:"eventAt"`
	Payload      map[string]any `json:"payload,omitempty"`
}
type ContentAnalyticsDaily struct {
	Date           time.Time `json:"date"`
	Locale         string    `json:"locale"`
	ContentType    string    `json:"contentType"`
	ContentID      int64     `json:"contentId"`
	Title          string    `json:"title,omitempty"`
	PagePath       string    `json:"pagePath"`
	Placement      string    `json:"placement"`
	UniqueVisitors int       `json:"uniqueVisitors"`
	PageViews      int       `json:"pageViews"`
	Reads          int       `json:"reads"`
	Impressions    int       `json:"impressions"`
	Clicks         int       `json:"clicks"`
	AvgReadSeconds int       `json:"avgReadSeconds"`
	CTR            float64   `json:"ctr"`
	UpdatedAt      time.Time `json:"updatedAt"`
}
type ContentAnalyticsFilters struct {
	From, To                       *time.Time
	Locale, ContentType, Placement string
	SourceID                       *int64
	Page, PageSize                 int
	Sort                           string
}

// ContentAnalyticsOverview is the BI dashboard summary for a selected range.
type ContentAnalyticsOverview struct {
	UniqueVisitors int       `json:"uniqueVisitors"`
	PageViews      int       `json:"pageViews"`
	Reads          int       `json:"reads"`
	Impressions    int       `json:"impressions"`
	Clicks         int       `json:"clicks"`
	AvgReadSeconds int       `json:"avgReadSeconds"`
	CTR            float64   `json:"ctr"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type ContentAnalyticsTrend struct {
	Date           time.Time `json:"date"`
	UniqueVisitors int       `json:"uniqueVisitors"`
	PageViews      int       `json:"pageViews"`
	Reads          int       `json:"reads"`
	Impressions    int       `json:"impressions"`
	Clicks         int       `json:"clicks"`
	CTR            float64   `json:"ctr"`
}

type ContentAnalyticsRanking struct {
	ContentType    string  `json:"contentType"`
	ContentID      int64   `json:"contentId"`
	PagePath       string  `json:"pagePath"`
	Title          string  `json:"title,omitempty"`
	Reads          int     `json:"reads"`
	UniqueReaders  int     `json:"uniqueReaders"`
	Impressions    int     `json:"impressions"`
	Clicks         int     `json:"clicks"`
	CTR            float64 `json:"ctr"`
	AvgReadSeconds int     `json:"avgReadSeconds"`
}

type ContentAnalyticsCTR struct {
	Placement   string  `json:"placement"`
	Impressions int     `json:"impressions"`
	Clicks      int     `json:"clicks"`
	CTR         float64 `json:"ctr"`
}

type ContentAnalyticsRankingsResult struct {
	Items    []ContentAnalyticsRanking `json:"items"`
	Page     int                       `json:"page"`
	PageSize int                       `json:"pageSize"`
	Total    int                       `json:"total"`
	HasNext  bool                      `json:"hasNext"`
}
type ContentAnalyticsRepository interface {
	CreateEvent(context.Context, *ContentAnalyticsEvent) error
	ListDaily(context.Context, ContentAnalyticsFilters) ([]ContentAnalyticsDaily, error)
	AggregateDaily(context.Context, time.Time, string) error
}
type ContentAnalyticsDistinctRepository interface {
	CountDistinctVisitors(context.Context, ContentAnalyticsFilters) (int, error)
}
type ContentAnalyticsService struct {
	repo   ContentAnalyticsRepository
	secret []byte
}

func NewContentAnalyticsService(repo ContentAnalyticsRepository, secret string) *ContentAnalyticsService {
	return &ContentAnalyticsService{repo: repo, secret: []byte(secret)}
}

func viewLifecycleEvent(name string) bool {
	return name == "heartbeat" || name == "page_heartbeat" || name == "page_leave"
}

func validAnalyticsPayload(e *ContentAnalyticsEvent) bool {
	if e == nil {
		return false
	}
	legacy := make(map[string]any, len(e.Payload))
	for key, value := range e.Payload {
		switch key {
		case "view_id", "viewId":
			if e.EventName != "page_view" && e.EventName != "content_impression" && !viewLifecycleEvent(e.EventName) {
				return false
			}
		case "duration_ms":
			if !viewLifecycleEvent(e.EventName) {
				return false
			}
		default:
			legacy[key] = value
		}
	}
	if domain.ValidateAnalyticsPayload(legacy) != nil {
		return false
	}
	return !viewLifecycleEvent(e.EventName) || analyticsViewID(e) != ""
}

// PrepareEvent validates and enriches an event without performing I/O. It is
// used by the HTTP handler before enqueueing asynchronous persistence.
func (s *ContentAnalyticsService) PrepareEvent(e *ContentAnalyticsEvent, ip, ua string) error {
	if e == nil || !allowedAnalyticsEvent(e.EventName) || !validAnalyticsPayload(e) || strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.EventID) != e.EventID || len(e.EventID) > 120 || strings.TrimSpace(e.PagePath) == "" || len(e.PagePath) > 2048 || strings.ContainsAny(e.PagePath, "\r\n") || !validAnalyticsDimensions(e) {
		return ErrContentAnalyticsInvalid
	}
	if e.EventAt.IsZero() {
		e.EventAt = time.Now().UTC()
	}
	if e.EventAt.Before(time.Now().UTC().Add(-48*time.Hour)) || e.EventAt.After(time.Now().UTC().Add(10*time.Minute)) || !validAnalyticsViewEvent(e) {
		return ErrContentAnalyticsInvalid
	}
	e.SessionHash = hashSession(s.secret, ip, ua)
	return nil
}
func (s *ContentAnalyticsService) PersistEvent(ctx context.Context, e *ContentAnalyticsEvent) error {
	if s == nil || s.repo == nil {
		return errors.New("analytics repository is unavailable")
	}
	return s.repo.CreateEvent(ctx, e)
}
func (s *ContentAnalyticsService) Ingest(ctx context.Context, e *ContentAnalyticsEvent, ip, ua string) error {
	if err := s.PrepareEvent(e, ip, ua); err != nil {
		return err
	}
	return s.PersistEvent(ctx, e)
}
func validAnalyticsDimensions(e *ContentAnalyticsEvent) bool {
	if e.ContentID != nil && *e.ContentID <= 0 {
		return false
	}
	if e.ContentType != nil && *e.ContentType != "news" && *e.ContentType != "article" && *e.ContentType != "page" {
		return false
	}
	for _, value := range []*string{e.Locale, e.Placement, e.ReferrerHost} {
		if value != nil && (strings.TrimSpace(*value) == "" || len(*value) > 120 || strings.ContainsAny(*value, "\r\n")) {
			return false
		}
	}
	return true
}
func allowedAnalyticsEvent(v string) bool {
	switch strings.TrimSpace(v) {
	case "page_view", "heartbeat", "page_heartbeat", "page_leave", "content_impression", "content_click", "content_read", "toc_click":
		return true
	default:
		return false
	}
}

const maxAnalyticsViewDurationMS = int64(4 * time.Hour / time.Millisecond)

// analyticsViewID returns the stable browser view identifier used to collapse
// heartbeat and leave events. Both spellings are accepted because payloads are
// sent by clients using either Go/SQL or JSON naming conventions.
func analyticsViewID(e *ContentAnalyticsEvent) string {
	if e == nil {
		return ""
	}
	for _, key := range []string{"view_id", "viewId"} {
		if value, ok := e.Payload[key].(string); ok {
			if value = strings.TrimSpace(value); value != "" {
				return value
			}
		}
	}
	return ""
}

func analyticsDurationMS(e *ContentAnalyticsEvent) (int64, bool) {
	if e == nil || e.Payload == nil {
		return 0, false
	}
	value, ok := e.Payload["duration_ms"]
	if !ok {
		return 0, false
	}
	var duration int64
	switch number := value.(type) {
	case int:
		duration = int64(number)
	case int64:
		duration = number
	case float64:
		if number != float64(int64(number)) {
			return 0, false
		}
		duration = int64(number)
	default:
		return 0, false
	}
	return duration, duration >= 0 && duration <= maxAnalyticsViewDurationMS
}

func validAnalyticsViewEvent(e *ContentAnalyticsEvent) bool {
	if !viewLifecycleEvent(e.EventName) {
		return true
	}
	_, durationOK := analyticsDurationMS(e)
	return analyticsViewID(e) != "" && durationOK
}
func (s *ContentAnalyticsService) Query(ctx context.Context, f ContentAnalyticsFilters) ([]ContentAnalyticsDaily, error) {
	return s.repo.ListDaily(ctx, f)
}

// Aggregate recomputes the current and immediately previous local day. This
// intentionally re-reads both buckets on each run so events arriving late at
// the day boundary are reflected without double counting.
func (s *ContentAnalyticsService) Aggregate(ctx context.Context, now time.Time, siteTimezone string) error {
	if s == nil || s.repo == nil {
		return errors.New("analytics repository is unavailable")
	}
	if strings.TrimSpace(siteTimezone) == "" {
		siteTimezone = "Asia/Shanghai"
	}
	if _, err := time.LoadLocation(siteTimezone); err != nil {
		return fmt.Errorf("invalid analytics timezone: %w", err)
	}
	return s.repo.AggregateDaily(ctx, now, siteTimezone)
}

func (s *ContentAnalyticsService) Overview(ctx context.Context, f ContentAnalyticsFilters) (ContentAnalyticsOverview, error) {
	rows, err := s.repo.ListDaily(ctx, f)
	if err != nil {
		return ContentAnalyticsOverview{}, err
	}
	var out ContentAnalyticsOverview
	var readSeconds int
	for _, row := range rows {
		out.UniqueVisitors += row.UniqueVisitors
		out.PageViews += row.PageViews
		out.Reads += row.Reads
		out.Impressions += row.Impressions
		out.Clicks += row.Clicks
		readSeconds += row.AvgReadSeconds * row.Reads
		if row.UpdatedAt.After(out.UpdatedAt) {
			out.UpdatedAt = row.UpdatedAt
		}
	}
	if out.Reads > 0 {
		out.AvgReadSeconds = readSeconds / out.Reads
	}
	if out.Impressions > 0 {
		out.CTR = float64(out.Clicks) / float64(out.Impressions)
	}
	if distinct, ok := s.repo.(ContentAnalyticsDistinctRepository); ok {
		if exact, exactErr := distinct.CountDistinctVisitors(ctx, f); exactErr != nil {
			return ContentAnalyticsOverview{}, exactErr
		} else {
			out.UniqueVisitors = exact
		}
	}
	return out, nil
}

func (s *ContentAnalyticsService) Trends(ctx context.Context, f ContentAnalyticsFilters) ([]ContentAnalyticsTrend, error) {
	rows, err := s.repo.ListDaily(ctx, f)
	if err != nil {
		return nil, err
	}
	byDate := make(map[string]*ContentAnalyticsTrend)
	for _, row := range rows {
		key := row.Date.Format("2006-01-02")
		trend := byDate[key]
		if trend == nil {
			trend = &ContentAnalyticsTrend{Date: row.Date}
			byDate[key] = trend
		}
		trend.UniqueVisitors += row.UniqueVisitors
		trend.PageViews += row.PageViews
		trend.Reads += row.Reads
		trend.Impressions += row.Impressions
		trend.Clicks += row.Clicks
	}
	out := make([]ContentAnalyticsTrend, 0, len(byDate))
	distinct, hasDistinct := s.repo.(ContentAnalyticsDistinctRepository)
	for _, trend := range byDate {
		if hasDistinct {
			dayStart := trend.Date
			dayEnd := dayStart.AddDate(0, 0, 1)
			dayFilter := f
			dayFilter.From, dayFilter.To = &dayStart, &dayStart
			dayFilter.To = &dayEnd
			if exact, exactErr := distinct.CountDistinctVisitors(ctx, dayFilter); exactErr != nil {
				return nil, exactErr
			} else {
				trend.UniqueVisitors = exact
			}
		}
		if trend.Impressions > 0 {
			trend.CTR = float64(trend.Clicks) / float64(trend.Impressions)
		}
		out = append(out, *trend)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date.Before(out[j].Date) })
	return out, nil
}

func (s *ContentAnalyticsService) Rankings(ctx context.Context, f ContentAnalyticsFilters) (ContentAnalyticsRankingsResult, error) {
	rows, err := s.repo.ListDaily(ctx, f)
	if err != nil {
		return ContentAnalyticsRankingsResult{}, err
	}
	type key struct {
		typ  string
		id   int64
		path string
	}
	grouped := make(map[key]*ContentAnalyticsRanking)
	for _, row := range rows {
		k := key{row.ContentType, row.ContentID, row.PagePath}
		item := grouped[k]
		if item == nil {
			item = &ContentAnalyticsRanking{ContentType: row.ContentType, ContentID: row.ContentID, PagePath: row.PagePath, Title: row.Title}
			grouped[k] = item
		}
		item.Reads += row.Reads
		item.UniqueReaders += row.UniqueVisitors
		item.Impressions += row.Impressions
		item.Clicks += row.Clicks
		item.AvgReadSeconds += row.AvgReadSeconds * row.Reads
	}
	out := make([]ContentAnalyticsRanking, 0, len(grouped))
	for _, item := range grouped {
		if item.Reads > 0 {
			item.AvgReadSeconds /= item.Reads
		}
		if item.Impressions > 0 {
			item.CTR = float64(item.Clicks) / float64(item.Impressions)
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool {
		if strings.EqualFold(f.Sort, "clicks") {
			return out[i].Clicks > out[j].Clicks
		}
		if strings.EqualFold(f.Sort, "ctr") {
			return out[i].CTR > out[j].CTR
		}
		return out[i].Reads > out[j].Reads
	})
	page, size := f.Page, f.PageSize
	if page < 1 {
		page = 1
	}
	if size < 1 || size > 200 {
		size = 20
	}
	start := (page - 1) * size
	if start > len(out) {
		start = len(out)
	}
	end := start + size
	if end > len(out) {
		end = len(out)
	}
	items := out[start:end]
	return ContentAnalyticsRankingsResult{Items: items, Page: page, PageSize: size, Total: len(out), HasNext: end < len(out)}, nil
}

func (s *ContentAnalyticsService) CTR(ctx context.Context, f ContentAnalyticsFilters) ([]ContentAnalyticsCTR, error) {
	rows, err := s.repo.ListDaily(ctx, f)
	if err != nil {
		return nil, err
	}
	byPlacement := make(map[string]*ContentAnalyticsCTR)
	for _, row := range rows {
		item := byPlacement[row.Placement]
		if item == nil {
			item = &ContentAnalyticsCTR{Placement: row.Placement}
			byPlacement[row.Placement] = item
		}
		item.Impressions += row.Impressions
		item.Clicks += row.Clicks
	}
	out := make([]ContentAnalyticsCTR, 0, len(byPlacement))
	for _, item := range byPlacement {
		if item.Impressions > 0 {
			item.CTR = float64(item.Clicks) / float64(item.Impressions)
		}
		out = append(out, *item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CTR > out[j].CTR })
	return out, nil
}

// ExportCSV serializes only daily aggregates. It deliberately has no event,
// session, IP, UA or source URL columns.
func (s *ContentAnalyticsService) ExportCSV(ctx context.Context, f ContentAnalyticsFilters) ([]byte, error) {
	rows, err := s.repo.ListDaily(ctx, f)
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write([]string{"date", "locale", "content_type", "content_id", "page_path", "placement", "unique_visitors", "page_views", "reads", "impressions", "clicks", "avg_read_seconds", "ctr"}); err != nil {
		return nil, err
	}
	for _, row := range rows {
		record := []string{row.Date.Format("2006-01-02"), row.Locale, row.ContentType, strconv.FormatInt(row.ContentID, 10), row.PagePath, row.Placement, strconv.Itoa(row.UniqueVisitors), strconv.Itoa(row.PageViews), strconv.Itoa(row.Reads), strconv.Itoa(row.Impressions), strconv.Itoa(row.Clicks), strconv.Itoa(row.AvgReadSeconds), strconv.FormatFloat(row.CTR, 'f', 6, 64)}
		if err := w.Write(record); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
func hashSession(secret []byte, ip, ua string) string {
	m := hmac.New(sha256.New, secret)
	m.Write([]byte(ip))
	m.Write([]byte("\x00"))
	m.Write([]byte(ua))
	return hex.EncodeToString(m.Sum(nil))
}
