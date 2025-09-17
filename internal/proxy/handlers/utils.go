package handlers

import (
	"net/http"
	"strconv"
	"strings"
)

// IsSuccessStatus 判断是否为成功状态码
// 成功状态码范围：200-399 (2xx-3xx)
func IsSuccessStatus(statusCode int) bool {
	return statusCode >= 200 && statusCode < 400
}

// IsRetryableStatus 判断是否为可重试状态码
// 可重试的状态码：
// - 400, 403, 429: 特定的4xx错误可以重试
// - 401, 404, 410: 特定的4xx错误不应重试
// - 5xx: 服务器错误通常可以重试
func IsRetryableStatus(statusCode int) bool {
	switch statusCode {
	case 400, 403, 429: // 可重试的4xx
		return true
	case 401, 404, 410: // 不可重试的4xx
		return false
	default:
		return statusCode >= 500 // 5xx通常可重试
	}
}

// GetStatusCodeFromError 从错误和响应中提取状态码
// 优先级：
// 1. 首先从 http.Response 获取状态码
// 2. 然后从错误信息字符串中解析状态码
// 3. 最后返回0表示网络错误或无法确定状态码的情况
func GetStatusCodeFromError(err error, resp *http.Response) int {
	// 优先从响应对象获取状态码
	if resp != nil {
		return resp.StatusCode
	}

	// 如果没有响应对象，尝试从错误信息中提取状态码
	if err == nil {
		return 0
	}

	errorStr := err.Error()

	// 常见的HTTP状态码提取模式
	statusCodes := []int{400, 401, 403, 404, 405, 406, 408, 409, 410, 413, 414, 415, 416, 417, 429, 500, 501, 502, 503, 504, 505}

	for _, code := range statusCodes {
		codeStr := strconv.Itoa(code)
		if strings.Contains(errorStr, codeStr) {
			return code
		}
	}

	// 尝试匹配HTTP状态码模式 "HTTP status xxx"
	if strings.Contains(errorStr, "HTTP status") {
		parts := strings.Fields(errorStr)
		for i, part := range parts {
			if part == "status" && i+1 < len(parts) {
				if code, err := strconv.Atoi(parts[i+1]); err == nil {
					return code
				}
			}
		}
	}

	// 尝试匹配 "status code xxx" 模式
	if strings.Contains(errorStr, "status code") {
		parts := strings.Fields(errorStr)
		for i, part := range parts {
			if part == "code" && i+1 < len(parts) {
				if code, err := strconv.Atoi(parts[i+1]); err == nil {
					return code
				}
			}
		}
	}

	// 无法提取状态码，返回0表示网络错误或其他未知错误
	return 0
}