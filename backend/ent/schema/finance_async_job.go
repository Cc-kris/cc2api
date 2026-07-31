package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

// FinanceAsyncJob is the durable execution envelope for finance jobs.
type FinanceAsyncJob struct{ ent.Schema }

func (FinanceAsyncJob) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "finance_async_jobs"}}
}

func (FinanceAsyncJob) Fields() []ent.Field {
	return []ent.Field{
		field.String("job_type").MaxLen(50),
		field.String("status").MaxLen(20),
		field.String("idempotency_key").MaxLen(200).Optional().Nillable(),
		field.String("request_checksum").MaxLen(128).Optional().Nillable(),
		field.JSON("parameters", map[string]any{}).Default(func() map[string]any { return map[string]any{} }).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("cursor", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Float("progress").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).Optional(),
		field.Int64("processed_count").NonNegative().Default(0),
		field.Int64("success_count").NonNegative().Default(0),
		field.Int64("failed_count").NonNegative().Default(0),
		field.String("lease_owner").MaxLen(200).Optional().Nillable(),
		field.Time("lease_expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("error_summary").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("operator_id").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("started_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("finished_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (FinanceAsyncJob) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("job_type", "operator_id", "idempotency_key"),
		index.Fields("job_type", "status", "created_at"),
	}
}
