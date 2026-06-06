// Package schema 定义 Ent ORM 的数据库 schema。
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

// RequestAuditLog stores request/response bodies for admin audit drill-down.
type RequestAuditLog struct {
	ent.Schema
}

// Annotations returns schema annotations.
func (RequestAuditLog) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entsql.Annotation{Table: "request_audit_logs"},
	}
}

// Fields defines request audit log columns.
func (RequestAuditLog) Fields() []ent.Field {
	return []ent.Field{
		field.String("request_id").MaxLen(128).Optional().Nillable(),
		field.Int64("user_id"),
		field.Int64("api_key_id"),
		field.Int64("account_id").Optional().Nillable(),
		field.Int64("group_id").Optional().Nillable(),
		field.String("platform").MaxLen(32).NotEmpty(),
		field.String("endpoint").MaxLen(128).Optional().Nillable(),
		field.String("model").MaxLen(128).Optional().Nillable(),
		field.Bool("stream").Default(false),
		field.Int("status_code").Optional().Nillable(),
		field.Int("duration_ms").Optional().Nillable(),
		field.Text("request_body").Optional().Nillable(),
		field.Text("response_body").Optional().Nillable(),
		field.Bool("request_body_truncated").Default(false),
		field.Bool("response_body_truncated").Default(false),
		field.Int("request_body_bytes").Default(0),
		field.Int("response_body_bytes").Default(0),
		field.Bool("is_mocked").Default(false),
		field.Int64("mock_rule_id").Optional().Nillable(),
		field.String("error_message").MaxLen(1024).Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

// Indexes defines request audit log indexes.
func (RequestAuditLog) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at"),
		index.Fields("user_id"),
		index.Fields("api_key_id"),
		index.Fields("account_id"),
		index.Fields("group_id"),
		index.Fields("platform"),
		index.Fields("model"),
		index.Fields("request_id"),
		index.Fields("is_mocked"),
	}
}
