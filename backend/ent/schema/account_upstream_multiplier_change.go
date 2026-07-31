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

// AccountUpstreamMultiplierChange records every upstream cost multiplier change.
type AccountUpstreamMultiplierChange struct {
	ent.Schema
}

func (AccountUpstreamMultiplierChange) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "account_upstream_multiplier_changes"}}
}

func (AccountUpstreamMultiplierChange) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id"),
		field.Float("old_multiplier").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}).Optional().Nillable(),
		field.Float("new_multiplier").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(10,4)"}),
		field.String("change_type").MaxLen(20),
		field.Time("effective_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("operator_id").Optional().Nillable(),
		field.String("reason").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AccountUpstreamMultiplierChange) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("account_id", "effective_at", "id"),
	}
}
