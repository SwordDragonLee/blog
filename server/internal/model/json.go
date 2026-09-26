package model

// JSON MySQL JSON 列的载荷类型（语义同 gorm.io/datatypes.JSON，实现取自同源代码）。
// 收编进项目内的原因：swag 生成 OpenAPI 文档时无法解析 module cache 里的外部类型
//（cannot find type definition: datatypes.JSON），本地定义后文档生成零特殊参数；
// 同时少一个仅用到一个类型的外部依赖。GORM 行为与原库完全一致（列类型仍为 JSON）。

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/schema"
)

// JSON JSON 列存储，实现 driver.Valuer 与 sql.Scanner。
type JSON json.RawMessage

// Value 写库值：空载荷存 NULL，其余按字符串绑定。
func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

// Scan 读库：[]byte 与 string 均按原始 JSON 载荷接收，NULL 存字面量 "null"。
func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = JSON("null")
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		if len(v) > 0 {
			bytes = make([]byte, len(v))
			copy(bytes, v)
		}
	case string:
		bytes = []byte(v)
	default:
		return errors.New(fmt.Sprint("Failed to unmarshal JSONB value:", value))
	}

	*j = JSON(json.RawMessage(bytes))
	return nil
}

// MarshalJSON 输出原始 JSON 而非 base64 编码的 []byte。
func (j JSON) MarshalJSON() ([]byte, error) {
	return json.RawMessage(j).MarshalJSON()
}

// UnmarshalJSON 反序列化原始 JSON。
func (j *JSON) UnmarshalJSON(b []byte) error {
	result := json.RawMessage{}
	err := result.UnmarshalJSON(b)
	*j = JSON(result)
	return err
}

// String 原始 JSON 文本。
func (j JSON) String() string {
	return string(j)
}

// GormDataType gorm 通用数据类型。
func (JSON) GormDataType() string {
	return "json"
}

// GormDBDataType 各方言的建列类型（MySQL 为 JSON）。
func (JSON) GormDBDataType(db *gorm.DB, field *schema.Field) string {
	switch db.Dialector.Name() {
	case "sqlite":
		return "JSON"
	case "mysql":
		return "JSON"
	case "postgres":
		return "JSONB"
	}
	return ""
}

// GormValue 写库表达式：MySQL 非 MariaDB 时 CAST 为 JSON，空载荷写 NULL。
func (j JSON) GormValue(ctx context.Context, db *gorm.DB) clause.Expr {
	if len(j) == 0 {
		return gorm.Expr("NULL")
	}

	data, _ := j.MarshalJSON()

	switch db.Dialector.Name() {
	case "mysql":
		if v, ok := db.Dialector.(*mysql.Dialector); ok && !strings.Contains(v.ServerVersion, "MariaDB") {
			return gorm.Expr("CAST(? AS JSON)", string(data))
		}
	}

	return gorm.Expr("?", string(data))
}
