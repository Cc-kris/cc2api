package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UpstreamFinanceSyncRun records probe, pricing, balance, quota and bill syncs.
type UpstreamFinanceSyncRun struct{ ent.Schema }

func (UpstreamFinanceSyncRun) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_finance_sync_runs"}}
}

func (UpstreamFinanceSyncRun) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("async_job_id").Optional().Nillable(),
		field.Int64("wallet_id"),
		field.String("sync_type").MaxLen(20),
		field.String("status").MaxLen(20),
		field.Int64("collected_count").NonNegative().Default(0),
		field.Int64("skipped_count").NonNegative().Default(0),
		field.Int("upstream_status").Optional().Nillable(),
		field.Int64("duration_ms").Optional().Nillable(),
		field.String("error_summary").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("started_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("finished_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamFinanceSyncRun) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "sync_type", "status"),
		index.Fields("wallet_id", "sync_type", "created_at"),
	}
}
