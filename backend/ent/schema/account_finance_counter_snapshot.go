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

// AccountFinanceCounterSnapshot preserves one immutable upstream cumulative-cost
// observation plus the local derivation result produced from its predecessor.
type AccountFinanceCounterSnapshot struct{ ent.Schema }

func (AccountFinanceCounterSnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "account_finance_counter_snapshots"}}
}

func (AccountFinanceCounterSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.Int64("account_finance_profile_id").Optional().Nillable(),
		field.String("scope_key").MaxLen(200),
		field.String("idempotency_key").MaxLen(200),
		field.String("upstream_counter_id").MaxLen(200).Optional().Nillable(),
		field.String("counter_period").MaxLen(100).Optional().Nillable(),
		field.Float("list_cost_total").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("actual_cost_total").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("unit_code").MaxLen(30),
		field.String("unit_semantics").MaxLen(30),
		field.String("currency").MaxLen(3).Optional().Nillable(),
		field.Time("upstream_observed_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("collected_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.JSON("safe_snapshot", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("checksum").MaxLen(64),
		field.Int64("previous_snapshot_id").Optional().Nillable(),
		field.Float("list_cost_delta").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("actual_cost_delta").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("observed_multiplier").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("derivation_status").MaxLen(40),
		field.String("anomaly_code").MaxLen(40).Optional().Nillable(),
		field.Int64("multiplier_change_id").Optional().Nillable(),
		field.Time("multiplier_effective_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AccountFinanceCounterSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "scope_key", "idempotency_key").Unique(),
		index.Fields("account_id", "scope_key", "collected_at", "id"),
		index.Fields("account_finance_profile_id", "collected_at", "id"),
		index.Fields("account_id", "derivation_status", "collected_at"),
	}
}
