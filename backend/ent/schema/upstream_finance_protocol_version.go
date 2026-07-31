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

// UpstreamFinanceProtocolVersion stores an immutable protocol definition.
type UpstreamFinanceProtocolVersion struct{ ent.Schema }

func (UpstreamFinanceProtocolVersion) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_finance_protocol_versions"}}
}

func (UpstreamFinanceProtocolVersion) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("protocol_id"),
		field.Int("version").Positive(),
		field.JSON("config", map[string]any{}),
		field.String("checksum").MaxLen(64),
		field.String("validation_status").MaxLen(20).Default("pending"),
		field.JSON("validation_result", map[string]any{}).Optional(),
		field.Time("published_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("created_by").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamFinanceProtocolVersion) Indexes() []ent.Index {
	return []ent.Index{index.Fields("protocol_id", "version").Unique(), index.Fields("protocol_id", "created_at")}
}
