package open123

import (
	"context"
	"fmt"
	"net/url"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"golang.org/x/time/rate"
)

// Client 123云盘客户端
type Client struct {
	clientID     string
	clientSecret string
	accessToken  string
	baseURL      string
	ua           string
	client       *resty.Client

	// 速率限制器
	limiterLock sync.RWMutex
	limiters    map[string]*rate.Limiter
}

// NewClient 创建新的客户端
func NewClient(accessToken string) *Client {
	client := resty.New()
	client.SetTimeout(time.Duration(DEFAULT_TIMEOUT) * time.Second)

	return &Client{
		clientID:     CLIENT_ID,
		clientSecret: CLIENT_SECRET,
		accessToken:  accessToken,
		baseURL:      OPEN_BASE_URL,
		ua:           DEFAULTUA,
		client:       client,
		limiters:     make(map[string]*rate.Limiter),
	}
}

// SetRateLimit 设置特定接口的QPS限制
// path: 接口路径，例如 "/upload/v1/file/create"
// qps: 每秒查询率
func (c *Client) SetRateLimit(path string, qps int) {
	c.limiterLock.Lock()
	defer c.limiterLock.Unlock()

	// 创建速率限制器，允许突发请求为1个
	c.limiters[path] = rate.NewLimiter(rate.Limit(qps), 1)
}

// doRequest 执行HTTP请求
func (c *Client) doRequest(ctx context.Context, method, requestURL string, body []byte) (*resty.Response, error) {
	// 提取URL路径用于速率限制
	parsedURL, err := url.Parse(requestURL)
	var pathKey string
	if err != nil {
		// 如果URL解析失败，直接使用完整URL作为key
		pathKey = requestURL
	} else {
		pathKey = parsedURL.Path
	}

	// 检查是否有为该路径设置速率限制
	c.limiterLock.RLock()
	limiter, exists := c.limiters[pathKey]
	c.limiterLock.RUnlock()

	// 如果存在速率限制器，则等待许可
	if exists {
		if err := limiter.Wait(ctx); err != nil {
			return nil, fmt.Errorf("rate limit wait error: %w", err)
		}
	}

	// 使用resty执行HTTP请求
	req := c.client.R().
		SetContext(ctx).
		SetHeader("Authorization", "Bearer "+c.accessToken).
		SetHeader("Content-Type", "application/json").
		SetHeader("Platform", "open_platform").
		SetHeader("User-Agent", c.ua)

	if body != nil {
		req = req.SetBody(body)
	}

	return req.Execute(method, requestURL)
}
