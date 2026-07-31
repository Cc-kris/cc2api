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

// UpstreamBalanceSnapshot keeps wallet cash and token quota as separate facts.
type UpstreamBalanceSnapshot struct{ ent.Schema }

func (UpstreamBalanceSnapshot) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_balance_snapshots"}}
}

func (UpstreamBalanceSnapshot) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("wallet_id"),
		field.String("dedupe_key").MaxLen(200),
		field.String("balance_kind").MaxLen(20),
		field.Float("balance_amount").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("total_quota").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("used_quota").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("currency").MaxLen(3),
		field.String("source").MaxLen(30),
		field.Time("collected_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("sync_status").MaxLen(20),
		field.JSON("safe_snapshot", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamBalanceSnapshot) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "dedupe_key").Unique(),
		index.Fields("wallet_id", "balance_kind", "collected_at"),
	}
}
