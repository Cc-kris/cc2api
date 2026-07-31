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

// SystemModelPriceVersion stores immutable versions of the system model price catalog.
type SystemModelPriceVersion struct {
	ent.Schema
}

func (SystemModelPriceVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "system_model_price_versions"}}
}

func (SystemModelPriceVersion) Fields() []ent.Field {
	return []ent.Field{
		field.String("catalog_checksum").MaxLen(128),
		field.String("provider").MaxLen(50),
		field.String("model_name").MaxLen(100),
		field.String("billing_mode").MaxLen(20),
		field.JSON("price_detail", map[string]any{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Time("effective_from").
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("effective_to").
			Optional().
			Nillable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SystemModelPriceVersion) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("catalog_checksum", "provider", "model_name", "billing_mode", "effective_from").Unique(),
		index.Fields("provider", "model_name", "effective_from", "effective_to"),
	}
}
