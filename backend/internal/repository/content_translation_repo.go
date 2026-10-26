package repository

import (
	"context"
	"time"

	dbent "github.com/Wei-Shaw/sub2api/ent"
	"github.com/Wei-Shaw/sub2api/ent/contenttranslation"
	"github.com/Wei-Shaw/sub2api/internal/domain"
	"github.com/Wei-Shaw/sub2api/internal/service"
)

type contentTranslationRepository struct{ client *dbent.Client }

func NewContentTranslationRepository(c *dbent.Client) service.ContentTranslationStore {
	return &contentTranslationRepository{client: c}
}
func (r *contentTranslationRepository) Get(ctx context.Context, id int64) (*service.ContentTranslation, error) {
	if r == nil || r.client == nil || id <= 0 {
		return nil, service.ErrContentTranslationNotFound
	}
	v, e := r.client.ContentTranslation.Query().Where(contenttranslation.IDEQ(id), contenttranslation.DeletedAtIsNil()).Only(ctx)
	if dbent.IsNotFound(e) {
		return nil, service.ErrContentTranslationNotFound
	}
	if e != nil {
		return nil, e
	}
	return fromTranslation(v), nil
}
func (r *contentTranslationRepository) UpsertDraft(ctx context.Context, t *service.ContentTranslation) (*service.ContentTranslation, error) {
	if r == nil || r.client == nil || t == nil {
		return nil, service.ErrContentArticleInvalid
	}
	tx, e := r.client.Tx(ctx)
	if e != nil {
		return nil, e
	}
	q := tx.Client().ContentTranslation.Query().Where(contenttranslation.ContentTypeEQ(t.ContentType), contenttranslation.ContentIDEQ(t.ContentID), contenttranslation.LocaleEQ(t.Locale), contenttranslation.SourceVersionEQ(t.SourceVersion), contenttranslation.DeletedAtIsNil())
	old, e := q.First(ctx)
	if e != nil && !dbent.IsNotFound(e) {
		_ = tx.Rollback()
		return nil, e
	}
	if old != nil {
		if old.Status == domain.ContentTranslationPublished {
			_ = tx.Rollback()
			return fromTranslation(old), nil
		}
		v, e := applyTranslation(tx.Client().ContentTranslation.UpdateOneID(old.ID), t).Save(ctx)
		if e != nil {
			_ = tx.Rollback()
			return nil, e
		}
		if e = tx.Commit(); e != nil {
			return nil, e
		}
		return fromTranslation(v), nil
	}
	v, e := createTranslation(tx.Client(), t).Save(ctx)
	if e != nil {
		_ = tx.Rollback()
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return fromTranslation(v), nil
}
func (r *contentTranslationRepository) Save(ctx context.Context, t *service.ContentTranslation) (*service.ContentTranslation, error) {
	if r == nil || r.client == nil || t == nil || t.ID <= 0 {
		return nil, service.ErrContentTranslationNotFound
	}
	u := applyTranslation(r.client.ContentTranslation.UpdateOneID(t.ID), t)
	if t.DeletedAt == nil {
		u.ClearDeletedAt()
	} else {
		u.SetDeletedAt(*t.DeletedAt)
	}
	v, e := u.Save(ctx)
	if dbent.IsNotFound(e) {
		return nil, service.ErrContentTranslationNotFound
	}
	if e != nil {
		return nil, e
	}
	return fromTranslation(v), nil
}
func (r *contentTranslationRepository) PublishCurrent(ctx context.Context, id int64, version int, at time.Time) (*service.ContentTranslation, error) {
	if r == nil || r.client == nil {
		return nil, service.ErrContentTranslationNotFound
	}
	tx, e := r.client.Tx(ctx)
	if e != nil {
		return nil, e
	}
	c := tx.Client()
	cur, e := c.ContentTranslation.Query().Where(contenttranslation.IDEQ(id), contenttranslation.SourceVersionEQ(version), contenttranslation.DeletedAtIsNil()).Only(ctx)
	if e != nil {
		_ = tx.Rollback()
		if dbent.IsNotFound(e) {
			return nil, service.ErrContentTranslationNotFound
		}
		return nil, e
	}
	_, e = c.ContentTranslation.Update().Where(contenttranslation.ContentTypeEQ(cur.ContentType), contenttranslation.ContentIDEQ(cur.ContentID), contenttranslation.LocaleEQ(cur.Locale), contenttranslation.StatusEQ(domain.ContentTranslationPublished), contenttranslation.IDNEQ(cur.ID)).SetStatus(domain.ContentTranslationUnpublished).ClearPublishedAt().Save(ctx)
	if e != nil {
		_ = tx.Rollback()
		return nil, e
	}
	v, e := c.ContentTranslation.UpdateOneID(id).SetStatus(domain.ContentTranslationPublished).SetPublishedAt(at).ClearFailureCode().ClearDeletedAt().Save(ctx)
	if e != nil {
		_ = tx.Rollback()
		return nil, e
	}
	if e = tx.Commit(); e != nil {
		return nil, e
	}
	return fromTranslation(v), nil
}
func (r *contentTranslationRepository) Unpublish(ctx context.Context, id int64, at time.Time) (*service.ContentTranslation, error) {
	t, e := r.Get(ctx, id)
	if e != nil {
		return nil, e
	}
	if t.Status != domain.ContentTranslationPublished {
		return nil, service.ErrContentTranslationState
	}
	t.Status = domain.ContentTranslationUnpublished
	t.PublishedAt = nil
	return r.Save(ctx, t)
}
func createTranslation(c *dbent.Client, t *service.ContentTranslation) *dbent.ContentTranslationCreate {
	return c.ContentTranslation.Create().SetContentType(t.ContentType).SetContentID(t.ContentID).SetSourceVersion(t.SourceVersion).SetSourceLocale(t.SourceLocale).SetLocale(t.Locale).SetSlug(t.Slug).SetTitle(t.Title).SetNillableExcerpt(t.Excerpt).SetNillableBodyMarkdown(t.BodyMarkdown).SetNillableBodyHTML(t.BodyHTML).SetNillableSeoTitle(t.SeoTitle).SetNillableSeoDescription(t.SeoDescription).SetNillableCanonicalURL(t.CanonicalURL).SetNillableTranslationModel(t.TranslationModel).SetNillableFailureCode(t.FailureCode).SetStatus(t.Status)
}
func applyTranslation(u *dbent.ContentTranslationUpdateOne, t *service.ContentTranslation) *dbent.ContentTranslationUpdateOne {
	u.SetContentType(t.ContentType).SetContentID(t.ContentID).SetSourceVersion(t.SourceVersion).SetSourceLocale(t.SourceLocale).SetLocale(t.Locale).SetSlug(t.Slug).SetTitle(t.Title).SetNillableExcerpt(t.Excerpt).SetNillableBodyMarkdown(t.BodyMarkdown).SetNillableBodyHTML(t.BodyHTML).SetNillableSeoTitle(t.SeoTitle).SetNillableSeoDescription(t.SeoDescription).SetNillableCanonicalURL(t.CanonicalURL).SetNillableTranslationModel(t.TranslationModel).SetNillableFailureCode(t.FailureCode).SetStatus(t.Status)
	if t.Excerpt == nil {
		u.ClearExcerpt()
	}
	if t.BodyMarkdown == nil {
		u.ClearBodyMarkdown()
	}
	if t.BodyHTML == nil {
		u.ClearBodyHTML()
	}
	if t.SeoTitle == nil {
		u.ClearSeoTitle()
	}
	if t.SeoDescription == nil {
		u.ClearSeoDescription()
	}
	if t.CanonicalURL == nil {
		u.ClearCanonicalURL()
	}
	if t.TranslationModel == nil {
		u.ClearTranslationModel()
	}
	if t.FailureCode == nil {
		u.ClearFailureCode()
	}
	return u
}
func fromTranslation(v *dbent.ContentTranslation) *service.ContentTranslation {
	return &service.ContentTranslation{ID: v.ID, CreatedAt: v.CreatedAt, UpdatedAt: v.UpdatedAt, DeletedAt: v.DeletedAt, ContentType: v.ContentType, ContentID: v.ContentID, SourceVersion: v.SourceVersion, SourceLocale: v.SourceLocale, Locale: v.Locale, Slug: v.Slug, Title: v.Title, Excerpt: v.Excerpt, BodyMarkdown: v.BodyMarkdown, BodyHTML: v.BodyHTML, SeoTitle: v.SeoTitle, SeoDescription: v.SeoDescription, CanonicalURL: v.CanonicalURL, TranslationModel: v.TranslationModel, FailureCode: v.FailureCode, Status: v.Status, ReviewedBy: v.ReviewedBy, ReviewedAt: v.ReviewedAt, PublishedAt: v.PublishedAt}
}
