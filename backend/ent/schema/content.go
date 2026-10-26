package schema

import (
	"time"

	"github.com/Wei-Shaw/sub2api/ent/schema/mixins"
	"github.com/Wei-Shaw/sub2api/internal/domain"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// ContentNewsSource stores an authorized public feed used by the news worker.
type ContentNewsSource struct{ ent.Schema }

func (ContentNewsSource) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_news_sources"}}
}

func (ContentNewsSource) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}, mixins.SoftDeleteMixin{}}
}

func (ContentNewsSource) Fields() []ent.Field {
	return []ent.Field{
		field.String("name").MaxLen(80).NotEmpty(),
		field.String("source_type").MaxLen(20).Default("rss").Validate(domain.ValidateContentNewsSourceType),
		field.JSON("mapping_json", map[string]string{}).Default(func() map[string]string { return map[string]string{} }).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("url").Optional().Nillable().Validate(domain.ValidateContentNewsSourceURL).SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("language").MaxLen(16).Default("zh-CN"),
		field.String("category").MaxLen(80).Optional().Nillable(),
		field.Bool("enabled").Default(false),
		field.Int("interval_minutes").Optional().Nillable(),
		field.Int("rate_limit_per_minute").Default(60),
		field.Time("terms_confirmed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("authorization_note").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("robots_policy").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("next_run_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_run_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("last_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("created_by").Optional().Nillable(),
		field.Int64("updated_by").Optional().Nillable(),
	}
}

func (ContentNewsSource) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("enabled"),
		index.Fields("language"),
		index.Fields("category"),
		index.Fields("next_run_at"),
		index.Fields("last_run_at"),
	}
}

// ContentNewsItem stores the sanitized source article body shown on CCAI.
type ContentNewsItem struct{ ent.Schema }

func (ContentNewsItem) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_news_items"}}
}

func (ContentNewsItem) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}, mixins.SoftDeleteMixin{}}
}

func (ContentNewsItem) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("source_id"),
		field.String("external_id").MaxLen(512).Optional().Nillable(),
		field.String("source_canonical_url").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("slug").MaxLen(80),
		field.String("locale").MaxLen(16).Default("zh-CN"),
		field.String("title").MaxLen(300).NotEmpty(),
		field.String("summary").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("body_html").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("body_text").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("body_source_type").MaxLen(24).Optional().Nillable().Validate(domain.ValidateContentNewsBodySourceType),
		field.Time("body_fetched_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("body_hash").MaxLen(64).Optional().Nillable(),
		field.JSON("media_json", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("image_url").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("source_name").MaxLen(120).Optional().Nillable(),
		field.Time("original_published_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("published_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("status").MaxLen(24).Default("draft").Validate(domain.ValidateContentStatus),
		field.String("publish_mode").MaxLen(24).Default("manual").Validate(domain.ValidateContentNewsPublishMode),
		field.String("seo_title").MaxLen(300).Optional().Nillable(),
		field.String("seo_description").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.Time("fetched_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("content_hash").MaxLen(64).Optional().Nillable(),
		field.Int64("created_by").Optional().Nillable(),
		field.Int64("updated_by").Optional().Nillable(),
	}
}

func (ContentNewsItem) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_id", "external_id").Unique().Annotations(entsql.IndexWhere("external_id IS NOT NULL AND deleted_at IS NULL")),
		index.Fields("source_canonical_url"),
		index.Fields("content_hash"),
		index.Fields("deleted_at"),
		index.Fields("status", "locale", "published_at"),
		index.Fields("slug", "locale").Unique(),
	}
}

func (ContentNewsItem) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("source", ContentNewsSource.Type).
			Ref("news_items").
			Field("source_id").
			Unique().
			Required(),
	}
}

func (ContentNewsSource) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("news_items", ContentNewsItem.Type),
		edge.To("fetch_runs", ContentFetchRun.Type),
	}
}

// ContentArticle stores an editorial article authored in the admin CMS.
type ContentArticle struct{ ent.Schema }

func (ContentArticle) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_articles"}}
}

func (ContentArticle) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}, mixins.SoftDeleteMixin{}}
}

