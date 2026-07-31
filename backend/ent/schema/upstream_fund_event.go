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

// UpstreamFundEvent records opening balances, top-ups, refunds and adjustments.
type UpstreamFundEvent struct{ ent.Schema }

func (UpstreamFundEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_fund_events"}}
}

func (UpstreamFundEvent) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("wallet_id"),
		field.String("event_type").MaxLen(30),
		field.Float("original_amount").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.String("currency").MaxLen(3),
		field.Float("fx_rate_to_usd").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.String("fx_source").MaxLen(80).Default("manual"),
		field.Time("fx_observed_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("fx_rate_version_id").Optional().Nillable(),
		field.Float("usd_amount").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("base_credit_units").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("bonus_credit_units").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("total_credit_units").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("base_recharge_ratio").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("effective_recharge_ratio").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("bonus_income_original").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("bonus_income_usd").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("bonus_status").MaxLen(30).Default("not_applicable"),
		field.Int64("reversed_event_id").Optional().Nillable(),
		field.Time("occurred_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("reference_no").MaxLen(200).Optional().Nillable(),
		field.String("note").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("operator_id").Optional().Nillable(),
		field.String("idempotency_key").MaxLen(200),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamFundEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "idempotency_key").Unique(),
		index.Fields("wallet_id", "occurred_at"),
		index.Fields("wallet_id", "reversed_event_id").Unique(),
		index.Fields("bonus_status", "occurred_at"),
	}
}
