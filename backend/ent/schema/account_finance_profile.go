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

// AccountFinanceProfile stores one immutable effective-dated account finance configuration.
type AccountFinanceProfile struct{ ent.Schema }

func (AccountFinanceProfile) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "account_finance_profiles"}}
}

func (AccountFinanceProfile) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("account_id").Positive(),
		field.Int64("wallet_id").Optional().Nillable(),
		field.Int64("protocol_version_id").Optional().Nillable(),
		field.String("cost_mode").MaxLen(40),
		field.String("pricing_group").MaxLen(100).Optional().Nillable(),
		field.String("endpoint_source").MaxLen(30).Default("account_base_url"),
		field.String("endpoint_base_url_snapshot").SchemaType(map[string]string{dialect.Postgres: "text"}).Default(""),
		field.String("credential_source").MaxLen(40).Default("account_api_key"),
		field.String("counter_scope").MaxLen(30).Default("account"),
		field.String("counter_scope_key").MaxLen(200).Optional().Nillable(),
		field.String("balance_unit_semantics").MaxLen(30).Default("none"),
		field.String("recharge_owner_type").MaxLen(20).Optional().Nillable(),
		field.Int64("recharge_owner_id").Optional().Nillable(),
		field.Int64("account_multiplier_change_id").Optional().Nillable(),
		field.Float("account_multiplier_snapshot").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Float("raw_upstream_multiplier").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("contract_type").MaxLen(30).Optional().Nillable(),
		field.Float("contract_multiplier").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Int64("contract_multiplier_change_id").Optional().Nillable(),
		field.String("readiness_status").MaxLen(30),
		field.JSON("readiness_detail", map[string]any{}).Optional().SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int("version").Positive().Default(1),
		field.Time("effective_from").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("effective_to").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("created_by").Optional().Nillable(),
		field.String("reason").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (AccountFinanceProfile) Indexes() []ent.Index {
	return []ent.Index{index.Fields("account_id", "effective_from"), index.Fields("account_id", "version").Unique()}
}
