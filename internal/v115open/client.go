package v115open

import (
	"Q115-STRM/internal/helpers"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"golang.org/x/time/rate"
	"resty.dev/v3"
)

// OpenClient HTTP客户端
type OpenClient struct {
	AppId     string // 应用ID
	AccountId uint   // 账号ID
	client    *resty.Client
	// 速率限制器
	rateLimiter     *rate.Limiter
	AccessToken     string // 访问令牌
	RefreshTokenStr string // 刷新令牌
}

// 全局HTTP客户端实例
var cachedClients map[string]*OpenClient = make(map[string]*OpenClient, 0)
var cachedClientsMutex sync.RWMutex

func UpdateToken(accountId uint, token string, refreshToken string) {
	for key, client := range cachedClients {
		if client.AccountId == accountId {
			client.SetAuthToken(token, refreshToken)
			helpers.AppLogger.Infof("更新115客户端 %s 的token成功", key)
		}
	}
}

// NewHttpClient 创建新的HTTP客户端
func GetClient(qps int, accountId uint, appId string, token string, refreshToken string) *OpenClient {
	cachedClientsMutex.RLock()
	defer cachedClientsMutex.RUnlock()
	clientKey := fmt.Sprintf("%d_%d", accountId, qps)
	if client, exists := cachedClients[clientKey]; exists {
		client.SetAuthToken(token, refreshToken)
		return client
	}

	client := resty.New()
	var openClient *OpenClient
	if qps == 0 {
		helpers.AppLogger.Infof("创建新的115客户端，普通调用无限速")
		openClient = &OpenClient{
			client:      client,
			AppId:       appId,
			AccountId:   accountId,
			rateLimiter: nil, // 每秒qps个请求
		}
	} else {
		helpers.AppLogger.Infof("创建新的115客户端，strm同步或刮削，qps=%d", qps)
		openClient = &OpenClient{
			client:      client,
			AppId:       appId,
			AccountId:   accountId,
			rateLimiter: rate.NewLimiter(rate.Every(time.Second), qps), // 每秒qps个请求
		}
	}
	openClient.SetAuthToken(token, refreshToken)
	cachedClients[clientKey] = openClient
	return openClient
}

// SetAuthToken 设置认证令牌
func (c *OpenClient) SetAuthToken(token string, refreshToken string) {
	c.AccessToken = token
	c.RefreshTokenStr = refreshToken
}

// request 执行HTTP请求的核心方法
func (c *OpenClient) request(url string, req *resty.Request) (*resty.Response, *RespBase[json.RawMessage], error) {
	req.SetResult(&RespBase[json.RawMessage]{}).SetForceResponseContentType("application/json")
	var response *resty.Response
	var err error
	method := req.Method
	switch method {
	case "GET":
		response, err = req.Get(url)
	case "POST":
		response, err = req.Post(url)
	default:
		return nil, nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}
	result := response.Result()
	resp := result.(*RespBase[json.RawMessage])
	if err != nil {
		return response, resp, err
	}
	helpers.V115Log.Infof("非认证访问 %s %s\nstate=%d, code=%d, msg=%s, data=%s\n", req.Method, req.URL, resp.State, resp.Code, resp.Message, string(resp.Data))
	switch resp.Code {
	case REFRESH_TOKEN_INVALID:
		return response, nil, fmt.Errorf("token invalid")
	case REQUEST_MAX_LIMIT_CODE:
		// 访问频率过高
		return response, nil, fmt.Errorf("rate limit exceeded")
	}

	return response, resp, nil
}

// doRequest 带重试的请求方法
func (c *OpenClient) doRequest(url string, req *resty.Request, options *RequestConfig) (*resty.Response, *RespBase[json.RawMessage], error) {
	// 设置超时时间
	req.SetTimeout(options.Timeout)
	// 设置默认头
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DEFAULTUA)
	}
	var lastErr error
	for attempt := 0; attempt <= options.MaxRetries; attempt++ {
		resp, respData, err := c.request(url, req)
		if err == nil {
			// 正常返回
			return resp, respData, nil
		}
		lastErr = err

		if err.Error() == "token invalid" {
			return nil, nil, err
		}
		// 如果是速率限制错误，等待暂停结束后重试
		if err.Error() == "rate limit exceeded" {
			helpers.V115Log.Warn("访问频率过高，正在暂停")
			continue
		}

		// 其他错误开始重试
		if attempt < options.MaxRetries {
			helpers.V115Log.Warnf("%s %s 请求失败:%+v", req.Method, req.URL, lastErr)
			helpers.V115Log.Warnf("%s %s 请求失败，%+v秒后重试 (第%d次尝试)", req.Method, req.URL, options.RetryDelay.Seconds(), attempt+1)
			time.Sleep(options.RetryDelay)
		}
	}
	return nil, nil, lastErr
}

