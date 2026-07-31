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

// FinanceFXRateVersion freezes one source-currency to USD conversion rate.
// Financial records reference this version instead of re-reading a live rate.
type FinanceFXRateVersion struct{ ent.Schema }

func (FinanceFXRateVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "finance_fx_rate_versions"}}
}

func (FinanceFXRateVersion) Fields() []ent.Field {
	return []ent.Field{
		field.String("currency").MaxLen(3),
		field.Float("rate_to_usd").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.String("source").MaxLen(80),
		field.Time("observed_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("effective_from").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("effective_to").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("checksum").MaxLen(128),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (FinanceFXRateVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("currency", "rate_to_usd", "source", "effective_from").Unique(),
		index.Fields("currency", "effective_from", "effective_to"),
	}
}
