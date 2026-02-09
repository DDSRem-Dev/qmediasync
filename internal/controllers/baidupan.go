package controllers

import (
	"Q115-STRM/internal/baidupan"
	"Q115-STRM/internal/helpers"
	"Q115-STRM/internal/models"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// BaiDuPanStatusResp 百度网盘状态响应
type BaiDuPanStatusResp struct {
	UserId      int64  `json:"user_id"`
	Username    string `json:"username"`
	UsedSpace   int64  `json:"used_space"`
	TotalSpace  int64  `json:"total_space"`
	MemberLevel string `json:"member_level"`
}

// GetBaiDuPanStatus 查询百度网盘账号状态
// @Summary 查询百度网盘账号状态
// @Description 获取指定百度网盘账号的登录状态及存储信息
// @Tags 百度网盘
// @Accept json
// @Produce json
// @Param account_id query integer true "账号ID"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /auth/baidupan-status [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetBaiDuPanStatus(c *gin.Context) {
	type statusReq struct {
		AccountId uint `json:"account_id" form:"account_id"`
	}
	var req statusReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	client := account.GetBaiDuPanClient()
	userInfo, err := client.GetUserInfo(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取百度网盘用户信息失败: " + err.Error(), Data: nil})
		return
	}
	quota, err := client.GetQuota(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "获取百度网盘用户配额失败: " + err.Error(), Data: nil})
		return
	}
	var memberLevel string
	switch *userInfo.VipType {
	case 0:
		memberLevel = "非会员"
	case 1:
		memberLevel = "VIP"
	case 2:
		memberLevel = "SVIP"
	default:
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "成功", Data: BaiDuPanStatusResp{
		UserId:      *userInfo.Uk,
		Username:    *userInfo.BaiduName,
		MemberLevel: memberLevel,
		UsedSpace:   *quota.Used,
		TotalSpace:  *quota.Total,
	}})
}

// GetBaiDuPanOAuthUrl 获取百度网盘OAuth登录地址
// @Summary 获取百度网盘OAuth登录地址
// @Description 生成跳转到百度OAuth授权服务器的连接给客户端
// @Tags 百度网盘
// @Accept json
// @Produce json
// @Param account_id query string true "账号ID"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /baidupan/oauth-url [get]
// @Security JwtAuth
// @Security ApiKeyAuth
func GetBaiDuPanOAuthUrl(c *gin.Context) {
	accountId := c.Query("account_id")

	if accountId == "" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "缺少账号ID参数", Data: nil})
		return
	}
	account, err := models.GetAccountById(uint(helpers.StringToInt(accountId)))
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}

	clientId := account.AppId
	if clientId == "" {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "账号缺少AppId配置", Data: nil})
		return
	}

	// 生成state参数
	type stateData struct {
		State     string `json:"state"`
		Time      int64  `json:"time"`
		ClientId  string `json:"client_id"`
		AccountId string `json:"account_id"`
	}
	stateObj := stateData{
		State:     helpers.RandStr(16),
		Time:      time.Now().Unix(),
		ClientId:  clientId,
		AccountId: accountId,
	}
	stateJson, _ := json.Marshal(stateObj)
	stateEncoded, err := helpers.Encrypt(string(stateJson))
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "生成OAuth登录地址失败: " + err.Error(), Data: nil})
		return
	}

	// 构建授权URL
	// 注意：redirect_uri需要与百度开放平台配置的一致
	oauthUrl := fmt.Sprintf("%s?action=code&state=%s", helpers.GlobalConfig.BaiduPanAuthServer, stateEncoded)
	c.JSON(http.StatusOK, APIResponse[string]{Code: Success, Message: "获取百度网盘OAuth登录地址成功", Data: oauthUrl})
}

// ConfirmBaiDuPanOAuthCode 确认百度网盘OAuth登录
// @Summary 确认百度网盘OAuth登录
// @Description 客户端将授权服务器返回的数据发送过来换取access token和refresh token并入库
// @Tags 百度网盘
// @Accept json
// @Produce json
// @Param account_id body string true "账号ID"
// @Param code body string true "授权码"
// @Success 200 {object} object
// @Failure 200 {object} object
// @Router /baidupan/oauth-confirm [post]
// @Security JwtAuth
// @Security ApiKeyAuth
func ConfirmBaiDuPanOAuthCode(c *gin.Context) {
	type oauthReq struct {
		AccountId uint   `json:"account_id" form:"account_id"`
		Data      string `json:"data" form:"data"`
	}
	var req oauthReq
	if err := c.ShouldBind(&req); err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "参数错误", Data: nil})
		return
	}
	account, err := models.GetAccountById(req.AccountId)
	if err != nil {
		c.JSON(http.StatusBadRequest, APIResponse[any]{Code: BadRequest, Message: "账号ID不存在", Data: nil})
		return
	}
	// 对req.Data解密
	decryptedData, err := helpers.Decrypt(req.Data)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "确认OAuth登录失败: " + err.Error(), Data: nil})
		return
	}
	var data *baidupan.RefreshResponse
	err = json.Unmarshal([]byte(decryptedData), &data)
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "确认OAuth登录失败: " + err.Error(), Data: nil})
		return
	}
	// 将token和刷新token保存到账号
	account.UpdateToken(data.AccessToken, data.RefreshToken, data.ExpiresIn)
	// 调用接口获取百度用户信息
	client := account.GetBaiDuPanClient()
	userInfo, err := client.GetUserInfo(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "确认OAuth登录失败: " + err.Error(), Data: nil})
		return
	}
	rs := account.UpdateUser(helpers.Int64ToString(*userInfo.Uk), *userInfo.BaiduName)
	if !rs {
		c.JSON(http.StatusOK, APIResponse[any]{Code: BadRequest, Message: "更新用户信息失败", Data: nil})
		return
	}
	c.JSON(http.StatusOK, APIResponse[any]{Code: Success, Message: "确认OAuth登录成功", Data: nil})
}
