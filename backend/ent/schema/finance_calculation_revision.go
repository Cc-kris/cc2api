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

// FinanceCalculationRevision is the append-only audit history for recalculation.
type FinanceCalculationRevision struct {
	ent.Schema
}

func (FinanceCalculationRevision) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "finance_calculation_revisions"}}
}

func (FinanceCalculationRevision) Fields() []ent.Field {
	return []ent.Field{
		field.String("entity_type").MaxLen(50),
		field.Int64("entity_id"),
		field.Int("revision").Positive(),
		field.JSON("old_result", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.JSON("new_result", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("reason").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("job_id").Optional().Nillable(),
		field.Int64("operator_id").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (FinanceCalculationRevision) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("entity_type", "entity_id", "revision").Unique(),
		index.Fields("job_id", "created_at"),
	}
}
