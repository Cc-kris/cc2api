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

// UsageFinanceRecord is the current cost projection for one immutable usage log.
type UsageFinanceRecord struct {
	ent.Schema
}

func (UsageFinanceRecord) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "usage_finance_records"}}
}

func (UsageFinanceRecord) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("usage_log_id").Unique(),
		field.Int64("user_id"),
		field.Int64("group_id").Optional().Nillable(),
		field.Int64("channel_id").Optional().Nillable(),
		field.Int64("account_id").Optional().Nillable(),
		field.Int64("wallet_id").Optional().Nillable(),
		field.Int64("upstream_id").Optional().Nillable(),
		field.Time("usage_created_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("requested_model").MaxLen(100),
		field.String("upstream_model").MaxLen(100).Optional().Nillable(),
		field.String("service_tier").MaxLen(50).Optional().Nillable(),
		field.String("billing_type").MaxLen(30),
		field.String("business_type").MaxLen(30),
		field.Float("usage_list_value").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("upstream_cost").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("cost_status").MaxLen(30),
		field.String("pricing_source").MaxLen(30).Optional().Nillable(),
		field.Int64("price_version_id").Optional().Nillable(),
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
		field.Int("current_revision").Positive().Default(1),
		field.JSON("calculation_detail", map[string]any{}).Default(func() map[string]any { return map[string]any{} }).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("calculated_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UsageFinanceRecord) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("usage_created_at", "id"),
		index.Fields("cost_status", "usage_created_at"),
		index.Fields("group_id", "channel_id", "upstream_id", "wallet_id", "account_id", "usage_created_at"),
		index.Fields("requested_model", "usage_created_at"),
		index.Fields("upstream_model", "usage_created_at"),
	}
}
