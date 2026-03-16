package netutil

import (
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}

	// 1. 尝试解析为 GRPC 状态码
	if s, ok := status.FromError(err); ok {
		return s.Code() == codes.ResourceExhausted
	}

	// 2. 备选方案：针对 REST 模式下的字符串判定
	errMsg := strings.ToUpper(err.Error())
	return strings.Contains(errMsg, "429") ||
		strings.Contains(errMsg, "RESOURCE_EXHAUSTED") ||
		strings.Contains(errMsg, "RATE_LIMIT") ||
		strings.Contains(errMsg, "QUOTA_EXCEEDED")
}
