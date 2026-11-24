package webapi

import "net/http"

// Стабильные коды ошибок API (RM0a). Поле code в JSON-ошибках.
const (
	CodeUnauthorized     = "unauthorized"
	CodeCSRF             = "csrf"
	CodeForbidden        = "forbidden"
	CodeConflict         = "conflict"
	CodeBusy             = "busy"
	CodeValidation       = "validation"
	CodeNotFound         = "not_found"
	CodeRateLimit        = "rate_limit"
	CodeMethodNotAllowed = "method_not_allowed"
	CodeUnavailable      = "unavailable"
	CodeTimeout          = "timeout"
	CodeInternal         = "internal"
)

// codeFromStatus — код по умолчанию для HTTP-статуса.
func codeFromStatus(status int) string {
	switch status {
	case http.StatusUnauthorized:
		return CodeUnauthorized
	case http.StatusForbidden:
		return CodeCSRF
	case http.StatusConflict:
		return CodeConflict
	case http.StatusBadRequest:
		return CodeValidation
	case http.StatusNotFound:
		return CodeNotFound
	case http.StatusTooManyRequests:
		return CodeRateLimit
	case http.StatusMethodNotAllowed:
		return CodeMethodNotAllowed
	case http.StatusServiceUnavailable:
		return CodeUnavailable
	case http.StatusGatewayTimeout, http.StatusRequestTimeout:
		return CodeTimeout
	default:
		if status >= 500 {
			return CodeInternal
		}
		return CodeValidation
	}
}
