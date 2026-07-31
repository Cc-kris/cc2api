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

// UpstreamWallet represents one settlement wallet under an upstream site.
type UpstreamWallet struct {
	ent.Schema
}

func (UpstreamWallet) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_wallets"}}
}

func (UpstreamWallet) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("upstream_id"),
		field.String("name").MaxLen(120),
		field.String("base_url").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("pricing_adapter").MaxLen(30).Default("manual"),
		field.String("pricing_group").MaxLen(100).Optional().Nillable(),
		field.String("balance_adapter").MaxLen(30).Default("manual"),
		field.String("quota_adapter").MaxLen(30).Default("none"),
		field.String("balance_scope_key").MaxLen(200).Optional().Nillable(),
		field.Bytes("finance_access_token_encrypted").Optional().Nillable(),
		field.Int64("protocol_version_id").Optional().Nillable(),
		field.String("currency").MaxLen(3).Default("USD"),
		field.String("balance_kind").MaxLen(20).Default("wallet_cash"),
		field.Bool("enabled").Default(true),
		field.Time("last_pricing_sync_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("pricing_sync_status").MaxLen(20).Default("idle"),
		field.String("pricing_sync_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("last_balance_sync_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("balance_sync_status").MaxLen(20).Default("idle"),
		field.String("balance_sync_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("last_quota_sync_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("quota_sync_status").MaxLen(20).Default("idle"),
		field.String("quota_sync_error").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("deleted_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamWallet) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("upstream_id", "enabled", "deleted_at"),
		index.Fields("balance_scope_key", "deleted_at"),
		index.Fields("protocol_version_id"),
	}
}
