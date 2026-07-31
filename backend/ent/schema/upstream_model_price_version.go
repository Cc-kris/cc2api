package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// UpstreamModelPriceVersion stores immutable exact or wildcard upstream prices.
type UpstreamModelPriceVersion struct {
	ent.Schema
}

func (UpstreamModelPriceVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_model_price_versions"}}
}

func (UpstreamModelPriceVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("wallet_id"),
		field.String("model_pattern").MaxLen(150),
		field.Bool("is_wildcard").Default(false),
		field.String("billing_mode").MaxLen(20),
		field.String("service_tier").MaxLen(50).Optional().Nillable(),
		field.JSON("price_detail", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("currency").MaxLen(3).Default("USD"),
		field.String("source").MaxLen(30),
		field.JSON("source_snapshot", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("checksum").MaxLen(128),
		field.Time("effective_from").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("effective_to").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamModelPriceVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "model_pattern", "effective_from", "effective_to"),
		index.Fields("wallet_id", "model_pattern", "billing_mode", "service_tier", "effective_from", "checksum").Unique(),
	}
}
