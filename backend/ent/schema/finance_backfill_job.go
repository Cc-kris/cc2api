package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// FinanceBackfillJob stores the approved scope of a historical recalculation.
type FinanceBackfillJob struct{ ent.Schema }

func (FinanceBackfillJob) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "finance_backfill_jobs"}}
}

func (FinanceBackfillJob) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("async_job_id").Unique(),
		field.Time("start_date").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.Time("end_date").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.String("mode").MaxLen(30),
		field.String("pricing_policy").MaxLen(30),
		field.String("preview_token_hash").MaxLen(128),
		field.Time("preview_expires_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("scope", map[string]any{}).Default(func() map[string]any { return map[string]any{} }).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("reason").SchemaType(map[string]string{dialect.Postgres: "text"}),
	}
}