// request 执行HTTP请求的核心方法
func (c *OpenClient) authRequest(ctx context.Context, url string, req *resty.Request, respData any, options *RequestConfig) (*resty.Response, []byte, error) {
	if options.RateLimited && c.rateLimiter != nil {
		if err := c.rateLimiter.Wait(ctx); err != nil {
			return nil, nil, fmt.Errorf("rate limit wait error: %w", err)
		}
	}
	req.SetForceResponseContentType("application/json")
	var response *resty.Response
	var err error
	method := req.Method
	req.SetContext(ctx)
	req.SetAuthToken(c.AccessToken).SetDoNotParseResponse(true)
	switch method {
	case "GET":
		response, err = req.Get(url)
	case "POST":
		response, err = req.Post(url)
	default:
		return nil, nil, fmt.Errorf("unsupported HTTP method: %s", method)
	}
	if err != nil {
		return response, nil, err
	}
	defer response.Body.Close() // ensure to close response body
	resBytes, ioErr := io.ReadAll(response.Body)
	if ioErr != nil {
		fmt.Println(ioErr)
		return response, nil, ioErr
	}
	resp := &RespBaseBool[json.RawMessage]{}
	bodyErr := json.Unmarshal(resBytes, resp)
	if bodyErr != nil {
		helpers.V115Log.Errorf("解析响应失败: %s", bodyErr.Error())
		return response, resBytes, bodyErr
	}
	helpers.V115Log.Infof("认证访问 %s %s\nstate=%v, code=%d, msg=%s, data=%s\n", req.Method, req.URL, resp.State, resp.Code, resp.Message, string(resp.Data))
	switch resp.Code {
	case ACCESS_TOKEN_AUTH_FAIL:
		helpers.V115Log.Warn("访问凭证已过期1")
		return response, nil, fmt.Errorf("token expired")
	case ACCESS_AUTH_INVALID:
		helpers.V115Log.Warn("访问凭证已过期2")
		return response, nil, fmt.Errorf("token expired")
	case ACCESS_TOKEN_EXPIRY_CODE:
		helpers.V115Log.Warn("访问凭证已过期3")
		return response, nil, fmt.Errorf("token expired")
	case REFRESH_TOKEN_INVALID:
		// 不需要重试，直接返回
		helpers.V115Log.Error("访问凭证无效，请重新登录")
		return response, nil, fmt.Errorf("token expired")
	case REQUEST_MAX_LIMIT_CODE:
		// 访问频率过高，暂停30秒
		// if !c.isPaused() {
		// 	c.setPause()
		// 	time.Sleep(30 * time.Second)
		// 	// 暂停结束后，重置暂停状态
		// 	c.resetPause()
		// }
		return response, nil, fmt.Errorf("rate limit exceeded")
	}
	if respData != nil && resp.State {
		// 解包
		if unmarshalErr := json.Unmarshal(resp.Data, respData); unmarshalErr != nil {
			respData = nil
			helpers.V115Log.Errorf("解包响应数据失败: %s", unmarshalErr.Error())
			return response, resBytes, nil
		}
	}
	if resp.Code != 0 {
		return response, resBytes, fmt.Errorf("错误码：%d，错误信息：%s", resp.Code, resp.Message)
	}
	return response, resBytes, nil
}

// doAuthRequest 带重试的认证请求方法
func (c *OpenClient) doAuthRequest(ctx context.Context, url string, req *resty.Request, options *RequestConfig, respData any) (*resty.Response, []byte, error) {
	if c.AccessToken == "" {
		// 没有token，直接报错
		return nil, nil, fmt.Errorf("115账号授权失效，请在网盘账号管理中重新授权")
	}
	req.SetTimeout(options.Timeout)
	// 设置默认头
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", DEFAULTUA)
	}
	var lastErr error
	for attempt := 0; attempt <= options.MaxRetries; attempt++ {
		resp, respBytes, err := c.authRequest(ctx, url, req, respData, options)
		if err == nil {
			return resp, respBytes, nil
		}

		lastErr = err
		// 如果是token过期错误，等待token刷新完成后重试
		if err.Error() == "token expired" {
			helpers.V115Log.Errorf("访问凭证过期，等待自动刷新后下次重试")
			lastErr = fmt.Errorf("访问凭证（Token）过期")
		}
		if err.Error() == "token invalid" {
			// 不需要重试，直接返回
			lastErr = fmt.Errorf("访问凭证（Token）无效，请重新登录")
			return nil, nil, lastErr
		}
		// 如果是速率限制错误，等待30秒后重试
		if err.Error() == "rate limit exceeded" {
			helpers.V115Log.Warn("访问频率过高，停止请求")
			// lastErr = fmt.Errorf("访问频率过高")
			// time.Sleep(30 * time.Second)
			// helpers.V115Log.Warn("因访问频率过高导致的暂停结束，开始重试请求")
			return nil, nil, lastErr
		}

		// 其他错误开始重试
		if attempt < options.MaxRetries {
			helpers.V115Log.Warnf("%s %s 请求失败:%+v", req.Method, req.URL, lastErr)
			helpers.V115Log.Warnf("%s %s 请求失败，%+v秒后重试 (第%d次尝试)", req.Method, req.URL, options.RetryDelay.Seconds(), attempt+1)
			time.Sleep(options.RetryDelay)
		}
	}
	return nil, nil, lastErr
}
