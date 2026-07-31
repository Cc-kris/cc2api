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

// PaymentProviderFeeEvent stores confirmed or uncollected payment fees.
type PaymentProviderFeeEvent struct{ ent.Schema }

func (PaymentProviderFeeEvent) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "payment_provider_fee_events"}}
}

func (PaymentProviderFeeEvent) Fields() []ent.Field {
	money := func(name string) ent.Field {
		return field.Float(name).GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable()
	}
	return []ent.Field{
		field.Int64("payment_order_id").Optional().Nillable(),
		field.String("provider").MaxLen(50),
		field.String("bill_event_id").MaxLen(200),
		money("gross_amount"), money("fee_amount"), money("net_amount"),
		field.String("currency").MaxLen(3),
		money("fx_rate_to_usd"), money("gross_usd_amount"), money("fee_usd_amount"), money("net_usd_amount"),
		field.String("fee_status").MaxLen(20),
		field.String("source").MaxLen(30),
		field.Time("occurred_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PaymentProviderFeeEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("provider", "bill_event_id").Unique(),
		index.Fields("payment_order_id", "occurred_at"),
		index.Fields("fee_status", "occurred_at"),
	}
}
