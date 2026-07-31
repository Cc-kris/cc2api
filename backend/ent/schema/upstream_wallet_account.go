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

// UpstreamWalletAccount stores effective-dated account-to-wallet ownership.
type UpstreamWalletAccount struct {
	ent.Schema
}

func (UpstreamWalletAccount) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_wallet_accounts"}}
}

func (UpstreamWalletAccount) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("wallet_id"),
		field.Int64("account_id"),
		field.Time("effective_from").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("effective_to").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.String("reason").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Int64("operator_id").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamWalletAccount) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "effective_from", "effective_to"),
		index.Fields("account_id", "effective_from", "effective_to"),
	}
}
