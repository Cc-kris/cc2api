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

// FinanceAlert stores actionable profitability and data-quality alerts.
type FinanceAlert struct{ ent.Schema }

func (FinanceAlert) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "finance_alerts"}}
}

func (FinanceAlert) Fields() []ent.Field {
	return []ent.Field{
		field.String("alert_type").MaxLen(50),
		field.String("severity").MaxLen(20),
		field.String("aggregation_key").MaxLen(300),
		field.String("title").MaxLen(200),
		field.String("description").SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.String("dimension_type").MaxLen(50).Optional().Nillable(),
		field.Int64("dimension_id").Optional().Nillable(),
		field.Float("impact_amount").GoType(decimal.Decimal{}).SchemaType(map[string]string{dialect.Postgres: "decimal(20,10)"}).Optional().Nillable(),
		field.Int64("request_count").NonNegative().Default(0),
		field.Int64("occurrence_count").Positive().Default(1),
		field.String("status").MaxLen(20).Default("open"),
		field.Time("first_occurred_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("last_occurred_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Int64("assignee_id").Optional().Nillable(),
		field.Int64("handled_by").Optional().Nillable(),
		field.String("handled_note").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "text"}),
		field.Time("handled_at").Optional().Nillable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (FinanceAlert) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "severity", "last_occurred_at"),
		index.Fields("aggregation_key"),
	}
}
