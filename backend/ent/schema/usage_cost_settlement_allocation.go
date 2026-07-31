package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
	"github.com/shopspring/decimal"
)

// UsageCostSettlementAllocation is an immutable revisioned allocation from one
// settlement interval to one request attempt.
type UsageCostSettlementAllocation struct{ ent.Schema }

func (UsageCostSettlementAllocation) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("settlement_interval_id").Positive(),
		field.Int64("usage_log_id").Positive(),
		field.Int("attempt_no").Positive(),
		field.Int("revision").Positive(),
		field.Float("standard_cost_weight").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("allocation_rate").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(24,16)"}),
		field.Float("allocated_cost").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Time("invalidated_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UsageCostSettlementAllocation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("settlement_interval_id", "usage_log_id", "attempt_no", "revision").Unique(),
		index.Fields("usage_log_id", "attempt_no", "invalidated_at"),
	}
}
