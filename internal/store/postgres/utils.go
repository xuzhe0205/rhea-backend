package postgres

import (
	"strconv"
	"strings"
)

// getStringPtr 从 interface map 中安全地提取字符串并返回其指针。
// 如果键不存在或类型不匹配，则返回 nil。
func getStringPtr(m map[string]interface{}, key string) *string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			// 在 Go 中不能直接对字面量或表达式取地址，
			// 所以我们需要先存入变量再返回地址。
			return &str
		}
	}
	return nil
}

// getBoolPtr 从 interface map 中安全地提取布尔值并返回其指针。
// 如果键不存在或类型不匹配，则返回 nil。
func getBoolPtr(m map[string]interface{}, key string) *bool {
	if val, ok := m[key]; ok {
		if b, ok := val.(bool); ok {
			return &b
		}
	}
	return nil
}

func vectorLiteral(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}

	var sb strings.Builder
	sb.WriteByte('[')
	for i, x := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(strconv.FormatFloat(float64(x), 'f', -1, 32))
	}
	sb.WriteByte(']')
	return sb.String()
}
