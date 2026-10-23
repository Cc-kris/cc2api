package repository

import (
	"context"
	stdsql "database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"entgo.io/ent/dialect/sql"
	"entgo.io/ent/dialect/sql/sqljson"
	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/contentarticle"
	"github.com/Wei-Shaw/sub2api/ent/contenttranslation"
	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type contentArticleAdminRepository struct{ client *dbent.Client }

type contentWritingTaskRepository struct{ db *stdsql.DB }

func NewContentArticleAdminRepository(client *dbent.Client) service.ContentArticleRepository {
	return &contentArticleAdminRepository{client: client}
}

func NewContentWritingTaskRepository(client *dbent.Client) service.ContentWritingTaskStore {
	var db *stdsql.DB
	if client != nil {
		if drv, ok := client.Driver().(*sql.Driver); ok {
			db = drv.DB()
		}
	}
	return &contentWritingTaskRepository{db: db}
}

func (r *contentWritingTaskRepository) Create(ctx context.Context, t *service.ContentWritingTask) (*service.ContentWritingTask, error) {
	if r == nil || r.db == nil || t == nil || strings.TrimSpace(t.ID) == "" {
		return nil, service.ErrContentArticleInvalid
	}
	keywords, _ := json.Marshal(t.Keywords)
	facts, _ := json.Marshal(t.Facts)
	links, _ := json.Marshal(t.InternalLinks)
	if _, err := r.db.ExecContext(ctx, `INSERT INTO content_writing_tasks (task_id,kind,article_id,translation_id,source_version,source_locale,target_locale,topic,audience,language,content_type,keywords,word_count,tone,facts,internal_links,model,provider_key_ref,status,result,error_code,version,created_at,updated_at) VALUES ($1,$2,NULLIF($3,0),NULLIF($4,0),$5,NULLIF($6,''),NULLIF($7,''),$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,NULLIF($18,''),$19,$20,$21,1,$22,$22) ON CONFLICT (task_id) DO NOTHING`, t.ID, t.Kind, t.ArticleID, t.TranslationID, t.SourceVersion, t.SourceLocale, t.TargetLocale, t.Topic, t.Audience, t.Language, t.ContentType, keywords, t.WordCount, t.Tone, facts, links, t.Model, safeTaskKeyRef(t.ProviderKeyRef), t.Status, t.Result, t.ErrorCode, t.CreatedAt); err != nil {
		return nil, err
	}
	return r.Get(ctx, t.ID)
}

func (r *contentWritingTaskRepository) Get(ctx context.Context, id string) (*service.ContentWritingTask, error) {
	if r == nil || r.db == nil || strings.TrimSpace(id) == "" {
		return nil, service.ErrContentWritingTaskNotFound
	}
	t, err := scanWritingTask(r.db.QueryRowContext(ctx, writingTaskSelect+" WHERE task_id=$1", id))
	if errors.Is(err, stdsql.ErrNoRows) {
		return nil, service.ErrContentWritingTaskNotFound
	}
	return t, err
}

func (r *contentWritingTaskRepository) List(ctx context.Context, kind string, limit int) ([]*service.ContentWritingTask, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("content writing task database unavailable")
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	query := writingTaskSelect
	args := []any{}
	if strings.TrimSpace(kind) != "" {
		query += " WHERE kind=$1"
		args = append(args, strings.TrimSpace(kind))
	}
	query += fmt.Sprintf(" ORDER BY created_at DESC LIMIT %d", limit)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*service.ContentWritingTask, 0, limit)
	for rows.Next() {
		t, err := scanWritingTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Delete removes a single writing task row.
func (r *contentWritingTaskRepository) Delete(ctx context.Context, id string) error {
	if r == nil || r.db == nil {
		return errors.New("content writing task database unavailable")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return service.ErrContentWritingTaskNotFound
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM content_writing_tasks WHERE task_id=$1`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return service.ErrContentWritingTaskNotFound
	}
	return nil
}

func (r *contentWritingTaskRepository) Update(ctx context.Context, t *service.ContentWritingTask) error {
	if r == nil || r.db == nil || t == nil || strings.TrimSpace(t.ID) == "" {
		return service.ErrContentWritingTaskNotFound
	}
	if t.Version <= 0 {
		t.Version = 1
	}
	keywords, _ := json.Marshal(t.Keywords)
	facts, _ := json.Marshal(t.Facts)
	links, _ := json.Marshal(t.InternalLinks)
	res, err := r.db.ExecContext(ctx, `UPDATE content_writing_tasks SET article_id=NULLIF($2,0),translation_id=NULLIF($3,0),source_version=$4,source_locale=NULLIF($5,''),target_locale=NULLIF($6,''),topic=$7,audience=$8,language=$9,content_type=$10,keywords=$11,word_count=$12,tone=$13,facts=$14,internal_links=$15,model=$16,provider_key_ref=NULLIF($17,''),status=$18,result=$19,error_code=$20,lease_until=NULL,version=version+1,updated_at=NOW() WHERE task_id=$1 AND version=$21`, t.ID, t.ArticleID, t.TranslationID, t.SourceVersion, t.SourceLocale, t.TargetLocale, t.Topic, t.Audience, t.Language, t.ContentType, keywords, t.WordCount, t.Tone, facts, links, t.Model, safeTaskKeyRef(t.ProviderKeyRef), t.Status, t.Result, t.ErrorCode, t.Version)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n != 1 {
		return service.ErrContentArticleConflict
	}
	t.Version++
	t.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *contentWritingTaskRepository) ClaimNext(ctx context.Context) (*service.ContentWritingTask, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("content writing task database unavailable")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	t, err := scanWritingTask(tx.QueryRowContext(ctx, writingTaskSelect+` WHERE status='queued' OR (status='running' AND lease_until < NOW()) ORDER BY created_at, task_id FOR UPDATE SKIP LOCKED LIMIT 1`))
	if errors.Is(err, stdsql.ErrNoRows) {
		return nil, tx.Commit()
	}
	if err != nil {
		return nil, err
	}
	lease := time.Now().UTC().Add(15 * time.Minute)
	if _, err = tx.ExecContext(ctx, `UPDATE content_writing_tasks SET status='running', lease_until=$2, version=version+1, updated_at=NOW() WHERE task_id=$1`, t.ID, lease); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	t.Status = service.ContentWritingRunning
	t.Version++
	t.UpdatedAt = time.Now().UTC()
	return t, nil
}

// RecoverStaleRunning requeues work left behind by a crashed process.
func (r *contentWritingTaskRepository) RecoverStaleRunning(ctx context.Context, olderThan time.Time) error {
	if r == nil || r.db == nil {
		return errors.New("content writing task database unavailable")
	}
	_, err := r.db.ExecContext(ctx, `UPDATE content_writing_tasks SET status='queued', lease_until=NULL, version=version+1, updated_at=NOW() WHERE status='running' AND (lease_until < NOW() OR updated_at < $1)`, olderThan)
	return err
}

/*
func (r *contentWritingTaskRepository) claimNextFrom(ctx context.Context, client *dbent.Client) (*service.ContentWritingTask, error) {
	rows, err := client.ContentAuditLog.Query().Where(contentauditlog.ActionEQ("content_writing_task")).Order(contentauditlog.ByCreatedAt(sql.OrderAsc()), contentauditlog.ByID(sql.OrderAsc())).All(ctx)
	if err != nil {
		return nil, err
	}
	latest := make(map[string]*service.ContentWritingTask)
	for _, row := range rows {
		id, ok := row.AfterSnapshot["task_id"].(string)
		if !ok || id == "" {
			continue
		}
		t := contentWritingTaskFromSnapshot(row.AfterSnapshot)
		latest[id] = t
	}
	candidates := make([]*service.ContentWritingTask, 0, len(latest))
	for _, t := range latest {
		if t.Status == service.ContentWritingRunning && t.UpdatedAt.Before(time.Now().UTC().Add(-15*time.Minute)) {
			t.Status = service.ContentWritingQueued
			if _, err := client.ContentAuditLog.Create().SetContentType("article").SetAction("content_writing_task").SetAfterSnapshot(contentWritingTaskSnapshot(t)).Save(ctx); err != nil {
				return nil, err
			}
		}
		candidates = append(candidates, t)
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].CreatedAt.Equal(candidates[j].CreatedAt) {
			return candidates[i].ID < candidates[j].ID
		}
		return candidates[i].CreatedAt.Before(candidates[j].CreatedAt)
	})
	for _, t := range candidates {
		if t.Status != service.ContentWritingQueued {
			continue
		}
		t.Status, t.UpdatedAt = service.ContentWritingRunning, time.Now().UTC()
		if _, err := client.ContentAuditLog.Create().SetContentType("article").SetAction("content_writing_task").SetAfterSnapshot(contentWritingTaskSnapshot(t)).Save(ctx); err != nil {
			return nil, err
		}
		return t, nil
	}
	return nil, nil
}
*/

const writingTaskSelect = `SELECT task_id,kind,COALESCE(article_id,0),COALESCE(translation_id,0),source_version,COALESCE(source_locale,''),COALESCE(target_locale,''),topic,COALESCE(audience,''),COALESCE(language,''),COALESCE(content_type,''),keywords,word_count,COALESCE(tone,''),facts,internal_links,model,COALESCE(provider_key_ref,''),status,result,error_code,version,created_at,updated_at FROM content_writing_tasks`

type rowScanner interface{ Scan(...any) error }

func scanWritingTask(row rowScanner) (*service.ContentWritingTask, error) {
	var t service.ContentWritingTask
	var keywords, facts, links []byte
	if err := row.Scan(&t.ID, &t.Kind, &t.ArticleID, &t.TranslationID, &t.SourceVersion, &t.SourceLocale, &t.TargetLocale, &t.Topic, &t.Audience, &t.Language, &t.ContentType, &keywords, &t.WordCount, &t.Tone, &facts, &links, &t.Model, &t.ProviderKeyRef, &t.Status, &t.Result, &t.ErrorCode, &t.Version, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(keywords, &t.Keywords); err != nil {
		return nil, fmt.Errorf("decode task keywords: %w", err)
	}
	if err := json.Unmarshal(facts, &t.Facts); err != nil {
		return nil, fmt.Errorf("decode task facts: %w", err)
	}
	if err := json.Unmarshal(links, &t.InternalLinks); err != nil {
		return nil, fmt.Errorf("decode task links: %w", err)
	}
	return &t, nil
}

func safeTaskKeyRef(value string) string {
	v := strings.TrimSpace(value)
	lower := strings.ToLower(v)
	if strings.HasPrefix(lower, "sk-") || strings.Contains(lower, "api_key=") || strings.Contains(lower, "apikey=") {
		return ""
	}
	return v
}
func (r *contentArticleAdminRepository) ListArticles(ctx context.Context, f service.ContentArticleFilters, page, size int) ([]service.ContentArticle, int, error) {
	qctx := ctx
	if f.IncludeDeleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	q := r.client.ContentArticle.Query().Order(contentarticle.ByUpdatedAt(sql.OrderDesc()), contentarticle.ByID(sql.OrderDesc()))
	if f.ReadMin != nil || f.ReadMax != nil {
		ids, err := r.articleIDsByReadRange(ctx, f.ReadMin, f.ReadMax)
		if err != nil {
			return nil, 0, err
		}
		if len(ids) == 0 {
			q = q.Where(contentarticle.IDEQ(-1))
		} else {
			q = q.Where(contentarticle.IDIn(ids...))
		}
	}
	if !f.IncludeDeleted {
		q = q.Where(contentarticle.DeletedAtIsNil())
	}
	if strings.TrimSpace(f.Status) != "" {
		q = q.Where(contentarticle.StatusEQ(strings.TrimSpace(f.Status)))
	}
	if strings.TrimSpace(f.Locale) != "" {
		q = q.Where(contentarticle.LocaleEQ(strings.TrimSpace(f.Locale)))
	}
	if strings.TrimSpace(f.Category) != "" {
		q = q.Where(contentarticle.CategoryEQ(strings.TrimSpace(f.Category)))
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		q = q.Where(contentarticle.Or(contentarticle.TitleContainsFold(s), contentarticle.SlugContainsFold(s)))
	}
	if f.Tags != "" {
		tag := strings.TrimSpace(f.Tags)
		q = q.Where(func(s *sql.Selector) { s.Where(sqljson.ValueContains(contentarticle.FieldTags, tag)) })
	}
	if f.Model != "" {
		q = q.Where(contentarticle.ModelContainsFold(strings.TrimSpace(f.Model)))
	}
	if f.AuthorID != nil {
		q = q.Where(contentarticle.AuthorIDEQ(*f.AuthorID))
	}
	if f.From != nil {
		q = q.Where(contentarticle.PublishedAtGTE(*f.From))
	}
	if f.To != nil {
		q = q.Where(contentarticle.PublishedAtLT(*f.To))
	}
	total, e := q.Count(qctx)
	if e != nil {
		return nil, 0, e
	}
	xs, e := q.Offset((page - 1) * size).Limit(size).All(qctx)
	if e != nil {
		return nil, 0, e
	}
	out := mapContentArticles(xs)
	if err := r.populateArticleReadCounts(ctx, out); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *contentArticleAdminRepository) articleAnalyticsDB() *stdsql.DB {
	if r == nil || r.client == nil {
		return nil
	}
	drv, ok := r.client.Driver().(*sql.Driver)
	if !ok {
		return nil
	}
	return drv.DB()
}

func (r *contentArticleAdminRepository) articleIDsByReadRange(ctx context.Context, min, max *int) ([]int64, error) {
	db := r.articleAnalyticsDB()
	if db == nil {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT content_id FROM content_analytics_daily WHERE content_type = 'article' GROUP BY content_id HAVING ($1::int IS NULL OR COALESCE(SUM(reads), 0) >= $1) AND ($2::int IS NULL OR COALESCE(SUM(reads), 0) <= $2)`, min, max)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *contentArticleAdminRepository) populateArticleReadCounts(ctx context.Context, items []service.ContentArticle) error {
	db := r.articleAnalyticsDB()
	if db == nil {
		return nil
	}
	for i := range items {
		if err := db.QueryRowContext(ctx, `SELECT COALESCE(SUM(reads), 0) FROM content_analytics_daily WHERE content_type = 'article' AND content_id = $1`, items[i].ID).Scan(&items[i].ReadCount); err != nil {
			return err
		}
	}
	return nil
}
func (r *contentArticleAdminRepository) GetArticle(ctx context.Context, id int64, deleted bool) (*service.ContentArticle, error) {
	qctx := ctx
	if deleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	m, e := r.client.ContentArticle.Query().Where(contentarticle.IDEQ(id)).Only(qctx)
	if e != nil {
		if dbent.IsNotFound(e) {
			return nil, service.ErrContentArticleNotFound
		}
		return nil, e
	}
	v := mapContentArticle(m)
	return &v, nil
}
func (r *contentArticleAdminRepository) CreateArticle(ctx context.Context, a *service.ContentArticle) (*service.ContentArticle, error) {
	b := r.client.ContentArticle.Create().SetLocale(a.Locale).SetSlug(a.Slug).SetTitle(a.Title).SetTags(a.Tags).SetStatus(a.Status).SetVersion(maxArticleVersion(a.Version))
	setArticleOptional(b, a)
	m, e := b.Save(ctx)
	if e != nil {
		return nil, e
	}
	v := mapContentArticle(m)
	return &v, nil
}
func (r *contentArticleAdminRepository) UpdateArticle(ctx context.Context, a *service.ContentArticle, expected int) (*service.ContentArticle, error) {
	b := r.client.ContentArticle.Update().Where(contentarticle.IDEQ(a.ID), contentarticle.VersionEQ(expected), contentarticle.DeletedAtIsNil()).SetLocale(a.Locale).SetSlug(a.Slug).SetTitle(a.Title).SetTags(a.Tags).SetStatus(a.Status).SetVersion(expected + 1)
	setArticleOptionalUpdate(b, a)
	n, e := b.Save(ctx)
	if e != nil {
		return nil, e
	}
	if n == 0 {
		return nil, service.ErrContentArticleConflict
	}
	return r.GetArticle(ctx, a.ID, false)
}
func (r *contentArticleAdminRepository) SetArticleStatus(ctx context.Context, id int64, status string, at *time.Time) (*service.ContentArticle, error) {
	b := r.client.ContentArticle.Update().Where(contentarticle.IDEQ(id), contentarticle.DeletedAtIsNil()).SetStatus(status).AddVersion(1)
	if at != nil {
		b.SetPublishedAt(*at)
	} else {
		b.ClearPublishedAt()
	}
	if _, e := b.Save(ctx); e != nil {
		return nil, e
	}
	return r.GetArticle(ctx, id, false)
}
func (r *contentArticleAdminRepository) SoftDeleteArticle(ctx context.Context, id int64, at time.Time) error {
	_, e := r.client.ContentArticle.Update().Where(contentarticle.IDEQ(id), contentarticle.DeletedAtIsNil()).SetDeletedAt(at).SetStatus(domain.ContentStatusDeleted).AddVersion(1).Save(ctx)
	return e
}
func (r *contentArticleAdminRepository) RestoreArticle(ctx context.Context, id int64) (*service.ContentArticle, error) {
	q := mixins.SkipSoftDelete(ctx)
	if _, e := r.client.ContentArticle.Query().Where(contentarticle.IDEQ(id)).Only(q); e != nil {
		if dbent.IsNotFound(e) {
			return nil, service.ErrContentArticleNotFound
		}
		return nil, e
	}
	m, e := r.client.ContentArticle.UpdateOneID(id).ClearDeletedAt().SetStatus(domain.ContentStatusDraft).AddVersion(1).Save(ctx)
	if e != nil {
		return nil, e
	}
	v := mapContentArticle(m)
	return &v, nil
}
func (r *contentArticleAdminRepository) ListTranslations(ctx context.Context, typ string, id int64, deleted bool) ([]service.ContentTranslation, error) {
	qctx := ctx
	if deleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	q := r.client.ContentTranslation.Query().Where(contenttranslation.ContentTypeEQ(typ), contenttranslation.ContentIDEQ(id)).Order(contenttranslation.ByUpdatedAt(sql.OrderDesc()))
	if !deleted {
		q = q.Where(contenttranslation.DeletedAtIsNil())
	}
	xs, e := q.All(qctx)
	if e != nil {
		return nil, e
	}
	return mapContentTranslations(xs), nil
}
func (r *contentArticleAdminRepository) GetTranslation(ctx context.Context, id int64, deleted bool) (*service.ContentTranslation, error) {
	qctx := ctx
	if deleted {
		qctx = mixins.SkipSoftDelete(ctx)
	}
	m, e := r.client.ContentTranslation.Query().Where(contenttranslation.IDEQ(id)).Only(qctx)
	if e != nil {
		if dbent.IsNotFound(e) {
			return nil, service.ErrContentTranslationNotFound
		}
		return nil, e
	}
	v := mapContentTranslation(m)
	return &v, nil
}
func (r *contentArticleAdminRepository) CreateTranslation(ctx context.Context, t *service.ContentTranslation) (*service.ContentTranslation, error) {
	b := r.client.ContentTranslation.Create().SetContentType(t.ContentType).SetContentID(t.ContentID).SetSourceVersion(t.SourceVersion).SetSourceLocale(t.SourceLocale).SetLocale(t.Locale).SetSlug(t.Slug).SetTitle(t.Title).SetStatus(t.Status)
	setTranslationOptional(b, t)
	m, e := b.Save(ctx)
	if e != nil {
		return nil, e
	}
	v := mapContentTranslation(m)
	return &v, nil
}
func (r *contentArticleAdminRepository) UpdateTranslation(ctx context.Context, t *service.ContentTranslation) (*service.ContentTranslation, error) {
	b := r.client.ContentTranslation.UpdateOneID(t.ID).SetContentType(t.ContentType).SetContentID(t.ContentID).SetSourceVersion(t.SourceVersion).SetSourceLocale(t.SourceLocale).SetLocale(t.Locale).SetSlug(t.Slug).SetTitle(t.Title).SetStatus(t.Status)
	if t.Excerpt != nil {
		b.SetExcerpt(*t.Excerpt)
	} else {
		b.ClearExcerpt()
	}
	if t.BodyMarkdown != nil {
		b.SetBodyMarkdown(*t.BodyMarkdown)
	} else {
		b.ClearBodyMarkdown()
	}
	if t.BodyHTML != nil {
		b.SetBodyHTML(*t.BodyHTML)
	} else {
		b.ClearBodyHTML()
	}
	if t.SeoTitle != nil {
		b.SetSeoTitle(*t.SeoTitle)
	} else {
		b.ClearSeoTitle()
	}
	if t.SeoDescription != nil {
		b.SetSeoDescription(*t.SeoDescription)
	} else {
		b.ClearSeoDescription()
	}
	if t.CanonicalURL != nil {
		b.SetCanonicalURL(*t.CanonicalURL)
	} else {
		b.ClearCanonicalURL()
	}
	if t.TranslationModel != nil {
		b.SetTranslationModel(*t.TranslationModel)
	} else {
		b.ClearTranslationModel()
	}
	if t.FailureCode != nil {
		b.SetFailureCode(*t.FailureCode)
	} else {
		b.ClearFailureCode()
	}
	if t.PublishedAt != nil {
		b.SetPublishedAt(*t.PublishedAt)
	} else {
		b.ClearPublishedAt()
	}
	m, e := b.Save(ctx)
	if e != nil {
		return nil, e
	}
	v := mapContentTranslation(m)
	return &v, nil
}
func (r *contentArticleAdminRepository) SoftDeleteTranslation(ctx context.Context, id int64, at time.Time) error {
	_, e := r.client.ContentTranslation.Update().Where(contenttranslation.IDEQ(id), contenttranslation.DeletedAtIsNil()).SetDeletedAt(at).SetStatus(domain.ContentTranslationDeleted).Save(ctx)
	return e
}

func maxArticleVersion(v int) int {
	if v < 1 {
		return 1
	}
	return v
}
func mapContentArticle(m *dbent.ContentArticle) service.ContentArticle {
	return service.ContentArticle{ID: m.ID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, DeletedAt: m.DeletedAt, Locale: m.Locale, Slug: m.Slug, Title: m.Title, Excerpt: m.Excerpt, BodyMarkdown: m.BodyMarkdown, BodyHTML: m.BodyHTML, CoverImage: m.CoverImage, CoverAlt: m.CoverAlt, Category: m.Category, Tags: append([]string(nil), m.Tags...), Status: m.Status, AuthorID: m.AuthorID, Model: m.Model, SeoTitle: m.SeoTitle, SeoDescription: m.SeoDescription, CanonicalURL: m.CanonicalURL, OgImage: m.OgImage, PublishedAt: m.PublishedAt, Version: m.Version}
}
func mapContentArticles(xs []*dbent.ContentArticle) []service.ContentArticle {
	out := make([]service.ContentArticle, 0, len(xs))
	for _, x := range xs {
		out = append(out, mapContentArticle(x))
	}
	return out
}
func mapContentTranslation(m *dbent.ContentTranslation) service.ContentTranslation {
	return service.ContentTranslation{ID: m.ID, CreatedAt: m.CreatedAt, UpdatedAt: m.UpdatedAt, DeletedAt: m.DeletedAt, ContentType: m.ContentType, ContentID: m.ContentID, SourceVersion: m.SourceVersion, SourceLocale: m.SourceLocale, Locale: m.Locale, Slug: m.Slug, Title: m.Title, Excerpt: m.Excerpt, BodyMarkdown: m.BodyMarkdown, BodyHTML: m.BodyHTML, SeoTitle: m.SeoTitle, SeoDescription: m.SeoDescription, CanonicalURL: m.CanonicalURL, TranslationModel: m.TranslationModel, FailureCode: m.FailureCode, Status: m.Status, ReviewedBy: m.ReviewedBy, ReviewedAt: m.ReviewedAt, PublishedAt: m.PublishedAt}
}
func mapContentTranslations(xs []*dbent.ContentTranslation) []service.ContentTranslation {
	out := make([]service.ContentTranslation, 0, len(xs))
	for _, x := range xs {
		out = append(out, mapContentTranslation(x))
	}
	return out
}
func setArticleOptional(b *dbent.ContentArticleCreate, a *service.ContentArticle) {
	if a.Excerpt != nil {
		b.SetExcerpt(*a.Excerpt)
	}
	if a.BodyMarkdown != nil {
		b.SetBodyMarkdown(*a.BodyMarkdown)
	}
	if a.BodyHTML != nil {
		b.SetBodyHTML(*a.BodyHTML)
	}
	if a.CoverImage != nil {
		b.SetCoverImage(*a.CoverImage)
	}
	if a.CoverAlt != nil {
		b.SetCoverAlt(*a.CoverAlt)
	}
	if a.Category != nil {
		b.SetCategory(*a.Category)
	}
	if a.AuthorID != nil {
		b.SetAuthorID(*a.AuthorID)
	}
	if a.Model != nil {
		b.SetModel(*a.Model)
	}
	if a.SeoTitle != nil {
		b.SetSeoTitle(*a.SeoTitle)
	}
	if a.SeoDescription != nil {
		b.SetSeoDescription(*a.SeoDescription)
	}
	if a.CanonicalURL != nil {
		b.SetCanonicalURL(*a.CanonicalURL)
	}
	if a.OgImage != nil {
		b.SetOgImage(*a.OgImage)
	}
	if a.PublishedAt != nil {
		b.SetPublishedAt(*a.PublishedAt)
	}
}
func setArticleOptionalUpdate(b *dbent.ContentArticleUpdate, a *service.ContentArticle) {
	if a.Excerpt != nil {
		b.SetExcerpt(*a.Excerpt)
	} else {
		b.ClearExcerpt()
	}
	if a.BodyMarkdown != nil {
		b.SetBodyMarkdown(*a.BodyMarkdown)
	} else {
		b.ClearBodyMarkdown()
	}
	if a.BodyHTML != nil {
		b.SetBodyHTML(*a.BodyHTML)
	} else {
		b.ClearBodyHTML()
	}
	if a.CoverImage != nil {
		b.SetCoverImage(*a.CoverImage)
	} else {
		b.ClearCoverImage()
	}
	if a.CoverAlt != nil {
		b.SetCoverAlt(*a.CoverAlt)
	} else {
		b.ClearCoverAlt()
	}
	if a.Category != nil {
		b.SetCategory(*a.Category)
	} else {
		b.ClearCategory()
	}
	if a.AuthorID != nil {
		b.SetAuthorID(*a.AuthorID)
	} else {
		b.ClearAuthorID()
	}
	if a.Model != nil {
		b.SetModel(*a.Model)
	} else {
		b.ClearModel()
	}
	if a.SeoTitle != nil {
		b.SetSeoTitle(*a.SeoTitle)
	} else {
		b.ClearSeoTitle()
	}
	if a.SeoDescription != nil {
		b.SetSeoDescription(*a.SeoDescription)
	} else {
		b.ClearSeoDescription()
	}
	if a.CanonicalURL != nil {
		b.SetCanonicalURL(*a.CanonicalURL)
	} else {
		b.ClearCanonicalURL()
	}
	if a.OgImage != nil {
		b.SetOgImage(*a.OgImage)
	} else {
		b.ClearOgImage()
	}
	if a.PublishedAt != nil {
		b.SetPublishedAt(*a.PublishedAt)
	} else {
		b.ClearPublishedAt()
	}
}
func setTranslationOptional(b *dbent.ContentTranslationCreate, t *service.ContentTranslation) {
	if t.Excerpt != nil {
		b.SetExcerpt(*t.Excerpt)
	}
	if t.BodyMarkdown != nil {
		b.SetBodyMarkdown(*t.BodyMarkdown)
	}
	if t.BodyHTML != nil {
		b.SetBodyHTML(*t.BodyHTML)
	}
	if t.SeoTitle != nil {
		b.SetSeoTitle(*t.SeoTitle)
	}
	if t.SeoDescription != nil {
		b.SetSeoDescription(*t.SeoDescription)
	}
	if t.CanonicalURL != nil {
		b.SetCanonicalURL(*t.CanonicalURL)
	}
	if t.TranslationModel != nil {
		b.SetTranslationModel(*t.TranslationModel)
	}
	if t.FailureCode != nil {
		b.SetFailureCode(*t.FailureCode)
	}
}
func setTranslationOptionalUpdate(b *dbent.ContentTranslationUpdate, t *service.ContentTranslation) {
	if t.Excerpt != nil {
		b.SetExcerpt(*t.Excerpt)
	} else {
		b.ClearExcerpt()
	}
	if t.BodyMarkdown != nil {
		b.SetBodyMarkdown(*t.BodyMarkdown)
	} else {
		b.ClearBodyMarkdown()
	}
	if t.BodyHTML != nil {
		b.SetBodyHTML(*t.BodyHTML)
	} else {
		b.ClearBodyHTML()
	}
	if t.SeoTitle != nil {
		b.SetSeoTitle(*t.SeoTitle)
	} else {
		b.ClearSeoTitle()
	}
	if t.SeoDescription != nil {
		b.SetSeoDescription(*t.SeoDescription)
	} else {
		b.ClearSeoDescription()
	}
	if t.CanonicalURL != nil {
		b.SetCanonicalURL(*t.CanonicalURL)
	} else {
		b.ClearCanonicalURL()
	}
	if t.TranslationModel != nil {
		b.SetTranslationModel(*t.TranslationModel)
	} else {
		b.ClearTranslationModel()
	}
	if t.FailureCode != nil {
		b.SetFailureCode(*t.FailureCode)
	} else {
		b.ClearFailureCode()
	}
}
