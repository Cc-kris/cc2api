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

// UsageFinanceCostSegment stores the cost result for one billable upstream attempt.
type UsageFinanceCostSegment struct {
	ent.Schema
}

func (UsageFinanceCostSegment) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "usage_finance_cost_segments"}}
}

func (UsageFinanceCostSegment) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("usage_finance_record_id"),
		field.Int("attempt_no").Positive(),
		field.Int64("account_id"),
		field.Int64("wallet_id").Optional().Nillable(),
		field.Int64("upstream_id").Optional().Nillable(),
		field.Int64("channel_id").Optional().Nillable(),
		field.String("upstream_model").MaxLen(100),
		field.String("service_tier").MaxLen(50).Optional().Nillable(),
		field.JSON("usage_detail", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Float("upstream_cost_multiplier_snapshot").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).Optional().Nillable(),
		field.Int64("upstream_multiplier_change_id").Optional().Nillable(),
		field.String("upstream_multiplier_source").MaxLen(30).Optional().Nillable(),
		field.Time("upstream_multiplier_effective_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("account_finance_profile_id").Optional().Nillable(),
		field.Int64("fx_rate_version_id").Optional().Nillable(),
		field.String("source_currency").MaxLen(3).Optional().Nillable(),
		field.Float("fx_rate_to_usd").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("fx_source").MaxLen(80).Optional().Nillable(),
		field.Time("fx_observed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("price_version_id").Optional().Nillable(),
		field.String("pricing_source").MaxLen(30).Optional().Nillable(),
		field.String("cost_status").MaxLen(30),
		field.Float("cost_amount").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.JSON("calculation_detail", map[string]any{}).Default(func() map[string]any { return map[string]any{} }).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UsageFinanceCostSegment) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("usage_finance_record_id", "attempt_no").Unique(),
		index.Fields("account_id", "created_at"),
		index.Fields("wallet_id", "created_at"),
	}
}
