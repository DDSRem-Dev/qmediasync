package v115open

const (
	// API常量
	ACCESS_TOKEN_AUTH_FAIL   = 40140126 // 刷新访问凭证
	ACCESS_TOKEN_EXPIRY_CODE = 40140125 // 刷新访问凭证
	ACCESS_AUTH_INVALID      = 40140124 // 刷新访问凭证
	REFRESH_TOKEN_INVALID    = 40140116 // 重新授权
	REQUEST_MAX_LIMIT_CODE   = 770004
	OPEN_BASE_URL            = "https://proapi.115.com"

	// 重试配置
	DEFAULT_MAX_RETRIES = 3
	DEFAULT_RETRY_DELAY = 1

	// 超时配置
	DEFAULT_TIMEOUT = 30 // 秒

	DEFAULTUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/138.0.0.0 Safari/537.36 Edg/138.0.0.0"
)