func (ContentArticle) Fields() []ent.Field {
	return []ent.Field{
		field.String("locale").MaxLen(16).Default("zh-CN"),
		field.String("slug").MaxLen(80),
		field.String("title").MaxLen(300).NotEmpty(),
		field.String("excerpt").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("body_markdown").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("body_html").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("cover_image").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("cover_alt").MaxLen(300).Optional().Nillable(),
		field.String("category").MaxLen(80).Optional().Nillable(),
		field.JSON("tags", []string{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("status").MaxLen(24).Default("draft").Validate(domain.ValidateContentStatus),
		field.Int64("author_id").Optional().Nillable(),
		field.String("model").MaxLen(200).Optional().Nillable(),
		field.String("seo_title").MaxLen(300).Optional().Nillable(),
		field.String("seo_description").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.JSON("seo_keywords", []string{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("canonical_url").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("og_image").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.Time("published_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("featured_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int("version").Default(1),
	}
}

func (ContentArticle) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("slug", "locale").Unique(),
		index.Fields("status", "locale", "published_at"),
		index.Fields("status", "featured_at").Annotations(entsql.IndexWhere("featured_at IS NOT NULL")),
		index.Fields("category"),
	}
}

// ContentTranslation stores an independently publishable content translation.
type ContentTranslation struct{ ent.Schema }

func (ContentTranslation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_translations"}}
}

func (ContentTranslation) Mixin() []ent.Mixin {
	return []ent.Mixin{mixins.TimeMixin{}, mixins.SoftDeleteMixin{}}
}

func (ContentTranslation) Fields() []ent.Field {
	return []ent.Field{
		field.String("content_type").MaxLen(24),
		field.Int64("content_id"),
		field.Int("source_version").Positive(),
		field.String("source_locale").MaxLen(16),
		field.String("locale").MaxLen(16),
		field.String("slug").MaxLen(80),
		field.String("title").MaxLen(300).NotEmpty(),
		field.String("excerpt").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("body_markdown").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("body_html").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("seo_title").MaxLen(300).Optional().Nillable(),
		field.String("seo_description").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("canonical_url").SchemaType(map[string]string{dialect.Postgres: "text"}).Optional().Nillable(),
		field.String("translation_model").MaxLen(200).Optional().Nillable(),
		field.String("failure_code").MaxLen(64).Optional().Nillable(),
		field.String("status").MaxLen(24).Default("translation_draft").Validate(domain.ValidateContentTranslationStatus),
		field.Int64("reviewed_by").Optional().Nillable(),
		field.Time("reviewed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("published_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ContentTranslation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("content_type", "content_id", "locale", "source_version").Unique(),
		index.Fields("status", "locale", "published_at"),
	}
}

// ContentAutomationSetting stores the single-site content scheduler settings.
type ContentAutomationSetting struct{ ent.Schema }

func (ContentAutomationSetting) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_automation_settings"}}
}

func (ContentAutomationSetting) Fields() []ent.Field {
	return []ent.Field{
		field.Bool("enabled").Default(false),
		field.String("timezone").MaxLen(64).Default("Asia/Shanghai"),
		field.String("fetch_cron").MaxLen(120).Default("0 */15 * * * *"),
		field.String("publish_mode").MaxLen(24).Default("manual_review").Validate(domain.ValidateContentAutomationPublishMode),
		field.Int("daily_fetch_limit").Default(0),
		field.Int("daily_publish_limit").Default(0),
		field.Time("publish_start_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "time"}),
		field.Time("publish_end_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "time"}),
		field.Int("dedupe_window_hours").Default(24),
		field.JSON("target_locales", []string{"zh-CN"}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int64("updated_by").Optional().Nillable(),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

// ContentTranslationSetting stores the single-site translation defaults.
type ContentTranslationSetting struct{ ent.Schema }

func (ContentTranslationSetting) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_translation_settings"}}
}

func (ContentTranslationSetting) Fields() []ent.Field {
	return []ent.Field{
		field.Bool("enabled").Default(true),
		field.String("default_model").MaxLen(200).Optional().Nillable(),
		field.String("source_locale").MaxLen(16).Default("zh-CN"),
		field.JSON("target_locales", []string{"en-US"}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("glossary", map[string]string{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Bool("auto_create_draft").Default(true),
		field.Int64("updated_by").Optional().Nillable(),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

// ContentFetchRun records one source fetch attempt.
type ContentFetchRun struct{ ent.Schema }

func (ContentFetchRun) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_fetch_runs"}}
}

func (ContentFetchRun) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("source_id"),
		field.String("trigger_type").MaxLen(24).Validate(domain.ValidateContentFetchTrigger),
		field.String("status").MaxLen(24).Default("queued").Validate(domain.ValidateContentFetchStatus),
		field.Time("started_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("finished_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int("fetched_count").Default(0),
		field.Int("created_count").Default(0),
		field.Int("duplicate_count").Default(0),
		field.Int("failed_count").Default(0),
		field.String("error_summary").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("task_key").MaxLen(200),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ContentFetchRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_id", "task_key").Unique(),
		index.Fields("status", "created_at"),
	}
}

func (ContentFetchRun) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("source", ContentNewsSource.Type).
			Ref("fetch_runs").
			Field("source_id").
			Unique().
			Required(),
	}
}

// ContentAnalyticsEvent stores a privacy-preserving public event.
type ContentAnalyticsEvent struct{ ent.Schema }

func (ContentAnalyticsEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_analytics_events"}}
}

func (ContentAnalyticsEvent) Fields() []ent.Field {
	return []ent.Field{
		field.String("event_id").MaxLen(128).Unique(),
		field.String("event_name").MaxLen(32),
		field.String("content_type").MaxLen(24).Optional().Nillable(),
		field.Int64("content_id").Optional().Nillable(),
		field.String("locale").MaxLen(16).Optional().Nillable(),
		field.String("placement").MaxLen(64).Optional().Nillable(),
		field.String("page_path").MaxLen(512),
		field.String("session_hash").MaxLen(128),
		field.String("referrer_host").MaxLen(255).Optional().Nillable(),
		field.Time("event_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("payload_json", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ContentAnalyticsEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("event_name", "event_at"),
		index.Fields("content_type", "content_id", "event_at"),
		index.Fields("session_hash", "event_at"),
	}
}

// ContentAnalyticsDaily stores aggregate metrics for BI queries.
type ContentAnalyticsDaily struct{ ent.Schema }

func (ContentAnalyticsDaily) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_analytics_daily"}}
}

func (ContentAnalyticsDaily) Fields() []ent.Field {
	return []ent.Field{
		field.Time("date").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.String("locale").MaxLen(16),
		field.String("content_type").MaxLen(24),
		field.Int64("content_id").Default(0),
		field.String("page_path").MaxLen(512),
		field.String("placement").MaxLen(64).Default(""),
		field.Int("unique_visitors").Default(0),
		field.Int("page_views").Default(0),
		field.Int("reads").Default(0),
		field.Int("impressions").Default(0),
		field.Int("clicks").Default(0),
		field.Int("avg_read_seconds").Default(0),
		field.Float("ctr").Default(0),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ContentAnalyticsDaily) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("date", "locale", "content_type", "content_id", "page_path", "placement").Unique(),
		index.Fields("date", "content_type", "reads"),
	}
}

// ContentSlugRedirect keeps historical public URLs after a slug change.
type ContentSlugRedirect struct{ ent.Schema }

func (ContentSlugRedirect) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_slug_redirects"}}
}

func (ContentSlugRedirect) Fields() []ent.Field {
	return []ent.Field{
		field.String("content_type").MaxLen(24),
		field.Int64("content_id"),
		field.String("locale").MaxLen(16),
		field.String("old_slug").MaxLen(80),
		field.String("new_slug").MaxLen(80),
		field.Int("status_code").Default(301),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ContentSlugRedirect) Indexes() []ent.Index {
	return []ent.Index{index.Fields("content_type", "locale", "old_slug").Unique()}
}

// ContentAuditLog stores content-management actions without secret values.
type ContentAuditLog struct{ ent.Schema }

func (ContentAuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "content_audit_logs"}}
}

func (ContentAuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("actor_id").Optional().Nillable(),
		field.String("content_type").MaxLen(24),
		field.Int64("content_id").Optional().Nillable(),
		field.String("action").MaxLen(64),
		field.JSON("before_snapshot", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("after_snapshot", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("request_id").MaxLen(128).Optional().Nillable(),
		field.Time("created_at").Immutable().Default(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (ContentAuditLog) Indexes() []ent.Index {
	return []ent.Index{index.Fields("content_type", "content_id", "created_at"), index.Fields("actor_id", "created_at")}
}
