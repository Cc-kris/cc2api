package schema

import (
	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// FinanceDailyAggregate stores revisioned daily reporting dimensions.
type FinanceDailyAggregate struct{ ent.Schema }

func (FinanceDailyAggregate) Annotations() []schema.Annotation {
	return []schema.Annotation{entsql.Annotation{Table: "finance_daily_aggregates"}}
}

func (FinanceDailyAggregate) Fields() []ent.Field {
	return []ent.Field{
		field.Time("aggregate_date").SchemaType(map[string]string{dialect.Postgres: "date"}),
		field.String("timezone").MaxLen(100),
		field.String("dimension_type").MaxLen(50),
		field.String("dimension_key").MaxLen(200),
		field.JSON("metric_detail", map[string]any{}).SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.Int64("source_revision"),
		field.Time("generated_at").SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (FinanceDailyAggregate) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("aggregate_date", "timezone", "dimension_type", "dimension_key").Unique(),
		index.Fields("dimension_type", "dimension_key", "aggregate_date"),
	}
}
