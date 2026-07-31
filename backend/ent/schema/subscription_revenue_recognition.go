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

// SubscriptionRevenueRecognition records daily recognized subscription revenue.
type SubscriptionRevenueRecognition struct{ ent.Schema }

func (SubscriptionRevenueRecognition) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "subscription_revenue_recognitions"}}
}

func (SubscriptionRevenueRecognition) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("payment_order_id"),
		field.Int64("user_id"),
		field.Int64("group_id").Optional().Nillable(),
		field.Time("recognition_date").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.Float("recognized_revenue").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}),
		field.Float("refund_reduction").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional(),
		field.Float("allocated_revenue").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional(),
		field.Float("unallocated_revenue").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional(),
		field.String("allocation_status").MaxLen(30),
		field.JSON("calculation_detail", map[string]any{}).Default(func() map[string]any { return map[string]any{} }).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int("current_revision").Positive().Default(1),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (SubscriptionRevenueRecognition) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("payment_order_id", "recognition_date").Unique(),
		index.Fields("recognition_date", "user_id", "group_id"),
	}
}
