package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

// UpstreamCostSettlementInterval freezes one cumulative upstream-cost delta and
// the local request interval it settles. Recharge records are deliberately not
// related to this entity.
type UpstreamCostSettlementInterval struct{ ent.Schema }

func (UpstreamCostSettlementInterval) Fields() []ent.Field {
	return []ent.Field{
		field.String("owner_type").MaxLen(20),
		field.Int64("owner_id").Positive(),
		field.Int64("account_id").Optional().Nillable(),
		field.Int64("account_finance_profile_id").Optional().Nillable(),
		field.Int64("wallet_id").Optional().Nillable(),
		field.String("scope_key").MaxLen(240),
		field.Int64("previous_snapshot_id").Positive(),
		field.Int64("current_snapshot_id").Positive(),
		field.Time("period_start").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("period_end").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("unit_semantics").MaxLen(30),
		field.String("currency").MaxLen(3).Optional().Nillable(),
		field.Int64("fx_rate_version_id").Optional().Nillable(),
		field.Float("fx_rate_to_usd").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("fx_source").MaxLen(80).Optional().Nillable(),
		field.Time("fx_observed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Float("list_cost_delta").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("actual_cost_delta").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("observed_multiplier").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("status").MaxLen(30).Default("pending"),
		field.Int("current_revision").Positive().Default(1),
		field.Int64("request_count").NonNegative().Default(0),
		field.Int64("segment_count").NonNegative().Default(0),
		field.Float("standard_cost_total").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("allocated_cost_total").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("difference_amount").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("error_summary").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.Time("settled_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamCostSettlementInterval) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("scope_key", "previous_snapshot_id", "current_snapshot_id").Unique(),
		index.Fields("owner_type", "owner_id", "period_end"),
		index.Fields("status", "created_at"),
	}
}
