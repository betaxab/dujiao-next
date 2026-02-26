package repository

import (
	"fmt"
	"strings"

	"gorm.io/gorm"
)

var localizedJSONSearchKeys = []string{"zh-CN", "zh-TW", "en-US"}

// dbDialectName 获取数据库方言名称，默认按 sqlite 处理。
func dbDialectName(db *gorm.DB) string {
	if db == nil || db.Dialector == nil {
		return "sqlite"
	}
	name := strings.ToLower(strings.TrimSpace(db.Dialector.Name()))
	if name == "" {
		return "sqlite"
	}
	return name
}

// jsonTextExpr 构建 JSON 字段文本提取表达式，兼容 sqlite 与 postgres。
func jsonTextExpr(db *gorm.DB, column, key string) string {
	return jsonTextExprByDialect(dbDialectName(db), column, key)
}

func jsonTextExprByDialect(dialect, column, key string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "postgres", "postgresql":
		// postgres 统一转 jsonb 后再使用 ->> 提取文本
		return fmt.Sprintf("(%s::jsonb ->> '%s')", column, key)
	default:
		// sqlite 使用 json_extract，语言键使用引号避免 - 等特殊字符问题
		return fmt.Sprintf("json_extract(%s, '$.\"%s\"')", column, key)
	}
}

// localizedJSONCoalesceExpr 生成多语言字段回退表达式。
func localizedJSONCoalesceExpr(db *gorm.DB, column string) string {
	parts := make([]string, 0, len(localizedJSONSearchKeys)+1)
	for _, key := range localizedJSONSearchKeys {
		parts = append(parts, jsonTextExpr(db, column, key))
	}
	parts = append(parts, "''")
	return fmt.Sprintf("COALESCE(%s)", strings.Join(parts, ", "))
}

// buildLocalizedLikeCondition 构建普通列 + JSON 多语言列的 LIKE 条件，并返回参数数量。
func buildLocalizedLikeCondition(db *gorm.DB, plainColumns, jsonColumns []string) (string, int) {
	return buildLocalizedLikeConditionByDialect(dbDialectName(db), plainColumns, jsonColumns)
}

func buildLocalizedLikeConditionByDialect(dialect string, plainColumns, jsonColumns []string) (string, int) {
	parts := make([]string, 0, len(plainColumns)+len(jsonColumns)*len(localizedJSONSearchKeys))
	argCount := 0
	operator := likeOperatorByDialect(dialect)

	for _, column := range plainColumns {
		trimmed := strings.TrimSpace(column)
		if trimmed == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s ?", trimmed, operator))
		argCount++
	}

	for _, column := range jsonColumns {
		trimmed := strings.TrimSpace(column)
		if trimmed == "" {
			continue
		}
		for _, key := range localizedJSONSearchKeys {
			parts = append(parts, fmt.Sprintf("%s %s ?", jsonTextExprByDialect(dialect, trimmed, key), operator))
			argCount++
		}
	}

	return strings.Join(parts, " OR "), argCount
}

func likeOperatorByDialect(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "postgres", "postgresql":
		return "ILIKE"
	default:
		return "LIKE"
	}
}

// repeatLikeArgs 生成重复的 LIKE 参数列表。
func repeatLikeArgs(like string, count int) []interface{} {
	args := make([]interface{}, 0, count)
	for i := 0; i < count; i++ {
		args = append(args, like)
	}
	return args
}
