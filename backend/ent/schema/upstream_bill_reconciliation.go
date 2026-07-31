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

// UpstreamBillReconciliation compares an upstream bill with calculated cost.
type UpstreamBillReconciliation struct{ ent.Schema }

func (UpstreamBillReconciliation) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "upstream_bill_reconciliations"}}
}

func (UpstreamBillReconciliation) Fields() []ent.Field {
	money := func(name string) ent.Field {
		return field.Float(name).GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"})
	}
	return []ent.Field{
		field.Int64("wallet_id"),
		field.Time("period_start").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("period_end").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		money("upstream_bill_amount"), money("system_cost_amount"), money("difference_amount"),
		field.Float("difference_rate").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.String("currency").MaxLen(3),
		field.String("source_reference").MaxLen(200).Optional().Nillable(),
		field.String("source_file_checksum").MaxLen(128),
		field.String("status").MaxLen(20),
		field.Int64("handled_by").Optional().Nillable(),
		field.String("handled_note").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("handled_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (UpstreamBillReconciliation) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("wallet_id", "period_start", "period_end", "source_file_checksum").Unique(),
		index.Fields("status", "period_start"),
	}
}
