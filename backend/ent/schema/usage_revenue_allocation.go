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

// UsageRevenueAllocation links recognized revenue to one usage record.
type UsageRevenueAllocation struct{ ent.Schema }

func (UsageRevenueAllocation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "usage_revenue_allocations"}}
}

func (UsageRevenueAllocation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("usage_log_id"),
		field.String("source_type").MaxLen(40),
		field.Int64("source_id").Optional().Nillable(),
		field.Float("allocated_amount").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.String("allocation_method").MaxLen(30),
		field.Time("recognition_date").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.Int("revision").Positive().Default(1),
		field.Time("invalidated_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("audit_detail", map[string]any{}).Default(func() map[string]any { return map[string]any{} }).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UsageRevenueAllocation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("usage_log_id", "source_type", "source_id", "revision").Unique(),
		index.Fields("recognition_date", "usage_log_id"),
	}
}
