package openlist

import (
	"Q115-STRM/internal/helpers"
	"net/http"
)

type TokenData struct {
	Token string `json:"token"`
}

// 获取开放平台token
// 用于自动登录开放平台
// POST /api/auth/login
func (c *Client) GetToken() (*TokenData, error) {
	type tokenReq struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	reqData := &tokenReq{
		Username: c.Username,
		Password: c.Password,
	}
	var result Resp[TokenData]
	req := c.client.R().SetBody(reqData).SetMethod(http.MethodPost).SetResult(&result)
	_, err := c.doRequest("/api/auth/login", req, MakeRequestConfig(0, 1, 5))
	if err != nil {
		helpers.OpenListLog.Errorf("开放平台获取访问凭证失败: %s", err.Error())
		return nil, err
	}
	tokenData := result.Data
	helpers.OpenListLog.Infof("开放平台获取访问凭证成功: %s", tokenData.Token)
	// 给客户端设置新的token
	c.SetAuthToken(tokenData.Token)
	// 通知models保存token到数据库
	helpers.PublishSync(helpers.SaveOpenListTokenEvent, map[string]any{
		"account_id": c.AccountId,
		"token":      tokenData.Token,
	})
	c.SetAuthToken(tokenData.Token)
	return &tokenData, nil
}
