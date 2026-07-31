package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
)

// FinanceExportJob stores immutable report-export parameters and output metadata.
type FinanceExportJob struct{ ent.Schema }

func (FinanceExportJob) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "finance_export_jobs"}}
}

func (FinanceExportJob) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("async_job_id").Unique(),
		field.String("report").MaxLen(50),
		field.String("format").MaxLen(20).Default("csv"),
		field.JSON("filters", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("timezone").MaxLen(100),
		field.String("storage_key").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("file_size").Optional().Nillable(),
		field.Int64("row_count").Optional().Nillable(),
		field.Time("expires_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}
