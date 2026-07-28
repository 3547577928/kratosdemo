package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
)

// User holds the schema definition for the User entity.
type User struct {
	ent.Schema
}

// Fields of the User.
func (User) Fields() []ent.Field {
	return []ent.Field{
		field.Int64("id").
			Positive().
			Immutable().
			StorageKey("id"),
		field.String("name").
			StorageKey("name").
			NotEmpty().
			MaxLen(50).
			Comment("用户名"),
		field.String("email").
			StorageKey("email").
			NotEmpty().
			MaxLen(100).
			Unique().
			Comment("邮箱"),
		field.String("password").
			StorageKey("password").
			NotEmpty().
			Sensitive().
			Comment("密码哈希"),
		field.Time("create_time").
			StorageKey("create_time").
			Immutable().
			Default(time.Now).
			Annotations(entsql.Default("CURRENT_TIMESTAMP")).
			Comment("创建时间"),
		field.Time("update_time").
			StorageKey("update_time").
			Default(time.Now).
			UpdateDefault(time.Now).
			Annotations(entsql.Default("CURRENT_TIMESTAMP")).
			Comment("更新时间"),
	}
}

// Edges of the User.
func (User) Edges() []ent.Edge {
	return nil
}
