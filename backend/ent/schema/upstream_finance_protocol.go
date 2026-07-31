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

// UpstreamFinanceProtocol is the stable identity of an upstream finance protocol.
type UpstreamFinanceProtocol struct{ ent.Schema }

func (UpstreamFinanceProtocol) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_finance_protocols"}}
}

func (UpstreamFinanceProtocol) Fields() []ent.Field {
	return []ent.Field{
		field.String("code").MaxLen(80).Unique().Immutable(),
		field.String("name").MaxLen(120),
		field.String("protocol_type").MaxLen(20),
		field.String("status").MaxLen(20).Default("draft"),
		field.Int64("current_version_id").Optional().Nillable(),
		field.Int64("created_by").Optional().Nillable(),
		field.Int64("updated_by").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamFinanceProtocol) Indexes() []ent.Index {
	return []ent.Index{index.Fields("status", "protocol_type"), index.Fields("updated_at", "id")}
}
